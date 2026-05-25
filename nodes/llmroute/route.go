package llmroute

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

type Node struct {
	llm.LLMNodeBase
	deps plugin.Deps
}

type routeConfig struct {
	Name        string
	Provider    string
	Model       string
	Endpoint    string
	APIKeyEnv   string
	Temperature float64
	MaxTokens   int
}

type routeDecision struct {
	Route  routeConfig
	Score  float64
	Reason string
}

type tokenUsage struct {
	Input  int
	Output int
	Total  int
}

func (n *Node) Init(ctx context.Context, deps plugin.Deps) error {
	n.deps = deps
	return n.LLMNodeBase.Init(ctx, deps)
}

func (n *Node) Process(ctx context.Context, wf model.Workflow, node model.Node, in model.Items) (model.Items, error) {
	promptTemplate, _ := node.Config["prompt"].(string)
	if promptTemplate == "" {
		return nil, errors.New("llm:route requires prompt")
	}
	cacheOpts := llm.SemanticCacheConfig(node.Config["semantic_cache"])
	out := make(model.Items, 0, len(in))
	for _, item := range in {
		if item == nil {
			item = model.Item{}
		}
		prompt, err := n.RenderPrompt(promptTemplate, item)
		if err != nil {
			return nil, err
		}
		decision := decideRoute(node.Config, prompt, item)
		cacheNodeID := string(node.ID) + ":" + decision.Route.Name
		if hit, ok := llm.LookupSemanticCache(decision.Route.Provider, decision.Route.Model, cacheNodeID, prompt, cacheOpts); ok {
			n.emitCall(ctx, wf, node, promptTemplate, prompt, hit.Output, decision, tokenUsage{}, 0, "cached", "", map[string]any{
				"cache_hit":        true,
				"cache_type":       "semantic",
				"cache_similarity": hit.Similarity,
				"cached_prompt":    hit.PromptHash,
			})
			out = append(out, routedItem(node, prompt, hit.Output, decision, true))
			continue
		}
		start := time.Now()
		output, usage, err := callRoute(ctx, decision.Route, prompt)
		latency := time.Since(start)
		if err != nil {
			n.emitCall(ctx, wf, node, promptTemplate, prompt, "", decision, usage, latency, "failed", err.Error(), nil)
			return nil, err
		}
		n.emitCall(ctx, wf, node, promptTemplate, prompt, output, decision, usage, latency, "succeeded", "", nil)
		llm.StoreSemanticCache(decision.Route.Provider, decision.Route.Model, cacheNodeID, prompt, output, cacheOpts)
		out = append(out, routedItem(node, prompt, output, decision, false))
	}
	return out, nil
}

func decideRoute(config map[string]any, prompt string, item model.Item) routeDecision {
	routes := parseRoutes(config["routes"])
	score, reason := complexityScore(config, prompt, item)
	threshold := floatConfig(config["complexity_threshold"], 0.55)
	routeName := "simple"
	if score >= threshold {
		routeName = "complex"
	}
	if forced, _ := config["route"].(string); forced != "" {
		routeName = forced
		reason = "forced route"
	}
	if field, _ := config["difficulty_field"].(string); field != "" {
		value := strings.ToLower(fmt.Sprint(item[field]))
		switch value {
		case "simple", "easy", "classification", "classify":
			routeName = "simple"
			reason = "difficulty field selected simple route"
		case "complex", "hard", "reasoning", "analysis":
			routeName = "complex"
			reason = "difficulty field selected complex route"
		}
	}
	route, ok := routes[routeName]
	if !ok {
		route = routes["simple"]
	}
	route.Name = routeName
	return routeDecision{Route: route, Score: score, Reason: reason}
}

func complexityScore(config map[string]any, prompt string, item model.Item) (float64, string) {
	text := strings.ToLower(prompt + " " + fmt.Sprint(item["task"]) + " " + fmt.Sprint(item["type"]))
	if taskType := strings.ToLower(fmt.Sprint(item["task_type"])); taskType != "" {
		switch taskType {
		case "classification", "classify", "tagging", "sentiment":
			return 0.15, "task_type indicates classification"
		case "reasoning", "analysis", "planning":
			return 0.85, "task_type indicates reasoning"
		}
	}
	score := 0.2
	if len(prompt) > 600 {
		score += 0.25
	}
	if len(prompt) > 1600 {
		score += 0.2
	}
	complexTerms := []string{"reason", "prove", "derive", "analyze", "multi-step", "tradeoff", "optimize", "debug", "architecture", "strategy"}
	for _, term := range stringSlice(config["complex_keywords"], complexTerms) {
		if strings.Contains(text, strings.ToLower(term)) {
			score += 0.12
		}
	}
	for _, term := range stringSlice(config["simple_keywords"], []string{"classify", "categorize", "label", "sentiment", "extract"}) {
		if strings.Contains(text, strings.ToLower(term)) {
			score -= 0.12
		}
	}
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}
	return score, "heuristic complexity score"
}

func parseRoutes(raw any) map[string]routeConfig {
	routes := map[string]routeConfig{
		"simple": {
			Name:      "simple",
			Provider:  "ollama",
			Model:     "llama3.2",
			Endpoint:  "http://localhost:11434/api/generate",
			MaxTokens: 512,
		},
		"complex": {
			Name:      "complex",
			Provider:  "openai",
			Model:     "gpt-4o",
			Endpoint:  "https://api.openai.com/v1/responses",
			APIKeyEnv: "OPENAI_API_KEY",
			MaxTokens: 1024,
		},
	}
	byName, ok := raw.(map[string]any)
	if !ok {
		return routes
	}
	for name, value := range byName {
		m, ok := value.(map[string]any)
		if !ok {
			continue
		}
		current := routes[name]
		current.Name = name
		current.Provider = stringConfig(m["provider"], current.Provider)
		current.Model = stringConfig(m["model"], current.Model)
		current.Endpoint = stringConfig(m["endpoint"], current.Endpoint)
		current.APIKeyEnv = stringConfig(m["api_key_env"], current.APIKeyEnv)
		current.Temperature = floatConfig(m["temperature"], current.Temperature)
		current.MaxTokens = int(floatConfig(m["max_tokens"], float64(current.MaxTokens)))
		if current.MaxTokens == 0 {
			current.MaxTokens = int(floatConfig(m["max_output_tokens"], 0))
		}
		routes[name] = current
	}
	return routes
}

func callRoute(ctx context.Context, route routeConfig, prompt string) (string, tokenUsage, error) {
	switch route.Provider {
	case "ollama":
		return callOllama(ctx, route, prompt)
	case "openai":
		return callOpenAI(ctx, route, prompt)
	case "anthropic", "claude":
		return callAnthropic(ctx, route, prompt)
	default:
		return "", tokenUsage{}, fmt.Errorf("unsupported route provider %q", route.Provider)
	}
}

func callOllama(ctx context.Context, route routeConfig, prompt string) (string, tokenUsage, error) {
	body, _ := json.Marshal(map[string]any{"model": route.Model, "prompt": prompt, "stream": false})
	resp, err := postJSON(ctx, route.Endpoint, "", body)
	if err != nil {
		return "", tokenUsage{}, err
	}
	var parsed struct {
		Response        string `json:"response"`
		PromptEvalCount int    `json:"prompt_eval_count"`
		EvalCount       int    `json:"eval_count"`
	}
	if err := json.Unmarshal(resp, &parsed); err != nil {
		return "", tokenUsage{}, err
	}
	return parsed.Response, tokenUsage{Input: parsed.PromptEvalCount, Output: parsed.EvalCount, Total: parsed.PromptEvalCount + parsed.EvalCount}, nil
}

func callOpenAI(ctx context.Context, route routeConfig, prompt string) (string, tokenUsage, error) {
	payload := map[string]any{"model": route.Model, "input": prompt}
	if route.MaxTokens > 0 {
		payload["max_output_tokens"] = route.MaxTokens
	}
	if route.Temperature != 0 && !strings.HasPrefix(strings.ToLower(route.Model), "gpt-5") {
		payload["temperature"] = route.Temperature
	}
	body, _ := json.Marshal(payload)
	resp, err := postJSON(ctx, route.Endpoint, bearerToken(route.APIKeyEnv, "OPENAI_API_KEY"), body)
	if err != nil {
		return "", tokenUsage{}, err
	}
	var parsed struct {
		OutputText string `json:"output_text"`
		Usage      struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
			TotalTokens  int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(resp, &parsed); err != nil {
		return "", tokenUsage{}, err
	}
	return parsed.OutputText, tokenUsage{Input: parsed.Usage.InputTokens, Output: parsed.Usage.OutputTokens, Total: parsed.Usage.TotalTokens}, nil
}

func callAnthropic(ctx context.Context, route routeConfig, prompt string) (string, tokenUsage, error) {
	if route.Endpoint == "" {
		route.Endpoint = "https://api.anthropic.com/v1/messages"
	}
	maxTokens := route.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 1024
	}
	payload := map[string]any{
		"model":      route.Model,
		"max_tokens": maxTokens,
		"messages":   []map[string]string{{"role": "user", "content": prompt}},
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, route.Endpoint, bytes.NewReader(body))
	if err != nil {
		return "", tokenUsage{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")
	apiKey := os.Getenv(stringConfig(route.APIKeyEnv, "ANTHROPIC_API_KEY"))
	if apiKey != "" {
		req.Header.Set("x-api-key", apiKey)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", tokenUsage{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", tokenUsage{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", tokenUsage{}, fmt.Errorf("anthropic error: status %s body=%s", resp.Status, strings.TrimSpace(string(raw)))
	}
	var parsed struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", tokenUsage{}, err
	}
	output := ""
	if len(parsed.Content) > 0 {
		output = parsed.Content[0].Text
	}
	return output, tokenUsage{Input: parsed.Usage.InputTokens, Output: parsed.Usage.OutputTokens, Total: parsed.Usage.InputTokens + parsed.Usage.OutputTokens}, nil
}

func postJSON(ctx context.Context, endpoint, token string, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("route provider error: status %s body=%s", resp.Status, strings.TrimSpace(string(raw)))
	}
	return raw, nil
}

func (n *Node) emitCall(ctx context.Context, wf model.Workflow, node model.Node, promptTemplate, prompt, output string, decision routeDecision, usage tokenUsage, latency time.Duration, status, errText string, extra map[string]any) {
	if extra == nil {
		extra = map[string]any{}
	}
	extra["route"] = decision.Route.Name
	extra["route_reason"] = decision.Reason
	extra["complexity_score"] = decision.Score
	n.EmitAIReasoningStep(ctx, plugin.ExecutionIDFromContext(ctx), wf, node, llm.AIReasoningStep{
		Provider:  decision.Route.Provider,
		Model:     decision.Route.Model,
		Endpoint:  decision.Route.Endpoint,
		Index:     1,
		Title:     "Routing decision",
		Text:      fmt.Sprintf("%s (score %.2f)", decision.Reason, decision.Score),
		Source:    "router",
		LatencyMS: 0,
		DeltaMS:   0,
		Status:    status,
	})
	n.EmitAIModelCall(ctx, plugin.ExecutionIDFromContext(ctx), wf, node, llm.AIModelCall{
		Provider:           decision.Route.Provider,
		Model:              decision.Route.Model,
		Endpoint:           decision.Route.Endpoint,
		PromptHash:         llm.PromptHash(prompt),
		PromptTemplateHash: llm.PromptHash(promptTemplate),
		PromptPreview:      llm.Preview(prompt, 300),
		OutputPreview:      llm.Preview(output, 300),
		InputTokens:        usage.Input,
		OutputTokens:       usage.Output,
		TotalTokens:        usage.Total,
		LatencyMS:          latency.Milliseconds(),
		Status:             status,
		Error:              errText,
		Extra:              extra,
	})
}

func routedItem(node model.Node, prompt, output string, decision routeDecision, cacheHit bool) model.Item {
	return model.Item{
		"prompt":           prompt,
		"output":           output,
		"model":            decision.Route.Model,
		"provider":         decision.Route.Provider,
		"node_id":          node.ID,
		"route":            decision.Route.Name,
		"route_reason":     decision.Reason,
		"complexity_score": decision.Score,
		"cache_hit":        cacheHit,
	}
}

func bearerToken(keyEnv, def string) string {
	if keyEnv == "" {
		keyEnv = def
	}
	return os.Getenv(keyEnv)
}

func stringConfig(raw any, def string) string {
	if value, ok := raw.(string); ok && value != "" {
		return value
	}
	return def
}

func floatConfig(raw any, def float64) float64 {
	switch value := raw.(type) {
	case int:
		return float64(value)
	case int64:
		return float64(value)
	case float64:
		return value
	case float32:
		return float64(value)
	case json.Number:
		if out, err := value.Float64(); err == nil {
			return out
		}
	}
	return def
}

func stringSlice(raw any, def []string) []string {
	switch value := raw.(type) {
	case []string:
		return append([]string(nil), value...)
	case []any:
		out := make([]string, 0, len(value))
		for _, item := range value {
			out = append(out, fmt.Sprint(item))
		}
		return out
	default:
		return def
	}
}

func init() {
	plugin.Register("llm:route", func() plugin.NodeHandler { return &Node{} })
}
