package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrGraderRequired                  = errors.New("verification grader is required")
	ErrVerificationMaxAttemptsExceeded = errors.New("verification max attempts exceeded")
)

type VerificationStatus string

const (
	VerificationStatusPassed          VerificationStatus = "passed"
	VerificationStatusFailed          VerificationStatus = "failed"
	VerificationStatusAgentFailed     VerificationStatus = "agent_failed"
	VerificationStatusMaxAttemptsUsed VerificationStatus = "max_attempts_used"
)

type Grader interface {
	Grade(ctx context.Context, attempt VerificationAttempt) (Grade, error)
}

type Grade struct {
	Passed   bool
	Summary  string
	Feedback string
	Metadata map[string]any
}

type VerificationAttempt struct {
	Index       int
	Goal        string
	AgentResult RunResult
	Grade       Grade
	StartedAt   time.Time
	FinishedAt  time.Time
}

type VerifiedRunResult struct {
	Goal       string
	Status     VerificationStatus
	Attempts   []VerificationAttempt
	Final      RunResult
	FinalGrade Grade
	Error      string
	StartedAt  time.Time
	FinishedAt time.Time
}

type FeedbackFormatter func(goal string, grades []Grade) string

type VerificationHarness struct {
	Agent           Harness
	Grader          Grader
	MaxAttempts     int
	FormatFeedback  FeedbackFormatter
	ContinueOnError bool
	Clock           func() time.Time
}

func (h VerificationHarness) Run(ctx context.Context, goal string) (VerifiedRunResult, error) {
	if h.Grader == nil {
		return VerifiedRunResult{}, ErrGraderRequired
	}

	goal = strings.TrimSpace(goal)
	result := VerifiedRunResult{
		Goal:      goal,
		Status:    VerificationStatusFailed,
		StartedAt: h.now(),
	}

	maxAttempts := h.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1
	}

	var grades []Grade
	for attemptIndex := 1; attemptIndex <= maxAttempts; attemptIndex++ {
		attempt := VerificationAttempt{
			Index:     attemptIndex,
			Goal:      h.formatGoal(goal, grades),
			StartedAt: h.now(),
		}

		agentResult, err := h.Agent.Run(ctx, attempt.Goal)
		attempt.AgentResult = agentResult
		if err != nil && !h.ContinueOnError {
			attempt.FinishedAt = h.now()
			result.Attempts = append(result.Attempts, attempt)
			result.Final = agentResult
			result.Status = VerificationStatusAgentFailed
			result.Error = err.Error()
			result.FinishedAt = h.now()
			return result, err
		}

		grade, gradeErr := h.Grader.Grade(ctx, attempt)
		if grade.Metadata != nil {
			grade.Metadata = cloneMap(grade.Metadata)
		}
		attempt.Grade = grade
		attempt.FinishedAt = h.now()
		result.Attempts = append(result.Attempts, attempt)
		result.Final = agentResult
		result.FinalGrade = grade

		if gradeErr != nil {
			result.Status = VerificationStatusFailed
			result.Error = fmt.Sprintf("grade attempt %d: %v", attemptIndex, gradeErr)
			result.FinishedAt = h.now()
			return result, fmt.Errorf("grade attempt %d: %w", attemptIndex, gradeErr)
		}
		if grade.Passed {
			result.Status = VerificationStatusPassed
			result.FinishedAt = h.now()
			return result, nil
		}

		grades = append(grades, grade)
	}

	result.Status = VerificationStatusMaxAttemptsUsed
	result.Error = ErrVerificationMaxAttemptsExceeded.Error()
	result.FinishedAt = h.now()
	return result, ErrVerificationMaxAttemptsExceeded
}

func (h VerificationHarness) formatGoal(goal string, grades []Grade) string {
	if len(grades) == 0 {
		return goal
	}
	if h.FormatFeedback != nil {
		return h.FormatFeedback(goal, cloneGrades(grades))
	}
	return defaultFeedbackGoal(goal, grades)
}

func defaultFeedbackGoal(goal string, grades []Grade) string {
	var b strings.Builder
	b.WriteString(goal)
	b.WriteString("\n\nPrevious verification feedback:")
	for i, grade := range grades {
		b.WriteString(fmt.Sprintf("\n%d. %s", i+1, strings.TrimSpace(grade.Summary)))
		feedback := strings.TrimSpace(grade.Feedback)
		if feedback != "" {
			b.WriteString(": ")
			b.WriteString(feedback)
		}
	}
	return b.String()
}

func cloneGrades(grades []Grade) []Grade {
	out := make([]Grade, len(grades))
	for i, grade := range grades {
		out[i] = grade
		out[i].Metadata = cloneMap(grade.Metadata)
	}
	return out
}

func (h VerificationHarness) now() time.Time {
	if h.Clock != nil {
		return h.Clock()
	}
	if h.Agent.Clock != nil {
		return h.Agent.Clock()
	}
	return time.Now().UTC()
}
