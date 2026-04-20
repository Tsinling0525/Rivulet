package review

import (
	"context"
	"testing"

	"github.com/Tsinling0525/rivulet/infra"
	apiinfra "github.com/Tsinling0525/rivulet/infra/api"
	"github.com/Tsinling0525/rivulet/model"
	"github.com/Tsinling0525/rivulet/plugin"
)

func TestGateCreatesPendingReviewAndStopsByDefault(t *testing.T) {
	t.Setenv("RIV_DATA_DIR", t.TempDir())
	store := infra.NewReviewStore()
	gate := &Gate{}
	if err := gate.Init(context.Background(), plugin.Deps{
		Bus:     apiinfra.NullBus{},
		Reviews: store,
	}); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}

	ctx := plugin.WithExecutionID(context.Background(), "run-1")
	out, err := gate.Process(ctx, model.Workflow{ID: "wf", Name: "Workflow", Kind: model.WorkflowKindAI}, model.Node{
		ID:   "review",
		Name: "Review",
		Type: "review:gate",
		Config: map[string]any{
			"output_field":   "draft",
			"context_fields": []any{"prompt"},
		},
	}, model.Items{{"prompt": "draft a reply", "draft": "hello"}})
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("expected default gate to stop output, got %d items", len(out))
	}

	reviews, err := store.List(context.Background(), model.ReviewPending)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(reviews) != 1 {
		t.Fatalf("expected 1 pending review, got %d", len(reviews))
	}
	if reviews[0].ProposedOutput != "hello" {
		t.Fatalf("expected proposed output to use configured field, got %v", reviews[0].ProposedOutput)
	}
	if reviews[0].Context["prompt"] != "draft a reply" {
		t.Fatalf("expected selected context to be recorded")
	}
}

func TestGateCanPassThroughAnnotatedItems(t *testing.T) {
	t.Setenv("RIV_DATA_DIR", t.TempDir())
	store := infra.NewReviewStore()
	gate := &Gate{}
	if err := gate.Init(context.Background(), plugin.Deps{
		Bus:     apiinfra.NullBus{},
		Reviews: store,
	}); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}

	out, err := gate.Process(plugin.WithExecutionID(context.Background(), "run-1"), model.Workflow{ID: "wf"}, model.Node{
		ID:   "review",
		Type: "review:gate",
		Config: map[string]any{
			"pass_through": true,
		},
	}, model.Items{{"output": "ok"}})
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 pass-through item, got %d", len(out))
	}
	if out[0]["review_required"] != true || out[0]["review_status"] != model.ReviewPending {
		t.Fatalf("expected review annotations, got %+v", out[0])
	}
	if out[0]["review_id"] == "" {
		t.Fatalf("expected review id annotation")
	}
}
