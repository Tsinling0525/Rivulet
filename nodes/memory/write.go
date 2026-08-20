package memory

import (
	"context"
	"errors"

	memcore "github.com/Tsinling0525/rivulet/memory"
	"github.com/Tsinling0525/rivulet/model"
	"github.com/Tsinling0525/rivulet/plugin"
)

type Write struct {
	deps plugin.Deps
}

func (n *Write) Init(ctx context.Context, deps plugin.Deps) error {
	n.deps = deps
	return nil
}

func (n *Write) Process(ctx context.Context, wf model.Workflow, node model.Node, in model.Items) (model.Items, error) {
	store := storeFor(node)
	out := make(model.Items, 0, len(in))
	for _, item := range in {
		if item == nil {
			item = model.Item{}
		}
		userID := userIDFor(node, item)
		text := textFor(node, item, "text_field", "memory")
		inputs := propositionInputs(node, item, text)
		if len(inputs) == 0 {
			return nil, errors.New("memory:write requires memory text or propositions")
		}

		var result memcore.WriteResult
		graph, err := store.Update(ctx, userID, func(graph *memcore.Graph) error {
			result = graph.AddPropositions(inputs, memcore.WriteOptions{
				Source:     stringConfig(node, "source", string(node.ID)),
				Confidence: floatConfig(node, "confidence", 1),
			})
			return nil
		})
		if err != nil {
			return nil, err
		}

		next := cloneItem(item)
		next["memory_user_id"] = userID
		next["memory_written"] = nodesToItems(result.Nodes)
		next["memory_edges"] = edgesToItems(result.Edges)
		next["memory_graph_nodes"] = len(graph.Nodes)
		next["memory_graph_edges"] = len(graph.Edges)
		out = append(out, next)
	}
	return out, nil
}
