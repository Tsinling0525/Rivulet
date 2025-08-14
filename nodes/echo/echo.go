package echo

import (
	"context"

	"rivulet/model"
	"rivulet/plugin"
)

type Echo struct{ deps plugin.Deps }

func (e *Echo) Init(ctx context.Context, deps plugin.Deps) error { e.deps = deps; return nil }

func (e *Echo) Process(ctx context.Context, wf model.Workflow, node model.Node, in model.Items) (model.Items, error) {
	// no-op: add a field from config and return
	label, _ := node.Config["label"].(string)
	out := make(model.Items, len(in))
	for i, it := range in {
		if it == nil {
			it = model.Item{}
		}
		it["echo_label"] = label
		out[i] = it
	}
	return out, nil
}

func init() { plugin.Register("echo", func() plugin.NodeHandler { return &Echo{} }) }
