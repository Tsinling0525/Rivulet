package infra

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Tsinling0525/rivulet/engine"
	"github.com/Tsinling0525/rivulet/model"
)

var ErrCheckpointNotFound = errors.New("checkpoint not found")

type CheckpointStatus string

const (
	CheckpointActive   CheckpointStatus = "active"
	CheckpointResumed  CheckpointStatus = "resumed"
	CheckpointRejected CheckpointStatus = "rejected"
)

type CheckpointRecord struct {
	ID              string            `json:"id"`
	RunID           string            `json:"run_id"`
	WorkflowID      string            `json:"workflow_id"`
	WorkflowVersion int               `json:"workflow_version,omitempty"`
	ReviewID        string            `json:"review_id"`
	PausedNodeID    model.ID          `json:"paused_node_id"`
	Status          CheckpointStatus  `json:"status"`
	Checkpoint      engine.Checkpoint `json:"checkpoint"`
	WorkflowRequest json.RawMessage   `json:"workflow_request"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
}

type CheckpointStore struct {
	mu  sync.Mutex
	dir string
}

func NewCheckpointStore() *CheckpointStore {
	return &CheckpointStore{dir: CheckpointStoreDir()}
}

func (s *CheckpointStore) checkpointPath(id string) string {
	return filepath.Join(s.dir, id+".json")
}

func (s *CheckpointStore) Create(record CheckpointRecord) (CheckpointRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	if record.ID == "" {
		record.ID = newCheckpointID()
	}
	record.Status = CheckpointActive
	record.CreatedAt = now
	record.UpdatedAt = now
	if err := s.saveLocked(record); err != nil {
		return CheckpointRecord{}, err
	}
	return record, nil
}

func (s *CheckpointStore) Get(id string) (CheckpointRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked(id)
}

func (s *CheckpointStore) FindActiveByReviewID(reviewID string) (CheckpointRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items, err := s.listLocked()
	if err != nil {
		return CheckpointRecord{}, err
	}
	for _, item := range items {
		if item.ReviewID == reviewID && item.Status == CheckpointActive {
			return item, nil
		}
	}
	return CheckpointRecord{}, ErrCheckpointNotFound
}

func (s *CheckpointStore) MarkResumed(id string) (CheckpointRecord, error) {
	return s.mark(id, CheckpointResumed)
}

func (s *CheckpointStore) MarkRejected(id string) (CheckpointRecord, error) {
	return s.mark(id, CheckpointRejected)
}

func (s *CheckpointStore) mark(id string, status CheckpointStatus) (CheckpointRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, err := s.loadLocked(id)
	if err != nil {
		return CheckpointRecord{}, err
	}
	record.Status = status
	record.UpdatedAt = time.Now().UTC()
	if err := s.saveLocked(record); err != nil {
		return CheckpointRecord{}, err
	}
	return record, nil
}

func (s *CheckpointStore) listLocked() ([]CheckpointRecord, error) {
	if err := ensureDir(s.dir); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	out := make([]CheckpointRecord, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(s.dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		var record CheckpointRecord
		if err := json.Unmarshal(raw, &record); err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	return out, nil
}

func (s *CheckpointStore) loadLocked(id string) (CheckpointRecord, error) {
	raw, err := os.ReadFile(s.checkpointPath(id))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return CheckpointRecord{}, ErrCheckpointNotFound
		}
		return CheckpointRecord{}, err
	}
	var record CheckpointRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		return CheckpointRecord{}, err
	}
	return record, nil
}

func (s *CheckpointStore) saveLocked(record CheckpointRecord) error {
	if err := ensureDir(s.dir); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.checkpointPath(record.ID), raw, 0o644)
}

func newCheckpointID() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("checkpoint-%d", time.Now().UnixNano())
	}
	return "checkpoint-" + hex.EncodeToString(buf[:])
}
