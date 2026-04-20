package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/Tsinling0525/rivulet/model"
	"github.com/Tsinling0525/rivulet/nodes/llm"
	"github.com/Tsinling0525/rivulet/plugin"
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
		start := time.Now()
		resp, err := client.Do(req)
		if err != nil {
			n.emitCall(ctx, wf, node, prompt, "", 0, 0, time.Since(start), err)
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			err := fmt.Errorf("ollama error: status %s", resp.Status)
			n.emitCall(ctx, wf, node, prompt, "", 0, 0, time.Since(start), err)
			return nil, err
		}
		var parsed struct {
			Response        string `json:"response"`
			PromptEvalCount int    `json:"prompt_eval_count"`
			EvalCount       int    `json:"eval_count"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
			resp.Body.Close()
			n.emitCall(ctx, wf, node, prompt, "", parsed.PromptEvalCount, parsed.EvalCount, time.Since(start), err)
			return nil, err
		}
		resp.Body.Close()
		n.emitCall(ctx, wf, node, prompt, parsed.Response, parsed.PromptEvalCount, parsed.EvalCount, time.Since(start), nil)

		out = append(out, model.Item{
			"prompt":  prompt,
			"output":  parsed.Response,
			"model":   n.cfg.Model,
			"node_id": node.ID,
		})
	}
	return out, nil
}

func (n *Node) AIMetadata(wf model.Workflow, node model.Node) model.AINodeMetadata {
	modelName, _ := node.Config["model"].(string)
	prompt, _ := node.Config["prompt"].(string)
	humanReview, _ := node.Config["human_review_required"].(bool)
	return model.AINodeMetadata{
		Provider:            "ollama",
		Model:               modelName,
		PromptTemplate:      prompt,
		HumanReviewRequired: humanReview,
	}
}

func (n *Node) emitCall(ctx context.Context, wf model.Workflow, node model.Node, prompt, output string, inputTokens, outputTokens int, latency time.Duration, callErr error) {
	status := "succeeded"
	errText := ""
	if callErr != nil {
		status = "failed"
		errText = callErr.Error()
	}
	humanReview, _ := node.Config["human_review_required"].(bool)
	n.EmitAIModelCall(ctx, plugin.ExecutionIDFromContext(ctx), wf, node, llm.AIModelCall{
		Provider:      "ollama",
		Model:         n.cfg.Model,
		Endpoint:      n.cfg.Endpoint,
		PromptHash:    llm.PromptHash(prompt),
		PromptPreview: llm.Preview(prompt, 300),
		OutputPreview: llm.Preview(output, 300),
		InputTokens:   inputTokens,
		OutputTokens:  outputTokens,
		TotalTokens:   inputTokens + outputTokens,
		LatencyMS:     latency.Milliseconds(),
		Status:        status,
		Error:         errText,
		HumanReview:   humanReview,
	})
}

func init() { plugin.Register("ollama", func() plugin.NodeHandler { return &Node{} }) }
