package merge

import (
	"context"

	"github.com/Tsinling0525/rivulet/model"
	"github.com/Tsinling0525/rivulet/plugin"
)

// Merge simply passes through inputs (fan-in handled by engine)
type Merge struct{ deps plugin.Deps }

func (n *Merge) Init(ctx context.Context, deps plugin.Deps) error { n.deps = deps; return nil }
func (n *Merge) Process(ctx context.Context, wf model.Workflow, node model.Node, in model.Items) (model.Items, error) {
	return in, nil
}

func init() { plugin.Register("merge", func() plugin.NodeHandler { return &Merge{} }) }
