package infra

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Tsinling0525/rivulet/format/n8n"
	apiinfra "github.com/Tsinling0525/rivulet/infra/api"
	_ "github.com/Tsinling0525/rivulet/nodes/echo"
	"github.com/Tsinling0525/rivulet/plugin"
)

func TestExecuteWorkflowPersistsRunsAndReplay(t *testing.T) {
	store := &RunStore{dir: filepath.Join(t.TempDir(), "runs")}
	deps := plugin.Deps{State: apiinfra.MemState{}, Bus: apiinfra.NullBus{}, Files: NewLocalFiles()}
	req := n8n.N8nRequest{
		Workflow: n8n.N8nWorkflow{
			ID:   "wf-run",
			Name: "Run test",
			Nodes: []n8n.N8nNode{
				{ID: "echo1", Name: "Echo", Type: "echo", Parameters: map[string]any{"label": "hi"}},
			},
		},
		Data: map[string]any{
			"echo1": []any{map[string]any{"message": "hello"}},
		},
	}

	outcome, err := ExecuteWorkflow(context.Background(), deps, store, ExecuteRequest{
		WorkflowID:      "wf-run",
		WorkflowVersion: 1,
		WorkflowRequest: req,
		Source:          "test",
		Trigger:         "manual",
	})
	if err != nil {
		t.Fatalf("ExecuteWorkflow returned error: %v", err)
	}
	if outcome.Run.Status != "succeeded" {
		t.Fatalf("expected succeeded run, got %q", outcome.Run.Status)
	}
	if len(outcome.Run.Events) == 0 {
		t.Fatalf("expected events to be recorded")
	}

	runs, err := store.List("wf-run", 10)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 persisted run, got %d", len(runs))
	}

	replay, err := store.Replay(context.Background(), deps, outcome.Run.ID, nil)
	if err != nil {
		t.Fatalf("Replay returned error: %v", err)
	}
	if replay.Run.Source != "replay" {
		t.Fatalf("expected replay source, got %q", replay.Run.Source)
	}
	if replay.Run.ID == outcome.Run.ID {
		t.Fatalf("expected replay to create a new run id")
	}
	if replay.Run.Status != "succeeded" {
		t.Fatalf("expected replay to succeed, got %q", replay.Run.Status)
	}
}
