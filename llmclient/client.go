// Package llmclient is the shared OpenAI-compatible model client used by both
// the workflow llm node and the coding agent CLI.
//
// Two endpoint shapes are supported, matching the vendors Rivulet talks to:
//
//	/chat/completions   chat messages (OpenAI, DeepSeek, Ollama, OpenRouter, ...)
//	anything else       the OpenAI Responses API (input / max_output_tokens)
package llmclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ErrMissingAPIKey is returned when a request would be sent without credentials.
var ErrMissingAPIKey = errors.New("missing API key")

// Message is one chat message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Config describes one model endpoint.
type Config struct {
	Endpoint        string
	APIKey          string
	Model           string
	MaxOutputTokens int
	ResponseFormat  string
	Temperature     *float64
	ExtraFields     map[string]any
	HTTPClient      *http.Client
	Timeout         time.Duration
}

// Result is a single completion.
type Result struct {
	Text  string
	Usage map[string]any
}

const (
	defaultModel         = "gpt-5-mini"
	defaultEndpoint      = "https://api.openai.com/v1/responses"
	defaultMaxTokens     = 1200
	defaultClientTimeout = 90 * time.Second
)

// Complete sends a single user message and returns the text.
func Complete(ctx context.Context, cfg Config, prompt string) (string, error) {
	result, err := Chat(ctx, cfg, []Message{{Role: "user", Content: prompt}})
	if err != nil {
		return "", err
	}
	return result.Text, nil
}

// Chat sends chat messages to the configured endpoint.
func Chat(ctx context.Context, cfg Config, messages []Message) (Result, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return Result{}, ErrMissingAPIKey
	}
	if len(messages) == 0 {
		return Result{}, errors.New("llmclient: no messages")
	}

	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = defaultModel
	}
	endpoint := strings.TrimSpace(cfg.Endpoint)
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	maxTokens := cfg.MaxOutputTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}

	chatShape := strings.Contains(endpoint, "/chat/completions")
	payload := map[string]any{"model": model}
	for key, value := range cfg.ExtraFields {
		payload[key] = value
	}
	if chatShape {
		payload["messages"] = messages
		payload["max_tokens"] = maxTokens
		if cfg.ResponseFormat != "" {
			payload["response_format"] = map[string]string{"type": cfg.ResponseFormat}
		}
	} else {
		payload["input"] = joinMessages(messages)
		payload["max_output_tokens"] = maxTokens
	}
	if cfg.Temperature != nil {
		payload["temperature"] = *cfg.Temperature
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return Result{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")

	client := cfg.HTTPClient
	if client == nil {
		timeout := cfg.Timeout
		if timeout <= 0 {
			timeout = defaultClientTimeout
		}
		client = &http.Client{Timeout: timeout}
	}

	resp, err := client.Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return Result{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Result{}, fmt.Errorf("model error: status %s body=%s", resp.Status, truncate(strings.TrimSpace(string(data)), 800))
	}
	return extract(endpoint, data)
}

func joinMessages(messages []Message) string {
	if len(messages) == 1 {
		return messages[0].Content
	}
	parts := make([]string, 0, len(messages))
	for _, message := range messages {
		parts = append(parts, message.Content)
	}
	return strings.Join(parts, "\n\n")
}

func extract(endpoint string, body []byte) (Result, error) {
	if strings.Contains(endpoint, "/chat/completions") {
		var parsed struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
			Usage map[string]any `json:"usage"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return Result{}, err
		}
		if len(parsed.Choices) == 0 {
			return Result{}, errors.New("model response contained no choices")
		}
		return Result{Text: parsed.Choices[0].Message.Content, Usage: parsed.Usage}, nil
	}

	var parsed struct {
		OutputText string         `json:"output_text"`
		Usage      map[string]any `json:"usage"`
		Output     []struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return Result{}, err
	}
	if strings.TrimSpace(parsed.OutputText) != "" {
		return Result{Text: parsed.OutputText, Usage: parsed.Usage}, nil
	}
	for _, output := range parsed.Output {
		for _, content := range output.Content {
			if content.Type == "output_text" || content.Type == "text" {
				return Result{Text: content.Text, Usage: parsed.Usage}, nil
			}
		}
	}
	return Result{}, errors.New("model response contained no output text")
}

func truncate(text string, max int) string {
	if len(text) <= max {
		return text
	}
	return text[:max] + "..."
}
