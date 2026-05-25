package infra

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
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
	PromptVersions               []PromptVersionMetric   `json:"prompt_versions"`
	ReasoningTraces              []ReasoningTraceMetric  `json:"reasoning_traces"`
}

// TeamMemberPerformance captures lightweight per-owner metrics for display.
type TeamMemberPerformance struct {
	Name                   string  `json:"name"`
	TasksCompleted         int     `json:"tasks_completed"`
	AverageDurationSeconds float64 `json:"average_duration_seconds"`
}

// PromptVersionMetric captures prompt-hash version impact across persisted AI model calls.
type PromptVersionMetric struct {
	GroupID            string           `json:"group_id"`
	Version            string           `json:"version"`
	VersionNumber      int              `json:"version_number"`
	PromptHash         string           `json:"prompt_hash"`
	PreviousPromptHash string           `json:"previous_prompt_hash,omitempty"`
	WorkflowID         string           `json:"workflow_id,omitempty"`
	WorkflowName       string           `json:"workflow_name,omitempty"`
	NodeID             string           `json:"node_id,omitempty"`
	Provider           string           `json:"provider,omitempty"`
	Model              string           `json:"model,omitempty"`
	FirstSeenAt        time.Time        `json:"first_seen_at"`
	LastSeenAt         time.Time        `json:"last_seen_at"`
	Calls              int              `json:"calls"`
	Succeeded          int              `json:"succeeded"`
	Failed             int              `json:"failed"`
	SuccessRate        float64          `json:"success_rate"`
	InputTokens        int              `json:"input_tokens"`
	OutputTokens       int              `json:"output_tokens"`
	TotalTokens        int              `json:"total_tokens"`
	AverageTotalTokens float64          `json:"average_total_tokens"`
	AverageLatencyMS   float64          `json:"average_latency_ms"`
	PromptPreview      string           `json:"prompt_preview,omitempty"`
	PromptPreviewDiff  []PromptDiffLine `json:"prompt_preview_diff,omitempty"`
}

// PromptDiffLine is a compact line-based diff between adjacent prompt versions.
type PromptDiffLine struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ReasoningTraceMetric is the dashboard-friendly view of AI reasoning progress.
type ReasoningTraceMetric struct {
	ExecutionID       string               `json:"execution_id"`
	WorkflowID        string               `json:"workflow_id,omitempty"`
	WorkflowName      string               `json:"workflow_name,omitempty"`
	WorkflowKind      string               `json:"workflow_kind,omitempty"`
	NodeID            string               `json:"node_id,omitempty"`
	Provider          string               `json:"provider,omitempty"`
	Model             string               `json:"model,omitempty"`
	Status            string               `json:"status,omitempty"`
	StartedAt         time.Time            `json:"started_at"`
	UpdatedAt         time.Time            `json:"updated_at"`
	TotalLatencyMS    int64                `json:"total_latency_ms,omitempty"`
	StepCount         int                  `json:"step_count"`
	PromptPreview     string               `json:"prompt_preview,omitempty"`
	OutputPreview     string               `json:"output_preview,omitempty"`
	SupportsReasoning bool                 `json:"supports_reasoning"`
	Live              bool                 `json:"live"`
	Steps             []ReasoningTraceStep `json:"steps,omitempty"`
}

type ReasoningTraceStep struct {
	Index      int       `json:"index"`
	Title      string    `json:"title"`
	Text       string    `json:"text,omitempty"`
	Source     string    `json:"source,omitempty"`
	Status     string    `json:"status,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
	LatencyMS  int64     `json:"latency_ms,omitempty"`
	DeltaMS    int64     `json:"delta_ms,omitempty"`
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

	var promptVersions []PromptVersionMetric
	if m.runs != nil {
		promptVersions, _ = m.runs.PromptVersionMetrics(12)
	}
	reasoningTraces := m.ReasoningTraces(16)

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
		PromptVersions:               promptVersions,
		ReasoningTraces:              reasoningTraces,
	}
}

// ReasoningTraces combines in-flight instance events with persisted AI run events.
func (m *InstanceManager) ReasoningTraces(limit int) []ReasoningTraceMetric {
	traces := map[string]*ReasoningTraceMetric{}

	for _, inst := range m.List() {
		snap := inst.Snapshot()
		if snap.Active.IsExecuting {
			run := RunRecord{
				ID:           snap.Active.ExecutionID,
				WorkflowID:   inst.WorkflowID,
				WorkflowName: inst.Name,
				WorkflowKind: inst.Workflow.Kind,
				Status:       "running",
				StartedAt:    snap.Active.StartedAt,
				Events:       snap.Active.Events,
			}
			collectReasoningTraces(traces, run, true)
		}
	}

	if m.runs != nil {
		runs, err := m.runs.List("", 40)
		if err == nil {
			for _, run := range runs {
				collectReasoningTraces(traces, run, false)
			}
		}
	}

	out := make([]ReasoningTraceMetric, 0, len(traces))
	for _, trace := range traces {
		if len(trace.Steps) == 0 && (trace.PromptPreview != "" || trace.OutputPreview != "" || trace.TotalLatencyMS > 0) {
			trace.Steps = append(trace.Steps, ReasoningTraceStep{
				Index:      1,
				Title:      "Model call completed",
				Text:       trace.OutputPreview,
				Source:     "ai_model_call",
				Status:     trace.Status,
				OccurredAt: trace.UpdatedAt,
				LatencyMS:  trace.TotalLatencyMS,
				DeltaMS:    trace.TotalLatencyMS,
			})
		}
		trace.StepCount = len(trace.Steps)
		if trace.StepCount == 0 {
			continue
		}
		sort.SliceStable(trace.Steps, func(i, j int) bool {
			if trace.Steps[i].Index == trace.Steps[j].Index {
				return trace.Steps[i].OccurredAt.Before(trace.Steps[j].OccurredAt)
			}
			return trace.Steps[i].Index < trace.Steps[j].Index
		})
		out = append(out, *trace)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Live != out[j].Live {
			return out[i].Live
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func collectReasoningTraces(traces map[string]*ReasoningTraceMetric, run RunRecord, live bool) {
	for _, event := range run.Events {
		if event.Type != "ai_reasoning_step" && event.Type != "ai_model_call" {
			continue
		}
		nodeID := event.NodeID
		if nodeID == "" {
			nodeID = stringField(event.Fields, "node")
		}
		if nodeID == "" {
			nodeID = "unknown"
		}
		key := run.ID + "::" + nodeID
		trace, ok := traces[key]
		if !ok {
			trace = &ReasoningTraceMetric{
				ExecutionID:  run.ID,
				WorkflowID:   run.WorkflowID,
				WorkflowName: run.WorkflowName,
				WorkflowKind: string(run.WorkflowKind),
				NodeID:       nodeID,
				Status:       run.Status,
				StartedAt:    run.StartedAt,
				UpdatedAt:    run.StartedAt,
				Live:         live,
			}
			traces[key] = trace
		}
		if trace.Status == "" {
			trace.Status = run.Status
		}
		if trace.Provider == "" {
			trace.Provider = stringField(event.Fields, "provider")
		}
		if trace.Model == "" {
			trace.Model = stringField(event.Fields, "model")
		}
		if trace.WorkflowKind == "" {
			trace.WorkflowKind = stringField(event.Fields, "workflow_kind")
		}
		if event.OccurredAt.After(trace.UpdatedAt) {
			trace.UpdatedAt = event.OccurredAt
		}
		trace.SupportsReasoning = trace.SupportsReasoning || supportsReasoningTrace(trace.Provider, trace.Model)

		switch event.Type {
		case "ai_reasoning_step":
			index := intField(event.Fields, "step_index")
			if index == 0 {
				index = len(trace.Steps) + 1
			}
			title := stringField(event.Fields, "title")
			if title == "" {
				title = "Reasoning step"
			}
			trace.Steps = append(trace.Steps, ReasoningTraceStep{
				Index:      index,
				Title:      title,
				Text:       stringField(event.Fields, "text"),
				Source:     stringField(event.Fields, "source"),
				Status:     stringField(event.Fields, "status"),
				OccurredAt: event.OccurredAt,
				LatencyMS:  int64(intField(event.Fields, "latency_ms")),
				DeltaMS:    int64(intField(event.Fields, "delta_ms")),
			})
		case "ai_model_call":
			trace.PromptPreview = firstNonEmpty(trace.PromptPreview, stringField(event.Fields, "prompt_preview"))
			trace.OutputPreview = firstNonEmpty(trace.OutputPreview, stringField(event.Fields, "output_preview"))
			trace.TotalLatencyMS = int64(intField(event.Fields, "latency_ms"))
			if status := stringField(event.Fields, "status"); status != "" {
				trace.Status = status
			}
			if extra, ok := event.Fields["extra"].(map[string]any); ok {
				if reason := stringField(extra, "route_reason"); reason != "" {
					trace.Steps = append(trace.Steps, ReasoningTraceStep{
						Index:      len(trace.Steps) + 1,
						Title:      "Routing decision",
						Text:       reason,
						Source:     "router",
						Status:     trace.Status,
						OccurredAt: event.OccurredAt,
					})
				}
			}
		}
	}
}

func supportsReasoningTrace(provider, modelName string) bool {
	value := strings.ToLower(provider + " " + modelName)
	return strings.Contains(value, "o1") ||
		strings.Contains(value, "o3") ||
		strings.Contains(value, "o4") ||
		strings.Contains(value, "gpt-5") ||
		strings.Contains(value, "deepseek") ||
		strings.Contains(value, "reason")
}

func firstNonEmpty(current, next string) string {
	if current != "" {
		return current
	}
	return next
}

type promptVersionAggregate struct {
	promptHash    string
	workflowID    string
	workflowName  string
	nodeID        string
	provider      string
	model         string
	firstSeenAt   time.Time
	lastSeenAt    time.Time
	calls         int
	succeeded     int
	failed        int
	inputTokens   int
	outputTokens  int
	totalTokens   int
	totalLatency  int64
	latencyCalls  int
	promptPreview string
}

// PromptVersionMetrics groups persisted ai_model_call events into git-like prompt versions.
func (s *RunStore) PromptVersionMetrics(limit int) ([]PromptVersionMetric, error) {
	runs, err := s.List("", 0)
	if err != nil {
		return nil, err
	}

	groups := map[string]map[string]*promptVersionAggregate{}
	for _, run := range runs {
		for _, event := range run.Events {
			if event.Type != "ai_model_call" {
				continue
			}
			promptHash := stringField(event.Fields, "prompt_template_hash")
			if promptHash == "" {
				promptHash = stringField(event.Fields, "prompt_hash")
			}
			if promptHash == "" {
				continue
			}
			workflowID := stringField(event.Fields, "workflow")
			if workflowID == "" {
				workflowID = run.WorkflowID
			}
			nodeID := event.NodeID
			if nodeID == "" {
				nodeID = stringField(event.Fields, "node")
			}
			provider := stringField(event.Fields, "provider")
			modelName := stringField(event.Fields, "model")
			groupID := strings.Join([]string{workflowID, nodeID, provider, modelName}, "::")

			byHash, ok := groups[groupID]
			if !ok {
				byHash = map[string]*promptVersionAggregate{}
				groups[groupID] = byHash
			}
			agg, ok := byHash[promptHash]
			if !ok {
				agg = &promptVersionAggregate{
					promptHash:   promptHash,
					workflowID:   workflowID,
					workflowName: run.WorkflowName,
					nodeID:       nodeID,
					provider:     provider,
					model:        modelName,
					firstSeenAt:  event.OccurredAt,
					lastSeenAt:   event.OccurredAt,
				}
				byHash[promptHash] = agg
			}
			if agg.workflowName == "" {
				agg.workflowName = run.WorkflowName
			}
			if event.OccurredAt.Before(agg.firstSeenAt) || agg.firstSeenAt.IsZero() {
				agg.firstSeenAt = event.OccurredAt
			}
			if event.OccurredAt.After(agg.lastSeenAt) {
				agg.lastSeenAt = event.OccurredAt
			}
			agg.calls++
			switch stringField(event.Fields, "status") {
			case "succeeded", "cached":
				agg.succeeded++
			case "failed":
				agg.failed++
			}
			if preview := stringField(event.Fields, "prompt_preview"); preview != "" && agg.promptPreview == "" {
				agg.promptPreview = preview
			}
			if tokens, ok := event.Fields["tokens"].(map[string]any); ok {
				agg.inputTokens += intField(tokens, "input")
				agg.outputTokens += intField(tokens, "output")
				agg.totalTokens += intField(tokens, "total")
			}
			if latency := intField(event.Fields, "latency_ms"); latency > 0 {
				agg.totalLatency += int64(latency)
				agg.latencyCalls++
			}
		}
	}

	out := make([]PromptVersionMetric, 0)
	for groupID, byHash := range groups {
		versions := make([]*promptVersionAggregate, 0, len(byHash))
		for _, agg := range byHash {
			versions = append(versions, agg)
		}
		sort.Slice(versions, func(i, j int) bool {
			if versions[i].firstSeenAt.Equal(versions[j].firstSeenAt) {
				return versions[i].promptHash < versions[j].promptHash
			}
			return versions[i].firstSeenAt.Before(versions[j].firstSeenAt)
		})

		for idx, agg := range versions {
			metric := PromptVersionMetric{
				GroupID:       groupID,
				Version:       "v" + strconv.Itoa(idx+1),
				VersionNumber: idx + 1,
				PromptHash:    agg.promptHash,
				WorkflowID:    agg.workflowID,
				WorkflowName:  agg.workflowName,
				NodeID:        agg.nodeID,
				Provider:      agg.provider,
				Model:         agg.model,
				FirstSeenAt:   agg.firstSeenAt,
				LastSeenAt:    agg.lastSeenAt,
				Calls:         agg.calls,
				Succeeded:     agg.succeeded,
				Failed:        agg.failed,
				InputTokens:   agg.inputTokens,
				OutputTokens:  agg.outputTokens,
				TotalTokens:   agg.totalTokens,
				PromptPreview: agg.promptPreview,
			}
			if agg.calls > 0 {
				metric.SuccessRate = (float64(agg.succeeded) / float64(agg.calls)) * 100
				metric.AverageTotalTokens = float64(agg.totalTokens) / float64(agg.calls)
			}
			if agg.latencyCalls > 0 {
				metric.AverageLatencyMS = float64(agg.totalLatency) / float64(agg.latencyCalls)
			}
			if idx > 0 {
				prev := versions[idx-1]
				metric.PreviousPromptHash = prev.promptHash
				metric.PromptPreviewDiff = diffPromptPreview(prev.promptPreview, agg.promptPreview)
			}
			out = append(out, metric)
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].LastSeenAt.Equal(out[j].LastSeenAt) {
			return out[i].VersionNumber > out[j].VersionNumber
		}
		return out[i].LastSeenAt.After(out[j].LastSeenAt)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func diffPromptPreview(prev, next string) []PromptDiffLine {
	prevLines := splitPromptLines(prev)
	nextLines := splitPromptLines(next)
	if len(prevLines) == 0 && len(nextLines) == 0 {
		return nil
	}
	lcs := make([][]int, len(prevLines)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(nextLines)+1)
	}
	for i := len(prevLines) - 1; i >= 0; i-- {
		for j := len(nextLines) - 1; j >= 0; j-- {
			if prevLines[i] == nextLines[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}

	diff := make([]PromptDiffLine, 0, len(prevLines)+len(nextLines))
	i, j := 0, 0
	for i < len(prevLines) && j < len(nextLines) {
		if prevLines[i] == nextLines[j] {
			diff = append(diff, PromptDiffLine{Type: "context", Text: trimDiffLine(prevLines[i])})
			i++
			j++
			continue
		}
		if lcs[i+1][j] >= lcs[i][j+1] {
			diff = append(diff, PromptDiffLine{Type: "removed", Text: trimDiffLine(prevLines[i])})
			i++
		} else {
			diff = append(diff, PromptDiffLine{Type: "added", Text: trimDiffLine(nextLines[j])})
			j++
		}
	}
	for i < len(prevLines) {
		diff = append(diff, PromptDiffLine{Type: "removed", Text: trimDiffLine(prevLines[i])})
		i++
	}
	for j < len(nextLines) {
		diff = append(diff, PromptDiffLine{Type: "added", Text: trimDiffLine(nextLines[j])})
		j++
	}
	if len(diff) > 12 {
		diff = append(diff[:12], PromptDiffLine{Type: "context", Text: "..."})
	}
	return diff
}

func splitPromptLines(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return strings.Split(value, "\n")
}

func trimDiffLine(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 140 {
		return value
	}
	return value[:140] + "..."
}

func stringField(fields map[string]any, key string) string {
	if fields == nil {
		return ""
	}
	switch value := fields[key].(type) {
	case string:
		return value
	case []byte:
		return string(value)
	default:
		if value != nil {
			return fmt.Sprint(value)
		}
		return ""
	}
}

func intField(fields map[string]any, key string) int {
	if fields == nil {
		return 0
	}
	switch value := fields[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	case float32:
		return int(value)
	default:
		return 0
	}
}
