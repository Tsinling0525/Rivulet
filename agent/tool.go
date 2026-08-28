package agent

import (
	"context"
	"fmt"
	"sync"
)

type Tool interface {
	Name() string
	Execute(ctx context.Context, call ToolCall) (Observation, error)
}

type ToolFunc struct {
	name string
	fn   func(context.Context, ToolCall) (Observation, error)
}

func NewToolFunc(name string, fn func(context.Context, ToolCall) (Observation, error)) ToolFunc {
	return ToolFunc{name: name, fn: fn}
}

func (t ToolFunc) Name() string {
	return t.name
}

func (t ToolFunc) Execute(ctx context.Context, call ToolCall) (Observation, error) {
	if t.fn == nil {
		return Observation{}, fmt.Errorf("tool %q has no executor", t.name)
	}
	return t.fn(ctx, call)
}

type ToolResolver interface {
	ResolveTool(name string) (Tool, bool)
}

type Registry struct {
	mu     sync.RWMutex
	tools  map[string]registeredTool
	nextID uint64
}

type registeredTool struct {
	tool Tool
	id   uint64
}

func NewRegistry(tools ...Tool) *Registry {
	r := &Registry{tools: map[string]registeredTool{}}
	for _, tool := range tools {
		_ = r.Register(tool)
	}
	return r
}

// Register makes a tool available and returns the matching cleanup operation.
// The disposer restores a tool it replaced, or removes the tool when it was the
// first registration. It is idempotent and will not undo a later registration
// of the same name.
func (r *Registry) Register(tool Tool) func() {
	if tool == nil || tool.Name() == "" {
		return func() {}
	}
	name := tool.Name()
	r.mu.Lock()
	previous, hadPrevious := r.tools[name]
	r.nextID++
	registrationID := r.nextID
	r.tools[name] = registeredTool{tool: tool, id: registrationID}
	r.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			r.mu.Lock()
			defer r.mu.Unlock()
			current, ok := r.tools[name]
			if !ok || current.id != registrationID {
				return
			}
			if hadPrevious {
				r.tools[name] = previous
				return
			}
			delete(r.tools, name)
		})
	}
}

func (r *Registry) ResolveTool(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tool, ok := r.tools[name]
	return tool.tool, ok
}
