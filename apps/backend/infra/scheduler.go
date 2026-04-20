package infra

import (
	"context"
	"time"

	"github.com/Tsinling0525/rivulet/plugin"
)

type ScheduleRunner struct {
	deps        plugin.Deps
	workflows   *WorkflowStore
	runs        *RunStore
	schedules   *ScheduleStore
	checkpoints *CheckpointStore
	limit       int
}

func NewScheduleRunner(deps plugin.Deps, workflows *WorkflowStore, runs *RunStore, schedules *ScheduleStore, checkpoints *CheckpointStore) *ScheduleRunner {
	return &ScheduleRunner{
		deps:        deps,
		workflows:   workflows,
		runs:        runs,
		schedules:   schedules,
		checkpoints: checkpoints,
		limit:       32,
	}
}

func (r *ScheduleRunner) Start(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Second
	}
	_ = r.schedules.ResetRunning()
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				r.RunDue(ctx, now.UTC())
			}
		}
	}()
}

func (r *ScheduleRunner) RunDue(ctx context.Context, now time.Time) {
	claimed, err := r.schedules.ClaimDue(now.UTC(), r.limit)
	if err != nil {
		return
	}
	for _, schedule := range claimed {
		schedule := schedule
		go r.runSchedule(ctx, schedule)
	}
}

func (r *ScheduleRunner) runSchedule(ctx context.Context, schedule Schedule) {
	record, req, err := r.workflows.LoadVersionRequest(schedule.WorkflowID, schedule.WorkflowVersion)
	if err != nil {
		run := failedScheduledRun(schedule, err)
		_ = r.runs.Save(run)
		_, _ = r.schedules.CompleteRun(schedule.ID, run, err)
		return
	}

	version := schedule.WorkflowVersion
	if version == 0 {
		version = record.ActiveVersion
	}
	outcome, err := ExecuteWorkflow(ctx, r.deps, r.runs, ExecuteRequest{
		WorkflowID:      record.ID,
		WorkflowVersion: version,
		WorkflowRequest: req,
		Inputs:          cloneItemsMap(schedule.Input),
		Source:          "schedule",
		Trigger:         "schedule",
		ScheduleID:      schedule.ID,
		Checkpoints:     r.checkpoints,
	})
	_, _ = r.schedules.CompleteRun(schedule.ID, outcome.Run, err)
}

func failedScheduledRun(schedule Schedule, err error) RunRecord {
	now := time.Now().UTC()
	return RunRecord{
		ID:              newRunID(),
		WorkflowID:      schedule.WorkflowID,
		WorkflowVersion: schedule.WorkflowVersion,
		Source:          "schedule",
		Trigger:         "schedule",
		ScheduleID:      schedule.ID,
		Status:          "failed",
		StartedAt:       now,
		FinishedAt:      now,
		Error:           err.Error(),
		Input:           cloneItemsMap(schedule.Input),
	}
}
