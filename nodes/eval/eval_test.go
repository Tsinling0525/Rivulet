package eval

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tsinling0525/rivulet/model"
	"github.com/Tsinling0525/rivulet/plugin"
)

type eventRecorder struct {
	events []string
	fields []map[string]any
}

func (r *eventRecorder) Emit(ctx context.Context, event string, fields map[string]any) error {
	r.events = append(r.events, event)
	r.fields = append(r.fields, fields)
	return nil
}

func TestEvalNodeCriteriaRoutesPassAndFail(t *testing.T) {
	bus := &eventRecorder{}
	node := &Node{}
	if err := node.Init(context.Background(), plugin.Deps{Bus: bus}); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}

	out, err := node.ProcessPorted(context.Background(), model.Workflow{ID: "wf"}, model.Node{
		ID:   "eval1",
		Type: "eval:node",
		Config: map[string]any{
			"target_field":   "output",
			"pass_threshold": 0.75,
			"criteria": []any{
				map[string]any{"name": "mentions_total", "type": "contains", "value": "total"},
				map[string]any{"name": "avoid_error", "type": "not_contains", "value": "error"},
			},
		},
	}, model.Items{
		{"output": "The total is 42."},
		{"output": "error: missing answer"},
	})
	if err != nil {
		t.Fatalf("ProcessPorted returned error: %v", err)
	}
	if len(out[model.PortMain]) != 2 {
		t.Fatalf("expected 2 main items, got %d", len(out[model.PortMain]))
	}
	if len(out[model.Port("pass")]) != 1 {
		t.Fatalf("expected 1 pass item, got %d", len(out[model.Port("pass")]))
	}
	if len(out[model.Port("fail")]) != 1 {
		t.Fatalf("expected 1 fail item, got %d", len(out[model.Port("fail")]))
	}

	evalData, ok := out[model.Port("pass")][0]["eval"].(evalResult)
	if !ok {
		t.Fatalf("expected eval result on output item")
	}
	if evalData.Score != 1 || !evalData.Passed {
		t.Fatalf("expected passing score 1, got %+v", evalData)
	}
	if len(bus.events) != 2 || bus.events[0] != "eval_result" {
		t.Fatalf("expected eval_result events, got %+v", bus.events)
	}
}

func TestEvalNodeJudgeScoresOutput(t *testing.T) {
	var requested bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = true
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("expected authorization header")
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("invalid request json: %v", err)
		}
		if payload["model"] != "judge-model" {
			t.Fatalf("expected judge model, got %v", payload["model"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output_text":"{\"score\":0.82,\"passed\":true,\"rationale\":\"meets criteria\",\"criteria\":[{\"name\":\"accuracy\",\"score\":0.82,\"passed\":true}]}"}`))
	}))
	defer server.Close()

	node := &Node{}
	if err := node.Init(context.Background(), plugin.Deps{}); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}
	out, err := node.Process(context.Background(), model.Workflow{ID: "wf"}, model.Node{
		ID:   "eval1",
		Type: "eval:node",
		Config: map[string]any{
			"target_field":   "output",
			"pass_threshold": 0.8,
			"criteria": []any{
				map[string]any{"name": "accuracy", "type": "non_empty"},
			},
			"judge": map[string]any{
				"provider": "openai",
				"model":    "judge-model",
				"endpoint": server.URL,
				"api_key":  "test-key",
			},
		},
	}, model.Items{{"output": "answer"}})
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	if !requested {
		t.Fatalf("expected judge endpoint to be called")
	}
	evalData, ok := out[0]["eval"].(evalResult)
	if !ok {
		t.Fatalf("expected eval result on output item")
	}
	if evalData.Mode != "judge" || evalData.Score != 0.82 || !evalData.Passed {
		t.Fatalf("unexpected judge result: %+v", evalData)
	}
}
