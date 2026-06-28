package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Tsinling0525/rivulet/agent"
)

const (
	traceOn  = "on"
	traceOff = "off"
)

var sensitiveAssignmentPattern = regexp.MustCompile(`(?i)(api[_-]?key|token|secret|password)\s*[:=]\s*[^ "'\n]+`)

type agentTraceRecord struct {
	StepIndex          int                      `json:"step_index"`
	PlanSummary        string                   `json:"plan_summary,omitempty"`
	ToolName           string                   `json:"tool_name,omitempty"`
	ToolArgs           map[string]any           `json:"tool_args,omitempty"`
	ObservationSummary string                   `json:"observation_summary,omitempty"`
	ObservationError   string                   `json:"observation_error,omitempty"`
	ReflectionDecision agent.ReflectionDecision `json:"reflection_decision,omitempty"`
	StartedAt          time.Time                `json:"started_at"`
	FinishedAt         time.Time                `json:"finished_at"`
}

func writeAgentTrace(cwd, mode string, result agent.RunResult) (string, error) {
	if strings.ToLower(strings.TrimSpace(mode)) == traceOff {
		return "", nil
	}
	if cwd == "" {
		cwd = "."
	}
	root, err := filepath.Abs(cwd)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, ".rivulet", "runs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	name := fmt.Sprintf("agent-%s-%d.jsonl", time.Now().UTC().Format("20060102T150405.000000000Z"), os.Getpid())
	path := filepath.Join(dir, name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o644)
	if err != nil {
		return "", err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	for _, step := range result.Steps {
		record := agentTraceRecord{
			StepIndex:          step.Index,
			PlanSummary:        step.Plan.Summary,
			ToolName:           step.Plan.ToolCall.Name,
			ToolArgs:           sanitizeTraceMap(step.Plan.ToolCall.Args),
			ObservationSummary: step.Observation.Summary,
			ObservationError:   sanitizeTraceString(step.Observation.Error),
			ReflectionDecision: step.Reflection.Decision,
			StartedAt:          step.StartedAt,
			FinishedAt:         step.FinishedAt,
		}
		if err := enc.Encode(record); err != nil {
			return "", err
		}
	}
	return displayPath(root, path), nil
}

func sanitizeTraceMap(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	out := make(map[string]any, len(src))
	for key, value := range src {
		if isSensitiveTraceKey(key) {
			out[key] = "<redacted>"
			continue
		}
		out[key] = sanitizeTraceValue(value)
	}
	return out
}

func sanitizeTraceValue(value any) any {
	switch v := value.(type) {
	case string:
		return sanitizeTraceString(v)
	case map[string]any:
		return sanitizeTraceMap(v)
	case map[string]string:
		out := make(map[string]any, len(v))
		for key, value := range v {
			if isSensitiveTraceKey(key) {
				out[key] = "<redacted>"
			} else {
				out[key] = sanitizeTraceString(value)
			}
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = sanitizeTraceValue(item)
		}
		return out
	default:
		return value
	}
}

func sanitizeTraceString(value string) string {
	return sensitiveAssignmentPattern.ReplaceAllString(value, "$1=<redacted>")
}

func isSensitiveTraceKey(key string) bool {
	key = strings.ToLower(key)
	return strings.Contains(key, "api_key") ||
		strings.Contains(key, "apikey") ||
		strings.Contains(key, "token") ||
		strings.Contains(key, "secret") ||
		strings.Contains(key, "password")
}
