package agent

import "time"

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
