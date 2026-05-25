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
	mu    sync.RWMutex
	tools map[string]Tool
}

func NewRegistry(tools ...Tool) *Registry {
	r := &Registry{tools: map[string]Tool{}}
	for _, tool := range tools {
		r.Register(tool)
	}
	return r
}

func (r *Registry) Register(tool Tool) {
	if tool == nil || tool.Name() == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[tool.Name()] = tool
}

func (r *Registry) ResolveTool(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tool, ok := r.tools[name]
	return tool, ok
}
