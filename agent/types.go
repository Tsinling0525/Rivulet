package agent

import (
	"context"
	"time"
)

// AgentLoop is the replaceable agent-policy boundary. Infrastructure such as
// model clients, tools, and persistence is injected into a loop implementation
// by the composition root rather than owned by this contract.
type AgentLoop interface {
	Run(ctx context.Context, goal string) (RunResult, error)
}

// ExecutionEvent is an append-oriented record of an agent run. It supplements
// the existing RunResult state so callers can adopt event persistence or replay
// incrementally without changing the current CLI result format.
type ExecutionEvent struct {
	Type       string         `json:"type"`
	StepIndex  int            `json:"step_index,omitempty"`
	Component  string         `json:"component"`
	OccurredAt time.Time      `json:"occurred_at"`
	Fields     map[string]any `json:"fields,omitempty"`
}

// ExecutionEventSink receives structured execution events. A sink should not
// alter agent policy; a failing observer is intentionally isolated from a run.
type ExecutionEventSink interface {
	AppendExecutionEvent(ctx context.Context, event ExecutionEvent) error
}

type RunStatus string

const (
	RunStatusCompleted        RunStatus = "completed"
	RunStatusFailed           RunStatus = "failed"
	RunStatusMaxStepsExceeded RunStatus = "max_steps_exceeded"
)

type ReflectionDecision string

const (
	DecisionStop   ReflectionDecision = "stop"
	DecisionReplan ReflectionDecision = "replan"
)

type State struct {
	Goal  string
	Steps []Step
}

type Plan struct {
	Summary  string
	ToolCall ToolCall
}

type ToolCall struct {
	Name string
	Args map[string]any
}

type Observation struct {
	ToolName string
	Summary  string
	Output   map[string]any
	Error    string
}

type Reflection struct {
	Summary  string
	Decision ReflectionDecision
}

type Step struct {
	Index       int
	Plan        Plan
	Observation Observation
	Reflection  Reflection
	StartedAt   time.Time
	FinishedAt  time.Time
}

type RunResult struct {
	Goal         string
	Status       RunStatus
	Steps        []Step
	Events       []ExecutionEvent
	FinalSummary string
	Error        string
	StartedAt    time.Time
	FinishedAt   time.Time
}

func (s State) Snapshot() State {
	return State{
		Goal:  s.Goal,
		Steps: cloneSteps(s.Steps),
	}
}

func cloneSteps(steps []Step) []Step {
	out := make([]Step, len(steps))
	for i, step := range steps {
		out[i] = step
		out[i].Plan.ToolCall.Args = cloneMap(step.Plan.ToolCall.Args)
		out[i].Observation.Output = cloneMap(step.Observation.Output)
	}
	return out
}

func cloneMap(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	out := make(map[string]any, len(src))
	for key, value := range src {
		out[key] = value
	}
	return out
}
