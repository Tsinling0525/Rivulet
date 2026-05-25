package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	DefaultModel            = "gemma4:e2b"
	DefaultGenerateEndpoint = "http://localhost:11434/api/generate"
	DefaultChatEndpoint     = "http://localhost:11434/api/chat"
)

// ChatMessage is the message shape accepted by Ollama's /api/chat endpoint.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type TokenUsage struct {
	PromptEvalCount int `json:"prompt_eval_count,omitempty"`
	EvalCount       int `json:"eval_count,omitempty"`
}

type ChatCompletion struct {
	Model   string      `json:"model"`
	Message ChatMessage `json:"message"`
	Reply   string      `json:"reply"`
	Usage   TokenUsage  `json:"usage,omitempty"`
}

type Client struct {
	HTTPClient   *http.Client
	ChatEndpoint string
}

func NewClient() *Client {
	return &Client{
		HTTPClient:   &http.Client{Timeout: 60 * time.Second},
		ChatEndpoint: chatEndpointFromEnv(),
	}
}

func chatEndpointFromEnv() string {
	if endpoint := strings.TrimSpace(os.Getenv("RIV_OLLAMA_CHAT_ENDPOINT")); endpoint != "" {
		return endpoint
	}
	host := strings.TrimRight(strings.TrimSpace(os.Getenv("OLLAMA_HOST")), "/")
	if host == "" {
		return DefaultChatEndpoint
	}
	if !strings.HasPrefix(host, "http://") && !strings.HasPrefix(host, "https://") {
		host = "http://" + host
	}
	return host + "/api/chat"
}

func (c *Client) Chat(ctx context.Context, model string, messages []ChatMessage) (ChatCompletion, error) {
	if c == nil {
		c = NewClient()
	}
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	endpoint := c.ChatEndpoint
	if endpoint == "" {
		endpoint = chatEndpointFromEnv()
	}
	if model == "" {
		model = DefaultModel
	}

	body, err := json.Marshal(map[string]any{
		"model":    model,
		"messages": messages,
		"stream":   false,
	})
	if err != nil {
		return ChatCompletion{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return ChatCompletion{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return ChatCompletion{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		var parsed struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(raw, &parsed); err == nil && parsed.Error != "" {
			return ChatCompletion{}, fmt.Errorf("ollama error: %s", parsed.Error)
		}
		return ChatCompletion{}, fmt.Errorf("ollama error: status %s", resp.Status)
	}

	var parsed struct {
		Model           string      `json:"model"`
		Message         ChatMessage `json:"message"`
		PromptEvalCount int         `json:"prompt_eval_count"`
		EvalCount       int         `json:"eval_count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return ChatCompletion{}, err
	}
	return ChatCompletion{
		Model:   parsed.Model,
		Message: parsed.Message,
		Reply:   parsed.Message.Content,
		Usage: TokenUsage{
			PromptEvalCount: parsed.PromptEvalCount,
			EvalCount:       parsed.EvalCount,
		},
	}, nil
}
