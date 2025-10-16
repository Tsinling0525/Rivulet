package infra

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/Tsinling0525/rivulet/engine"
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
	ID           string
	Name         string
	WorkflowPath string
	Workflow     model.Workflow
	CreatedAt    time.Time
	State        InstanceState

	q       chan map[model.ID]model.Items
	cancel  context.CancelFunc
	deps    plugin.Deps
	logMu   sync.Mutex
	logs    []string
	maxLogs int
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

type InstanceManager struct {
	mu    sync.Mutex
	items map[string]*Instance
	deps  plugin.Deps
	newID func() string
}

func NewInstanceManager() *InstanceManager {
	deps := plugin.Deps{State: apiinfra.MemState{}, Bus: apiinfra.NullBus{}, Files: NewLocalFiles()}
	return &InstanceManager{
		items: make(map[string]*Instance),
		deps:  deps,
		newID: func() string { return fmt.Sprintf("inst-%d", time.Now().UnixNano()) },
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
	wf, inputs := n8n.ToRivulet(req)

	inst := &Instance{
		ID:           m.newID(),
		Name:         wf.Name,
		WorkflowPath: path,
		Workflow:     wf,
		CreatedAt:    time.Now(),
		State:        InstanceRunning,
		q:            make(chan map[model.ID]model.Items, 64),
		deps:         m.deps,
		maxLogs:      1000,
	}

	ctx, cancel := context.WithCancel(context.Background())
	inst.cancel = cancel
	eng := engine.New(m.deps)

	go func() {
		inst.logf("instance started: %s", inst.ID)
		// Auto-enqueue initial inputs from the workflow file if present
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
				execID := fmt.Sprintf("exec-%d", time.Now().UnixNano())
				inst.logf("execution started: %s", execID)
				res, err := eng.Run(ctx, execID, inst.Workflow, inputs)
				if err != nil {
					inst.logf("execution %s error: %v", execID, err)
					continue
				}
				// summarize results
				total := 0
				for _, items := range res {
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
