package nodes

import (
	"context"

	"github.com/Tsinling0525/rivulet/model"
	"github.com/Tsinling0525/rivulet/plugin"
)

type Node interface {
	Type() string
	ID() string
	Init(ctx context.Context, deps plugin.Deps) error
	Process(ctx context.Context, wf model.Workflow, node model.Node, in model.Items) (model.Items, error)
}

type NodeFactory func(id string, params map[string]any) (Node, error)
