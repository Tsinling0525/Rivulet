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

type tokenUsage struct {
	Input  int
	Output int
	Total  int
}

type generationResult struct {
	Output    string
	Usage     tokenUsage
	Reasoning []llm.AIReasoningStep
}

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
	cacheOpts := llm.SemanticCacheConfig(node.Config["semantic_cache"])
	out := make(model.Items, 0, len(in))
	for _, item := range in {
		if item == nil {
			item = model.Item{}
		}
		prompt, err := n.RenderPrompt(n.cfg.Prompt, item)
		if err != nil {
			return nil, err
		}
		if hit, ok := llm.LookupSemanticCache("openai", n.cfg.Model, string(node.ID), prompt, cacheOpts); ok {
			n.emitCachedCall(ctx, wf, node, prompt, hit.Output, hit)
			out = append(out, model.Item{
				"prompt":     prompt,
				"output":     hit.Output,
				"model":      n.cfg.Model,
				"node_id":    node.ID,
				"cache_hit":  true,
				"cache_type": "semantic",
			})
			continue
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
		start := time.Now()
		n.emitReasoningStep(ctx, wf, node, llm.AIReasoningStep{
			Provider:  "openai",
			Model:     n.cfg.Model,
			Endpoint:  n.cfg.Endpoint,
			Index:     1,
			Title:     "Prompt submitted",
			Text:      "Request sent to the model endpoint.",
			Source:    "lifecycle",
			LatencyMS: 0,
			DeltaMS:   0,
			Status:    "running",
		})
		resp, err := client.Do(req)
		if err != nil {
			n.emitReasoningStep(ctx, wf, node, failureReasoningStep("openai", n.cfg.Model, n.cfg.Endpoint, time.Since(start), err))
			n.emitCall(ctx, wf, node, prompt, "", tokenUsage{}, time.Since(start), err)
			return nil, err
		}
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			n.emitReasoningStep(ctx, wf, node, failureReasoningStep("openai", n.cfg.Model, n.cfg.Endpoint, time.Since(start), readErr))
			n.emitCall(ctx, wf, node, prompt, "", tokenUsage{}, time.Since(start), readErr)
			return nil, readErr
		}
		if resp.StatusCode != http.StatusOK {
			err := fmt.Errorf("openai error: status %s body=%s", resp.Status, strings.TrimSpace(string(body)))
			n.emitReasoningStep(ctx, wf, node, failureReasoningStep("openai", n.cfg.Model, n.cfg.Endpoint, time.Since(start), err))
			n.emitCall(ctx, wf, node, prompt, "", tokenUsage{}, time.Since(start), err)
			return nil, err
		}

		result, err := n.extractGenerationResult(n.cfg.Endpoint, body)
		elapsed := time.Since(start)
		if err != nil {
			n.emitReasoningStep(ctx, wf, node, failureReasoningStep("openai", n.cfg.Model, n.cfg.Endpoint, elapsed, err))
			n.emitCall(ctx, wf, node, prompt, "", result.Usage, elapsed, err)
			return nil, err
		}
		result.Reasoning = withReasoningTiming(result.Reasoning, elapsed)
		n.emitReasoningSteps(ctx, wf, node, result.Reasoning)
		n.emitReasoningStep(ctx, wf, node, llm.AIReasoningStep{
			Provider:  "openai",
			Model:     n.cfg.Model,
			Endpoint:  n.cfg.Endpoint,
			Index:     len(result.Reasoning) + 2,
			Title:     "Response completed",
			Text:      "Model response received and parsed.",
			Source:    "lifecycle",
			LatencyMS: elapsed.Milliseconds(),
			DeltaMS:   elapsed.Milliseconds(),
			Status:    "succeeded",
		})
		n.emitCall(ctx, wf, node, prompt, result.Output, result.Usage, elapsed, nil)
		llm.StoreSemanticCache("openai", n.cfg.Model, string(node.ID), prompt, result.Output, cacheOpts)

		out = append(out, model.Item{
			"prompt":  prompt,
			"output":  result.Output,
			"model":   n.cfg.Model,
			"node_id": node.ID,
		})
	}
	return out, nil
}

func (n *ChatGPTNode) AIMetadata(wf model.Workflow, node model.Node) model.AINodeMetadata {
	modelName, _ := node.Config["model"].(string)
	if modelName == "" {
		modelName = "gpt-5-mini"
	}
	prompt, _ := node.Config["prompt"].(string)
	humanReview, _ := node.Config["human_review_required"].(bool)
	return model.AINodeMetadata{
		Provider:            "openai",
		Model:               modelName,
		PromptTemplate:      prompt,
		HumanReviewRequired: humanReview,
	}
}

func (n *ChatGPTNode) emitCall(ctx context.Context, wf model.Workflow, node model.Node, prompt, output string, usage tokenUsage, latency time.Duration, callErr error) {
	status := "succeeded"
	errText := ""
	if callErr != nil {
		status = "failed"
		errText = callErr.Error()
	}
	humanReview, _ := node.Config["human_review_required"].(bool)
	n.EmitAIModelCall(ctx, plugin.ExecutionIDFromContext(ctx), wf, node, llm.AIModelCall{
		Provider:           "openai",
		Model:              n.cfg.Model,
		Endpoint:           n.cfg.Endpoint,
		PromptHash:         llm.PromptHash(prompt),
		PromptTemplateHash: llm.PromptHash(n.cfg.Prompt),
		PromptPreview:      llm.Preview(prompt, 300),
		OutputPreview:      llm.Preview(output, 300),
		InputTokens:        usage.Input,
		OutputTokens:       usage.Output,
		TotalTokens:        usage.Total,
		LatencyMS:          latency.Milliseconds(),
		Status:             status,
		Error:              errText,
		HumanReview:        humanReview,
	})
}

func (n *ChatGPTNode) emitReasoningStep(ctx context.Context, wf model.Workflow, node model.Node, step llm.AIReasoningStep) {
	n.EmitAIReasoningStep(ctx, plugin.ExecutionIDFromContext(ctx), wf, node, step)
}

func (n *ChatGPTNode) emitReasoningSteps(ctx context.Context, wf model.Workflow, node model.Node, steps []llm.AIReasoningStep) {
	n.EmitAIReasoningSteps(ctx, plugin.ExecutionIDFromContext(ctx), wf, node, steps)
}

func (n *ChatGPTNode) emitCachedCall(ctx context.Context, wf model.Workflow, node model.Node, prompt, output string, hit llm.SemanticCacheHit) {
	humanReview, _ := node.Config["human_review_required"].(bool)
	n.EmitAIModelCall(ctx, plugin.ExecutionIDFromContext(ctx), wf, node, llm.AIModelCall{
		Provider:           "openai",
		Model:              n.cfg.Model,
		Endpoint:           n.cfg.Endpoint,
		PromptHash:         llm.PromptHash(prompt),
		PromptTemplateHash: llm.PromptHash(n.cfg.Prompt),
		PromptPreview:      llm.Preview(prompt, 300),
		OutputPreview:      llm.Preview(output, 300),
		Status:             "cached",
		HumanReview:        humanReview,
		Extra: map[string]any{
			"cache_hit":        true,
			"cache_type":       "semantic",
			"cache_similarity": hit.Similarity,
			"cached_prompt":    hit.PromptHash,
		},
	})
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
	output, _, err := n.extractOutputWithUsage(endpoint, body)
	return output, err
}

func (n *ChatGPTNode) extractOutputWithUsage(endpoint string, body []byte) (string, tokenUsage, error) {
	result, err := n.extractGenerationResult(endpoint, body)
	return result.Output, result.Usage, err
}

func (n *ChatGPTNode) extractGenerationResult(endpoint string, body []byte) (generationResult, error) {
	if strings.Contains(endpoint, "/chat/completions") {
		var parsed struct {
			Choices []struct {
				Message struct {
					Content          string `json:"content"`
					ReasoningContent string `json:"reasoning_content"`
				} `json:"message"`
			} `json:"choices"`
			Usage struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
				TotalTokens      int `json:"total_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return generationResult{}, err
		}
		usage := tokenUsage{
			Input:  parsed.Usage.PromptTokens,
			Output: parsed.Usage.CompletionTokens,
			Total:  parsed.Usage.TotalTokens,
		}
		if len(parsed.Choices) == 0 {
			return generationResult{Usage: usage}, nil
		}
		message := parsed.Choices[0].Message
		reasoning := llm.ReasoningStepsFromText("openai", n.cfg.Model, n.cfg.Endpoint, "reasoning_content", "Reasoning", message.ReasoningContent, 2, 0)
		return generationResult{Output: message.Content, Usage: usage, Reasoning: reasoning}, nil
	}

	var parsed struct {
		OutputText string `json:"output_text"`
		Output     []struct {
			Type    string `json:"type"`
			Text    string `json:"text"`
			Summary []struct {
				Text string `json:"text"`
			} `json:"summary"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
			TotalTokens  int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return generationResult{}, err
	}
	usage := tokenUsage{
		Input:  parsed.Usage.InputTokens,
		Output: parsed.Usage.OutputTokens,
		Total:  parsed.Usage.TotalTokens,
	}
	reasoningTexts := make([]string, 0)
	for _, output := range parsed.Output {
		if output.Type != "reasoning" {
			continue
		}
		if strings.TrimSpace(output.Text) != "" {
			reasoningTexts = append(reasoningTexts, output.Text)
		}
		for _, summary := range output.Summary {
			if strings.TrimSpace(summary.Text) != "" {
				reasoningTexts = append(reasoningTexts, summary.Text)
			}
		}
		for _, content := range output.Content {
			if strings.TrimSpace(content.Text) != "" {
				reasoningTexts = append(reasoningTexts, content.Text)
			}
		}
	}
	reasoning := llm.ReasoningStepsFromText("openai", n.cfg.Model, n.cfg.Endpoint, "reasoning_summary", "Reasoning summary", strings.Join(reasoningTexts, "\n\n"), 2, 0)
	if parsed.OutputText != "" {
		return generationResult{Output: parsed.OutputText, Usage: usage, Reasoning: reasoning}, nil
	}
	for _, output := range parsed.Output {
		for _, content := range output.Content {
			if content.Type == "output_text" || content.Type == "text" {
				return generationResult{Output: content.Text, Usage: usage, Reasoning: reasoning}, nil
			}
		}
	}
	return generationResult{Usage: usage, Reasoning: reasoning}, nil
}

func failureReasoningStep(provider, model, endpoint string, latency time.Duration, err error) llm.AIReasoningStep {
	return llm.AIReasoningStep{
		Provider:  provider,
		Model:     model,
		Endpoint:  endpoint,
		Index:     2,
		Title:     "Request failed",
		Text:      err.Error(),
		Source:    "lifecycle",
		LatencyMS: latency.Milliseconds(),
		DeltaMS:   latency.Milliseconds(),
		Status:    "failed",
	}
}

func withReasoningTiming(steps []llm.AIReasoningStep, totalLatency time.Duration) []llm.AIReasoningStep {
	if len(steps) == 0 || totalLatency <= 0 {
		return steps
	}
	totalMS := totalLatency.Milliseconds()
	out := append([]llm.AIReasoningStep(nil), steps...)
	var prev int64
	for idx := range out {
		latencyMS := int64(float64(totalMS) * (float64(idx+1) / float64(len(out)+1)))
		out[idx].LatencyMS = latencyMS
		out[idx].DeltaMS = latencyMS - prev
		prev = latencyMS
	}
	return out
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
