package review

import (
	"context"
	"fmt"

	"github.com/Tsinling0525/rivulet/model"
	"github.com/Tsinling0525/rivulet/plugin"
)

type Gate struct {
	deps plugin.Deps
}

func (g *Gate) Init(ctx context.Context, deps plugin.Deps) error {
	g.deps = deps
	return nil
}

func (g *Gate) Process(ctx context.Context, wf model.Workflow, node model.Node, in model.Items) (model.Items, error) {
	if g.deps.Reviews == nil {
		return nil, fmt.Errorf("review store not configured")
	}

	outputField, _ := node.Config["output_field"].(string)
	if outputField == "" {
		outputField = "output"
	}
	passThrough, _ := node.Config["pass_through"].(bool)
	contextFields := stringSlice(node.Config["context_fields"])

	out := make(model.Items, 0, len(in))
	for _, item := range in {
		if item == nil {
			item = model.Item{}
		}
		review, err := g.deps.Reviews.Create(ctx, model.ReviewCreate{
			RunID:          plugin.ExecutionIDFromContext(ctx),
			WorkflowID:     wf.ID,
			WorkflowName:   wf.Name,
			WorkflowKind:   wf.Kind,
			NodeID:         node.ID,
			NodeName:       node.Name,
			Input:          item,
			ProposedOutput: proposedOutput(item, outputField),
			Context:        selectedContext(item, contextFields),
		})
		if err != nil {
			return nil, err
		}
		if g.deps.Bus != nil {
			_ = g.deps.Bus.Emit(ctx, "review_requested", map[string]any{
				"exec":      plugin.ExecutionIDFromContext(ctx),
				"workflow":  wf.ID,
				"node":      node.ID,
				"review_id": review.ID,
				"status":    review.Status,
			})
		}
		if passThrough {
			next := cloneReviewItem(item)
			next["review_id"] = review.ID
			next["review_status"] = review.Status
			next["review_required"] = true
			out = append(out, next)
		}
	}
	return out, nil
}

func proposedOutput(item model.Item, outputField string) any {
	if value, ok := item[outputField]; ok {
		return value
	}
	return item
}

func selectedContext(item model.Item, fields []string) model.Item {
	if len(fields) == 0 {
		return item
	}
	out := model.Item{}
	for _, field := range fields {
		if value, ok := item[field]; ok {
			out[field] = value
		}
	}
	return out
}

func stringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func cloneReviewItem(src model.Item) model.Item {
	out := model.Item{}
	for key, value := range src {
		out[key] = value
	}
	return out
}

func init() { plugin.Register("review:gate", func() plugin.NodeHandler { return &Gate{} }) }
