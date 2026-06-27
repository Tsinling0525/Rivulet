package agent

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type graderFunc func(context.Context, VerificationAttempt) (Grade, error)

func (f graderFunc) Grade(ctx context.Context, attempt VerificationAttempt) (Grade, error) {
	return f(ctx, attempt)
}

func TestVerificationHarnessPassesFirstAttempt(t *testing.T) {
	harness := VerificationHarness{
		Agent: Harness{
			Planner: plannerFunc(func(ctx context.Context, state State) (Plan, error) {
				return Plan{
					Summary:  "echo goal",
					ToolCall: ToolCall{Name: "echo", Args: map[string]any{"goal": state.Goal}},
				}, nil
			}),
			Reflector: reflectorFunc(func(ctx context.Context, state State, obs Observation) (Reflection, error) {
				return Reflection{Summary: "done", Decision: DecisionStop}, nil
			}),
			Tools: NewRegistry(NewToolFunc("echo", func(ctx context.Context, call ToolCall) (Observation, error) {
				return Observation{Output: map[string]any{"goal": call.Args["goal"]}}, nil
			})),
			MaxSteps: 1,
		},
		Grader: graderFunc(func(ctx context.Context, attempt VerificationAttempt) (Grade, error) {
			if attempt.Index != 1 {
				t.Fatalf("unexpected attempt index: %d", attempt.Index)
			}
			if attempt.AgentResult.Status != RunStatusCompleted {
				t.Fatalf("unexpected agent status: %s", attempt.AgentResult.Status)
			}
			return Grade{Passed: true, Summary: "meets rubric"}, nil
		}),
		MaxAttempts: 2,
	}

	result, err := harness.Run(context.Background(), "write docs")
	if err != nil {
		t.Fatalf("verification failed: %v", err)
	}
	if result.Status != VerificationStatusPassed {
		t.Fatalf("unexpected status: %s", result.Status)
	}
	if len(result.Attempts) != 1 {
		t.Fatalf("expected one attempt, got %d", len(result.Attempts))
	}
	if !result.FinalGrade.Passed {
		t.Fatalf("expected final grade to pass")
	}
}

func TestVerificationHarnessRetriesWithFeedback(t *testing.T) {
	var seenGoals []string
	harness := VerificationHarness{
		Agent: Harness{
			Planner: plannerFunc(func(ctx context.Context, state State) (Plan, error) {
				seenGoals = append(seenGoals, state.Goal)
				return Plan{Summary: "echo", ToolCall: ToolCall{Name: "echo"}}, nil
			}),
			Reflector: reflectorFunc(func(ctx context.Context, state State, obs Observation) (Reflection, error) {
				return Reflection{Summary: "done", Decision: DecisionStop}, nil
			}),
			Tools:    NewRegistry(NewToolFunc("echo", func(ctx context.Context, call ToolCall) (Observation, error) { return Observation{}, nil })),
			MaxSteps: 1,
		},
		Grader: graderFunc(func(ctx context.Context, attempt VerificationAttempt) (Grade, error) {
			if attempt.Index == 1 {
				return Grade{Passed: false, Summary: "missing source", Feedback: "cite the source URL"}, nil
			}
			return Grade{Passed: true, Summary: "source included"}, nil
		}),
		MaxAttempts: 2,
	}

	result, err := harness.Run(context.Background(), "write docs")
	if err != nil {
		t.Fatalf("verification failed: %v", err)
	}
	if result.Status != VerificationStatusPassed {
		t.Fatalf("unexpected status: %s", result.Status)
	}
	if len(result.Attempts) != 2 {
		t.Fatalf("expected two attempts, got %d", len(result.Attempts))
	}
	if len(seenGoals) != 2 {
		t.Fatalf("expected two seen goals, got %d", len(seenGoals))
	}
	if seenGoals[0] != "write docs" {
		t.Fatalf("unexpected first goal: %q", seenGoals[0])
	}
	if !strings.Contains(seenGoals[1], "cite the source URL") {
		t.Fatalf("expected feedback in second goal, got %q", seenGoals[1])
	}
}

func TestVerificationHarnessReturnsMaxAttemptsExceeded(t *testing.T) {
	harness := VerificationHarness{
		Agent: Harness{
			Planner: plannerFunc(func(ctx context.Context, state State) (Plan, error) {
				return Plan{Summary: "echo", ToolCall: ToolCall{Name: "echo"}}, nil
			}),
			Reflector: reflectorFunc(func(ctx context.Context, state State, obs Observation) (Reflection, error) {
				return Reflection{Summary: "done", Decision: DecisionStop}, nil
			}),
			Tools:    NewRegistry(NewToolFunc("echo", func(ctx context.Context, call ToolCall) (Observation, error) { return Observation{}, nil })),
			MaxSteps: 1,
		},
		Grader: graderFunc(func(ctx context.Context, attempt VerificationAttempt) (Grade, error) {
			return Grade{Passed: false, Summary: "still wrong", Feedback: "try again"}, nil
		}),
		MaxAttempts: 2,
	}

	result, err := harness.Run(context.Background(), "write docs")
	if !errors.Is(err, ErrVerificationMaxAttemptsExceeded) {
		t.Fatalf("expected ErrVerificationMaxAttemptsExceeded, got %v", err)
	}
	if result.Status != VerificationStatusMaxAttemptsUsed {
		t.Fatalf("unexpected status: %s", result.Status)
	}
	if len(result.Attempts) != 2 {
		t.Fatalf("expected two attempts, got %d", len(result.Attempts))
	}
}

func TestVerificationHarnessRequiresGrader(t *testing.T) {
	harness := VerificationHarness{}

	_, err := harness.Run(context.Background(), "write docs")
	if !errors.Is(err, ErrGraderRequired) {
		t.Fatalf("expected ErrGraderRequired, got %v", err)
	}
}
