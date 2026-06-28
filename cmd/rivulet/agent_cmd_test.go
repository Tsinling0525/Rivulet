package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	if client.MaxOutputTokens != 4096 {
		t.Fatalf("unexpected max output tokens: %d", client.MaxOutputTokens)
	}
	thinking, ok := client.ExtraFields["thinking"].(map[string]string)
	if !ok {
		t.Fatalf("unexpected thinking mode type: %#v", client.ExtraFields["thinking"])
	}
	if thinking["type"] != "disabled" {
		t.Fatalf("unexpected thinking mode: %#v", thinking)
	}
}

func TestNewAgentTextClientRejectsUnknownProvider(t *testing.T) {
	_, err := newAgentTextClient(agentCLIOptions{Provider: "unknown"})
	if err == nil {
		t.Fatalf("expected unsupported provider error")
	}
}

func TestPlannerRepairsInvalidJSON(t *testing.T) {
	client := &scriptedTextClient{responses: []string{
		`{"summary":"broken"`,
		`{"summary":"list files","tool_call":{"name":"list_files","args":{"path":"."}}}`,
	}}
	planner := jsonPlanner{Client: client, CWD: t.TempDir()}

	plan, err := planner.Plan(context.Background(), agent.State{Goal: "inspect"})
	if err != nil {
		t.Fatalf("planner failed: %v", err)
	}
	if client.calls != 2 {
		t.Fatalf("expected repair call, got %d calls", client.calls)
	}
	if plan.ToolCall.Name != "list_files" {
		t.Fatalf("unexpected tool: %q", plan.ToolCall.Name)
	}
}

func TestReflectorRepairsInvalidJSON(t *testing.T) {
	client := &scriptedTextClient{responses: []string{
		``,
		`{"summary":"continue","decision":"replan"}`,
	}}
	reflector := jsonReflector{Client: client}

	reflection, err := reflector.Reflect(context.Background(), agent.State{Goal: "inspect"}, agent.Observation{})
	if err != nil {
		t.Fatalf("reflector failed: %v", err)
	}
	if client.calls != 2 {
		t.Fatalf("expected repair call, got %d calls", client.calls)
	}
	if reflection.Decision != agent.DecisionReplan {
		t.Fatalf("unexpected decision: %q", reflection.Decision)
	}
}

func TestPlannerPromptRequiresGitDiffAfterMutations(t *testing.T) {
	prompt := plannerPrompt(t.TempDir(), agent.State{Goal: "edit files"})
	for _, want := range []string{
		"edit_file, replace_lines, or write_file",
		"git diff",
		"replace_lines",
		"mutating",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("expected planner prompt to contain %q", want)
		}
	}
}

func TestReflectorForcesGitDiffAfterMutation(t *testing.T) {
	client := &scriptedTextClient{responses: []string{
		`{"summary":"done","decision":"stop"}`,
	}}
	reflector := jsonReflector{Client: client}
	state := agent.State{
		Goal: "edit files",
		Steps: []agent.Step{{
			Plan: agent.Plan{ToolCall: agent.ToolCall{Name: "replace_lines"}},
		}},
	}

	reflection, err := reflector.Reflect(context.Background(), state, agent.Observation{})
	if err != nil {
		t.Fatalf("reflector failed: %v", err)
	}
	if reflection.Decision != agent.DecisionReplan {
		t.Fatalf("expected replan until git diff, got %q", reflection.Decision)
	}
	if !strings.Contains(reflection.Summary, "git diff") {
		t.Fatalf("expected git diff guidance, got %q", reflection.Summary)
	}
}

func TestReflectorAllowsStopAfterGitDiff(t *testing.T) {
	client := &scriptedTextClient{responses: []string{
		`{"summary":"done","decision":"stop"}`,
	}}
	reflector := jsonReflector{Client: client}
	state := agent.State{
		Goal: "edit files",
		Steps: []agent.Step{
			{Plan: agent.Plan{ToolCall: agent.ToolCall{Name: "write_file"}}},
			{Plan: agent.Plan{ToolCall: agent.ToolCall{Name: "shell", Args: map[string]any{"command": "git diff -- cmd/rivulet"}}}},
		},
	}

	reflection, err := reflector.Reflect(context.Background(), state, agent.Observation{})
	if err != nil {
		t.Fatalf("reflector failed: %v", err)
	}
	if reflection.Decision != agent.DecisionStop {
		t.Fatalf("expected stop after git diff, got %q", reflection.Decision)
	}
}

func TestReflectorAllowsStopAfterDryRunMutation(t *testing.T) {
	client := &scriptedTextClient{responses: []string{
		`{"summary":"dry run only","decision":"stop"}`,
	}}
	reflector := jsonReflector{Client: client}
	state := agent.State{
		Goal: "preview edit",
		Steps: []agent.Step{{
			Plan:        agent.Plan{ToolCall: agent.ToolCall{Name: "replace_lines"}},
			Observation: agent.Observation{Output: map[string]any{"dry_run": true}},
		}},
	}

	reflection, err := reflector.Reflect(context.Background(), state, agent.Observation{})
	if err != nil {
		t.Fatalf("reflector failed: %v", err)
	}
	if reflection.Decision != agent.DecisionStop {
		t.Fatalf("expected stop after dry-run mutation, got %q", reflection.Decision)
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

func TestReadFileToolCanReturnLineNumbers(t *testing.T) {
	root := t.TempDir()
	registry := newCodingToolRegistry(root, &strings.Builder{})
	tool, ok := registry.ResolveTool("write_file")
	if !ok {
		t.Fatalf("write_file tool missing")
	}
	_, err := tool.Execute(context.Background(), agent.ToolCall{
		Name: "write_file",
		Args: map[string]any{"path": "lines.txt", "content": "alpha\nbeta\ngamma\n"},
	})
	if err != nil {
		t.Fatalf("write_file failed: %v", err)
	}

	tool, ok = registry.ResolveTool("read_file")
	if !ok {
		t.Fatalf("read_file tool missing")
	}
	obs, err := tool.Execute(context.Background(), agent.ToolCall{
		Name: "read_file",
		Args: map[string]any{"path": "lines.txt", "line_numbers": true},
	})
	if err != nil {
		t.Fatalf("read_file failed: %v", err)
	}
	numbered, _ := obs.Output["numbered_content"].(string)
	if !strings.Contains(numbered, "     2\tbeta") {
		t.Fatalf("expected numbered content, got %q", numbered)
	}
}

func TestReplaceLinesToolReplacesRange(t *testing.T) {
	root := t.TempDir()
	registry := newCodingToolRegistry(root, &strings.Builder{})
	tool, ok := registry.ResolveTool("write_file")
	if !ok {
		t.Fatalf("write_file tool missing")
	}
	_, err := tool.Execute(context.Background(), agent.ToolCall{
		Name: "write_file",
		Args: map[string]any{"path": "lines.txt", "content": "one\ntwo\nthree\nfour\n"},
	})
	if err != nil {
		t.Fatalf("write_file failed: %v", err)
	}

	tool, ok = registry.ResolveTool("replace_lines")
	if !ok {
		t.Fatalf("replace_lines tool missing")
	}
	obs, err := tool.Execute(context.Background(), agent.ToolCall{
		Name: "replace_lines",
		Args: map[string]any{"path": "lines.txt", "start_line": 2, "end_line": 3, "content": "TWO\nTHREE"},
	})
	if err != nil {
		t.Fatalf("replace_lines failed: %v", err)
	}
	if obs.Output["removed_lines"] != 2 {
		t.Fatalf("unexpected removed lines: %#v", obs.Output["removed_lines"])
	}

	tool, ok = registry.ResolveTool("read_file")
	if !ok {
		t.Fatalf("read_file tool missing")
	}
	obs, err = tool.Execute(context.Background(), agent.ToolCall{
		Name: "read_file",
		Args: map[string]any{"path": "lines.txt"},
	})
	if err != nil {
		t.Fatalf("read_file failed: %v", err)
	}
	if obs.Output["content"] != "one\nTWO\nTHREE\nfour\n" {
		t.Fatalf("unexpected content: %#v", obs.Output["content"])
	}
}

func TestReplaceLinesToolCanInsertAndAppend(t *testing.T) {
	root := t.TempDir()
	registry := newCodingToolRegistry(root, &strings.Builder{})
	tool, ok := registry.ResolveTool("write_file")
	if !ok {
		t.Fatalf("write_file tool missing")
	}
	_, err := tool.Execute(context.Background(), agent.ToolCall{
		Name: "write_file",
		Args: map[string]any{"path": "lines.txt", "content": "one\nthree\n"},
	})
	if err != nil {
		t.Fatalf("write_file failed: %v", err)
	}

	tool, ok = registry.ResolveTool("replace_lines")
	if !ok {
		t.Fatalf("replace_lines tool missing")
	}
	_, err = tool.Execute(context.Background(), agent.ToolCall{
		Name: "replace_lines",
		Args: map[string]any{"path": "lines.txt", "start_line": 2, "end_line": 1, "content": "two"},
	})
	if err != nil {
		t.Fatalf("insert replace_lines failed: %v", err)
	}
	_, err = tool.Execute(context.Background(), agent.ToolCall{
		Name: "replace_lines",
		Args: map[string]any{"path": "lines.txt", "start_line": 4, "end_line": 3, "content": "four\n"},
	})
	if err != nil {
		t.Fatalf("append replace_lines failed: %v", err)
	}

	tool, ok = registry.ResolveTool("read_file")
	if !ok {
		t.Fatalf("read_file tool missing")
	}
	obs, err := tool.Execute(context.Background(), agent.ToolCall{
		Name: "read_file",
		Args: map[string]any{"path": "lines.txt"},
	})
	if err != nil {
		t.Fatalf("read_file failed: %v", err)
	}
	if obs.Output["content"] != "one\ntwo\nthree\nfour\n" {
		t.Fatalf("unexpected content: %#v", obs.Output["content"])
	}
}

func TestApproveNeverDoesNotReplaceLines(t *testing.T) {
	root := t.TempDir()
	registry := newCodingToolRegistry(root, &strings.Builder{}, approveAlways)
	tool, ok := registry.ResolveTool("write_file")
	if !ok {
		t.Fatalf("write_file tool missing")
	}
	_, err := tool.Execute(context.Background(), agent.ToolCall{
		Name: "write_file",
		Args: map[string]any{"path": "lines.txt", "content": "one\ntwo\n"},
	})
	if err != nil {
		t.Fatalf("write_file failed: %v", err)
	}

	registry = newCodingToolRegistry(root, &strings.Builder{}, approveNever)
	tool, ok = registry.ResolveTool("replace_lines")
	if !ok {
		t.Fatalf("replace_lines tool missing")
	}
	obs, err := tool.Execute(context.Background(), agent.ToolCall{
		Name: "replace_lines",
		Args: map[string]any{"path": "lines.txt", "start_line": 2, "end_line": 2, "content": "TWO"},
	})
	if err != nil {
		t.Fatalf("replace_lines dry run failed: %v", err)
	}
	if obs.Output["dry_run"] != true {
		t.Fatalf("expected dry_run output, got %#v", obs.Output)
	}

	tool, ok = registry.ResolveTool("read_file")
	if !ok {
		t.Fatalf("read_file tool missing")
	}
	obs, err = tool.Execute(context.Background(), agent.ToolCall{
		Name: "read_file",
		Args: map[string]any{"path": "lines.txt"},
	})
	if err != nil {
		t.Fatalf("read_file failed: %v", err)
	}
	if obs.Output["content"] != "one\ntwo\n" {
		t.Fatalf("unexpected dry-run content: %#v", obs.Output["content"])
	}
}

func TestApproveNeverDoesNotWriteFile(t *testing.T) {
	root := t.TempDir()
	registry := newCodingToolRegistry(root, &strings.Builder{}, approveNever)
	tool, ok := registry.ResolveTool("write_file")
	if !ok {
		t.Fatalf("write_file tool missing")
	}

	obs, err := tool.Execute(context.Background(), agent.ToolCall{
		Name: "write_file",
		Args: map[string]any{"path": "dry-run.txt", "content": "should not exist"},
	})
	if err != nil {
		t.Fatalf("write_file dry run failed: %v", err)
	}
	if obs.Output["dry_run"] != true {
		t.Fatalf("expected dry_run output, got %#v", obs.Output)
	}
	if _, err := os.Stat(filepath.Join(root, "dry-run.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected dry-run file not to exist, stat err=%v", err)
	}
}

func TestApproveNeverDoesNotRunShell(t *testing.T) {
	root := t.TempDir()
	registry := newCodingToolRegistry(root, &strings.Builder{}, approveNever)
	tool, ok := registry.ResolveTool("shell")
	if !ok {
		t.Fatalf("shell tool missing")
	}

	obs, err := tool.Execute(context.Background(), agent.ToolCall{
		Name: "shell",
		Args: map[string]any{"command": "touch should-not-exist"},
	})
	if err != nil {
		t.Fatalf("shell dry run failed: %v", err)
	}
	if obs.Output["dry_run"] != true {
		t.Fatalf("expected dry_run output, got %#v", obs.Output)
	}
	if _, err := os.Stat(filepath.Join(root, "should-not-exist")); !os.IsNotExist(err) {
		t.Fatalf("expected shell dry-run file not to exist, stat err=%v", err)
	}
}

func TestWriteAgentTraceCreatesJSONL(t *testing.T) {
	root := t.TempDir()
	result := agent.RunResult{
		Steps: []agent.Step{{
			Index: 1,
			Plan: agent.Plan{
				Summary: "inspect files",
				ToolCall: agent.ToolCall{
					Name: "shell",
					Args: map[string]any{
						"command":          "echo DEEPSEEK_API_KEY=secret-value",
						"DEEPSEEK_API_KEY": "secret-value",
					},
				},
			},
			Observation: agent.Observation{Summary: "ran command", Error: "token=secret-value"},
			Reflection:  agent.Reflection{Decision: agent.DecisionReplan},
		}},
	}

	relPath, err := writeAgentTrace(root, traceOn, result)
	if err != nil {
		t.Fatalf("write trace failed: %v", err)
	}
	if relPath == "" {
		t.Fatalf("expected trace path")
	}
	path := filepath.Join(root, relPath)
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open trace failed: %v", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		t.Fatalf("expected one trace line")
	}
	var record agentTraceRecord
	if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
		t.Fatalf("decode trace failed: %v", err)
	}
	if record.StepIndex != 1 || record.ToolName != "shell" {
		t.Fatalf("unexpected trace record: %#v", record)
	}
	if record.ToolArgs["DEEPSEEK_API_KEY"] != "<redacted>" {
		t.Fatalf("expected API key argument to be redacted: %#v", record.ToolArgs)
	}
	if strings.Contains(record.ToolArgs["command"].(string), "secret-value") {
		t.Fatalf("expected command assignment to be redacted: %#v", record.ToolArgs["command"])
	}
	if strings.Contains(record.ObservationError, "secret-value") {
		t.Fatalf("expected observation error to be redacted: %q", record.ObservationError)
	}
	if scanner.Scan() {
		t.Fatalf("expected only one trace line")
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan trace failed: %v", err)
	}
}

func TestWriteAgentTraceOffDoesNothing(t *testing.T) {
	root := t.TempDir()
	relPath, err := writeAgentTrace(root, traceOff, agent.RunResult{
		Steps: []agent.Step{{Index: 1}},
	})
	if err != nil {
		t.Fatalf("trace off failed: %v", err)
	}
	if relPath != "" {
		t.Fatalf("expected no trace path, got %q", relPath)
	}
	if _, err := os.Stat(filepath.Join(root, ".rivulet")); !os.IsNotExist(err) {
		t.Fatalf("expected trace directory not to exist, stat err=%v", err)
	}
}

type scriptedTextClient struct {
	responses []string
	calls     int
}

func (c *scriptedTextClient) Complete(ctx context.Context, prompt string) (string, error) {
	if c.calls >= len(c.responses) {
		return "", fmt.Errorf("unexpected model call %d", c.calls+1)
	}
	response := c.responses[c.calls]
	c.calls++
	return response, nil
}
