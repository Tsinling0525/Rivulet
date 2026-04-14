package infra

import (
	"sort"
	"time"
)

// DashboardMetrics represents aggregated statistics that power the UI dashboard.
type DashboardMetrics struct {
	WorkflowCompletionRate       float64                 `json:"workflow_completion_rate"`
	WorkflowCompletionTrend      float64                 `json:"workflow_completion_trend"`
	TotalTasks                   int                     `json:"total_tasks"`
	SuccessfulExecutions         int                     `json:"successful_executions"`
	FailedExecutions             int                     `json:"failed_executions"`
	TaskStatus                   map[string]int          `json:"task_status"`
	TeamPerformance              []TeamMemberPerformance `json:"team_performance"`
	AverageTaskCompletionSeconds float64                 `json:"average_task_completion_seconds"`
	AverageTaskCompletionTrend   float64                 `json:"average_task_completion_trend"`
	LastUpdated                  time.Time               `json:"last_updated"`
	Instances                    int                     `json:"instances"`
}

// TeamMemberPerformance captures lightweight per-owner metrics for display.
type TeamMemberPerformance struct {
	Name                   string  `json:"name"`
	TasksCompleted         int     `json:"tasks_completed"`
	AverageDurationSeconds float64 `json:"average_duration_seconds"`
}

// DashboardMetrics aggregates execution data across all instances.
func (m *InstanceManager) DashboardMetrics() DashboardMetrics {
	instances := m.List()
	snapshots := make([]InstanceSnapshot, 0, len(instances))
	for _, inst := range instances {
		snapshots = append(snapshots, inst.Snapshot())
	}

	taskStatus := map[string]int{
		"todo":        0,
		"in_progress": 0,
		"completed":   0,
		"blocked":     0,
	}

	var (
		totalExecutions int
		successful      int
		failed          int
		totalDuration   time.Duration
	)

	type teamAgg struct {
		name      string
		completed int
		duration  time.Duration
	}

	team := map[string]*teamAgg{}

	for _, snap := range snapshots {
		totalExecutions += snap.Stats.TotalExecutions
		successful += snap.Stats.SuccessfulExecutions
		failed += snap.Stats.FailedExecutions
		totalDuration += snap.Stats.TotalSuccessDuration

		taskStatus["todo"] += snap.QueueLength
		if snap.State == InstanceRunning {
			taskStatus["in_progress"]++
		} else {
			taskStatus["completed"]++
		}
		taskStatus["blocked"] += snap.Stats.FailedExecutions

		name := snap.Name
		if name == "" {
			name = "Unassigned"
		}
		entry, ok := team[name]
		if !ok {
			entry = &teamAgg{name: name}
			team[name] = entry
		}
		entry.completed += snap.Stats.SuccessfulExecutions
		entry.duration += snap.Stats.TotalSuccessDuration
	}

	teamPerformance := make([]TeamMemberPerformance, 0, len(team))
	for _, entry := range team {
		avgSeconds := 0.0
		if entry.completed > 0 {
			avgSeconds = entry.duration.Seconds() / float64(entry.completed)
		}
		teamPerformance = append(teamPerformance, TeamMemberPerformance{
			Name:                   entry.name,
			TasksCompleted:         entry.completed,
			AverageDurationSeconds: avgSeconds,
		})
	}

	sort.Slice(teamPerformance, func(i, j int) bool {
		return teamPerformance[i].TasksCompleted > teamPerformance[j].TasksCompleted
	})

	totalTasks := taskStatus["todo"] + taskStatus["in_progress"] + taskStatus["completed"] + taskStatus["blocked"]
	completionRate := 0.0
	if totalExecutions > 0 {
		completionRate = (float64(successful) / float64(totalExecutions)) * 100
	}

	avgSeconds := 0.0
	if successful > 0 {
		avgSeconds = totalDuration.Seconds() / float64(successful)
	}

	return DashboardMetrics{
		WorkflowCompletionRate:       completionRate,
		WorkflowCompletionTrend:      0, // Future: compare with historical window
		TotalTasks:                   totalTasks,
		SuccessfulExecutions:         successful,
		FailedExecutions:             failed,
		TaskStatus:                   taskStatus,
		TeamPerformance:              teamPerformance,
		AverageTaskCompletionSeconds: avgSeconds,
		AverageTaskCompletionTrend:   0, // Future: compare with historical window
		LastUpdated:                  time.Now(),
		Instances:                    len(snapshots),
	}
}
