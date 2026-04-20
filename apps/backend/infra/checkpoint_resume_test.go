package infra

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Tsinling0525/rivulet/engine"
	"github.com/Tsinling0525/rivulet/format/n8n"
	apiinfra "github.com/Tsinling0525/rivulet/infra/api"
	_ "github.com/Tsinling0525/rivulet/nodes/echo"
	_ "github.com/Tsinling0525/rivulet/nodes/review"
	"github.com/Tsinling0525/rivulet/plugin"
)

func TestExecuteWorkflowPausesAndResumesFromReviewCheckpoint(t *testing.T) {
	root := t.TempDir()
	runs := &RunStore{dir: filepath.Join(root, "runs")}
	checkpoints := &CheckpointStore{dir: filepath.Join(root, "checkpoints")}
	reviews := &ReviewStore{dir: filepath.Join(root, "reviews")}
	deps := plugin.Deps{State: apiinfra.MemState{}, Bus: apiinfra.NullBus{}, Files: NewLocalFiles(), Reviews: reviews}

	req := n8n.N8nRequest{
		Workflow: n8n.N8nWorkflow{
			ID:   "wf-review",
			Name: "Review Workflow",
			Nodes: []n8n.N8nNode{
				{
					ID:         "review",
					Name:       "Review",
					Type:       "review:gate",
					Parameters: map[string]any{"output_field": "output"},
				},
				{
					ID:         "echo",
					Name:       "Echo",
					Type:       "echo",
					Parameters: map[string]any{"label": "approved"},
				},
			},
			Connections: map[string]n8n.N8nConnections{
				"review": {Main: [][]n8n.N8nConnection{{{Node: "echo", Type: "main"}}}},
			},
		},
		Data: map[string]any{
			"review": []any{map[string]any{"output": "draft reply"}},
		},
	}

	outcome, err := ExecuteWorkflow(context.Background(), deps, runs, ExecuteRequest{
		WorkflowRequest: req,
		Source:          "test",
		Trigger:         "manual",
		Checkpoints:     checkpoints,
	})
	if !errors.Is(err, engine.ErrExecutionPaused) {
		t.Fatalf("expected execution paused, got %v", err)
	}
	if outcome.Run.Status != "paused" {
		t.Fatalf("expected paused run, got %q", outcome.Run.Status)
	}
	if outcome.Run.CheckpointID == "" || outcome.Run.ReviewID == "" {
		t.Fatalf("expected run to record checkpoint and review ids: %+v", outcome.Run)
	}

	pending, err := reviews.List(context.Background(), "pending")
	if err != nil {
		t.Fatalf("List reviews returned error: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending review, got %d", len(pending))
	}

	resumed, err := ResumeCheckpoint(context.Background(), deps, runs, checkpoints, outcome.Run.CheckpointID)
	if err != nil {
		t.Fatalf("ResumeCheckpoint returned error: %v", err)
	}
	if resumed.Run.Status != "succeeded" {
		t.Fatalf("expected resumed run to succeed, got %q", resumed.Run.Status)
	}
	if len(resumed.Result["echo"]) != 1 {
		t.Fatalf("expected downstream echo result after resume, got %+v", resumed.Result)
	}

	checkpoint, err := checkpoints.Get(outcome.Run.CheckpointID)
	if err != nil {
		t.Fatalf("Get checkpoint returned error: %v", err)
	}
	if checkpoint.Status != CheckpointResumed {
		t.Fatalf("expected checkpoint resumed, got %q", checkpoint.Status)
	}
}
