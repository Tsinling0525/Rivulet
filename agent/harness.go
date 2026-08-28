package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrPlannerRequired            = errors.New("agent planner is required")
	ErrReflectorRequired          = errors.New("agent reflector is required")
	ErrReflectionDecisionRequired = errors.New("agent reflection decision must be stop or replan")
	ErrMaxStepsExceeded           = errors.New("agent max steps exceeded")
)

type Planner interface {
	Plan(ctx context.Context, state State) (Plan, error)
}

type Reflector interface {
	Reflect(ctx context.Context, state State, observation Observation) (Reflection, error)
}

type Harness struct {
	Planner   Planner
	Reflector Reflector
	Tools     ToolResolver
	Events    ExecutionEventSink
	MaxSteps  int
	Clock     func() time.Time
}

func (h Harness) Run(ctx context.Context, goal string) (RunResult, error) {
	if h.Planner == nil {
		return RunResult{}, ErrPlannerRequired
	}
	if h.Reflector == nil {
		return RunResult{}, ErrReflectorRequired
	}

	goal = strings.TrimSpace(goal)
	result := RunResult{
		Goal:      goal,
		Status:    RunStatusFailed,
		StartedAt: h.now(),
	}
	h.appendEvent(ctx, &result, "agent_run_started", 0, "agent_loop", map[string]any{"goal": goal})
	if goal == "" {
		err := errors.New("agent goal is required")
		result.Error = err.Error()
		result.FinishedAt = h.now()
		h.appendEvent(ctx, &result, "agent_run_failed", 0, "agent_loop", map[string]any{"error": err.Error()})
		return result, err
	}

	maxSteps := h.MaxSteps
	if maxSteps <= 0 {
		maxSteps = 1
	}

	state := State{Goal: goal}
	for stepIndex := 1; stepIndex <= maxSteps; stepIndex++ {
		h.appendEvent(ctx, &result, "agent_step_started", stepIndex, "agent_loop", nil)
		step, err := h.runStep(ctx, state, stepIndex)
		result.Steps = append(result.Steps, step)
		state.Steps = append(state.Steps, step)
		h.appendEvent(ctx, &result, "agent_step_completed", stepIndex, "agent_loop", map[string]any{
			"tool":       step.Plan.ToolCall.Name,
			"tool_error": step.Observation.Error,
			"decision":   string(step.Reflection.Decision),
		})
		if err != nil {
			result.Status = RunStatusFailed
			result.Error = err.Error()
			result.FinishedAt = h.now()
			h.appendEvent(ctx, &result, "agent_run_failed", stepIndex, "agent_loop", map[string]any{"error": err.Error()})
			return result, err
		}

		switch step.Reflection.Decision {
		case DecisionStop:
			result.Status = RunStatusCompleted
			result.FinalSummary = step.Reflection.Summary
			result.FinishedAt = h.now()
			h.appendEvent(ctx, &result, "agent_run_completed", stepIndex, "agent_loop", map[string]any{"summary": result.FinalSummary})
			return result, nil
		case DecisionReplan:
			continue
		default:
			result.Status = RunStatusFailed
			result.Error = ErrReflectionDecisionRequired.Error()
			result.FinishedAt = h.now()
			h.appendEvent(ctx, &result, "agent_run_failed", stepIndex, "agent_loop", map[string]any{"error": result.Error})
			return result, ErrReflectionDecisionRequired
		}
	}

	result.Status = RunStatusMaxStepsExceeded
	result.Error = ErrMaxStepsExceeded.Error()
	result.FinishedAt = h.now()
	h.appendEvent(ctx, &result, "agent_run_max_steps_exceeded", maxSteps, "agent_loop", map[string]any{"error": result.Error})
	return result, ErrMaxStepsExceeded
}

func (h Harness) runStep(ctx context.Context, state State, index int) (Step, error) {
	step := Step{
		Index:     index,
		StartedAt: h.now(),
	}

	plan, err := h.Planner.Plan(ctx, state.Snapshot())
	if err != nil {
		step.FinishedAt = h.now()
		return step, fmt.Errorf("plan step %d: %w", index, err)
	}
	step.Plan = plan

	observation := h.executeTool(ctx, plan.ToolCall)
	step.Observation = observation

	reflectState := state.Snapshot()
	reflectState.Steps = append(reflectState.Steps, step)
	reflection, err := h.Reflector.Reflect(ctx, reflectState, observation)
	if err != nil {
		step.FinishedAt = h.now()
		return step, fmt.Errorf("reflect step %d: %w", index, err)
	}
	step.Reflection = reflection
	step.FinishedAt = h.now()
	return step, nil
}

func (h Harness) executeTool(ctx context.Context, call ToolCall) Observation {
	if strings.TrimSpace(call.Name) == "" {
		return Observation{Error: "tool name is required"}
	}
	if h.Tools == nil {
		return Observation{ToolName: call.Name, Error: "tool registry is not configured"}
	}

	tool, ok := h.Tools.ResolveTool(call.Name)
	if !ok {
		return Observation{ToolName: call.Name, Error: fmt.Sprintf("unknown tool %q", call.Name)}
	}

	observation, err := tool.Execute(ctx, ToolCall{Name: call.Name, Args: cloneMap(call.Args)})
	if observation.ToolName == "" {
		observation.ToolName = call.Name
	}
	if err != nil {
		observation.Error = err.Error()
	}
	return observation
}

func (h Harness) now() time.Time {
	if h.Clock != nil {
		return h.Clock()
	}
	return time.Now().UTC()
}

func (h Harness) appendEvent(ctx context.Context, result *RunResult, eventType string, stepIndex int, component string, fields map[string]any) {
	event := ExecutionEvent{
		Type:       eventType,
		StepIndex:  stepIndex,
		Component:  component,
		OccurredAt: h.now(),
		Fields:     cloneMap(fields),
	}
	result.Events = append(result.Events, event)
	if h.Events != nil {
		_ = h.Events.AppendExecutionEvent(ctx, event)
	}
}
