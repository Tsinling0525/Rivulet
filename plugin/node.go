package plugin

import (
	"context"

	"github.com/Tsinling0525/rivulet/model"
)

type Deps struct {
	State StateStore
	Bus   EventBus
}

type NodeHandler interface {
	Init(ctx context.Context, deps Deps) error
	Process(ctx context.Context, wf model.Workflow, node model.Node, in model.Items) (model.Items, error)
}

type StateStore interface {
	SaveNodeState(ctx context.Context, execID string, nodeID model.ID, state map[string]any) error
	LoadNodeState(ctx context.Context, execID string, nodeID model.ID) (map[string]any, error)
}

type EventBus interface {
	Emit(ctx context.Context, event string, fields map[string]any) error
}
