// Package dsl parses and validates Dify-style workflow DSL documents.
//
// The document envelope follows Dify's DSL export format (version 0.6.0):
//
//	version: "0.6.0"
//	kind: app
//	app: {name, mode, description}
//	workflow:
//	  graph:
//	    nodes: [{id, type: custom, position, data: {type, title, ...}}]
//	    edges: [{id, source, sourceHandle, target, targetHandle}]
//	  features: {}
//	  environment_variables: []
//
// Only the YAML/JSON shape is shared with Dify; execution is Rivulet's own.
package dsl

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Version is the Dify DSL version this package targets.
const Version = "0.6.0"

// App modes supported by the runner.
const (
	ModeWorkflow     = "workflow"
	ModeAdvancedChat = "advanced-chat"
)

// App is the top-level DSL document.
type App struct {
	Version      string       `yaml:"version"`
	Kind         string       `yaml:"kind"`
	App          AppMeta      `yaml:"app"`
	Workflow     Workflow     `yaml:"workflow"`
	Dependencies []Dependency `yaml:"dependencies,omitempty"`
}

// AppMeta is the app metadata block.
type AppMeta struct {
	Name        string `yaml:"name"`
	Mode        string `yaml:"mode"`
	Description string `yaml:"description,omitempty"`
	Icon        string `yaml:"icon,omitempty"`
	IconType    string `yaml:"icon_type,omitempty"`
}

// Dependency is an accepted-but-unused Dify plugin dependency entry.
type Dependency struct {
	Type  string `yaml:"type,omitempty"`
	Value any    `yaml:"value,omitempty"`
}

// Workflow is the workflow content block.
type Workflow struct {
	Graph                 Graph          `yaml:"graph"`
	Features              map[string]any `yaml:"features,omitempty"`
	EnvironmentVariables  []Variable     `yaml:"environment_variables,omitempty"`
	ConversationVariables []Variable     `yaml:"conversation_variables,omitempty"`
}

// Variable is an environment or conversation variable declaration.
type Variable struct {
	ID          string `yaml:"id,omitempty"`
	Name        string `yaml:"name"`
	Value       any    `yaml:"value,omitempty"`
	ValueType   string `yaml:"value_type,omitempty"`
	Description string `yaml:"description,omitempty"`
}

// Graph is the node/edge graph.
type Graph struct {
	Nodes    []Node         `yaml:"nodes"`
	Edges    []Edge         `yaml:"edges"`
	Viewport map[string]any `yaml:"viewport,omitempty"`
}

// Node is a graph node. Node-specific configuration lives in Data.Extra.
type Node struct {
	ID       string         `yaml:"id"`
	Type     string         `yaml:"type,omitempty"`
	Position map[string]any `yaml:"position,omitempty"`
	ParentID string         `yaml:"parentId,omitempty"`
	Data     NodeData       `yaml:"data"`
}

// NodeData holds the common node fields plus the node-specific remainder.
type NodeData struct {
	Type  string         `yaml:"type"`
	Title string         `yaml:"title,omitempty"`
	Desc  string         `yaml:"desc,omitempty"`
	Extra map[string]any `yaml:",inline"`
}

// Edge is a directed connection between two nodes.
type Edge struct {
	ID           string         `yaml:"id,omitempty"`
	Source       string         `yaml:"source"`
	SourceHandle string         `yaml:"sourceHandle,omitempty"`
	Target       string         `yaml:"target"`
	TargetHandle string         `yaml:"targetHandle,omitempty"`
	Type         string         `yaml:"type,omitempty"`
	Data         map[string]any `yaml:"data,omitempty"`
}

// Handle is the source handle, defaulting to Dify's "source".
func (e Edge) Handle() string {
	if e.SourceHandle == "" {
		return HandleSource
	}
	return e.SourceHandle
}

// Default output handle used by every node with a single output port.
const (
	HandleSource = "source"
	HandleTrue   = "true"
	HandleFalse  = "false"
)

// Selector is Dify's array-form variable reference: [node_id, field].
type Selector []string

// Node returns the referenced node ID (empty for sys/env/conversation pools).
func (s Selector) Node() string {
	if len(s) == 0 {
		return ""
	}
	return s[0]
}

// Field returns everything after the node ID, joined by dots.
func (s Selector) Field() string {
	if len(s) < 2 {
		return ""
	}
	out := s[1]
	for _, part := range s[2:] {
		out += "." + part
	}
	return out
}

// String renders the selector as node.field.
func (s Selector) String() string {
	if len(s) == 0 {
		return ""
	}
	return s.Node() + "." + s.Field()
}

// Valid reports whether the selector has at least a node and a field.
func (s Selector) Valid() bool { return len(s) >= 2 }

// SelectorOf coerces a decoded YAML value into a Selector.
func SelectorOf(value any) (Selector, error) {
	switch v := value.(type) {
	case nil:
		return nil, nil
	case Selector:
		return v, nil
	case []string:
		return Selector(v), nil
	case []any:
		out := make(Selector, 0, len(v))
		for _, item := range v {
			text, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("variable selector elements must be strings, got %T", item)
			}
			out = append(out, text)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("variable selector must be a list, got %T", value)
	}
}

// Decode re-encodes the node-specific fields into a typed configuration struct.
func (d NodeData) Decode(target any) error {
	if len(d.Extra) == 0 {
		return nil
	}
	raw, err := yaml.Marshal(d.Extra)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(raw, target)
}

// Field returns a raw node-specific field.
func (d NodeData) Field(name string) any { return d.Extra[name] }

// StringField returns a node-specific string field, trimmed.
func (d NodeData) StringField(name string) string {
	text, _ := d.Extra[name].(string)
	return text
}

// SelectorField decodes a node-specific selector field.
func (d NodeData) SelectorField(name string) (Selector, error) {
	raw, ok := d.Extra[name]
	if !ok {
		return nil, nil
	}
	return SelectorOf(raw)
}

// Parse decodes a DSL document from YAML or JSON bytes.
func Parse(data []byte) (App, error) {
	var app App
	if err := yaml.Unmarshal(data, &app); err != nil {
		return App{}, fmt.Errorf("parse workflow DSL: %w", err)
	}
	return app, nil
}

// Load reads and parses a DSL document from disk.
func Load(path string) (App, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return App{}, err
	}
	app, err := Parse(data)
	if err != nil {
		return App{}, fmt.Errorf("%s: %w", path, err)
	}
	return app, nil
}

// NodeByID returns the node with the given ID.
func (a App) NodeByID(id string) (Node, bool) {
	for _, node := range a.Workflow.Graph.Nodes {
		if node.ID == id {
			return node, true
		}
	}
	return Node{}, false
}

// StartNode returns the unique start node, or false when there is not exactly one.
func (a App) StartNode() (Node, bool) {
	var found Node
	count := 0
	for _, node := range a.Workflow.Graph.Nodes {
		if node.Data.Type == NodeStart {
			found = node
			count++
		}
	}
	return found, count == 1
}

// EnvironmentValues renders the environment variable block as a name -> value map.
func (a App) EnvironmentValues() map[string]any {
	out := make(map[string]any, len(a.Workflow.EnvironmentVariables))
	for _, variable := range a.Workflow.EnvironmentVariables {
		if variable.Name == "" {
			continue
		}
		out[variable.Name] = variable.Value
	}
	return out
}

// Node type identifiers, matching Dify's BlockEnum values where implemented.
const (
	NodeStart     = "start"
	NodeEnd       = "end"
	NodeAnswer    = "answer"
	NodeLLM       = "llm"
	NodeCode      = "code"
	NodeIfElse    = "if-else"
	NodeTemplate  = "template-transform"
	NodeHTTP      = "http-request"
	NodeAggregate = "variable-aggregator"
)
