package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"rivulet/model"
	"rivulet/nodes/llm"
	"rivulet/plugin"
)

type Node struct {
	llm.LLMNodeBase
	cfg llm.LLMConfig
}

func (n *Node) Init(ctx context.Context, deps plugin.Deps) error {
	return n.LLMNodeBase.Init(ctx, deps)
}

func (n *Node) Process(ctx context.Context, wf model.Workflow, node model.Node, in model.Items) (model.Items, error) {
	// map node.Config into cfg
	n.cfg.Model, _ = node.Config["model"].(string)
	n.cfg.Prompt, _ = node.Config["prompt"].(string)
	if t, ok := node.Config["temperature"].(float64); ok {
		n.cfg.Temperature = t
	} else {
		n.cfg.Temperature = 0.7
	}
	if mt, ok := node.Config["max_tokens"].(int); ok {
		n.cfg.MaxTokens = mt
	} else {
		n.cfg.MaxTokens = 512
	}
	n.cfg.Endpoint, _ = node.Config["endpoint"].(string)
	if n.cfg.Endpoint == "" {
		n.cfg.Endpoint = "http://localhost:11434/api/generate"
	}

	client := &http.Client{Timeout: 60 * time.Second}
	out := make(model.Items, 0, len(in))
	for _, item := range in {
		if item == nil {
			item = model.Item{}
		}
		prompt, err := n.RenderPrompt(n.cfg.Prompt, item)
		if err != nil {
			return nil, err
		}

		reqBody := map[string]any{
			"model":  n.cfg.Model,
			"prompt": prompt,
			"stream": false,
		}
		data, _ := json.Marshal(reqBody)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.cfg.Endpoint, bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("ollama error: status %s", resp.Status)
		}
		var parsed struct {
			Response string `json:"response"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
			return nil, err
		}

		out = append(out, model.Item{
			"prompt":  prompt,
			"output":  parsed.Response,
			"model":   n.cfg.Model,
			"node_id": node.ID,
		})
	}
	return out, nil
}

func init() { plugin.Register("ollama", func() plugin.NodeHandler { return &Node{} }) }
