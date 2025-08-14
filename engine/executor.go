package engine

import (
	"context"
	"fmt"
	"time"

	"rivulet/model"
	"rivulet/plugin"
)

type Engine struct {
	Deps plugin.Deps
}

func New(deps plugin.Deps) *Engine { return &Engine{Deps: deps} }

func (e *Engine) Run(ctx context.Context, execID string, wf model.Workflow, inputs map[model.ID]model.Items) (map[model.ID]model.Items, error) {
	order, _, outs := topo(wf)
	results := map[model.ID]model.Items{}

	for _, nodeID := range order {
		var node model.Node
		for _, n := range wf.Nodes {
			if n.ID == nodeID {
				node = n
				break
			}
		}
		handler, ok := plugin.New(node.Type)
		if !ok {
			return nil, fmt.Errorf("unknown node type: %s", node.Type)
		}
		if err := handler.Init(ctx, e.Deps); err != nil {
			return nil, err
		}

		in := inputs[nodeID]
		// collect from predecessors if not provided
		if in == nil {
			in = model.Items{}
		}

		runCtx := ctx
		if node.Timeout > 0 {
			var cancel context.CancelFunc
			runCtx, cancel = context.WithTimeout(ctx, node.Timeout)
			defer cancel()
		}

		e.Deps.Bus.Emit(ctx, "node_started", map[string]any{"exec": execID, "node": node.ID})
		out, err := handler.Process(runCtx, wf, node, in)
		if err != nil {
			return nil, err
		}
		e.Deps.Bus.Emit(ctx, "node_completed", map[string]any{"exec": execID, "node": node.ID, "count": len(out)})

		results[node.ID] = out
		// fan-out to successors as naive concat
		for _, succ := range outs[node.ID] {
			inputs[succ] = append(inputs[succ], out...)
		}
	}

	e.Deps.Bus.Emit(ctx, "execution_completed", map[string]any{"exec": execID, "at": time.Now().UTC()})
	return results, nil
}
