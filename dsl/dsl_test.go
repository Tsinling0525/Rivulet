package dsl

import (
	"strings"
	"testing"
)

const validWorkflow = `
version: "0.6.0"
kind: app
app:
  name: Greeter
  mode: workflow
workflow:
  graph:
    nodes:
      - id: "start_node"
        type: custom
        data:
          type: start
          title: Start
          variables:
            - variable: name
              label: name
              type: text-input
              required: true
      - id: "template_node"
        type: custom
        data:
          type: template-transform
          title: Template
          template: "Hello {{ arg1 }}"
          variables:
            - variable: arg1
              value_selector: ["start_node", "name"]
      - id: "end_node"
        type: custom
        data:
          type: end
          title: End
          outputs:
            - variable: greeting
              value_selector: ["template_node", "output"]
              value_type: string
    edges:
      - id: "e1"
        source: "start_node"
        sourceHandle: "source"
        target: "template_node"
        targetHandle: "target"
      - id: "e2"
        source: "template_node"
        target: "end_node"
  features: {}
  environment_variables:
    - name: GREETING_PREFIX
      value: "hi"
      value_type: string
`

func mustParse(t *testing.T, document string) App {
	t.Helper()
	app, err := Parse([]byte(document))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return app
}

func TestParseValidWorkflow(t *testing.T) {
	app := mustParse(t, validWorkflow)

	if app.App.Name != "Greeter" || app.App.Mode != ModeWorkflow {
		t.Fatalf("unexpected app metadata: %+v", app.App)
	}
	if len(app.Workflow.Graph.Nodes) != 3 || len(app.Workflow.Graph.Edges) != 2 {
		t.Fatalf("unexpected graph size: %d nodes, %d edges",
			len(app.Workflow.Graph.Nodes), len(app.Workflow.Graph.Edges))
	}
	start, ok := app.StartNode()
	if !ok || start.ID != "start_node" {
		t.Fatalf("start node = %+v, ok=%v", start, ok)
	}
	if env := app.EnvironmentValues(); env["GREETING_PREFIX"] != "hi" {
		t.Fatalf("environment values = %#v", env)
	}
	if problems := Validate(app); HasErrors(problems) {
		t.Fatalf("expected valid workflow, got %v", problems)
	}
}

func TestParseAcceptsJSON(t *testing.T) {
	app, err := Parse([]byte(`{"version":"0.6.0","kind":"app",
		"app":{"name":"j","mode":"workflow"},
		"workflow":{"graph":{"nodes":[{"id":"s","data":{"type":"start"}},
			{"id":"e","data":{"type":"end"}}],
			"edges":[{"source":"s","target":"e"}]}}}`))
	if err != nil {
		t.Fatalf("parse json: %v", err)
	}
	if app.App.Name != "j" {
		t.Fatalf("unexpected name %q", app.App.Name)
	}
}

func TestParsePreservesNodeSpecificFields(t *testing.T) {
	app := mustParse(t, validWorkflow)
	node, ok := app.NodeByID("template_node")
	if !ok {
		t.Fatal("template node missing")
	}
	if node.Data.Type != NodeTemplate || node.Data.Title != "Template" {
		t.Fatalf("unexpected node data: %+v", node.Data)
	}
	selector, err := node.Data.SelectorField("nosuch")
	if err != nil || selector != nil {
		t.Fatalf("expected nil selector, got %v err=%v", selector, err)
	}
}

func TestNodeDataDecodeIntoStruct(t *testing.T) {
	app := mustParse(t, `
version: "0.6.0"
kind: app
app: {name: x, mode: workflow}
workflow:
  graph:
    nodes:
      - id: "http_node"
        data:
          type: http-request
          method: POST
          url: "https://example.com"
          authorization:
            type: api-key
            config:
              type: bearer
              api_key: "k"
      - id: "s"
        data: {type: start}
      - id: "e"
        data: {type: end}
    edges:
      - {source: s, target: http_node}
      - {source: http_node, target: e}
`)
	node, _ := app.NodeByID("http_node")
	var cfg struct {
		Method        string `yaml:"method"`
		URL           string `yaml:"url"`
		Authorization struct {
			Type   string `yaml:"type"`
			Config struct {
				APIKey string `yaml:"api_key"`
			} `yaml:"config"`
		} `yaml:"authorization"`
	}
	if err := node.Data.Decode(&cfg); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if cfg.Method != "POST" || cfg.URL != "https://example.com" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if cfg.Authorization.Type != "api-key" || cfg.Authorization.Config.APIKey != "k" {
		t.Fatalf("unexpected authorization: %+v", cfg.Authorization)
	}
}

func TestSelectorForms(t *testing.T) {
	selector, err := SelectorOf([]any{"node", "field", "nested"})
	if err != nil {
		t.Fatalf("selector: %v", err)
	}
	if selector.Node() != "node" || selector.Field() != "field.nested" || selector.String() != "node.field.nested" {
		t.Fatalf("unexpected selector %q", selector.String())
	}
	if (Selector{"only"}).Valid() {
		t.Fatal("single-element selector should be invalid")
	}
	if _, err := SelectorOf([]any{"node", 5}); err == nil {
		t.Fatal("expected an error for a non-string selector element")
	}
	if _, err := SelectorOf("node.field"); err == nil {
		t.Fatal("expected an error for a string selector")
	}
}

func TestEdgeHandleDefaultsToSource(t *testing.T) {
	edge := Edge{Source: "a", Target: "b"}
	if edge.Handle() != HandleSource {
		t.Fatalf("handle = %q, want source", edge.Handle())
	}
	edge.SourceHandle = HandleTrue
	if edge.Handle() != HandleTrue {
		t.Fatalf("handle = %q, want true", edge.Handle())
	}
}

func TestValidateReportsStructuralProblems(t *testing.T) {
	tests := []struct {
		name     string
		document string
		want     string
	}{
		{
			name: "kind must be app",
			document: `
kind: workflow
app: {mode: workflow}
workflow:
  graph:
    nodes: [{id: s, data: {type: start}}, {id: e, data: {type: end}}]
    edges: [{source: s, target: e}]
`,
			want: "kind must be",
		},
		{
			name: "mode must be supported",
			document: `
kind: app
app: {mode: chat}
workflow:
  graph:
    nodes: [{id: s, data: {type: start}}]
    edges: []
`,
			want: "app.mode must be",
		},
		{
			name: "duplicate node id",
			document: `
kind: app
app: {mode: workflow}
workflow:
  graph:
    nodes: [{id: s, data: {type: start}}, {id: s, data: {type: end}}]
    edges: []
`,
			want: "duplicate node id",
		},
		{
			name: "two start nodes",
			document: `
kind: app
app: {mode: workflow}
workflow:
  graph:
    nodes: [{id: s1, data: {type: start}}, {id: s2, data: {type: start}}, {id: e, data: {type: end}}]
    edges: [{source: s1, target: e}]
`,
			want: "exactly one start node",
		},
		{
			name: "no end node in workflow mode",
			document: `
kind: app
app: {mode: workflow}
workflow:
  graph:
    nodes: [{id: s, data: {type: start}}]
    edges: []
`,
			want: "at least one end node",
		},
		{
			name: "edge to unknown node",
			document: `
kind: app
app: {mode: workflow}
workflow:
  graph:
    nodes: [{id: s, data: {type: start}}, {id: e, data: {type: end}}]
    edges: [{source: s, target: ghost}]
`,
			want: "unknown target node",
		},
		{
			name: "cycle",
			document: `
kind: app
app: {mode: workflow}
workflow:
  graph:
    nodes:
      - {id: s, data: {type: start}}
      - {id: t, data: {type: template-transform, template: "x"}}
      - {id: e, data: {type: end}}
    edges:
      - {source: s, target: t}
      - {source: t, target: e}
      - {source: e, target: t}
`,
			want: "cycle",
		},
		{
			name: "unsupported node type",
			document: `
kind: app
app: {mode: workflow}
workflow:
  graph:
    nodes:
      - {id: s, data: {type: start}}
      - {id: it, data: {type: iteration}, parentId: s}
      - {id: e, data: {type: end}}
    edges: [{source: s, target: e}]
`,
			want: "not implemented",
		},
		{
			name: "reference to unknown node",
			document: `
kind: app
app: {mode: workflow}
workflow:
  graph:
    nodes:
      - {id: s, data: {type: start}}
      - id: t
        data:
          type: template-transform
          template: "{{#ghost.value#}}"
      - {id: e, data: {type: end}}
    edges: [{source: s, target: t}, {source: t, target: e}]
`,
			want: "points at an unknown node",
		},
		{
			name: "if-else handle mismatch",
			document: `
kind: app
app: {mode: workflow}
workflow:
  graph:
    nodes:
      - {id: s, data: {type: start}}
      - id: c
        data:
          type: if-else
          cases:
            - case_id: "yes"
              conditions:
                - variable_selector: [s, value]
                  comparison_operator: is
                  value: "1"
      - {id: e, data: {type: end}}
    edges:
      - {source: s, target: c}
      - {source: c, sourceHandle: "nope", target: e}
`,
			want: "does not match any if-else case",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app := mustParse(t, tc.document)
			problems := Validate(app)
			if !HasErrors(problems) {
				t.Fatalf("expected errors, got %v", problems)
			}
			joined := renderProblems(problems)
			if !strings.Contains(joined, tc.want) {
				t.Fatalf("problems = %s, want containing %q", joined, tc.want)
			}
		})
	}
}

func TestValidateWarnsWithoutFailing(t *testing.T) {
	tests := []struct {
		name     string
		document string
		want     string
	}{
		{
			name: "missing version",
			document: `
kind: app
app: {mode: workflow}
workflow:
  graph:
    nodes: [{id: s, data: {type: start}}, {id: e, data: {type: end}}]
    edges: [{source: s, target: e}]
`,
			want: "missing version",
		},
		{
			name: "unreachable node",
			document: `
kind: app
app: {mode: workflow}
workflow:
  graph:
    nodes:
      - {id: s, data: {type: start}}
      - {id: e, data: {type: end}}
      - {id: orphan, data: {type: template-transform, template: "x"}}
    edges: [{source: s, target: e}]
`,
			want: "unreachable",
		},
		{
			name: "fail-branch edge",
			document: `
kind: app
app: {mode: workflow}
workflow:
  graph:
    nodes:
      - {id: s, data: {type: start}}
      - {id: h, data: {type: http-request, url: "https://example.com"}}
      - {id: e, data: {type: end}}
    edges:
      - {source: s, target: h}
      - {source: h, target: e}
      - {source: h, sourceHandle: fail-branch, target: e}
`,
			want: "fail-branch",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			problems := Validate(mustParse(t, tc.document))
			if HasErrors(problems) {
				t.Fatalf("expected warnings only, got %v", problems)
			}
			if !strings.Contains(renderProblems(problems), tc.want) {
				t.Fatalf("problems = %s, want containing %q", renderProblems(problems), tc.want)
			}
		})
	}
}

func renderProblems(problems []Problem) string {
	parts := make([]string, 0, len(problems))
	for _, problem := range problems {
		parts = append(parts, problem.String())
	}
	return strings.Join(parts, "\n")
}
