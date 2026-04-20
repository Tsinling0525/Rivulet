package infra

import (
	"context"
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

	"github.com/Tsinling0525/rivulet/engine"
	"github.com/Tsinling0525/rivulet/format/n8n"
	"github.com/Tsinling0525/rivulet/model"
	"github.com/Tsinling0525/rivulet/plugin"
)

var ErrRunNotFound = errors.New("run not found")

type RunEvent struct {
	Type       string         `json:"type"`
	OccurredAt time.Time      `json:"occurred_at"`
	NodeID     string         `json:"node_id,omitempty"`
	Fields     map[string]any `json:"fields,omitempty"`
}

type RunRecord struct {
	ID              string                    `json:"id"`
	WorkflowID      string                    `json:"workflow_id,omitempty"`
	WorkflowName    string                    `json:"workflow_name,omitempty"`
	WorkflowKind    model.WorkflowKind        `json:"workflow_kind,omitempty"`
	AI              *model.AIWorkflowMetadata `json:"ai,omitempty"`
	WorkflowVersion int                       `json:"workflow_version,omitempty"`
	Source          string                    `json:"source,omitempty"`
	Trigger         string                    `json:"trigger,omitempty"`
	InstanceID      string                    `json:"instance_id,omitempty"`
	ScheduleID      string                    `json:"schedule_id,omitempty"`
	Status          string                    `json:"status"`
	StartedAt       time.Time                 `json:"started_at"`
	FinishedAt      time.Time                 `json:"finished_at"`
	DurationMS      int64                     `json:"duration_ms"`
	Input           map[model.ID]model.Items  `json:"input,omitempty"`
	Result          map[model.ID]model.Items  `json:"result,omitempty"`
	Error           string                    `json:"error,omitempty"`
	Events          []RunEvent                `json:"events,omitempty"`
	WorkflowRequest json.RawMessage           `json:"workflow_request"`
}

type RunStore struct {
	mu  sync.Mutex
	dir string
}

type ExecuteRequest struct {
	RunID           string
	WorkflowID      string
	WorkflowVersion int
	WorkflowRequest n8n.N8nRequest
	RawRequest      []byte
	Inputs          map[model.ID]model.Items
	Source          string
	Trigger         string
	InstanceID      string
	ScheduleID      string
}

type ExecuteResult struct {
	Run    RunRecord
	Result map[model.ID]model.Items
}

type recordingBus struct {
	next   plugin.EventBus
	events *[]RunEvent
}

func (b recordingBus) Emit(ctx context.Context, event string, fields map[string]any) error {
	copied := map[string]any{}
	for key, value := range fields {
		copied[key] = value
	}
	nodeID := ""
	if raw, ok := fields["node"]; ok {
		nodeID = fmt.Sprint(raw)
	}
	*b.events = append(*b.events, RunEvent{
		Type:       event,
		OccurredAt: time.Now().UTC(),
		NodeID:     nodeID,
		Fields:     copied,
	})
	if b.next != nil {
		return b.next.Emit(ctx, event, fields)
	}
	return nil
}

func NewRunStore() *RunStore {
	return &RunStore{dir: RunHistoryDir()}
}

func (s *RunStore) Save(run RunRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := ensureDir(s.dir); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.dir, run.ID+".json"), raw, 0o644)
}

func (s *RunStore) Get(id string) (RunRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked(id)
}

func (s *RunStore) List(workflowID string, limit int) ([]RunRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := ensureDir(s.dir); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	out := make([]RunRecord, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		run, err := s.loadLocked(entry.Name()[:len(entry.Name())-len(filepath.Ext(entry.Name()))])
		if err != nil {
			return nil, err
		}
		if workflowID != "" && run.WorkflowID != workflowID {
			continue
		}
		out = append(out, run)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].StartedAt.After(out[j].StartedAt)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *RunStore) Replay(ctx context.Context, deps plugin.Deps, id string, inputs map[model.ID]model.Items) (ExecuteResult, error) {
	run, err := s.Get(id)
	if err != nil {
		return ExecuteResult{}, err
	}
	var req n8n.N8nRequest
	if err := json.Unmarshal(run.WorkflowRequest, &req); err != nil {
		return ExecuteResult{}, err
	}
	if inputs == nil {
		inputs = cloneItemsMap(run.Input)
	}
	return ExecuteWorkflow(ctx, deps, s, ExecuteRequest{
		WorkflowID:      run.WorkflowID,
		WorkflowVersion: run.WorkflowVersion,
		WorkflowRequest: req,
		RawRequest:      append([]byte(nil), run.WorkflowRequest...),
		Inputs:          inputs,
		Source:          "replay",
		Trigger:         "replay",
		InstanceID:      run.InstanceID,
		ScheduleID:      run.ScheduleID,
	})
}

func ExecuteWorkflow(ctx context.Context, deps plugin.Deps, store *RunStore, req ExecuteRequest) (ExecuteResult, error) {
	workflow, defaultInputs := n8n.ToRivulet(req.WorkflowRequest)
	inputs := cloneItemsMap(req.Inputs)
	if inputs == nil {
		inputs = cloneItemsMap(defaultInputs)
	}

	workflowID := req.WorkflowID
	if workflowID == "" {
		workflowID = string(workflow.ID)
	}

	run := RunRecord{
		ID:              req.RunID,
		WorkflowID:      workflowID,
		WorkflowName:    workflow.Name,
		WorkflowKind:    workflow.Kind,
		AI:              workflow.AI,
		WorkflowVersion: req.WorkflowVersion,
		Source:          req.Source,
		Trigger:         req.Trigger,
		InstanceID:      req.InstanceID,
		ScheduleID:      req.ScheduleID,
		Status:          "running",
		StartedAt:       time.Now().UTC(),
		Input:           cloneItemsMap(inputs),
		WorkflowRequest: normalizeWorkflowRequest(req.RawRequest, req.WorkflowRequest),
	}
	if run.ID == "" {
		run.ID = newRunID()
	}

	events := make([]RunEvent, 0, 8)
	runDeps := deps
	runDeps.Bus = recordingBus{next: deps.Bus, events: &events}

	eng := engine.New(runDeps)
	result, err := eng.Run(ctx, run.ID, workflow, inputs)
	run.Events = append(run.Events, events...)
	run.FinishedAt = time.Now().UTC()
	run.DurationMS = run.FinishedAt.Sub(run.StartedAt).Milliseconds()
	if err != nil {
		run.Status = "failed"
		run.Error = err.Error()
		if store != nil {
			if saveErr := store.Save(run); saveErr != nil {
				return ExecuteResult{}, saveErr
			}
		}
		return ExecuteResult{Run: run}, err
	}

	run.Status = "succeeded"
	run.Result = cloneItemsMap(result)
	if store != nil {
		if err := store.Save(run); err != nil {
			return ExecuteResult{}, err
		}
	}
	return ExecuteResult{Run: run, Result: result}, nil
}

func (s *RunStore) loadLocked(id string) (RunRecord, error) {
	raw, err := os.ReadFile(filepath.Join(s.dir, id+".json"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return RunRecord{}, ErrRunNotFound
		}
		return RunRecord{}, err
	}
	var run RunRecord
	if err := json.Unmarshal(raw, &run); err != nil {
		return RunRecord{}, err
	}
	return run, nil
}

func newRunID() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("run-%d", time.Now().UnixNano())
	}
	return "run-" + hex.EncodeToString(buf[:])
}
