package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Tsinling0525/rivulet/agent"
)

type textClient interface {
	Complete(ctx context.Context, prompt string) (string, error)
}

type openAITextClient struct {
	APIKey          string
	Model           string
	Endpoint        string
	MaxOutputTokens int
	ResponseFormat  string
	HTTPClient      *http.Client
}

func (c openAITextClient) Complete(ctx context.Context, prompt string) (string, error) {
	if c.APIKey == "" {
		return "", fmt.Errorf("OPENAI_API_KEY is not set")
	}
	model := strings.TrimSpace(c.Model)
	if model == "" {
		model = "gpt-5-mini"
	}
	endpoint := strings.TrimSpace(c.Endpoint)
	if endpoint == "" {
		endpoint = "https://api.openai.com/v1/responses"
	}
	maxTokens := c.MaxOutputTokens
	if maxTokens <= 0 {
		maxTokens = 1200
	}

	payload := map[string]any{
		"model": model,
	}
	if strings.Contains(endpoint, "/chat/completions") {
		payload["messages"] = []map[string]string{{"role": "user", "content": prompt}}
		payload["max_tokens"] = maxTokens
		if c.ResponseFormat != "" {
			payload["response_format"] = map[string]string{"type": c.ResponseFormat}
		}
	} else {
		payload["input"] = prompt
		payload["max_output_tokens"] = maxTokens
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")

	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 90 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("openai error: status %s body=%s", resp.Status, strings.TrimSpace(string(data)))
	}
	return extractOpenAIText(endpoint, data)
}

type jsonPlanner struct {
	Client textClient
	CWD    string
}

func (p jsonPlanner) Plan(ctx context.Context, state agent.State) (agent.Plan, error) {
	if p.Client == nil {
		return agent.Plan{}, fmt.Errorf("planner client is required")
	}
	prompt := plannerPrompt(p.CWD, state)
	text, err := p.Client.Complete(ctx, prompt)
	if err != nil {
		return agent.Plan{}, err
	}
	var parsed struct {
		Summary  string `json:"summary"`
		ToolCall struct {
			Name string         `json:"name"`
			Args map[string]any `json:"args"`
		} `json:"tool_call"`
	}
	if err := unmarshalJSONResponse(text, &parsed); err != nil {
		return agent.Plan{}, err
	}
	return agent.Plan{
		Summary: strings.TrimSpace(parsed.Summary),
		ToolCall: agent.ToolCall{
			Name: strings.TrimSpace(parsed.ToolCall.Name),
			Args: parsed.ToolCall.Args,
		},
	}, nil
}

type jsonReflector struct {
	Client textClient
}

func (r jsonReflector) Reflect(ctx context.Context, state agent.State, obs agent.Observation) (agent.Reflection, error) {
	if r.Client == nil {
		return agent.Reflection{}, fmt.Errorf("reflector client is required")
	}
	prompt := reflectorPrompt(state, obs)
	text, err := r.Client.Complete(ctx, prompt)
	if err != nil {
		return agent.Reflection{}, err
	}
	var parsed struct {
		Summary  string `json:"summary"`
		Decision string `json:"decision"`
	}
	if err := unmarshalJSONResponse(text, &parsed); err != nil {
		return agent.Reflection{}, err
	}
	decision := agent.ReflectionDecision(strings.TrimSpace(parsed.Decision))
	return agent.Reflection{Summary: strings.TrimSpace(parsed.Summary), Decision: decision}, nil
}

func plannerPrompt(cwd string, state agent.State) string {
	return fmt.Sprintf(`You are Rivulet Agent, a minimal Claude Code style coding CLI.
You are operating inside workspace: %s

Pick exactly one tool call for the next step. Return only JSON:
{"summary":"short reason for the next action","tool_call":{"name":"tool_name","args":{}}}

Available tools:
- list_files: {"path":"optional directory","recursive":false,"max_entries":100}
- read_file: {"path":"file path","offset":0,"limit":20000}
- edit_file: {"path":"file path","old":"exact text to replace","new":"replacement text","replace_all":false}
- write_file: {"path":"file path","content":"full file content","append":false}
- shell: {"command":"command to run","timeout_seconds":30}

Rules:
- Inspect before editing when you are unsure.
- Prefer edit_file for small changes.
- Run targeted tests or build commands after code changes when possible.
- Use shell for project commands such as rg, go test, make test, git diff.
- Do not explain outside JSON.

Goal:
%s

State:
%s
`, cwd, state.Goal, summarizeAgentState(state))
}

func reflectorPrompt(state agent.State, obs agent.Observation) string {
	return fmt.Sprintf(`You are the reflection step for Rivulet Agent.
Decide whether the goal is complete or whether the planner should continue.

Return only JSON:
{"summary":"what happened and what remains","decision":"stop|replan"}

Stop only when the user's goal is actually satisfied. If a tool failed, tests failed,
or more inspection/editing is needed, choose "replan".

Goal:
%s

Latest observation:
%s

State:
%s
`, state.Goal, marshalCompact(obs), summarizeAgentState(state))
}

func summarizeAgentState(state agent.State) string {
	type stepSummary struct {
		Index       int            `json:"index"`
		Plan        string         `json:"plan"`
		Tool        string         `json:"tool"`
		Args        map[string]any `json:"args,omitempty"`
		Observation string         `json:"observation,omitempty"`
		Error       string         `json:"error,omitempty"`
		Reflection  string         `json:"reflection,omitempty"`
	}
	steps := make([]stepSummary, 0, len(state.Steps))
	for _, step := range state.Steps {
		steps = append(steps, stepSummary{
			Index:       step.Index,
			Plan:        truncateText(step.Plan.Summary, 800),
			Tool:        step.Plan.ToolCall.Name,
			Args:        truncateMap(step.Plan.ToolCall.Args, 1000),
			Observation: truncateText(marshalCompact(step.Observation.Output), 4000),
			Error:       truncateText(step.Observation.Error, 1200),
			Reflection:  truncateText(step.Reflection.Summary, 800),
		})
	}
	return marshalCompact(steps)
}

func extractOpenAIText(endpoint string, body []byte) (string, error) {
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
			return "", fmt.Errorf("openai response contained no choices")
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
	if strings.TrimSpace(parsed.OutputText) != "" {
		return parsed.OutputText, nil
	}
	for _, output := range parsed.Output {
		for _, content := range output.Content {
			if content.Type == "output_text" || content.Type == "text" {
				return content.Text, nil
			}
		}
	}
	return "", fmt.Errorf("openai response contained no output text")
}

func unmarshalJSONResponse(text string, target any) error {
	raw := strings.TrimSpace(text)
	if strings.HasPrefix(raw, "```") {
		raw = strings.TrimPrefix(raw, "```json")
		raw = strings.TrimPrefix(raw, "```")
		raw = strings.TrimSuffix(raw, "```")
		raw = strings.TrimSpace(raw)
	}
	if err := json.Unmarshal([]byte(raw), target); err == nil {
		return nil
	}
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start >= 0 && end > start {
		return json.Unmarshal([]byte(raw[start:end+1]), target)
	}
	return fmt.Errorf("model did not return JSON: %s", truncateText(raw, 400))
}

func marshalCompact(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(data)
}

func truncateMap(src map[string]any, max int) map[string]any {
	if src == nil {
		return nil
	}
	out := make(map[string]any, len(src))
	for key, value := range src {
		switch v := value.(type) {
		case string:
			out[key] = truncateText(v, max)
		default:
			out[key] = value
		}
	}
	return out
}
