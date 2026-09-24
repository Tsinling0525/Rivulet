// Package expr resolves Dify-style variable references against run state.
//
// Two reference forms exist in the DSL:
//
//	node.field      array-form selectors in structured fields, e.g. [llm_node, text]
//	{{#node.field#}}  template form inside text fields
//
// Both read from a Pool of node outputs plus the reserved sys, env, and
// conversation pools.
package expr

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/Tsinling0525/rivulet/dsl"
)

// Reserved pool names, matching Dify's variable prefixes.
const (
	PoolSystem       = "sys"
	PoolEnvironment  = "env"
	PoolConversation = "conversation"
)

// Pool holds node outputs and reserved variables for one run.
type Pool map[string]map[string]any

// NewPool returns an empty pool with initialised reserved namespaces.
func NewPool() Pool {
	return Pool{
		PoolSystem:       {},
		PoolEnvironment:  {},
		PoolConversation: {},
	}
}

// SetPool replaces all fields of a namespace.
func (p Pool) SetPool(name string, fields map[string]any) {
	if fields == nil {
		fields = map[string]any{}
	}
	p[name] = fields
}

// Set records a node's output fields.
func (p Pool) Set(nodeID string, fields map[string]any) {
	if fields == nil {
		fields = map[string]any{}
	}
	p[nodeID] = fields
}

// Fields returns a node's output fields.
func (p Pool) Fields(nodeID string) map[string]any { return p[nodeID] }

// Snapshot copies the pool so a running node cannot observe later writes.
func (p Pool) Snapshot() Pool {
	out := make(Pool, len(p))
	for namespace, fields := range p {
		copied := make(map[string]any, len(fields))
		for key, value := range fields {
			copied[key] = value
		}
		out[namespace] = copied
	}
	return out
}

// Lookup resolves a selector path. The field part may be dotted to walk nested
// objects, so [code_node, result.items] and {{#code_node.result.items#}} both work.
//
// A node that failed under continue-on-error carries a "*" wildcard, which makes
// any of its references resolve instead of aborting the run.
func (p Pool) Lookup(selector dsl.Selector) (any, bool) {
	if len(selector) == 0 {
		return nil, false
	}
	fields, ok := p[selector.Node()]
	if !ok {
		return nil, false
	}
	parts := strings.Split(selector.Field(), ".")
	if len(parts) == 0 || parts[0] == "" {
		return nil, false
	}
	var current any = fields
	for _, part := range parts {
		container, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = container[part]
		if !ok {
			if fallback, hasFallback := fields[wildcard]; hasFallback {
				return fallback, true
			}
			return nil, false
		}
	}
	return current, true
}

// wildcard marks a node whose values are unavailable but must still resolve.
const wildcard = "*"

// LookupSelectorString resolves a "node.field" string.
func (p Pool) LookupSelectorString(ref string) (any, bool) {
	return p.Lookup(dsl.Selector(strings.Split(strings.TrimSpace(ref), ".")))
}

var (
	templateRefPattern = regexp.MustCompile(`\{\{#\s*([A-Za-z0-9_.\-]+)\s*#\}\}`)
	namedVarPattern    = regexp.MustCompile(`\{\{\s*([A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z0-9_\-]+)*)\s*\}\}`)
)

// References returns every {{#node.field#}} reference in a string.
func References(text string) []dsl.Selector {
	var out []dsl.Selector
	for _, match := range templateRefPattern.FindAllStringSubmatch(text, -1) {
		out = append(out, dsl.Selector(strings.Split(match[1], ".")))
	}
	return out
}

// Render replaces both reference forms: {{#node.field#}} from the pool and
// {{ name }} from the supplied named variables. Unknown references are errors:
// a silently empty variable hides typos in the DSL.
func Render(text string, pool Pool, vars map[string]any) (string, error) {
	rendered, err := RenderTemplate(text, pool)
	if err != nil {
		return "", err
	}
	return RenderVars(rendered, vars)
}

// RenderTemplate replaces {{#node.field#}} references from the pool.
func RenderTemplate(text string, pool Pool) (string, error) {
	var failure error
	out := templateRefPattern.ReplaceAllStringFunc(text, func(match string) string {
		parts := templateRefPattern.FindStringSubmatch(match)
		value, ok := pool.LookupSelectorString(parts[1])
		if !ok {
			if failure == nil {
				failure = fmt.Errorf("unknown variable reference {{#%s#}}", parts[1])
			}
			return match
		}
		return Stringify(value)
	})
	return out, failure
}

// RenderVars replaces {{ name }} placeholders from a named variable map.
func RenderVars(text string, vars map[string]any) (string, error) {
	if len(vars) == 0 {
		return text, nil
	}
	var failure error
	out := namedVarPattern.ReplaceAllStringFunc(text, func(match string) string {
		name := strings.TrimSpace(namedVarPattern.FindStringSubmatch(match)[1])
		value, ok := lookupPath(vars, name)
		if !ok {
			if failure == nil {
				failure = fmt.Errorf("unknown template variable {{ %s }}", name)
			}
			return match
		}
		return Stringify(value)
	})
	return out, failure
}

func lookupPath(vars map[string]any, path string) (any, bool) {
	parts := strings.Split(path, ".")
	var current any = vars
	for _, part := range parts {
		container, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = container[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

// Stringify renders a value the way it appears inside a rendered template.
func Stringify(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case bool:
		return strconv.FormatBool(v)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(v), 'f', -1, 32)
	case json.Number:
		return v.String()
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(raw)
	}
}

// Selection converts a resolved value into a list, for iterator-style use.
func Selection(value any) ([]any, bool) {
	switch v := value.(type) {
	case []any:
		return v, true
	case []string:
		out := make([]any, 0, len(v))
		for _, item := range v {
			out = append(out, item)
		}
		return out, true
	case nil:
		return nil, false
	default:
		return nil, false
	}
}
