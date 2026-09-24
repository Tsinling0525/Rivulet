package expr

import (
	"strings"
	"testing"

	"github.com/Tsinling0525/rivulet/dsl"
)

func TestLookupWalksNestedFields(t *testing.T) {
	pool := NewPool()
	pool.Set("code_node", map[string]any{
		"result": map[string]any{"items": []any{"a", "b"}},
		"count":  2,
	})

	tests := []struct {
		name     string
		selector dsl.Selector
		want     string
		wantOK   bool
	}{
		{"flat", dsl.Selector{"code_node", "count"}, "2", true},
		{"nested", dsl.Selector{"code_node", "result.items"}, `["a","b"]`, true},
		{"missing field", dsl.Selector{"code_node", "nope"}, "", false},
		{"missing node", dsl.Selector{"other_node", "count"}, "", false},
		{"no field", dsl.Selector{"code_node"}, "", false},
		{"empty", nil, "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			value, ok := pool.Lookup(tc.selector)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v (value %v)", ok, tc.wantOK, value)
			}
			if ok && Stringify(value) != tc.want {
				t.Fatalf("value = %q, want %q", Stringify(value), tc.want)
			}
		})
	}
}

func TestLookupUsesWildcardForFailedNodes(t *testing.T) {
	pool := NewPool()
	pool.Set("http_node", map[string]any{"*": "", "status_code": 0})

	value, ok := pool.Lookup(dsl.Selector{"http_node", "body"})
	if !ok {
		t.Fatal("expected the wildcard to resolve an unknown field")
	}
	if Stringify(value) != "" {
		t.Fatalf("value = %q, want empty string", Stringify(value))
	}
	if value, ok := pool.Lookup(dsl.Selector{"http_node", "status_code"}); !ok || Stringify(value) != "0" {
		t.Fatalf("declared default should win: %v ok=%v", value, ok)
	}
}

func TestSnapshotIsolatesLaterWrites(t *testing.T) {
	pool := NewPool()
	pool.Set("a", map[string]any{"value": 1})
	snapshot := pool.Snapshot()
	pool.Set("a", map[string]any{"value": 2})
	pool.Set("b", map[string]any{"value": 3})

	if value, _ := snapshot.Lookup(dsl.Selector{"a", "value"}); Stringify(value) != "1" {
		t.Fatalf("snapshot saw a later write: %v", value)
	}
	if _, ok := snapshot.Lookup(dsl.Selector{"b", "value"}); ok {
		t.Fatal("snapshot saw a node added after the copy")
	}
}

func TestRenderTemplate(t *testing.T) {
	pool := NewPool()
	pool.Set("start_node", map[string]any{"query": "hello", "count": 3})
	pool.SetPool(PoolEnvironment, map[string]any{"TONE": "polite"})

	tests := []struct {
		name    string
		text    string
		want    string
		wantErr string
	}{
		{name: "plain", text: "no refs", want: "no refs"},
		{name: "single", text: "Q: {{#start_node.query#}}", want: "Q: hello"},
		{name: "number", text: "{{#start_node.count#}} items", want: "3 items"},
		{name: "env", text: "be {{#env.TONE#}}", want: "be polite"},
		{name: "repeated", text: "{{#start_node.query#}}/{{#start_node.query#}}", want: "hello/hello"},
		{name: "unknown node", text: "{{#ghost.value#}}", wantErr: "unknown variable reference {{#ghost.value#}}"},
		{name: "unknown field", text: "{{#start_node.nope#}}", wantErr: "unknown variable reference"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := RenderTemplate(tc.text, pool)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			if got != tc.want {
				t.Fatalf("rendered = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRenderVarsAndReferences(t *testing.T) {
	vars := map[string]any{"arg1": "world", "obj": map[string]any{"key": "v"}}
	rendered, err := RenderVars("Hello {{ arg1 }} / {{obj.key}}", vars)
	if err != nil {
		t.Fatalf("render vars: %v", err)
	}
	if rendered != "Hello world / v" {
		t.Fatalf("rendered = %q", rendered)
	}

	if _, err := RenderVars("{{ missing }}", vars); err == nil {
		t.Fatal("expected an error for an unknown template variable")
	}

	refs := References("a {{#n1.f#}} b {{#n2.g#}} c")
	if len(refs) != 2 || refs[0].Node() != "n1" || refs[1].String() != "n2.g" {
		t.Fatalf("references = %v", refs)
	}
}

func TestRenderCombinesBothForms(t *testing.T) {
	pool := NewPool()
	pool.Set("start_node", map[string]any{"name": "Ada"})
	rendered, err := Render("Hi {{ name }} ({{#start_node.name#}})", pool, map[string]any{"name": "Ada"})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if rendered != "Hi Ada (Ada)" {
		t.Fatalf("rendered = %q", rendered)
	}
}

func TestStringifyTypes(t *testing.T) {
	tests := []struct {
		value any
		want  string
	}{
		{nil, ""},
		{"text", "text"},
		{true, "true"},
		{7, "7"},
		{int64(7), "7"},
		{2.5, "2.5"},
		{2.0, "2"},
		{[]any{"a", "b"}, `["a","b"]`},
		{map[string]any{"k": "v"}, `{"k":"v"}`},
	}
	for _, tc := range tests {
		if got := Stringify(tc.value); got != tc.want {
			t.Fatalf("Stringify(%#v) = %q, want %q", tc.value, got, tc.want)
		}
	}
}

func TestSelection(t *testing.T) {
	if items, ok := Selection([]any{1, 2}); !ok || len(items) != 2 {
		t.Fatalf("selection = %v ok=%v", items, ok)
	}
	if _, ok := Selection("not a list"); ok {
		t.Fatal("expected a string not to be a selection")
	}
}
