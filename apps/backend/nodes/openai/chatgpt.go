package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Tsinling0525/rivulet/model"
	"github.com/Tsinling0525/rivulet/nodes/llm"
	"github.com/Tsinling0525/rivulet/plugin"
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
		n.cfg.Model = "gpt-5-mini"
	}
	n.cfg.Prompt, _ = node.Config["prompt"].(string)
	if t, ok := numberFromAny(node.Config["temperature"]); ok {
		n.cfg.Temperature = t
	}
	if mt, ok := intFromAny(node.Config["max_output_tokens"]); ok {
		n.cfg.MaxTokens = mt
	} else if mt, ok := intFromAny(node.Config["max_tokens"]); ok {
		n.cfg.MaxTokens = mt
	} else {
		n.cfg.MaxTokens = 512
	}
	n.cfg.Endpoint, _ = node.Config["endpoint"].(string)
	if n.cfg.Endpoint == "" {
		n.cfg.Endpoint = "https://api.openai.com/v1/responses"
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

		payload := n.buildPayload(node, prompt)
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
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
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("openai error: status %s body=%s", resp.Status, strings.TrimSpace(string(body)))
		}

		content, err := n.extractOutput(n.cfg.Endpoint, body)
		if err != nil {
			return nil, err
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

func (n *ChatGPTNode) buildPayload(node model.Node, prompt string) map[string]any {
	if strings.Contains(n.cfg.Endpoint, "/chat/completions") {
		payload := map[string]any{
			"model":    n.cfg.Model,
			"messages": []map[string]string{{"role": "user", "content": prompt}},
		}
		if n.cfg.Temperature != 0 {
			payload["temperature"] = n.cfg.Temperature
		}
		if n.cfg.MaxTokens > 0 {
			payload["max_tokens"] = n.cfg.MaxTokens
		}
		return payload
	}

	payload := map[string]any{
		"model": n.cfg.Model,
		"input": prompt,
	}
	if n.cfg.MaxTokens > 0 {
		payload["max_output_tokens"] = n.cfg.MaxTokens
	}
	if effort, _ := node.Config["reasoning_effort"].(string); effort != "" {
		payload["reasoning"] = map[string]any{"effort": effort}
	}
	if verbosity, _ := node.Config["verbosity"].(string); verbosity != "" {
		payload["text"] = map[string]any{"verbosity": verbosity}
	}
	if n.cfg.Temperature != 0 && !strings.HasPrefix(strings.ToLower(n.cfg.Model), "gpt-5") {
		payload["temperature"] = n.cfg.Temperature
	}
	return payload
}

func (n *ChatGPTNode) extractOutput(endpoint string, body []byte) (string, error) {
	if strings.Contains(endpoint, "/chat/completions") {
		var parsed struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return "", err
		}
		if len(parsed.Choices) == 0 {
			return "", nil
		}
		return parsed.Choices[0].Message.Content, nil
	}

	var parsed struct {
		OutputText string `json:"output_text"`
		Output     []struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", err
	}
	if parsed.OutputText != "" {
		return parsed.OutputText, nil
	}
	for _, output := range parsed.Output {
		for _, content := range output.Content {
			if content.Type == "output_text" || content.Type == "text" {
				return content.Text, nil
			}
		}
	}
	return "", nil
}

func intFromAny(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	default:
		return 0, false
	}
}

func numberFromAny(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}

func init() { plugin.Register("chatgpt", func() plugin.NodeHandler { return &ChatGPTNode{} }) }
