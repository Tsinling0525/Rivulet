package infra

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Tsinling0525/rivulet/model"
)

func TestReviewStoreCreateApproveReject(t *testing.T) {
	store := &ReviewStore{dir: filepath.Join(t.TempDir(), "reviews")}

	review, err := store.Create(context.Background(), model.ReviewCreate{
		RunID:          "run-1",
		WorkflowID:     "wf-ai",
		WorkflowName:   "AI Workflow",
		WorkflowKind:   model.WorkflowKindAI,
		NodeID:         "review",
		NodeName:       "Human Review",
		Input:          model.Item{"message": "hello"},
		ProposedOutput: "draft reply",
		Context:        model.Item{"prompt": "draft"},
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if review.Status != model.ReviewPending {
		t.Fatalf("expected pending review, got %q", review.Status)
	}

	pending, err := store.List(context.Background(), model.ReviewPending)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending review, got %d", len(pending))
	}

	approved, err := store.Approve(context.Background(), review.ID, "tester", "looks good")
	if err != nil {
		t.Fatalf("Approve returned error: %v", err)
	}
	if approved.Status != model.ReviewApproved {
		t.Fatalf("expected approved review, got %q", approved.Status)
	}
	if approved.Reviewer != "tester" || approved.Comment != "looks good" {
		t.Fatalf("expected decision metadata to be recorded")
	}

	pending, err = store.List(context.Background(), model.ReviewPending)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected no pending reviews after approval, got %d", len(pending))
	}
}
