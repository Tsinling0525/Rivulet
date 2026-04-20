package engine

import (
	"errors"
	"fmt"
	"time"

	"github.com/Tsinling0525/rivulet/model"
)

var ErrExecutionPaused = errors.New("execution paused")

type Checkpoint struct {
	RunID        string
	WorkflowID   model.ID
	PausedNodeID model.ID
	ReviewID     string
	Inbound      map[model.ID]map[model.Port]model.Items
	Results      map[model.ID]model.Items
	Completed    []model.ID
	PausedAt     time.Time
}

type PausedError struct {
	ReviewID string
	NodeID   model.ID
	NodeName string
	Output   model.Items

	Checkpoint *Checkpoint
}

func (e *PausedError) Error() string {
	if e == nil {
		return ErrExecutionPaused.Error()
	}
	if e.ReviewID == "" {
		return fmt.Sprintf("%s at node %s", ErrExecutionPaused, e.NodeID)
	}
	return fmt.Sprintf("%s at node %s pending review %s", ErrExecutionPaused, e.NodeID, e.ReviewID)
}

func (e *PausedError) Unwrap() error {
	return ErrExecutionPaused
}
