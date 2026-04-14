package openai

import (
	"testing"

	"github.com/Tsinling0525/rivulet/model"
)

func TestBuildPayloadUsesResponsesAPIByDefault(t *testing.T) {
	node := model.Node{
		Config: map[string]any{
			"reasoning_effort": "low",
			"verbosity":        "low",
		},
	}

	n := &ChatGPTNode{}
	n.cfg.Model = "gpt-5-mini"
	n.cfg.Endpoint = "https://api.openai.com/v1/responses"
	n.cfg.MaxTokens = 200

	payload := n.buildPayload(node, "hello")

	if payload["model"] != "gpt-5-mini" {
		t.Fatalf("expected model to be gpt-5-mini, got %v", payload["model"])
	}
	if payload["input"] != "hello" {
		t.Fatalf("expected input to be hello, got %v", payload["input"])
	}
	if payload["max_output_tokens"] != 200 {
		t.Fatalf("expected max_output_tokens to be 200, got %v", payload["max_output_tokens"])
	}
	if _, ok := payload["temperature"]; ok {
		t.Fatalf("did not expect temperature for GPT-5 responses payload")
	}

	reasoning, ok := payload["reasoning"].(map[string]any)
	if !ok || reasoning["effort"] != "low" {
		t.Fatalf("expected reasoning.effort=low, got %v", payload["reasoning"])
	}

	text, ok := payload["text"].(map[string]any)
	if !ok || text["verbosity"] != "low" {
		t.Fatalf("expected text.verbosity=low, got %v", payload["text"])
	}
}

func TestBuildPayloadKeepsLegacyChatCompletionsCompatibility(t *testing.T) {
	node := model.Node{}

	n := &ChatGPTNode{}
	n.cfg.Model = "gpt-4.1"
	n.cfg.Endpoint = "https://api.openai.com/v1/chat/completions"
	n.cfg.MaxTokens = 128
	n.cfg.Temperature = 0.4

	payload := n.buildPayload(node, "hello")

	if payload["model"] != "gpt-4.1" {
		t.Fatalf("expected model to be gpt-4.1, got %v", payload["model"])
	}
	if payload["max_tokens"] != 128 {
		t.Fatalf("expected max_tokens to be 128, got %v", payload["max_tokens"])
	}
	if payload["temperature"] != 0.4 {
		t.Fatalf("expected temperature to be 0.4, got %v", payload["temperature"])
	}
	if _, ok := payload["input"]; ok {
		t.Fatalf("did not expect responses-style input field in legacy payload")
	}
}

func TestExtractOutputParsesResponsesOutputText(t *testing.T) {
	n := &ChatGPTNode{}
	body := []byte(`{"output_text":"rewritten text"}`)

	out, err := n.extractOutput("https://api.openai.com/v1/responses", body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "rewritten text" {
		t.Fatalf("expected rewritten text, got %q", out)
	}
}

func TestExtractOutputParsesLegacyChatCompletions(t *testing.T) {
	n := &ChatGPTNode{}
	body := []byte(`{"choices":[{"message":{"content":"legacy text"}}]}`)

	out, err := n.extractOutput("https://api.openai.com/v1/chat/completions", body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "legacy text" {
		t.Fatalf("expected legacy text, got %q", out)
	}
}
