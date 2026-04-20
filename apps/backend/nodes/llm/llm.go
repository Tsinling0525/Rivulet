package llm

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"text/template"
	"time"

	"github.com/Tsinling0525/rivulet/model"
	"github.com/Tsinling0525/rivulet/plugin"
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

type AIModelCall struct {
	Provider      string         `json:"provider"`
	Model         string         `json:"model"`
	Endpoint      string         `json:"endpoint,omitempty"`
	PromptHash    string         `json:"prompt_hash"`
	PromptPreview string         `json:"prompt_preview,omitempty"`
	OutputPreview string         `json:"output_preview,omitempty"`
	InputTokens   int            `json:"input_tokens,omitempty"`
	OutputTokens  int            `json:"output_tokens,omitempty"`
	TotalTokens   int            `json:"total_tokens,omitempty"`
	LatencyMS     int64          `json:"latency_ms,omitempty"`
	Status        string         `json:"status"`
	Error         string         `json:"error,omitempty"`
	HumanReview   bool           `json:"human_review_required,omitempty"`
	Extra         map[string]any `json:"extra,omitempty"`
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

func (b *LLMNodeBase) EmitAIModelCall(ctx context.Context, execID string, wf model.Workflow, node model.Node, call AIModelCall) {
	if b.deps.Bus == nil {
		return
	}
	fields := map[string]any{
		"exec":          execID,
		"workflow":      wf.ID,
		"workflow_kind": wf.Kind,
		"node":          node.ID,
		"provider":      call.Provider,
		"model":         call.Model,
		"endpoint":      call.Endpoint,
		"prompt_hash":   call.PromptHash,
		"status":        call.Status,
		"latency_ms":    call.LatencyMS,
	}
	if call.PromptPreview != "" {
		fields["prompt_preview"] = call.PromptPreview
	}
	if call.OutputPreview != "" {
		fields["output_preview"] = call.OutputPreview
	}
	if call.InputTokens > 0 || call.OutputTokens > 0 || call.TotalTokens > 0 {
		fields["tokens"] = map[string]any{
			"input":  call.InputTokens,
			"output": call.OutputTokens,
			"total":  call.TotalTokens,
		}
	}
	if call.Error != "" {
		fields["error"] = call.Error
	}
	if call.HumanReview {
		fields["human_review_required"] = true
	}
	if len(call.Extra) > 0 {
		fields["extra"] = call.Extra
	}
	_ = b.deps.Bus.Emit(ctx, "ai_model_call", fields)
}

func PromptHash(prompt string) string {
	sum := sha256.Sum256([]byte(prompt))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func Preview(value string, max int) string {
	if max <= 0 || len(value) <= max {
		return value
	}
	return value[:max] + "..."
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
