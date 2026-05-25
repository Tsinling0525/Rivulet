package llmroute

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/Tsinling0525/rivulet/model"
	"github.com/Tsinling0525/rivulet/plugin"
)

func TestLLMRouteChoosesSimpleAndUsesSemanticCache(t *testing.T) {
	t.Setenv("RIV_SEMANTIC_CACHE_PATH", filepath.Join(t.TempDir(), "cache.json"))
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"response":          "invoice",
			"prompt_eval_count": 4,
			"eval_count":        2,
		})
	}))
	defer server.Close()

	node := &Node{}
	if err := node.Init(context.Background(), plugin.Deps{}); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}
	cfg := map[string]any{
		"prompt":         "Classify {{.text}}",
		"semantic_cache": map[string]any{"enabled": true, "threshold": 0.5},
		"routes": map[string]any{
			"simple": map[string]any{
				"provider": "ollama",
				"model":    "tiny",
				"endpoint": server.URL,
			},
			"complex": map[string]any{
				"provider": "openai",
				"model":    "gpt-4o",
				"endpoint": server.URL,
			},
		},
	}
	first, err := node.Process(context.Background(), model.Workflow{ID: "wf"}, model.Node{ID: "router1", Type: "llm:route", Config: cfg}, model.Items{{"text": "invoice status", "task_type": "classification"}})
	if err != nil {
		t.Fatalf("first Process returned error: %v", err)
	}
	second, err := node.Process(context.Background(), model.Workflow{ID: "wf"}, model.Node{ID: "router1", Type: "llm:route", Config: cfg}, model.Items{{"text": "the invoice status", "task_type": "classification"}})
	if err != nil {
		t.Fatalf("second Process returned error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected second call to hit semantic cache, provider calls=%d", calls)
	}
	if first[0]["route"] != "simple" || second[0]["cache_hit"] != true {
		t.Fatalf("unexpected routed outputs: first=%+v second=%+v", first[0], second[0])
	}
}

func TestLLMRouteChoosesComplex(t *testing.T) {
	decision := decideRoute(map[string]any{
		"routes": map[string]any{
			"simple":  map[string]any{"provider": "ollama", "model": "small"},
			"complex": map[string]any{"provider": "openai", "model": "gpt-4o"},
		},
	}, "Analyze the architecture tradeoffs and reason step by step about failures.", model.Item{})
	if decision.Route.Name != "complex" || decision.Route.Provider != "openai" {
		t.Fatalf("expected complex OpenAI route, got %+v", decision)
	}
}
