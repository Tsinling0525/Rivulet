package nodes

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"

	"github.com/Tsinling0525/rivulet/dsl"
	"github.com/Tsinling0525/rivulet/expr"
	"github.com/Tsinling0525/rivulet/llmclient"
	"gopkg.in/yaml.v3"
)

func nodeYAML(t *testing.T, document string) dsl.Node {
	t.Helper()
	var node dsl.Node
	if err := yaml.Unmarshal([]byte(document), &node); err != nil {
		t.Fatalf("parse node: %v", err)
	}
	return node
}

func request(node dsl.Node, pool expr.Pool, inputs map[string]any) Request {
	return Request{Node: node, Pool: pool, Inputs: inputs, WorkDir: ""}
}

func TestRegistryCoversEveryDocumentedType(t *testing.T) {
	registry := Registry()
	want := []string{
		dsl.NodeStart, dsl.NodeEnd, dsl.NodeAnswer, dsl.NodeLLM, dsl.NodeCode,
		dsl.NodeIfElse, dsl.NodeTemplate, dsl.NodeHTTP, dsl.NodeAggregate,
	}
	for _, nodeType := range want {
		if _, ok := registry[nodeType]; !ok {
			t.Fatalf("registry is missing node type %q", nodeType)
		}
	}
	if len(registry) != len(want) {
		t.Fatalf("registry has %d handlers, want %d", len(registry), len(want))
	}
	types := Types()
	if len(types) != len(want) {
		t.Fatalf("Types() = %v, want %d entries", types, len(want))
	}
	for i := 1; i < len(types); i++ {
		if types[i-1] >= types[i] {
			t.Fatalf("Types() is not sorted: %v", types)
		}
	}
	for _, unsupported := range []string{"iteration", "knowledge-retrieval", "agent", "tool"} {
		if _, ok := dsl.Unsupported[unsupported]; !ok {
			t.Fatalf("dsl.Unsupported is missing %q", unsupported)
		}
	}
}

func TestStartNode(t *testing.T) {
	node := nodeYAML(t, `
id: start_node
data:
  type: start
  variables:
    - variable: name
      type: text-input
      required: true
    - variable: tone
      type: text-input
`)
	start := Start{}

	if problems := start.Validate(node); len(problems) != 0 {
		t.Fatalf("validate: %v", problems)
	}

	output, err := start.Run(context.Background(), request(node, expr.NewPool(), map[string]any{"name": "Ada"}))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if output.Fields["name"] != "Ada" {
		t.Fatalf("fields = %#v", output.Fields)
	}

	if _, err := start.Run(context.Background(), request(node, expr.NewPool(), nil)); err == nil {
		t.Fatal("expected an error for the missing required input")
	} else if !strings.Contains(err.Error(), "missing required input(s): name") {
		t.Fatalf("unexpected error: %v", err)
	}

	output, err = start.Run(context.Background(), request(node, expr.NewPool(), map[string]any{"name": "Ada", "extra": "kept"}))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if output.Fields["extra"] != "kept" {
		t.Fatalf("undeclared input should pass through: %#v", output.Fields)
	}
}

func TestEndNodeResolvesOutputs(t *testing.T) {
	node := nodeYAML(t, `
id: end_node
data:
  type: end
  outputs:
    - variable: greeting
      value_selector: [template_node, output]
      value_type: string
    - variable: missing
      value_selector: [ghost_node, output]
`)
	pool := expr.NewPool()
	pool.Set("template_node", map[string]any{"output": "hi"})

	output, err := (End{}).Run(context.Background(), request(node, pool, nil))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if output.Fields["greeting"] != "hi" {
		t.Fatalf("greeting = %#v", output.Fields["greeting"])
	}
	if value, ok := output.Fields["missing"]; !ok || value != nil {
		t.Fatalf("missing output should be present as nil, got %#v (present=%v)", value, ok)
	}
}

func TestTemplateNode(t *testing.T) {
	node := nodeYAML(t, `
id: template_node
data:
  type: template-transform
  template: "Hello {{ arg1 }} ({{#start_node.name#}})"
  variables:
    - variable: arg1
      value_selector: [start_node, name]
`)
	pool := expr.NewPool()
	pool.Set("start_node", map[string]any{"name": "Ada"})

	if problems := (Template{}).Validate(node); len(problems) != 0 {
		t.Fatalf("validate: %v", problems)
	}
	output, err := (Template{}).Run(context.Background(), request(node, pool, nil))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if output.Fields["output"] != "Hello Ada (Ada)" {
		t.Fatalf("output = %#v", output.Fields["output"])
	}

	empty := nodeYAML(t, "id: t\ndata:\n  type: template-transform\n")
	if problems := (Template{}).Validate(empty); len(problems) == 0 {
		t.Fatal("expected an empty template to be rejected")
	}
}

func TestVariableAggregatorPicksAvailableBranch(t *testing.T) {
	node := nodeYAML(t, `
id: aggregate_node
data:
  type: variable-aggregator
  output_type: string
  variables:
    - [template_a, output]
    - [template_b, output]
`)
	pool := expr.NewPool()
	pool.Set("template_b", map[string]any{"output": "from b"})

	if problems := (VariableAggregator{}).Validate(node); len(problems) != 0 {
		t.Fatalf("validate: %v", problems)
	}
	output, err := (VariableAggregator{}).Run(context.Background(), request(node, pool, nil))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if output.Fields["output"] != "from b" {
		t.Fatalf("output = %#v", output.Fields["output"])
	}

	if _, err := (VariableAggregator{}).Run(context.Background(), request(node, expr.NewPool(), nil)); err == nil {
		t.Fatal("expected an error when no branch produced a value")
	}
}

func TestIfElseOperators(t *testing.T) {
	tests := []struct {
		name     string
		operator string
		value    any
		expected any
		want     bool
		wantErr  bool
	}{
		{name: "contains", operator: "contains", expected: "ell", want: true},
		{name: "not contains", operator: "not contains", expected: "xyz", want: true},
		{name: "start with", operator: "start with", expected: "he", want: true},
		{name: "end with", operator: "end with", expected: "lo", want: true},
		{name: "is", operator: "is", expected: "hello", want: true},
		{name: "is not", operator: "is not", expected: "hello", want: false},
		{name: "empty on value", operator: "empty", expected: nil, want: false},
		{name: "not empty", operator: "not empty", expected: nil, want: true},
		{name: "null", operator: "null", expected: nil, want: false},
		{name: "not null", operator: "not null", expected: nil, want: true},
		{name: "in list", operator: "in", expected: []any{"a", "hello"}, want: true},
		{name: "not in list", operator: "not in", expected: []any{"a", "b"}, want: true},
		{name: "unknown operator", operator: "matches", expected: "x", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := compare("hello", tc.operator, tc.expected)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("compare: %v", err)
			}
			if got != tc.want {
				t.Fatalf("compare(%q) = %v, want %v", tc.operator, got, tc.want)
			}
		})
	}
}

func TestIfElseNumericComparison(t *testing.T) {
	node := nodeYAML(t, `
id: cond
data:
  type: if-else
  cases:
    - case_id: "yes"
      logical_operator: and
      conditions:
        - variable_selector: [start_node, count]
          comparison_operator: ">"
          value: 5
          varType: number
    - case_id: "big"
      conditions:
        - variable_selector: [start_node, count]
          comparison_operator: "="
          value: 10
`)
	pool := expr.NewPool()
	pool.Set("start_node", map[string]any{"count": 10})

	output, err := (IfElse{}).Run(context.Background(), request(node, pool, nil))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if output.Branch != "yes" {
		t.Fatalf("branch = %q, want the first matching case", output.Branch)
	}

	pool.Set("start_node", map[string]any{"count": 1})
	output, err = (IfElse{}).Run(context.Background(), request(node, pool, nil))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if output.Branch != dsl.HandleFalse {
		t.Fatalf("branch = %q, want false", output.Branch)
	}
}

func TestIfElseLogicalOr(t *testing.T) {
	node := nodeYAML(t, `
id: cond
data:
  type: if-else
  cases:
    - case_id: "either"
      logical_operator: or
      conditions:
        - variable_selector: [start_node, a]
          comparison_operator: is
          value: nope
        - variable_selector: [start_node, b]
          comparison_operator: is
          value: yep
`)
	pool := expr.NewPool()
	pool.Set("start_node", map[string]any{"a": "x", "b": "yep"})

	output, err := (IfElse{}).Run(context.Background(), request(node, pool, nil))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if output.Branch != "either" {
		t.Fatalf("branch = %q, want either", output.Branch)
	}
}

func TestIfElseValidation(t *testing.T) {
	tests := []struct {
		name     string
		document string
		want     string
	}{
		{
			name: "no cases",
			document: `
id: cond
data:
  type: if-else
`,
			want: "at least one case",
		},
		{
			name: "unknown operator",
			document: `
id: cond
data:
  type: if-else
  cases:
    - case_id: "a"
      conditions:
        - variable_selector: [s, v]
          comparison_operator: matches
          value: x
`,
			want: "unknown comparison_operator",
		},
		{
			name: "missing selector",
			document: `
id: cond
data:
  type: if-else
  cases:
    - case_id: "a"
      conditions:
        - comparison_operator: is
          value: x
`,
			want: "no variable_selector",
		},
		{
			name: "bad logical operator",
			document: `
id: cond
data:
  type: if-else
  cases:
    - case_id: "a"
      logical_operator: xor
      conditions:
        - variable_selector: [s, v]
          comparison_operator: is
          value: x
`,
			want: "logical_operator",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			problems := (IfElse{}).Validate(nodeYAML(t, tc.document))
			if len(problems) == 0 {
				t.Fatal("expected validation problems")
			}
			if !strings.Contains(fmt.Sprint(problems), tc.want) {
				t.Fatalf("problems = %v, want containing %q", problems, tc.want)
			}
		})
	}
}

func TestHTTPRequestNode(t *testing.T) {
	var seen struct {
		method, contentType, authorization, query, body string
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		seen.method = r.Method
		seen.contentType = r.Header.Get("Content-Type")
		seen.authorization = r.Header.Get("Authorization")
		seen.query = r.URL.RawQuery
		seen.body = string(raw)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"echo":{"name":"Ada"},"code":200}`)
	}))
	defer server.Close()

	node := nodeYAML(t, fmt.Sprintf(`
id: http_node
data:
  type: http-request
  method: POST
  url: "%s/echo"
  params: "limit=2"
  headers: "X-Trace: {{#start_node.trace#}}"
  authorization:
    type: api-key
    config:
      type: bearer
      api_key: "{{#start_node.api_key#}}"
  body:
    type: json
    data:
      name: "{{#start_node.name#}}"
      count: 3
`, server.URL))
	pool := expr.NewPool()
	pool.Set("start_node", map[string]any{"name": "Ada", "trace": "abc", "api_key": "sk-test"})

	if problems := (HTTPRequest{}).Validate(node); len(problems) != 0 {
		t.Fatalf("validate: %v", problems)
	}
	output, err := (HTTPRequest{}).Run(context.Background(), request(node, pool, nil))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if seen.method != http.MethodPost || seen.contentType != "application/json" {
		t.Fatalf("request = %+v", seen)
	}
	if seen.authorization != "Bearer sk-test" {
		t.Fatalf("authorization = %q", seen.authorization)
	}
	if seen.query != "limit=2" {
		t.Fatalf("query = %q", seen.query)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(seen.body), &body); err != nil {
		t.Fatalf("body %q is not JSON: %v", seen.body, err)
	}
	if body["name"] != "Ada" || body["count"] != float64(3) {
		t.Fatalf("body = %#v", body)
	}
	if output.Fields["status_code"] != 200 {
		t.Fatalf("status_code = %#v", output.Fields["status_code"])
	}
	decoded, ok := output.Fields["body"].(map[string]any)
	if !ok || decoded["code"] != float64(200) {
		t.Fatalf("decoded body = %#v", output.Fields["body"])
	}
}

func TestHTTPRequestNodeFailsOnErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadGateway)
	}))
	defer server.Close()

	node := nodeYAML(t, fmt.Sprintf("id: h\ndata:\n  type: http-request\n  url: %q\n", server.URL))
	_, err := (HTTPRequest{}).Run(context.Background(), request(node, expr.NewPool(), nil))
	if err == nil {
		t.Fatal("expected an error for a 502 response")
	}
	if !strings.Contains(err.Error(), "502") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHTTPRequestNodeFormsAndCustomAuth(t *testing.T) {
	var body, authorization, contentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		authorization = r.Header.Get("X-Api-Key")
		contentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	node := nodeYAML(t, fmt.Sprintf(`
id: h
data:
  type: http-request
  method: POST
  url: "%s/form"
  authorization:
    type: custom
    config:
      header: X-Api-Key
      value: "static-key"
  body:
    type: x-www-form-urlencoded
    data:
      - key: name
        value: "Ada Lovelace"
`, server.URL))

	if _, err := (HTTPRequest{}).Run(context.Background(), request(node, expr.NewPool(), nil)); err != nil {
		t.Fatalf("run: %v", err)
	}
	if contentType != "application/x-www-form-urlencoded" {
		t.Fatalf("content type = %q", contentType)
	}
	if body != "name=Ada+Lovelace" {
		t.Fatalf("body = %q", body)
	}
	if authorization != "static-key" {
		t.Fatalf("authorization = %q", authorization)
	}
}

func TestHTTPRequestNodeValidation(t *testing.T) {
	tests := []struct {
		name     string
		document string
		want     string
	}{
		{name: "no url", document: "id: h\ndata:\n  type: http-request\n", want: "url must not be empty"},
		{name: "bad method", document: "id: h\ndata:\n  type: http-request\n  url: x\n  method: FETCH\n", want: "method"},
		{name: "bad body type", document: "id: h\ndata:\n  type: http-request\n  url: x\n  body: {type: xml}\n", want: "body.type"},
		{name: "bad auth type", document: "id: h\ndata:\n  type: http-request\n  url: x\n  authorization: {type: oauth2}\n", want: "authorization.type"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			problems := (HTTPRequest{}).Validate(nodeYAML(t, tc.document))
			if len(problems) == 0 || !strings.Contains(fmt.Sprint(problems), tc.want) {
				t.Fatalf("problems = %v, want containing %q", problems, tc.want)
			}
		})
	}
}

func TestCodeNodeRunsPython(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is not installed")
	}
	node := nodeYAML(t, `
id: code_node
data:
  type: code
  code_language: python3
  code: |
    def main(text, count):
        return {"output": text.upper() * count, "length": len(text)}
  variables:
    - variable: text
      value_selector: [start_node, text]
    - variable: count
      value_selector: [start_node, count]
  outputs:
    output: {type: string}
    length: {type: number}
`)
	pool := expr.NewPool()
	pool.Set("start_node", map[string]any{"text": "ab", "count": 2})

	if problems := (Code{}).Validate(node); len(problems) != 0 {
		t.Fatalf("validate: %v", problems)
	}
	output, err := (Code{}).Run(context.Background(), request(node, pool, nil))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if output.Fields["output"] != "ABAB" {
		t.Fatalf("output = %#v", output.Fields["output"])
	}
	if output.Fields["length"] != float64(2) {
		t.Fatalf("length = %#v", output.Fields["length"])
	}
}

func TestCodeNodeReportsPythonFailure(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is not installed")
	}
	node := nodeYAML(t, `
id: code_node
data:
  type: code
  code: |
    def main():
        raise ValueError("boom")
`)
	_, err := (Code{}).Run(context.Background(), request(node, expr.NewPool(), nil))
	if err == nil {
		t.Fatal("expected the python failure to surface")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCodeNodeValidation(t *testing.T) {
	tests := []struct {
		name     string
		document string
		want     string
	}{
		{name: "empty code", document: "id: c\ndata:\n  type: code\n", want: "code must not be empty"},
		{name: "javascript", document: "id: c\ndata:\n  type: code\n  code: x\n  code_language: javascript\n", want: "not implemented"},
		{name: "unnamed variable", document: "id: c\ndata:\n  type: code\n  code: x\n  variables: [{value_selector: [s, v]}]\n", want: "has no name"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			problems := (Code{}).Validate(nodeYAML(t, tc.document))
			if len(problems) == 0 || !strings.Contains(fmt.Sprint(problems), tc.want) {
				t.Fatalf("problems = %v, want containing %q", problems, tc.want)
			}
		})
	}
}

func TestLLMNodeWarnings(t *testing.T) {
	node := nodeYAML(t, `
id: l
data:
  type: llm
  model: {provider: langgenius/openai/openai, name: gpt-4o-mini}
  prompt_template: [{role: user, text: "hi"}]
  vision: {enabled: true}
  context: {enabled: true}
  structured_output: {enabled: true}
`)
	problems := (LLM{}).Validate(node)
	if dsl.HasErrors(problems) {
		t.Fatalf("expected warnings only, got %v", problems)
	}
	joined := fmt.Sprint(problems)
	for _, want := range []string{"vision is not implemented", "context", "structured_output"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("problems = %v, want containing %q", problems, want)
		}
	}
}

func TestLLMNodeValidationErrors(t *testing.T) {
	tests := []struct {
		name     string
		document string
		want     string
	}{
		{
			name:     "no prompt",
			document: "id: l\ndata:\n  type: llm\n  model: {name: m}\n",
			want:     "prompt_template must contain at least one message",
		},
		{
			name:     "bad role",
			document: "id: l\ndata:\n  type: llm\n  model: {name: m}\n  prompt_template: [{role: tool, text: x}]\n",
			want:     "unsupported role",
		},
		{
			name:     "no model name warns",
			document: "id: l\ndata:\n  type: llm\n  prompt_template: [{role: user, text: x}]\n",
			want:     "model.name is empty",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			problems := (LLM{}).Validate(nodeYAML(t, tc.document))
			if len(problems) == 0 || !strings.Contains(fmt.Sprint(problems), tc.want) {
				t.Fatalf("problems = %v, want containing %q", problems, tc.want)
			}
		})
	}
}

func TestLLMNodeCallsChatCompletions(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "test-key")
	var gotModel, gotAuth string
	var gotMessages []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var payload map[string]any
		_ = json.Unmarshal(raw, &payload)
		gotModel, _ = payload["model"].(string)
		gotAuth = r.Header.Get("Authorization")
		if messages, ok := payload["messages"].([]any); ok {
			for _, message := range messages {
				gotMessages = append(gotMessages, message.(map[string]any))
			}
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"content":"hello from model"}}],"usage":{"total_tokens":12}}`)
	}))
	defer server.Close()

	node := nodeYAML(t, `
id: llm_node
data:
  type: llm
  model:
    provider: deepseek
    name: deepseek-v4-flash
    completion_params:
      temperature: 0.2
      top_p: 0.9
  prompt_template:
    - role: system
      text: "You are {{#start_node.persona#}}"
    - role: user
      text: "Say hi to {{#start_node.name#}}"
`)
	pool := expr.NewPool()
	pool.Set("start_node", map[string]any{"persona": "terse", "name": "Ada"})

	req := request(node, pool, nil)
	// Point the node at the stub endpoint; the model name still comes from the DSL.
	req.Model = &llmclient.Config{Endpoint: server.URL + "/chat/completions", APIKey: "test-key"}

	output, err := (LLM{}).Run(context.Background(), req)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if output.Fields["text"] != "hello from model" {
		t.Fatalf("text = %#v", output.Fields["text"])
	}
	if gotModel != "deepseek-v4-flash" {
		t.Fatalf("model = %q", gotModel)
	}
	if !strings.HasPrefix(gotAuth, "Bearer ") {
		t.Fatalf("authorization = %q", gotAuth)
	}
	if len(gotMessages) != 2 || gotMessages[0]["role"] != "system" || gotMessages[0]["content"] != "You are terse" {
		t.Fatalf("messages = %#v", gotMessages)
	}
	if gotMessages[1]["content"] != "Say hi to Ada" {
		t.Fatalf("user message = %#v", gotMessages[1])
	}
}

func TestLLMNodeRequiresKeyAndModel(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	node := nodeYAML(t, `
id: l
data:
  type: llm
  model: {provider: openai, name: gpt-4o-mini}
  prompt_template: [{role: user, text: "hi"}]
`)
	_, err := (LLM{}).Run(context.Background(), request(node, expr.NewPool(), nil))
	if err == nil || !strings.Contains(err.Error(), "no API key") {
		t.Fatalf("unexpected error: %v", err)
	}

	noModel := nodeYAML(t, "id: l\ndata:\n  type: llm\n  prompt_template: [{role: user, text: hi}]\n")
	if _, err := (LLM{}).Run(context.Background(), request(noModel, expr.NewPool(), nil)); err == nil {
		t.Fatal("expected an error when no model is configured")
	}
}

func TestLLMNodeUnknownProvider(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "k")
	node := nodeYAML(t, `
id: l
data:
  type: llm
  model: {provider: anthropic, name: claude}
  prompt_template: [{role: user, text: "hi"}]
`)
	if _, err := (LLM{}).Run(context.Background(), request(node, expr.NewPool(), nil)); err == nil ||
		!strings.Contains(err.Error(), "not supported") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLLMNodeOverrideWins(t *testing.T) {
	var gotModel string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var payload map[string]any
		_ = json.Unmarshal(raw, &payload)
		gotModel, _ = payload["model"].(string)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer server.Close()

	node := nodeYAML(t, `
id: l
data:
  type: llm
  model: {provider: openai, name: from-dsl}
  prompt_template: [{role: user, text: "hi"}]
`)
	req := request(node, expr.NewPool(), nil)
	req.Model = &llmclient.Config{Endpoint: server.URL + "/chat/completions", APIKey: "override-key", Model: "from-cli"}

	if _, err := (LLM{}).Run(context.Background(), req); err != nil {
		t.Fatalf("run: %v", err)
	}
	if gotModel != "from-cli" {
		t.Fatalf("model = %q, want the CLI override", gotModel)
	}
}

func TestProviderMapping(t *testing.T) {
	tests := map[string]string{
		"langgenius/openai/openai": "openai",
		"deepseek":                 "deepseek",
		"langgenius/ollama/ollama": "ollama",
		"":                         "",
		"anthropic":                "",
	}
	for provider, want := range tests {
		if got := providerOf(provider); got != want {
			t.Fatalf("providerOf(%q) = %q, want %q", provider, got, want)
		}
	}
	if endpoint, keyEnv, ok := ProviderDefaults("deepseek"); !ok || keyEnv != "DEEPSEEK_API_KEY" || endpoint == "" {
		t.Fatalf("ProviderDefaults(deepseek) = %q, %q, %v", endpoint, keyEnv, ok)
	}
	if _, _, ok := ProviderDefaults("anthropic"); ok {
		t.Fatal("expected anthropic to be unsupported")
	}
}
