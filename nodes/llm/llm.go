package llm

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"text/template"
	"time"

	"rivulet/model"
	"rivulet/plugin"
)

// LLMConfig holds common parameters for LLM providers
type LLMConfig struct {
	Model       string
	Prompt      string
	Temperature float64
	MaxTokens   int
	Endpoint    string
}

// LLMProvider is implemented by specific providers (Ollama, ChatGPT)
type LLMProvider interface {
	Generate(ctx context.Context, cfg LLMConfig, renderedPrompt string) (string, error)
}

// LLMNodeBase offers shared behavior for LLM nodes
type LLMNodeBase struct {
	deps plugin.Deps
}

func (b *LLMNodeBase) Init(ctx context.Context, deps plugin.Deps) error { b.deps = deps; return nil }

// RenderPrompt renders cfg.Prompt as Go template using the current item fields
func (b *LLMNodeBase) RenderPrompt(prompt string, item model.Item) (string, error) {
	if prompt == "" {
		return "", fmt.Errorf("prompt is required")
	}
	tpl, err := template.New("prompt").Parse(prompt)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, item); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// readEnvDefault reads env var or returns default
func readEnvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// httpClient returns a tuned http client
func httpClient(timeout time.Duration) *http.Client {
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	return &http.Client{Timeout: timeout}
}
