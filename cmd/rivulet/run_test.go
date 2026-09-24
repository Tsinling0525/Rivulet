package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tsinling0525/rivulet/dsl"
	"github.com/Tsinling0525/rivulet/workflow"
)

const examplesDir = "../../examples"

func TestExamplesValidateWithoutErrors(t *testing.T) {
	entries, err := filepath.Glob(filepath.Join(examplesDir, "*.dify.yml"))
	if err != nil {
		t.Fatalf("glob examples: %v", err)
	}
	if len(entries) < 4 {
		t.Fatalf("found %d examples, want at least 4", len(entries))
	}
	for _, path := range entries {
		t.Run(filepath.Base(path), func(t *testing.T) {
			app, err := dsl.Load(path)
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			if problems := workflow.Validate(app, nil); dsl.HasErrors(problems) {
				t.Fatalf("validation errors: %v", problems)
			}
		})
	}
}

func TestRunWorkflowCLIRunsExampleOffline(t *testing.T) {
	var out strings.Builder
	err := runWorkflowCLIWithIO([]string{
		"--file", filepath.Join(examplesDir, "hello.dify.yml"),
		"--input", "name=Ada",
	}, &out, strings.NewReader(""))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	output := out.String()
	for _, want := range []string{"Hello", "greeting: Hello Ada", "start", "template-transform", "end"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunWorkflowCLIBranchExample(t *testing.T) {
	tests := []struct {
		count string
		want  string
	}{
		{count: "9", want: "9 is greater than five"},
		{count: "2", want: "2 is five or less"},
	}
	for _, tc := range tests {
		t.Run(tc.count, func(t *testing.T) {
			var out strings.Builder
			err := runWorkflowCLIWithIO([]string{
				"--file", filepath.Join(examplesDir, "branch.dify.yml"),
				"--input", "count=" + tc.count,
			}, &out, strings.NewReader(""))
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			if !strings.Contains(out.String(), tc.want) {
				t.Fatalf("output missing %q:\n%s", tc.want, out.String())
			}
			if !strings.Contains(out.String(), "skipped") {
				t.Fatalf("expected the untaken branch to be reported as skipped:\n%s", out.String())
			}
		})
	}
}

func TestRunWorkflowCLIJSONAndTrace(t *testing.T) {
	tracePath := filepath.Join(t.TempDir(), "trace.json")
	var out strings.Builder
	err := runWorkflowCLIWithIO([]string{
		"--file", filepath.Join(examplesDir, "hello.dify.yml"),
		"--input", "name=Ada",
		"--json",
		"--trace", tracePath,
	}, &out, strings.NewReader(""))
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	var payload struct {
		App     string         `json:"app"`
		Mode    string         `json:"mode"`
		Outputs map[string]any `json:"outputs"`
		Steps   []struct {
			NodeID string `json:"NodeID"`
			Status string `json:"Status"`
		} `json:"steps"`
	}
	if err := json.Unmarshal([]byte(out.String()), &payload); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	if payload.App != "Hello" || payload.Mode != "workflow" {
		t.Fatalf("payload = %+v", payload)
	}
	if payload.Outputs["greeting"] != "Hello Ada, this workflow ran locally." {
		t.Fatalf("outputs = %#v", payload.Outputs)
	}
	if len(payload.Steps) != 3 {
		t.Fatalf("steps = %+v", payload.Steps)
	}

	raw, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatalf("read trace: %v", err)
	}
	var trace struct {
		Outputs map[string]any `json:"outputs"`
		Steps   []any          `json:"steps"`
	}
	if err := json.Unmarshal(raw, &trace); err != nil {
		t.Fatalf("trace is not JSON: %v", err)
	}
	if len(trace.Steps) != 3 || trace.Outputs["greeting"] == nil {
		t.Fatalf("trace = %+v", trace)
	}
}

func TestRunWorkflowCLIHTTPAndCodeExample(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":7,"name":"rivulet","nested":{"a":1}}`)
	}))
	defer server.Close()

	var out strings.Builder
	err := runWorkflowCLIWithIO([]string{
		"--file", filepath.Join(examplesDir, "http-and-code.dify.yml"),
		"--input", "url=" + server.URL + "/item",
	}, &out, strings.NewReader(""))
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out.String())
	}
	output := out.String()
	if !strings.Contains(output, "status: 200") {
		t.Fatalf("missing status output:\n%s", output)
	}
	if !strings.Contains(output, "payload_shape: id,name,nested") {
		t.Fatalf("missing payload shape:\n%s", output)
	}
}

func TestRunWorkflowCLIRejectsInvalidWorkflow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.dify.yml")
	document := `
kind: app
app: {name: Broken, mode: workflow}
workflow:
  graph:
    nodes:
      - {id: start_node, data: {type: start}}
    edges: []
`
	if err := os.WriteFile(path, []byte(document), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	var out strings.Builder
	err := runWorkflowCLIWithIO([]string{"--file", path}, &out, strings.NewReader(""))
	if err == nil || !strings.Contains(err.Error(), "not valid") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "at least one end node") {
		t.Fatalf("error should explain the problem: %v", err)
	}
}

func TestRunWorkflowCLIRequiresFileAndValidFlags(t *testing.T) {
	var out strings.Builder
	if err := runWorkflowCLIWithIO(nil, &out, strings.NewReader("")); err == nil {
		t.Fatal("expected --file to be required")
	}
	if err := runWorkflowCLIWithIO([]string{"--file", "x.yml", "--concurrency", "-1"}, &out, strings.NewReader("")); err == nil ||
		!strings.Contains(err.Error(), "--concurrency") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunWorkflowCLIReadsInputFileAndStdin(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inputs.json")
	if err := os.WriteFile(path, []byte(`{"name":"FromFile"}`), 0o644); err != nil {
		t.Fatalf("write inputs: %v", err)
	}
	var out strings.Builder
	err := runWorkflowCLIWithIO([]string{
		"--file", filepath.Join(examplesDir, "hello.dify.yml"),
		"--input-file", path,
	}, &out, strings.NewReader(""))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out.String(), "Hello FromFile") {
		t.Fatalf("output:\n%s", out.String())
	}

	out.Reset()
	err = runWorkflowCLIWithIO([]string{
		"--file", filepath.Join(examplesDir, "hello.dify.yml"),
		"--input-file", "-",
	}, &out, strings.NewReader(`{"name":"FromStdin"}`))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out.String(), "Hello FromStdin") {
		t.Fatalf("output:\n%s", out.String())
	}
}

func TestRunWorkflowCLIInlineInputWinsOverFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inputs.json")
	if err := os.WriteFile(path, []byte(`{"name":"FromFile"}`), 0o644); err != nil {
		t.Fatalf("write inputs: %v", err)
	}
	var out strings.Builder
	err := runWorkflowCLIWithIO([]string{
		"--file", filepath.Join(examplesDir, "hello.dify.yml"),
		"--input-file", path,
		"--input", "name=FromFlag",
	}, &out, strings.NewReader(""))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out.String(), "Hello FromFlag") {
		t.Fatalf("output:\n%s", out.String())
	}
}

func TestValidateWorkflowCLI(t *testing.T) {
	var out strings.Builder
	if err := validateWorkflowCLIWithIO([]string{"--file", filepath.Join(examplesDir, "hello.dify.yml")}, &out); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if !strings.Contains(out.String(), "ok: Hello") {
		t.Fatalf("output:\n%s", out.String())
	}

	if err := validateWorkflowCLIWithIO(nil, &out); err == nil {
		t.Fatal("expected --file to be required")
	}
}

func TestValidateWorkflowCLIReportsWarningsWithoutFailing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "warn.dify.yml")
	document := `
kind: app
app: {name: Warned, mode: workflow}
workflow:
  graph:
    nodes:
      - {id: start_node, data: {type: start}}
      - {id: end_node, data: {type: end}}
      - {id: orphan, data: {type: template-transform, template: "x"}}
    edges: [{source: start_node, target: end_node}]
`
	if err := os.WriteFile(path, []byte(document), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	var out strings.Builder
	if err := validateWorkflowCLIWithIO([]string{"--file", path}, &out); err != nil {
		t.Fatalf("warnings should not fail validation: %v", err)
	}
	if !strings.Contains(out.String(), "unreachable") {
		t.Fatalf("output:\n%s", out.String())
	}
}

func TestListNodesCLI(t *testing.T) {
	var out strings.Builder
	if err := listNodesCLIWithIO(nil, &out); err != nil {
		t.Fatalf("nodes: %v", err)
	}
	output := out.String()
	for _, want := range []string{"if-else", "llm", "http-request", "not implemented", "iteration"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
	if err := listNodesCLIWithIO([]string{"extra"}, &out); err == nil {
		t.Fatal("expected an error for unexpected arguments")
	}
}

func TestCoerceInputValue(t *testing.T) {
	tests := []struct {
		in   string
		want any
	}{
		{"7", float64(7)},
		{"true", true},
		{"false", false},
		{"null", nil},
		{`{"a":1}`, map[string]any{"a": float64(1)}},
		{"hello", "hello"},
		{"1.5", 1.5},
		{"nowhere", "nowhere"},
	}
	for _, tc := range tests {
		got := coerceInputValue(tc.in)
		if fmt.Sprint(got) != fmt.Sprint(tc.want) {
			t.Fatalf("coerceInputValue(%q) = %#v, want %#v", tc.in, got, tc.want)
		}
	}
}

func TestResolveInputsRejectsNonObjectFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(path, []byte(`[1,2]`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := resolveInputs(path, nil, strings.NewReader("")); err == nil {
		t.Fatal("expected an error for a JSON array input file")
	}
}

func TestBuildModelOverride(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "env-key")

	override, err := buildModelOverride(workflowCLIOptions{})
	if err != nil || override != nil {
		t.Fatalf("expected no override, got %+v err=%v", override, err)
	}

	override, err = buildModelOverride(workflowCLIOptions{Provider: "openai", Model: "gpt-4o-mini"})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if override.Endpoint != "https://api.openai.com/v1/chat/completions" || override.APIKey != "env-key" {
		t.Fatalf("override = %+v", override)
	}

	override, err = buildModelOverride(workflowCLIOptions{Provider: "ollama"})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if override.APIKey != "ollama" || !strings.Contains(override.Endpoint, "11434") {
		t.Fatalf("ollama override = %+v", override)
	}

	if _, err := buildModelOverride(workflowCLIOptions{Provider: "anthropic"}); err == nil {
		t.Fatal("expected an unsupported provider error")
	}

	override, err = buildModelOverride(workflowCLIOptions{Endpoint: "http://127.0.0.1:1/x", APIKey: "k"})
	if err != nil || override == nil || override.Endpoint != "http://127.0.0.1:1/x" {
		t.Fatalf("explicit endpoint override = %+v err=%v", override, err)
	}
}

func TestRunWorkflowCLIUsesModelOverrideForLLMExample(t *testing.T) {
	var gotModel string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var payload map[string]any
		_ = json.Unmarshal(body, &payload)
		gotModel, _ = payload["model"].(string)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"content":"stubbed answer"}}]}`)
	}))
	defer server.Close()

	var out strings.Builder
	err := runWorkflowCLIWithIO([]string{
		"--file", filepath.Join(examplesDir, "llm-chat.dify.yml"),
		"--input", "query=hello",
		"--endpoint", server.URL + "/chat/completions",
		"--model", "stub-model",
		"--api-key", "test-key",
	}, &out, strings.NewReader(""))
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out.String())
	}
	if gotModel != "stub-model" {
		t.Fatalf("model = %q, want the CLI override", gotModel)
	}
	if !strings.Contains(out.String(), "stubbed answer") {
		t.Fatalf("output:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "answers:") {
		t.Fatalf("chatflow answers should be printed:\n%s", out.String())
	}
}
