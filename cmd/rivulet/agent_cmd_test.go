package main

import (
	"context"
	"strings"
	"testing"

	"github.com/Tsinling0525/rivulet/agent"
)

func TestUnmarshalJSONResponseAcceptsFencedJSON(t *testing.T) {
	var parsed struct {
		Summary string `json:"summary"`
	}
	err := unmarshalJSONResponse("```json\n{\"summary\":\"ok\"}\n```", &parsed)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed.Summary != "ok" {
		t.Fatalf("unexpected summary: %q", parsed.Summary)
	}
}

func TestResolveWorkspacePathRejectsOutsidePath(t *testing.T) {
	root := t.TempDir()
	_, err := resolveWorkspacePath(root, "../outside.txt")
	if err == nil {
		t.Fatalf("expected outside path to be rejected")
	}
}

func TestNewAgentTextClientDeepSeekDefaults(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "test-key")

	client, err := newAgentTextClient(agentCLIOptions{Provider: "deepseek"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client.Model != "deepseek-v4-flash" {
		t.Fatalf("unexpected model: %q", client.Model)
	}
	if client.Endpoint != "https://api.deepseek.com/chat/completions" {
		t.Fatalf("unexpected endpoint: %q", client.Endpoint)
	}
	if client.ResponseFormat != "json_object" {
		t.Fatalf("unexpected response format: %q", client.ResponseFormat)
	}
}

func TestNewAgentTextClientRejectsUnknownProvider(t *testing.T) {
	_, err := newAgentTextClient(agentCLIOptions{Provider: "unknown"})
	if err == nil {
		t.Fatalf("expected unsupported provider error")
	}
}

func TestEditFileToolReplacesText(t *testing.T) {
	root := t.TempDir()
	registry := newCodingToolRegistry(root, &strings.Builder{})
	tool, ok := registry.ResolveTool("write_file")
	if !ok {
		t.Fatalf("write_file tool missing")
	}
	_, err := tool.Execute(context.Background(), agent.ToolCall{
		Name: "write_file",
		Args: map[string]any{"path": "hello.txt", "content": "hello world"},
	})
	if err != nil {
		t.Fatalf("write_file failed: %v", err)
	}

	tool, ok = registry.ResolveTool("edit_file")
	if !ok {
		t.Fatalf("edit_file tool missing")
	}
	obs, err := tool.Execute(context.Background(), agent.ToolCall{
		Name: "edit_file",
		Args: map[string]any{"path": "hello.txt", "old": "world", "new": "rivulet"},
	})
	if err != nil {
		t.Fatalf("edit_file failed: %v", err)
	}
	if obs.Output["replacements"] != 1 {
		t.Fatalf("unexpected replacements: %#v", obs.Output["replacements"])
	}

	tool, ok = registry.ResolveTool("read_file")
	if !ok {
		t.Fatalf("read_file tool missing")
	}
	obs, err = tool.Execute(context.Background(), agent.ToolCall{
		Name: "read_file",
		Args: map[string]any{"path": "hello.txt"},
	})
	if err != nil {
		t.Fatalf("read_file failed: %v", err)
	}
	if obs.Output["content"] != "hello rivulet" {
		t.Fatalf("unexpected content: %#v", obs.Output["content"])
	}
}
