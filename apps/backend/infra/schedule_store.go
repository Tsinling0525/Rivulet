package infra

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"crypto/rand"

	"github.com/Tsinling0525/rivulet/model"
)

var ErrScheduleNotFound = errors.New("schedule not found")

type Schedule struct {
	ID              string                   `json:"id"`
	WorkflowID      string                   `json:"workflow_id"`
	WorkflowVersion int                      `json:"workflow_version,omitempty"`
	IntervalSeconds int                      `json:"interval_seconds"`
	Input           map[model.ID]model.Items `json:"input,omitempty"`
	Enabled         bool                     `json:"enabled"`
	Running         bool                     `json:"running"`
	CreatedAt       time.Time                `json:"created_at"`
	UpdatedAt       time.Time                `json:"updated_at"`
	NextRunAt       time.Time                `json:"next_run_at"`
	LastRunAt       time.Time                `json:"last_run_at,omitempty"`
	LastRunID       string                   `json:"last_run_id,omitempty"`
	LastStatus      string                   `json:"last_status,omitempty"`
	LastError       string                   `json:"last_error,omitempty"`
}

type ScheduleStore struct {
	mu  sync.Mutex
	dir string
}

type ScheduleCreate struct {
	WorkflowID      string
	WorkflowVersion int
	IntervalSeconds int
	Input           map[model.ID]model.Items
	Enabled         bool
	NextRunAt       time.Time
}

func NewScheduleStore() *ScheduleStore {
	return &ScheduleStore{dir: ScheduleStoreDir()}
}

func (s *ScheduleStore) schedulePath(id string) string {
	return filepath.Join(s.dir, id+".json")
}

func (s *ScheduleStore) List() ([]Schedule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listLocked()
}

func (s *ScheduleStore) Get(id string) (Schedule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked(id)
}

func (s *ScheduleStore) Create(req ScheduleCreate) (Schedule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if req.WorkflowID == "" {
		return Schedule{}, fmt.Errorf("workflow_id is required")
	}
	if req.IntervalSeconds <= 0 {
		return Schedule{}, fmt.Errorf("interval_seconds must be greater than zero")
	}
	now := time.Now().UTC()
	nextRunAt := req.NextRunAt
	if nextRunAt.IsZero() {
		nextRunAt = now.Add(time.Duration(req.IntervalSeconds) * time.Second)
	}
	schedule := Schedule{
		ID:              newScheduleID(),
		WorkflowID:      req.WorkflowID,
		WorkflowVersion: req.WorkflowVersion,
		IntervalSeconds: req.IntervalSeconds,
		Input:           cloneItemsMap(req.Input),
		Enabled:         req.Enabled,
		CreatedAt:       now,
		UpdatedAt:       now,
		NextRunAt:       nextRunAt.UTC(),
	}
	if err := s.saveLocked(schedule); err != nil {
		return Schedule{}, err
	}
	return schedule, nil
}

func (s *ScheduleStore) Pause(id string) (Schedule, error) {
	return s.setEnabled(id, false)
}

func (s *ScheduleStore) Resume(id string) (Schedule, error) {
	return s.setEnabled(id, true)
}

func (s *ScheduleStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := s.loadLocked(id); err != nil {
		return err
	}
	return os.Remove(s.schedulePath(id))
}

func (s *ScheduleStore) ResetRunning() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	items, err := s.listLocked()
	if err != nil {
		return err
	}
	for _, schedule := range items {
		if !schedule.Running {
			continue
		}
		schedule.Running = false
		schedule.UpdatedAt = time.Now().UTC()
		if err := s.saveLocked(schedule); err != nil {
			return err
		}
	}
	return nil
}

func (s *ScheduleStore) ClaimDue(now time.Time, limit int) ([]Schedule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items, err := s.listLocked()
	if err != nil {
		return nil, err
	}
	claimed := make([]Schedule, 0)
	for _, schedule := range items {
		if limit > 0 && len(claimed) >= limit {
			break
		}
		if !schedule.Enabled || schedule.Running || schedule.NextRunAt.After(now) {
			continue
		}
		schedule.Running = true
		schedule.UpdatedAt = now.UTC()
		schedule.NextRunAt = nextIntervalAfter(schedule.NextRunAt, now, schedule.IntervalSeconds)
		if err := s.saveLocked(schedule); err != nil {
			return nil, err
		}
		claimed = append(claimed, schedule)
	}
	return claimed, nil
}

func (s *ScheduleStore) CompleteRun(id string, run RunRecord, runErr error) (Schedule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	schedule, err := s.loadLocked(id)
	if err != nil {
		return Schedule{}, err
	}
	schedule.Running = false
	schedule.LastRunAt = run.StartedAt
	schedule.LastRunID = run.ID
	schedule.LastStatus = run.Status
	schedule.LastError = ""
	if runErr != nil {
		schedule.LastError = runErr.Error()
	}
	schedule.UpdatedAt = time.Now().UTC()
	if err := s.saveLocked(schedule); err != nil {
		return Schedule{}, err
	}
	return schedule, nil
}

func (s *ScheduleStore) setEnabled(id string, enabled bool) (Schedule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	schedule, err := s.loadLocked(id)
	if err != nil {
		return Schedule{}, err
	}
	schedule.Enabled = enabled
	schedule.Running = false
	schedule.UpdatedAt = time.Now().UTC()
	if enabled && schedule.NextRunAt.Before(schedule.UpdatedAt) {
		schedule.NextRunAt = schedule.UpdatedAt.Add(time.Duration(schedule.IntervalSeconds) * time.Second)
	}
	if err := s.saveLocked(schedule); err != nil {
		return Schedule{}, err
	}
	return schedule, nil
}

func (s *ScheduleStore) listLocked() ([]Schedule, error) {
	if err := ensureDir(s.dir); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	out := make([]Schedule, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(s.dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		var schedule Schedule
		if err := json.Unmarshal(raw, &schedule); err != nil {
			return nil, err
		}
		out = append(out, schedule)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].NextRunAt.Before(out[j].NextRunAt)
	})
	return out, nil
}

func (s *ScheduleStore) loadLocked(id string) (Schedule, error) {
	raw, err := os.ReadFile(s.schedulePath(id))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Schedule{}, ErrScheduleNotFound
		}
		return Schedule{}, err
	}
	var schedule Schedule
	if err := json.Unmarshal(raw, &schedule); err != nil {
		return Schedule{}, err
	}
	return schedule, nil
}

func (s *ScheduleStore) saveLocked(schedule Schedule) error {
	if err := ensureDir(s.dir); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(schedule, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.schedulePath(schedule.ID), raw, 0o644)
}

func nextIntervalAfter(nextRunAt, now time.Time, intervalSeconds int) time.Time {
	interval := time.Duration(intervalSeconds) * time.Second
	if interval <= 0 {
		interval = time.Second
	}
	next := nextRunAt.UTC()
	if next.IsZero() {
		next = now.UTC()
	}
	for !next.After(now) {
		next = next.Add(interval)
	}
	return next
}

func newScheduleID() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("sched-%d", time.Now().UnixNano())
	}
	return "sched-" + hex.EncodeToString(buf[:])
}
