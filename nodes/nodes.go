// Package nodes implements the workflow node handlers.
//
// Handlers are registered explicitly through Registry; there is no init-time
// registration and no blank-import requirement, so a build cannot silently lack
// a node type.
package nodes

import (
	"context"
	"fmt"
	"net/http"
	"sort"

	"github.com/Tsinling0525/rivulet/dsl"
	"github.com/Tsinling0525/rivulet/expr"
	"github.com/Tsinling0525/rivulet/llmclient"
)

// Request is everything a handler may read for one node execution.
type Request struct {
	// Node is the node definition being executed.
	Node dsl.Node
	// Mode is the app mode (workflow or advanced-chat).
	Mode string
	// Pool is a read-only snapshot of the run variables, taken by the engine
	// before the node started so parallel nodes cannot race.
	Pool expr.Pool
	// Inputs is the resolved start-node input map.
	Inputs map[string]any
	// Model overrides the llm node's provider configuration (CLI flags/env).
	Model *llmclient.Config
	// HTTPClient is shared by http-request nodes.
	HTTPClient *http.Client
	// WorkDir is the directory used for temp files created by code nodes.
	WorkDir string
	// Logf records progress for the node trace.
	Logf func(format string, args ...any)
}

// Log writes a progress line when a logger is configured.
func (r Request) Log(format string, args ...any) {
	if r.Logf == nil {
		return
	}
	r.Logf(format, args...)
}

// Output is the result of one node execution.
type Output struct {
	// Fields are exposed to downstream nodes under the node's ID.
	Fields map[string]any
	// Branch selects the outgoing edge handle. Empty means the default "source".
	Branch string
}

// Handler executes one node type.
type Handler interface {
	// Type is the DSL node type this handler implements.
	Type() string
	// Validate checks node configuration statically.
	Validate(node dsl.Node) []dsl.Problem
	// Run executes the node.
	Run(ctx context.Context, req Request) (Output, error)
}

// Registry returns every built-in handler, keyed by node type.
func Registry() map[string]Handler {
	handlers := []Handler{
		Start{},
		End{},
		Answer{},
		LLM{},
		Code{},
		IfElse{},
		Template{},
		HTTPRequest{},
		VariableAggregator{},
	}
	out := make(map[string]Handler, len(handlers))
	for _, handler := range handlers {
		out[handler.Type()] = handler
	}
	return out
}

// Types returns the sorted list of implemented node types.
func Types() []string {
	registry := Registry()
	out := make([]string, 0, len(registry))
	for nodeType := range registry {
		out = append(out, nodeType)
	}
	sort.Strings(out)
	return out
}

// Resolve reads a variable selector from the request pool.
func (r Request) Resolve(selector dsl.Selector) (any, bool) {
	return r.Pool.Lookup(selector)
}

// Render resolves both reference forms in text.
func (r Request) Render(text string) (string, error) {
	return expr.Render(text, r.Pool, nil)
}

// Error builds a node-scoped error.
func nodeError(node dsl.Node, format string, args ...any) error {
	return fmt.Errorf("node %s (%s): %s", node.ID, node.Data.Type, fmt.Sprintf(format, args...))
}

func problem(node dsl.Node, format string, args ...any) dsl.Problem {
	return dsl.Problem{
		Severity: dsl.SeverityError,
		NodeID:   node.ID,
		Message:  fmt.Sprintf(format, args...),
	}
}

// decodeExtra decodes the node-specific DSL fields into a typed struct.
func decodeExtra(node dsl.Node, target any) []dsl.Problem {
	if err := node.Data.Decode(target); err != nil {
		return []dsl.Problem{problem(node, "invalid configuration: %v", err)}
	}
	return nil
}

// selectorList decodes the DSL's list-of-selectors form:
//
//	variables:
//	  - [node_id, field]
func selectorList(raw any) ([]dsl.Selector, error) {
	if raw == nil {
		return nil, nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("expected a list of variable selectors, got %T", raw)
	}
	out := make([]dsl.Selector, 0, len(items))
	for _, item := range items {
		selector, err := dsl.SelectorOf(item)
		if err != nil {
			return nil, err
		}
		out = append(out, selector)
	}
	return out, nil
}
