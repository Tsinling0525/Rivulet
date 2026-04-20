package infra

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tsinling0525/rivulet/format/n8n"
	apiinfra "github.com/Tsinling0525/rivulet/infra/api"
	"github.com/Tsinling0525/rivulet/model"
	_ "github.com/Tsinling0525/rivulet/nodes/echo"
	"github.com/Tsinling0525/rivulet/plugin"
)

func TestScheduleRunnerPersistsScheduledRun(t *testing.T) {
	root := t.TempDir()
	workflows := &WorkflowStore{dir: filepath.Join(root, "workflows")}
	runs := &RunStore{dir: filepath.Join(root, "runs")}
	schedules := &ScheduleStore{dir: filepath.Join(root, "schedules")}

	req := n8n.N8nRequest{
		Workflow: n8n.N8nWorkflow{
			ID:   "wf-scheduled",
			Name: "Scheduled",
			Nodes: []n8n.N8nNode{
				{ID: "echo1", Name: "Echo", Type: "echo", Parameters: map[string]any{"label": "scheduled"}},
			},
		},
	}
	if _, err := workflows.Create(req, "", true); err != nil {
		t.Fatalf("Create workflow returned error: %v", err)
	}

	schedule, err := schedules.Create(ScheduleCreate{
		WorkflowID:      "wf-scheduled",
		IntervalSeconds: 60,
		Input:           map[model.ID]model.Items{"echo1": {{"message": "scheduled"}}},
		Enabled:         true,
		NextRunAt:       time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("Create schedule returned error: %v", err)
	}

	deps := plugin.Deps{State: apiinfra.MemState{}, Bus: apiinfra.NullBus{}, Files: NewLocalFiles()}
	runner := NewScheduleRunner(deps, workflows, runs, schedules)
	runner.runSchedule(context.Background(), schedule)

	persistedRuns, err := runs.List("wf-scheduled", 10)
	if err != nil {
		t.Fatalf("List runs returned error: %v", err)
	}
	if len(persistedRuns) != 1 {
		t.Fatalf("expected 1 persisted run, got %d", len(persistedRuns))
	}
	if persistedRuns[0].Source != "schedule" || persistedRuns[0].ScheduleID != schedule.ID {
		t.Fatalf("expected scheduled run metadata, got source=%q schedule_id=%q", persistedRuns[0].Source, persistedRuns[0].ScheduleID)
	}

	updated, err := schedules.Get(schedule.ID)
	if err != nil {
		t.Fatalf("Get schedule returned error: %v", err)
	}
	if updated.LastRunID != persistedRuns[0].ID || updated.LastStatus != "succeeded" {
		t.Fatalf("expected schedule last run details, got run=%q status=%q", updated.LastRunID, updated.LastStatus)
	}
}
