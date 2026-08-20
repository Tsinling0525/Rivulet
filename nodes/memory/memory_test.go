package memory

import (
	"context"
	"strings"
	"testing"

	"github.com/Tsinling0525/rivulet/model"
	"github.com/Tsinling0525/rivulet/plugin"
)

func TestMemoryNodesPersistUpdateAndQuery(t *testing.T) {
	ctx := context.Background()
	storeDir := t.TempDir()
	wf := model.Workflow{ID: "wf_memory", Name: "Memory"}
	cfg := map[string]any{
		"store_dir":       storeDir,
		"default_user_id": "u1",
	}

	write := &Write{}
	if err := write.Init(ctx, plugin.Deps{}); err != nil {
		t.Fatal(err)
	}
	_, err := write.Process(ctx, wf, model.Node{ID: "write", Type: "memory:write", Config: cfg}, model.Items{{
		"memory": "The user bikes to work every weekday.",
	}})
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}

	update := &Update{}
	if err := update.Init(ctx, plugin.Deps{}); err != nil {
		t.Fatal(err)
	}
	updateOut, err := update.Process(ctx, wf, model.Node{ID: "update", Type: "memory:update", Config: cfg}, model.Items{{
		"observation": "I broke my leg yesterday and will be in a cast for six weeks.",
	}})
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if triggered, _ := updateOut[0]["memory_update_triggered"].(bool); !triggered {
		t.Fatalf("expected update to trigger maintenance: %+v", updateOut[0])
	}

	query := &Query{}
	if err := query.Init(ctx, plugin.Deps{}); err != nil {
		t.Fatal(err)
	}
	queryOut, err := query.Process(ctx, wf, model.Node{ID: "query", Type: "memory:query", Config: cfg}, model.Items{{
		"query": "What bike route should I take to work tomorrow?",
	}})
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}

	warnings, ok := queryOut[0]["memory_warnings"].([]map[string]any)
	if !ok {
		t.Fatalf("expected warning list, got %T", queryOut[0]["memory_warnings"])
	}
	found := false
	for _, warning := range warnings {
		if warning["proposition"] == "The user bikes to work every weekday" && warning["state"] == "unknown-current" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected unknown-current bike warning, got %+v", warnings)
	}
}

func TestMemoryNodesRejectMissingRequiredText(t *testing.T) {
	ctx := context.Background()
	wf := model.Workflow{ID: "wf_memory"}
	config := map[string]any{"store_dir": t.TempDir()}
	tests := []struct {
		name    string
		handler plugin.NodeHandler
		node    model.Node
		want    string
	}{
		{
			name:    "write",
			handler: &Write{},
			node:    model.Node{ID: "write", Type: "memory:write", Config: config},
			want:    "memory:write requires memory text or propositions",
		},
		{
			name:    "update",
			handler: &Update{},
			node:    model.Node{ID: "update", Type: "memory:update", Config: config},
			want:    "memory:update requires observation text",
		},
		{
			name:    "query",
			handler: &Query{},
			node:    model.Node{ID: "query", Type: "memory:query", Config: config},
			want:    "memory:query requires query text",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.handler.Init(ctx, plugin.Deps{}); err != nil {
				t.Fatal(err)
			}
			_, err := tc.handler.Process(ctx, wf, tc.node, model.Items{{}})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q, got %v", tc.want, err)
			}
		})
	}
}

func TestMemoryQueryReturnsStableEmptySchema(t *testing.T) {
	ctx := context.Background()
	query := &Query{}
	if err := query.Init(ctx, plugin.Deps{}); err != nil {
		t.Fatal(err)
	}
	out, err := query.Process(ctx, model.Workflow{ID: "wf_memory"}, model.Node{
		ID:   "query",
		Type: "memory:query",
		Config: map[string]any{
			"store_dir": t.TempDir(),
		},
	}, model.Items{{"query": "unrecorded topic"}})
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	for _, field := range []string{"memories", "memory_warnings"} {
		values, ok := out[0][field].([]map[string]any)
		if !ok || len(values) != 0 {
			t.Fatalf("expected empty %s list, got %#v", field, out[0][field])
		}
	}
}
