package memory

import (
	"context"
	"errors"

	memcore "github.com/Tsinling0525/rivulet/memory"
	"github.com/Tsinling0525/rivulet/model"
	"github.com/Tsinling0525/rivulet/plugin"
)

type Update struct {
	deps plugin.Deps
}

func (n *Update) Init(ctx context.Context, deps plugin.Deps) error {
	n.deps = deps
	return nil
}

func (n *Update) Process(ctx context.Context, wf model.Workflow, node model.Node, in model.Items) (model.Items, error) {
	store := storeFor(node)
	out := make(model.Items, 0, len(in))
	for _, item := range in {
		if item == nil {
			item = model.Item{}
		}
		userID := userIDFor(node, item)
		observation := textFor(node, item, "observation_field", "observation")
		if observation == "" {
			return nil, errors.New("memory:update requires observation text")
		}

		var result memcore.UpdateResult
		graph, err := store.Update(ctx, userID, func(graph *memcore.Graph) error {
			result = graph.UpdateWithObservation(observation, memcore.UpdateOptions{
				Source:               stringConfig(node, "source", string(node.ID)),
				TriggerThreshold:     floatConfig(node, "trigger_threshold", 0.25),
				DirectThreshold:      floatConfig(node, "direct_threshold", 0.7),
				SuspicionThreshold:   floatConfig(node, "suspicion_threshold", 0.35),
				StaleThreshold:       floatConfig(node, "stale_threshold", 0.72),
				UncertaintyThreshold: floatConfig(node, "uncertainty_threshold", 0.45),
				EdgeThreshold:        floatConfig(node, "edge_threshold", 0.55),
				PathDecay:            floatConfig(node, "path_decay", 0.95),
				MaxDepth:             intConfig(node, "max_depth", 2),
				StoreObservation:     boolConfig(node, "store_observation", true),
			})
			return nil
		})
		if err != nil {
			return nil, err
		}

		next := cloneItem(item)
		next["memory_user_id"] = userID
		next["memory_update_triggered"] = result.Triggered
		next["memory_trigger_score"] = result.TriggerScore
		next["memory_observations"] = nodesToItems(result.Observations)
		next["memory_direct_invalidations"] = invalidationsToItems(result.Direct)
		next["memory_propagated_invalidations"] = invalidationsToItems(result.Propagated)
		next["memory_edges_updated"] = edgesToItems(result.EdgesUpdated)
		next["memory_nodes_examined"] = result.NodesExamined
		next["memory_graph_nodes"] = len(graph.Nodes)
		next["memory_graph_edges"] = len(graph.Edges)
		out = append(out, next)
	}
	return out, nil
}
