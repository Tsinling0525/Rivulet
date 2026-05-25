package merge

import (
	"context"

	"github.com/Tsinling0525/rivulet/model"
	"github.com/Tsinling0525/rivulet/plugin"
)

type Concat struct{ deps plugin.Deps }

func (n *Concat) Init(ctx context.Context, deps plugin.Deps) error { n.deps = deps; return nil }

func (n *Concat) Process(ctx context.Context, wf model.Workflow, node model.Node, in model.Items) (model.Items, error) {
	// For now 'in' is already cumulative from predecessors; just pass through
	return in, nil
}

func init() { plugin.Register("merge.concat", func() plugin.NodeHandler { return &Concat{} }) }
