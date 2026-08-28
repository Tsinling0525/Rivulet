package agent

import (
	"context"
	"errors"
	"sync"
	"testing"
)

type plannerFunc func(context.Context, State) (Plan, error)

func (f plannerFunc) Plan(ctx context.Context, state State) (Plan, error) {
	return f(ctx, state)
}

type eventSink struct {
	mu     sync.Mutex
	events []ExecutionEvent
}

func (s *eventSink) AppendExecutionEvent(ctx context.Context, event ExecutionEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	return nil
}

func TestHarnessExposesReplaceableLoopAndExecutionEvents(t *testing.T) {
	sink := &eventSink{}
	var loop AgentLoop = Harness{
		Planner: plannerFunc(func(context.Context, State) (Plan, error) {
			return Plan{ToolCall: ToolCall{Name: "echo"}}, nil
		}),
		Reflector: reflectorFunc(func(context.Context, State, Observation) (Reflection, error) {
			return Reflection{Summary: "done", Decision: DecisionStop}, nil
		}),
		Tools: NewRegistry(NewToolFunc("echo", func(context.Context, ToolCall) (Observation, error) {
			return Observation{Summary: "ok"}, nil
		})),
		Events: sink,
	}

	result, err := loop.Run(context.Background(), "complete one step")
	if err != nil {
		t.Fatalf("run loop: %v", err)
	}
	if len(result.Events) < 3 {
		t.Fatalf("events = %d, want start, step, and completion", len(result.Events))
	}
	if result.Events[0].Type != "agent_run_started" {
		t.Fatalf("first event = %q, want agent_run_started", result.Events[0].Type)
	}
	if got := result.Events[len(result.Events)-1].Type; got != "agent_run_completed" {
		t.Fatalf("last event = %q, want agent_run_completed", got)
	}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	if len(sink.events) != len(result.Events) {
		t.Fatalf("sink events = %d, result events = %d", len(sink.events), len(result.Events))
	}
}

type reflectorFunc func(context.Context, State, Observation) (Reflection, error)

func (f reflectorFunc) Reflect(ctx context.Context, state State, obs Observation) (Reflection, error) {
	return f(ctx, state, obs)
}

func TestHarnessRunsOnePlanToolObservationReflection(t *testing.T) {
	registry := NewRegistry(NewToolFunc("echo", func(ctx context.Context, call ToolCall) (Observation, error) {
		return Observation{
			Summary: "echoed message",
			Output:  map[string]any{"message": call.Args["message"]},
		}, nil
	}))

	harness := Harness{
		Planner: plannerFunc(func(ctx context.Context, state State) (Plan, error) {
			if state.Goal != "say hello" {
				t.Fatalf("unexpected goal: %q", state.Goal)
			}
			if len(state.Steps) != 0 {
				t.Fatalf("expected no previous steps, got %d", len(state.Steps))
			}
			return Plan{
				Summary:  "call echo once",
				ToolCall: ToolCall{Name: "echo", Args: map[string]any{"message": "hello"}},
			}, nil
		}),
		Reflector: reflectorFunc(func(ctx context.Context, state State, obs Observation) (Reflection, error) {
			if obs.Error != "" {
				t.Fatalf("unexpected observation error: %s", obs.Error)
			}
			if got := obs.Output["message"]; got != "hello" {
				t.Fatalf("unexpected tool output: %#v", got)
			}
			if len(state.Steps) != 1 {
				t.Fatalf("expected current step in reflection state, got %d", len(state.Steps))
			}
			return Reflection{Summary: "goal satisfied", Decision: DecisionStop}, nil
		}),
		Tools:    registry,
		MaxSteps: 3,
	}

	result, err := harness.Run(context.Background(), "say hello")
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if result.Status != RunStatusCompleted {
		t.Fatalf("unexpected status: %s", result.Status)
	}
	if len(result.Steps) != 1 {
		t.Fatalf("expected one step, got %d", len(result.Steps))
	}
	if result.Steps[0].Observation.ToolName != "echo" {
		t.Fatalf("unexpected tool name: %s", result.Steps[0].Observation.ToolName)
	}
	if result.FinalSummary != "goal satisfied" {
		t.Fatalf("unexpected final summary: %q", result.FinalSummary)
	}
}

func TestHarnessCanReplanAfterToolError(t *testing.T) {
	registry := NewRegistry(NewToolFunc("echo", func(ctx context.Context, call ToolCall) (Observation, error) {
		return Observation{Summary: "recovered", Output: map[string]any{"ok": true}}, nil
	}))

	harness := Harness{
		Planner: plannerFunc(func(ctx context.Context, state State) (Plan, error) {
			if len(state.Steps) == 0 {
				return Plan{Summary: "try missing tool", ToolCall: ToolCall{Name: "missing"}}, nil
			}
			return Plan{Summary: "recover with echo", ToolCall: ToolCall{Name: "echo"}}, nil
		}),
		Reflector: reflectorFunc(func(ctx context.Context, state State, obs Observation) (Reflection, error) {
			if obs.Error != "" {
				return Reflection{Summary: "tool failed; replan", Decision: DecisionReplan}, nil
			}
			return Reflection{Summary: "recovered", Decision: DecisionStop}, nil
		}),
		Tools:    registry,
		MaxSteps: 2,
	}

	result, err := harness.Run(context.Background(), "recover from a bad tool choice")
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if result.Status != RunStatusCompleted {
		t.Fatalf("unexpected status: %s", result.Status)
	}
	if len(result.Steps) != 2 {
		t.Fatalf("expected two steps, got %d", len(result.Steps))
	}
	if result.Steps[0].Reflection.Decision != DecisionReplan {
		t.Fatalf("expected first step to replan")
	}
	if result.Steps[1].Observation.Error != "" {
		t.Fatalf("unexpected second step error: %s", result.Steps[1].Observation.Error)
	}
}

func TestHarnessReturnsMaxStepsExceeded(t *testing.T) {
	harness := Harness{
		Planner: plannerFunc(func(ctx context.Context, state State) (Plan, error) {
			return Plan{Summary: "repeat", ToolCall: ToolCall{Name: "missing"}}, nil
		}),
		Reflector: reflectorFunc(func(ctx context.Context, state State, obs Observation) (Reflection, error) {
			return Reflection{Summary: "still not done", Decision: DecisionReplan}, nil
		}),
		Tools:    NewRegistry(),
		MaxSteps: 1,
	}

	result, err := harness.Run(context.Background(), "never stop")
	if !errors.Is(err, ErrMaxStepsExceeded) {
		t.Fatalf("expected ErrMaxStepsExceeded, got %v", err)
	}
	if result.Status != RunStatusMaxStepsExceeded {
		t.Fatalf("unexpected status: %s", result.Status)
	}
	if len(result.Steps) != 1 {
		t.Fatalf("expected one recorded step, got %d", len(result.Steps))
	}
}
