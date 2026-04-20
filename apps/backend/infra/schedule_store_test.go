package infra

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Tsinling0525/rivulet/model"
)

func TestScheduleStoreClaimDueAndComplete(t *testing.T) {
	store := &ScheduleStore{dir: filepath.Join(t.TempDir(), "schedules")}
	now := time.Date(2026, 4, 21, 10, 0, 0, 0, time.UTC)

	schedule, err := store.Create(ScheduleCreate{
		WorkflowID:      "wf-demo",
		IntervalSeconds: 60,
		Input:           map[model.ID]model.Items{"start": {{"trigger": "schedule"}}},
		Enabled:         true,
		NextRunAt:       now.Add(-time.Second),
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	claimed, err := store.ClaimDue(now, 10)
	if err != nil {
		t.Fatalf("ClaimDue returned error: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("expected 1 due schedule, got %d", len(claimed))
	}
	if !claimed[0].Running {
		t.Fatalf("expected claimed schedule to be marked running")
	}
	if !claimed[0].NextRunAt.After(now) {
		t.Fatalf("expected next_run_at to advance after claim")
	}

	claimedAgain, err := store.ClaimDue(now, 10)
	if err != nil {
		t.Fatalf("ClaimDue returned error: %v", err)
	}
	if len(claimedAgain) != 0 {
		t.Fatalf("expected running schedule not to be claimed again")
	}

	run := RunRecord{
		ID:        "run-1",
		Status:    "succeeded",
		StartedAt: now,
	}
	completed, err := store.CompleteRun(schedule.ID, run, nil)
	if err != nil {
		t.Fatalf("CompleteRun returned error: %v", err)
	}
	if completed.Running {
		t.Fatalf("expected completed schedule not to be running")
	}
	if completed.LastRunID != "run-1" || completed.LastStatus != "succeeded" {
		t.Fatalf("expected last run details to be recorded, got %q %q", completed.LastRunID, completed.LastStatus)
	}
}
