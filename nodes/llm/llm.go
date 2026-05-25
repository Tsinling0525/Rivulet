package llm

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
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
	Provider           string         `json:"provider"`
	Model              string         `json:"model"`
	Endpoint           string         `json:"endpoint,omitempty"`
	PromptHash         string         `json:"prompt_hash"`
	PromptTemplateHash string         `json:"prompt_template_hash,omitempty"`
	PromptPreview      string         `json:"prompt_preview,omitempty"`
	OutputPreview      string         `json:"output_preview,omitempty"`
	InputTokens        int            `json:"input_tokens,omitempty"`
	OutputTokens       int            `json:"output_tokens,omitempty"`
	TotalTokens        int            `json:"total_tokens,omitempty"`
	LatencyMS          int64          `json:"latency_ms,omitempty"`
	Status             string         `json:"status"`
	Error              string         `json:"error,omitempty"`
	HumanReview        bool           `json:"human_review_required,omitempty"`
	Extra              map[string]any `json:"extra,omitempty"`
}

// AIReasoningStep captures model-visible reasoning progress. It is intended for
// provider-supplied summaries or explicit reasoning text, not hidden model state.
type AIReasoningStep struct {
	Provider  string
	Model     string
	Endpoint  string
	Index     int
	Title     string
	Text      string
	Source    string
	LatencyMS int64
	DeltaMS   int64
	Status    string
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
	if call.PromptTemplateHash != "" {
		fields["prompt_template_hash"] = call.PromptTemplateHash
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

func (b *LLMNodeBase) EmitAIReasoningStep(ctx context.Context, execID string, wf model.Workflow, node model.Node, step AIReasoningStep) {
	if b.deps.Bus == nil {
		return
	}
	fields := map[string]any{
		"exec":          execID,
		"workflow":      wf.ID,
		"workflow_kind": wf.Kind,
		"node":          node.ID,
		"provider":      step.Provider,
		"model":         step.Model,
		"endpoint":      step.Endpoint,
		"step_index":    step.Index,
		"title":         step.Title,
		"text":          step.Text,
		"source":        step.Source,
		"latency_ms":    step.LatencyMS,
		"delta_ms":      step.DeltaMS,
		"status":        step.Status,
	}
	_ = b.deps.Bus.Emit(ctx, "ai_reasoning_step", fields)
}

func (b *LLMNodeBase) EmitAIReasoningSteps(ctx context.Context, execID string, wf model.Workflow, node model.Node, steps []AIReasoningStep) {
	for _, step := range steps {
		b.EmitAIReasoningStep(ctx, execID, wf, node, step)
	}
}

func ReasoningStepsFromText(provider, model, endpoint, source, titlePrefix, text string, startIndex int, totalLatency time.Duration) []AIReasoningStep {
	parts := SplitReasoningText(text)
	if len(parts) == 0 {
		return nil
	}
	steps := make([]AIReasoningStep, 0, len(parts))
	totalMS := totalLatency.Milliseconds()
	for idx, part := range parts {
		stepIndex := startIndex + idx
		latencyMS := totalMS
		if totalMS > 0 {
			latencyMS = int64(float64(totalMS) * (float64(idx+1) / float64(len(parts)+1)))
		}
		deltaMS := latencyMS
		if idx > 0 {
			deltaMS -= steps[idx-1].LatencyMS
		}
		steps = append(steps, AIReasoningStep{
			Provider:  provider,
			Model:     model,
			Endpoint:  endpoint,
			Index:     stepIndex,
			Title:     fmt.Sprintf("%s %d", titlePrefix, idx+1),
			Text:      part,
			Source:    source,
			LatencyMS: latencyMS,
			DeltaMS:   deltaMS,
			Status:    "streamed",
		})
	}
	return steps
}

func SplitReasoningText(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	paragraphs := regexp.MustCompile(`\n\s*\n+`).Split(text, -1)
	if len(paragraphs) == 1 {
		lines := strings.Split(text, "\n")
		parts := make([]string, 0, len(lines))
		for _, line := range lines {
			line = strings.TrimSpace(strings.TrimLeft(line, "-*0123456789. )"))
			if line != "" {
				parts = append(parts, line)
			}
		}
		if len(parts) > 1 {
			return limitReasoningSteps(parts)
		}
	}

	parts := make([]string, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph != "" {
			parts = append(parts, paragraph)
		}
	}
	return limitReasoningSteps(parts)
}

func ExtractThinkBlocks(text string) string {
	matches := regexp.MustCompile(`(?is)<think>\s*(.*?)\s*</think>`).FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return ""
	}
	parts := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) > 1 && strings.TrimSpace(match[1]) != "" {
			parts = append(parts, strings.TrimSpace(match[1]))
		}
	}
	return strings.Join(parts, "\n\n")
}

func limitReasoningSteps(parts []string) []string {
	const maxSteps = 12
	const maxChars = 420
	if len(parts) > maxSteps {
		parts = parts[:maxSteps]
	}
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) > maxChars {
			part = part[:maxChars] + "..."
		}
		out = append(out, part)
	}
	return out
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
