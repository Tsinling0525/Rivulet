package infra

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/Tsinling0525/rivulet/format/n8n"
	apiinfra "github.com/Tsinling0525/rivulet/infra/api"
	"github.com/Tsinling0525/rivulet/model"
	"github.com/Tsinling0525/rivulet/plugin"
)

type InstanceState string

const (
	InstanceRunning InstanceState = "running"
	InstanceStopped InstanceState = "stopped"
)

type Instance struct {
	ID              string
	Name            string
	WorkflowPath    string
	WorkflowID      string
	WorkflowVersion int
	Workflow        model.Workflow
	WorkflowRequest n8n.N8nRequest
	WorkflowRaw     []byte
	CreatedAt       time.Time
	State           InstanceState

	q       chan map[model.ID]model.Items
	cancel  context.CancelFunc
	deps    plugin.Deps
	logMu   sync.Mutex
	logs    []string
	maxLogs int
	statsMu sync.Mutex
	stats   InstanceStats
	lastRun ExecutionRecord
	active  ActiveExecution
}

func (i *Instance) logf(format string, a ...any) {
	i.logMu.Lock()
	defer i.logMu.Unlock()
	line := time.Now().Format(time.RFC3339) + " " + fmt.Sprintf(format, a...)
	if i.logs == nil {
		i.logs = make([]string, 0, 256)
	}
	i.logs = append(i.logs, line)
	if i.maxLogs <= 0 {
		i.maxLogs = 1000
	}
	if len(i.logs) > i.maxLogs {
		// trim oldest
		i.logs = i.logs[len(i.logs)-i.maxLogs:]
	}
}

// InstanceStats tracks execution metrics for an instance.
type InstanceStats struct {
	TotalExecutions      int
	SuccessfulExecutions int
	FailedExecutions     int
	TotalSuccessDuration time.Duration
	LastRunAt            time.Time
}

// ExecutionRecord captures the latest execution details for UI inspection.
type ExecutionRecord struct {
	ExecutionID     string                    `json:"execution_id"`
	Status          string                    `json:"status,omitempty"`
	WorkflowID      string                    `json:"workflow_id,omitempty"`
	WorkflowKind    model.WorkflowKind        `json:"workflow_kind,omitempty"`
	AI              *model.AIWorkflowMetadata `json:"ai,omitempty"`
	WorkflowVersion int                       `json:"workflow_version,omitempty"`
	Source          string                    `json:"source,omitempty"`
	Trigger         string                    `json:"trigger,omitempty"`
	ScheduleID      string                    `json:"schedule_id,omitempty"`
	StartedAt       time.Time                 `json:"started_at"`
	FinishedAt      time.Time                 `json:"finished_at"`
	DurationMS      int64                     `json:"duration_ms"`
	Input           map[model.ID]model.Items  `json:"input,omitempty"`
	Result          map[model.ID]model.Items  `json:"result,omitempty"`
	Error           string                    `json:"error,omitempty"`
	Events          []RunEvent                `json:"events,omitempty"`
}

// ActiveExecution describes the current in-flight execution, if any.
type ActiveExecution struct {
	ExecutionID string    `json:"execution_id,omitempty"`
	StartedAt   time.Time `json:"started_at,omitempty"`
	IsExecuting bool      `json:"is_executing"`
}

// InstanceSnapshot captures a consistent view of instance state and metrics.
type InstanceSnapshot struct {
	ID          string
	Name        string
	State       InstanceState
	QueueLength int
	Stats       InstanceStats
	LastRun     ExecutionRecord
	Active      ActiveExecution
}

// Snapshot returns a point-in-time snapshot of the instance state.
func (i *Instance) Snapshot() InstanceSnapshot {
	i.statsMu.Lock()
	statsCopy := i.stats
	lastRunCopy := i.lastRun
	activeCopy := i.active
	i.statsMu.Unlock()

	return InstanceSnapshot{
		ID:          i.ID,
		Name:        i.Name,
		State:       i.State,
		QueueLength: len(i.q),
		Stats:       statsCopy,
		LastRun:     lastRunCopy,
		Active:      activeCopy,
	}
}

type InstanceManager struct {
	mu        sync.Mutex
	items     map[string]*Instance
	deps      plugin.Deps
	workflows *WorkflowStore
	runs      *RunStore
	newID     func() string
}

func NewInstanceManager(workflows *WorkflowStore, runs *RunStore) *InstanceManager {
	deps := plugin.Deps{State: apiinfra.MemState{}, Bus: apiinfra.NullBus{}, Files: NewLocalFiles(), Reviews: NewReviewStore()}
	return &InstanceManager{
		items:     make(map[string]*Instance),
		deps:      deps,
		workflows: workflows,
		runs:      runs,
		newID:     func() string { return fmt.Sprintf("inst-%d", time.Now().UnixNano()) },
	}
}

func (m *InstanceManager) List() []*Instance {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*Instance, 0, len(m.items))
	for _, v := range m.items {
		out = append(out, v)
	}
	return out
}

func (m *InstanceManager) Get(id string) (*Instance, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.items[id]
	return v, ok
}

func (m *InstanceManager) CreateFromWorkflowPath(path string) (*Instance, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var req n8n.N8nRequest
	if err := json.Unmarshal(b, &req); err != nil {
		return nil, err
	}
	return m.createFromRequest(path, "", 0, req, b)
}

func (m *InstanceManager) CreateFromWorkflowID(id string, version int) (*Instance, error) {
	if m.workflows == nil {
		return nil, fmt.Errorf("workflow store not configured")
	}
	record, req, err := m.workflows.LoadVersionRequest(id, version)
	if err != nil {
		return nil, err
	}
	actualVersion := version
	if actualVersion == 0 {
		actualVersion = record.ActiveVersion
	}
	raw, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	return m.createFromRequest("", id, actualVersion, req, raw)
}

func (m *InstanceManager) createFromRequest(path, workflowID string, version int, req n8n.N8nRequest, raw []byte) (*Instance, error) {
	wf, inputs := n8n.ToRivulet(req)
	if workflowID == "" {
		workflowID = string(wf.ID)
	}

	inst := &Instance{
		ID:              m.newID(),
		Name:            wf.Name,
		WorkflowPath:    path,
		WorkflowID:      workflowID,
		WorkflowVersion: version,
		Workflow:        wf,
		WorkflowRequest: req,
		WorkflowRaw:     append([]byte(nil), raw...),
		CreatedAt:       time.Now(),
		State:           InstanceRunning,
		q:               make(chan map[model.ID]model.Items, 64),
		deps:            m.deps,
		maxLogs:         1000,
	}

	ctx, cancel := context.WithCancel(context.Background())
	inst.cancel = cancel

	go func() {
		inst.logf("instance started: %s", inst.ID)
		if len(inputs) > 0 {
			select {
			case inst.q <- inputs:
			default:
				inst.logf("initial inputs dropped: queue full")
			}
		}
		for {
			select {
			case <-ctx.Done():
				inst.State = InstanceStopped
				inst.logf("instance stopped: %s", inst.ID)
				return
			case inputs := <-inst.q:
				execID := newRunID()
				inst.logf("execution started: %s", execID)
				start := time.Now().UTC()
				inst.statsMu.Lock()
				inst.active = ActiveExecution{
					ExecutionID: execID,
					StartedAt:   start,
					IsExecuting: true,
				}
				inst.statsMu.Unlock()

				outcome, err := ExecuteWorkflow(ctx, inst.deps, m.runs, ExecuteRequest{
					RunID:           execID,
					WorkflowID:      inst.WorkflowID,
					WorkflowVersion: inst.WorkflowVersion,
					WorkflowRequest: inst.WorkflowRequest,
					RawRequest:      inst.WorkflowRaw,
					Inputs:          inputs,
					Source:          "instance",
					Trigger:         "instance_enqueue",
					InstanceID:      inst.ID,
				})

				inst.statsMu.Lock()
				inst.stats.TotalExecutions++
				inst.stats.LastRunAt = time.Now().UTC()
				inst.lastRun = executionRecordFromRun(outcome.Run)
				inst.active = ActiveExecution{}
				if err != nil {
					inst.stats.FailedExecutions++
					inst.statsMu.Unlock()
					inst.logf("execution %s error: %v", execID, err)
					continue
				}
				inst.stats.SuccessfulExecutions++
				inst.stats.TotalSuccessDuration += time.Duration(outcome.Run.DurationMS) * time.Millisecond
				inst.statsMu.Unlock()

				total := 0
				for _, items := range outcome.Result {
					total += len(items)
				}
				inst.logf("execution %s completed, total items: %d", execID, total)
			}
		}
	}()

	m.mu.Lock()
	m.items[inst.ID] = inst
	m.mu.Unlock()
	return inst, nil
}

func (m *InstanceManager) Stop(id string) error {
	m.mu.Lock()
	inst, ok := m.items[id]
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("instance not found")
	}
	if inst.cancel != nil {
		inst.cancel()
	}
	return nil
}

func (m *InstanceManager) Enqueue(id string, inputs map[string]model.Items) error {
	m.mu.Lock()
	inst, ok := m.items[id]
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("instance not found")
	}
	// Convert map[string]model.Items to map[model.ID]model.Items for queue
	converted := make(map[model.ID]model.Items, len(inputs))
	for k, v := range inputs {
		converted[model.ID(k)] = v
	}
	select {
	case inst.q <- converted:
		return nil
	default:
		return fmt.Errorf("queue full")
	}
}

func (m *InstanceManager) Logs(id string) ([]string, error) {
	m.mu.Lock()
	inst, ok := m.items[id]
	m.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("instance not found")
	}
	inst.logMu.Lock()
	defer inst.logMu.Unlock()
	out := make([]string, len(inst.logs))
	copy(out, inst.logs)
	return out, nil
}

func executionRecordFromRun(run RunRecord) ExecutionRecord {
	return ExecutionRecord{
		ExecutionID:     run.ID,
		Status:          run.Status,
		WorkflowID:      run.WorkflowID,
		WorkflowKind:    run.WorkflowKind,
		AI:              run.AI,
		WorkflowVersion: run.WorkflowVersion,
		Source:          run.Source,
		Trigger:         run.Trigger,
		ScheduleID:      run.ScheduleID,
		StartedAt:       run.StartedAt,
		FinishedAt:      run.FinishedAt,
		DurationMS:      run.DurationMS,
		Input:           cloneItemsMap(run.Input),
		Result:          cloneItemsMap(run.Result),
		Error:           run.Error,
		Events:          append([]RunEvent(nil), run.Events...),
	}
}
