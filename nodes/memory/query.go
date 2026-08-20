package memory

import (
	"context"
	"errors"

	memcore "github.com/Tsinling0525/rivulet/memory"
	"github.com/Tsinling0525/rivulet/model"
	"github.com/Tsinling0525/rivulet/plugin"
)

type Query struct {
	deps plugin.Deps
}

func (n *Query) Init(ctx context.Context, deps plugin.Deps) error {
	n.deps = deps
	return nil
}

func (n *Query) Process(ctx context.Context, wf model.Workflow, node model.Node, in model.Items) (model.Items, error) {
	store := storeFor(node)
	out := make(model.Items, 0, len(in))
	for _, item := range in {
		if item == nil {
			item = model.Item{}
		}
		userID := userIDFor(node, item)
		query := textFor(node, item, "query_field", "query")
		if query == "" {
			return nil, errors.New("memory:query requires query text")
		}

		graph, err := store.Load(ctx, userID)
		if err != nil {
			return nil, err
		}
		result := graph.Query(query, memcore.QueryOptions{
			MaxResults:      intConfig(node, "max_results", 8),
			States:          parseStates(node.Config["states"], []memcore.ValidityState{memcore.StateActive}),
			WarningStates:   parseStates(node.Config["warning_states"], []memcore.ValidityState{memcore.StateSuspect, memcore.StateUnknownCurrent, memcore.StateStale}),
			IncludeWarnings: boolConfig(node, "include_warnings", true),
		})

		next := cloneItem(item)
		next["memory_user_id"] = userID
		next["memories"] = matchesToItems(result.Matches)
		next["memory_context"] = memoryContext(result.Matches)
		next["memory_warnings"] = matchesToItems(result.Warnings)
		next["memory_graph_nodes"] = len(graph.Nodes)
		next["memory_graph_edges"] = len(graph.Edges)
		out = append(out, next)
	}
	return out, nil
}
