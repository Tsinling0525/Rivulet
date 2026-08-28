package agent

import (
	"context"
	"testing"
)

func TestRegistryRegisterDisposerRestoresPriorTool(t *testing.T) {
	first := NewToolFunc("echo", func(_ context.Context, _ ToolCall) (Observation, error) { return Observation{Summary: "first"}, nil })
	second := NewToolFunc("echo", func(_ context.Context, _ ToolCall) (Observation, error) { return Observation{Summary: "second"}, nil })
	registry := NewRegistry(first)
	dispose := registry.Register(second)

	resolved, ok := registry.ResolveTool("echo")
	if !ok {
		t.Fatal("expected replacement tool to be visible")
	}
	observation, err := resolved.Execute(context.Background(), ToolCall{Name: "echo"})
	if err != nil || observation.Summary != "second" {
		t.Fatalf("replacement tool result = %#v, %v", observation, err)
	}
	dispose()
	resolved, ok = registry.ResolveTool("echo")
	if !ok {
		t.Fatal("expected disposer to restore the previous tool")
	}
	observation, err = resolved.Execute(context.Background(), ToolCall{Name: "echo"})
	if err != nil || observation.Summary != "first" {
		t.Fatalf("restored tool result = %#v, %v", observation, err)
	}
}
