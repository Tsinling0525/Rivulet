package main

import (
	"context"
	"fmt"
	"time"

	"github.com/Tsinling0525/rivulet/engine"
	"github.com/Tsinling0525/rivulet/model"
	_ "github.com/Tsinling0525/rivulet/nodes/ollama" // register ollama node
	"github.com/Tsinling0525/rivulet/plugin"
)

type nullBus struct{}

func (n nullBus) Emit(ctx context.Context, event string, fields map[string]any) error { return nil }

type memState struct{}

func (m memState) SaveNodeState(context.Context, string, model.ID, map[string]any) error { return nil }
func (m memState) LoadNodeState(context.Context, string, model.ID) (map[string]any, error) {
	return map[string]any{}, nil
}

func main() {
	deps := plugin.Deps{State: memState{}, Bus: nullBus{}}
	eng := engine.New(deps)

	wf := model.Workflow{
		ID:   "wf_ollama_1",
		Name: "OllamaSimple",
		Nodes: []model.Node{
			{
				ID:      "n1",
				Type:    "ollama",
				Name:    "Ollama",
				Timeout: 60 * time.Second,
				Config: map[string]any{
					"model":  "gemma3:latest", // change to your local model name
					"prompt": "Summarize: {{.text}}",
				},
			},
		},
	}

	inputs := map[model.ID]model.Items{
		"n1": {{"text": "Rivulet is a lightweight, n8n-inspired workflow engine written in Go."}},
	}

	res, err := eng.Run(context.Background(), "exec-ollama-001", wf, inputs)
	if err != nil {
		panic(err)
	}
	fmt.Println(res)
}
