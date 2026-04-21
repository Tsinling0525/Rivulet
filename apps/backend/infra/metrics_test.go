package infra

import (
	"path/filepath"
	"testing"
	"time"
)

func TestPromptVersionMetricsAggregatesByPromptHash(t *testing.T) {
	store := &RunStore{dir: filepath.Join(t.TempDir(), "runs")}
	base := time.Date(2026, 4, 21, 10, 0, 0, 0, time.UTC)

	runs := []RunRecord{
		{
			ID:           "run-1",
			WorkflowID:   "wf-ai",
			WorkflowName: "AI Workflow",
			Status:       "succeeded",
			StartedAt:    base,
			Events: []RunEvent{
				{
					Type:       "ai_model_call",
					OccurredAt: base,
					NodeID:     "llm1",
					Fields: map[string]any{
						"workflow":       "wf-ai",
						"node":           "llm1",
						"provider":       "openai",
						"model":          "gpt-5-mini",
						"prompt_hash":    "sha256:aaa",
						"prompt_preview": "Summarize this ticket",
						"status":         "succeeded",
						"latency_ms":     120,
						"tokens": map[string]any{
							"input":  10,
							"output": 5,
							"total":  15,
						},
					},
				},
			},
		},
		{
			ID:           "run-2",
			WorkflowID:   "wf-ai",
			WorkflowName: "AI Workflow",
			Status:       "failed",
			StartedAt:    base.Add(time.Minute),
			Events: []RunEvent{
				{
					Type:       "ai_model_call",
					OccurredAt: base.Add(time.Minute),
					NodeID:     "llm1",
					Fields: map[string]any{
						"workflow":       "wf-ai",
						"node":           "llm1",
						"provider":       "openai",
						"model":          "gpt-5-mini",
						"prompt_hash":    "sha256:bbb",
						"prompt_preview": "Summarize this ticket\nInclude risk",
						"status":         "failed",
						"latency_ms":     240,
						"tokens": map[string]any{
							"input":  12,
							"output": 6,
							"total":  18,
						},
					},
				},
			},
		},
		{
			ID:           "run-3",
			WorkflowID:   "wf-ai",
			WorkflowName: "AI Workflow",
			Status:       "succeeded",
			StartedAt:    base.Add(2 * time.Minute),
			Events: []RunEvent{
				{
					Type:       "ai_model_call",
					OccurredAt: base.Add(2 * time.Minute),
					NodeID:     "llm1",
					Fields: map[string]any{
						"workflow":       "wf-ai",
						"node":           "llm1",
						"provider":       "openai",
						"model":          "gpt-5-mini",
						"prompt_hash":    "sha256:bbb",
						"prompt_preview": "Summarize this ticket\nInclude risk",
						"status":         "succeeded",
						"latency_ms":     360,
						"tokens": map[string]any{
							"input":  14,
							"output": 8,
							"total":  22,
						},
					},
				},
			},
		},
	}
	for _, run := range runs {
		if err := store.Save(run); err != nil {
			t.Fatalf("Save returned error: %v", err)
		}
	}

	metrics, err := store.PromptVersionMetrics(0)
	if err != nil {
		t.Fatalf("PromptVersionMetrics returned error: %v", err)
	}
	if len(metrics) != 2 {
		t.Fatalf("expected 2 prompt versions, got %d", len(metrics))
	}

	latest := metrics[0]
	if latest.Version != "v2" || latest.PromptHash != "sha256:bbb" {
		t.Fatalf("expected latest version v2 sha256:bbb, got %s %s", latest.Version, latest.PromptHash)
	}
	if latest.Calls != 2 || latest.Succeeded != 1 || latest.Failed != 1 {
		t.Fatalf("expected v2 calls/succeeded/failed 2/1/1, got %d/%d/%d", latest.Calls, latest.Succeeded, latest.Failed)
	}
	if latest.SuccessRate != 50 {
		t.Fatalf("expected v2 success rate 50, got %.1f", latest.SuccessRate)
	}
	if latest.TotalTokens != 40 || latest.AverageTotalTokens != 20 {
		t.Fatalf("expected v2 token totals 40 avg 20, got %d %.1f", latest.TotalTokens, latest.AverageTotalTokens)
	}
	if latest.PreviousPromptHash != "sha256:aaa" {
		t.Fatalf("expected previous prompt hash sha256:aaa, got %q", latest.PreviousPromptHash)
	}
	if len(latest.PromptPreviewDiff) == 0 {
		t.Fatalf("expected v2 to include a prompt diff")
	}

	earliest := metrics[1]
	if earliest.Version != "v1" || earliest.PromptHash != "sha256:aaa" {
		t.Fatalf("expected earliest version v1 sha256:aaa, got %s %s", earliest.Version, earliest.PromptHash)
	}
}

func TestReasoningTracesBuildTimelineFromRunEvents(t *testing.T) {
	store := &RunStore{dir: filepath.Join(t.TempDir(), "runs")}
	base := time.Date(2026, 4, 21, 10, 0, 0, 0, time.UTC)
	run := RunRecord{
		ID:           "run-reasoning",
		WorkflowID:   "wf-ai",
		WorkflowName: "AI Workflow",
		Status:       "succeeded",
		StartedAt:    base,
		Events: []RunEvent{
			{
				Type:       "ai_reasoning_step",
				OccurredAt: base.Add(100 * time.Millisecond),
				NodeID:     "llm1",
				Fields: map[string]any{
					"node":       "llm1",
					"provider":   "openai",
					"model":      "gpt-5-mini",
					"step_index": 1,
					"title":      "Prompt submitted",
					"text":       "Request sent.",
					"source":     "lifecycle",
					"latency_ms": 100,
					"delta_ms":   100,
					"status":     "running",
				},
			},
			{
				Type:       "ai_reasoning_step",
				OccurredAt: base.Add(350 * time.Millisecond),
				NodeID:     "llm1",
				Fields: map[string]any{
					"node":       "llm1",
					"provider":   "openai",
					"model":      "gpt-5-mini",
					"step_index": 2,
					"title":      "Reasoning summary 1",
					"text":       "Check the input shape.",
					"source":     "reasoning_summary",
					"latency_ms": 350,
					"delta_ms":   250,
					"status":     "streamed",
				},
			},
			{
				Type:       "ai_model_call",
				OccurredAt: base.Add(500 * time.Millisecond),
				NodeID:     "llm1",
				Fields: map[string]any{
					"node":           "llm1",
					"provider":       "openai",
					"model":          "gpt-5-mini",
					"prompt_preview": "Summarize this ticket",
					"output_preview": "Done",
					"status":         "succeeded",
					"latency_ms":     500,
				},
			},
		},
	}
	if err := store.Save(run); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	mgr := NewInstanceManager(nil, store, nil)
	traces := mgr.ReasoningTraces(0)
	if len(traces) != 1 {
		t.Fatalf("expected 1 trace, got %d", len(traces))
	}
	trace := traces[0]
	if trace.ExecutionID != "run-reasoning" || trace.NodeID != "llm1" {
		t.Fatalf("unexpected trace identity: %+v", trace)
	}
	if !trace.SupportsReasoning {
		t.Fatalf("expected gpt-5-mini trace to support reasoning")
	}
	if trace.TotalLatencyMS != 500 {
		t.Fatalf("expected total latency 500, got %d", trace.TotalLatencyMS)
	}
	if trace.StepCount != 2 || len(trace.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d / %d", trace.StepCount, len(trace.Steps))
	}
	if trace.Steps[1].Title != "Reasoning summary 1" || trace.Steps[1].DeltaMS != 250 {
		t.Fatalf("unexpected second step: %+v", trace.Steps[1])
	}
}
