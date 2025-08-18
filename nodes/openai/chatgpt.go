package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"rivulet/model"
	"rivulet/nodes/llm"
	"rivulet/plugin"
)

type ChatGPTNode struct {
	llm.LLMNodeBase
	cfg    llm.LLMConfig
	apiKey string
}

func (n *ChatGPTNode) Init(ctx context.Context, deps plugin.Deps) error {
	n.apiKey = os.Getenv("OPENAI_API_KEY")
	if n.apiKey == "" {
		return errors.New("OPENAI_API_KEY is not set")
	}
	return n.LLMNodeBase.Init(ctx, deps)
}

func (n *ChatGPTNode) Process(ctx context.Context, wf model.Workflow, node model.Node, in model.Items) (model.Items, error) {
	n.cfg.Model, _ = node.Config["model"].(string)
	if n.cfg.Model == "" {
		n.cfg.Model = "gpt-3.5-turbo"
	}
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
		n.cfg.Endpoint = "https://api.openai.com/v1/chat/completions"
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
		payload := map[string]any{
			"model":       n.cfg.Model,
			"messages":    []map[string]string{{"role": "user", "content": prompt}},
			"temperature": n.cfg.Temperature,
		}
		data, _ := json.Marshal(payload)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.cfg.Endpoint, bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+n.apiKey)
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("openai error: status %s", resp.Status)
		}
		var parsed struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
			return nil, err
		}
		content := ""
		if len(parsed.Choices) > 0 {
			content = parsed.Choices[0].Message.Content
		}
		out = append(out, model.Item{
			"prompt":  prompt,
			"output":  content,
			"model":   n.cfg.Model,
			"node_id": node.ID,
		})
	}
	return out, nil
}

func init() { plugin.Register("chatgpt", func() plugin.NodeHandler { return &ChatGPTNode{} }) }
