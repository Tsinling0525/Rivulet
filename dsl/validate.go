package dsl

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Severity classifies a validation problem.
type Severity string

const (
	// SeverityError means the workflow cannot run.
	SeverityError Severity = "error"
	// SeverityWarning means the workflow can run but is likely wrong.
	SeverityWarning Severity = "warning"
)

// Problem is a single validation finding.
type Problem struct {
	Severity Severity
	NodeID   string
	Message  string
}

func (p Problem) String() string {
	if p.NodeID == "" {
		return fmt.Sprintf("%s: %s", p.Severity, p.Message)
	}
	return fmt.Sprintf("%s: node %q: %s", p.Severity, p.NodeID, p.Message)
}

// HasErrors reports whether any problem is fatal.
func HasErrors(problems []Problem) bool {
	for _, problem := range problems {
		if problem.Severity == SeverityError {
			return true
		}
	}
	return false
}

// Errors returns only the fatal problems.
func Errors(problems []Problem) []Problem {
	out := make([]Problem, 0, len(problems))
	for _, problem := range problems {
		if problem.Severity == SeverityError {
			out = append(out, problem)
		}
	}
	return out
}

// Unsupported lists Dify node types that Rivulet deliberately does not implement.
var Unsupported = map[string]string{
	"iteration":           "iteration containers are not supported; expand the loop or use an http-request node",
	"loop":                "loop containers are not supported",
	"iteration-start":     "iteration containers are not supported",
	"loop-start":          "loop containers are not supported",
	"tool":                "plugin tools are not supported; call the API with an http-request node",
	"knowledge-retrieval": "knowledge bases are not supported",
	"agent":               "agent nodes are not supported; use the llm node or the rivulet agent CLI",
	"parameter-extractor": "not implemented",
	"question-classifier": "not implemented; use if-else nodes",
	"document-extractor":  "not implemented",
	"list-operator":       "not implemented",
	"assigner":            "not implemented",
	"variable-assigner":   "not implemented",
	"datasource":          "not implemented",
	"trigger-schedule":    "triggers are not supported; schedule externally and pass inputs in",
	"trigger-webhook":     "triggers are not supported; invoke the workflow with rivulet run",
	"trigger-plugin":      "triggers are not supported",
}

var templateRefPattern = regexp.MustCompile(`\{\{#\s*([A-Za-z0-9_.\-]+)\s*#\}\}`)

// Validate checks graph structure and variable references. It does not execute
// anything and does not know about node-specific configuration; callers combine
// it with per-node validation from the node registry.
func Validate(app App) []Problem {
	var problems []Problem
	problems = append(problems, validateEnvelope(app)...)

	nodes := app.Workflow.Graph.Nodes
	seen := make(map[string]Node, len(nodes))
	for _, node := range nodes {
		if node.ID == "" {
			problems = append(problems, Problem{SeverityError, "", "node without an id"})
			continue
		}
		if _, dup := seen[node.ID]; dup {
			problems = append(problems, Problem{SeverityError, node.ID, "duplicate node id"})
			continue
		}
		seen[node.ID] = node
		if node.Data.Type == "" {
			problems = append(problems, Problem{SeverityError, node.ID, "node without data.type"})
		}
		if node.ParentID != "" {
			problems = append(problems, Problem{SeverityError, node.ID,
				"parentId (containers) is not supported by Rivulet"})
		}
		if reason, ok := Unsupported[node.Data.Type]; ok {
			problems = append(problems, Problem{SeverityError, node.ID,
				fmt.Sprintf("node type %q is not implemented: %s", node.Data.Type, reason)})
		}
	}

	problems = append(problems, validateStartAndEnd(app, seen)...)
	problems = append(problems, validateEdges(app, seen)...)
	problems = append(problems, validateReferences(app, seen)...)
	problems = append(problems, validateReachability(app, seen)...)
	return problems
}

func validateEnvelope(app App) []Problem {
	var problems []Problem
	if app.Kind != "app" {
		problems = append(problems, Problem{SeverityError, "",
			fmt.Sprintf("kind must be \"app\", got %q", app.Kind)})
	}
	if app.App.Mode != ModeWorkflow && app.App.Mode != ModeAdvancedChat {
		problems = append(problems, Problem{SeverityError, "",
			fmt.Sprintf("app.mode must be %q or %q, got %q", ModeWorkflow, ModeAdvancedChat, app.App.Mode)})
	}
	if app.Version == "" {
		problems = append(problems, Problem{SeverityWarning, "",
			fmt.Sprintf("missing version; this runner implements Dify DSL %s", Version)})
	} else if major(app.Version) != major(Version) {
		problems = append(problems, Problem{SeverityWarning, "",
			fmt.Sprintf("DSL version %s differs in major version from the implemented %s", app.Version, Version)})
	}
	return problems
}

func major(version string) string {
	if idx := strings.Index(version, "."); idx > 0 {
		return version[:idx]
	}
	return version
}

func validateStartAndEnd(app App, seen map[string]Node) []Problem {
	var problems []Problem
	var starts, terminals []string
	for id, node := range seen {
		switch node.Data.Type {
		case NodeStart:
			starts = append(starts, id)
		case NodeEnd, NodeAnswer:
			terminals = append(terminals, id)
		}
	}
	sort.Strings(starts)
	sort.Strings(terminals)

	switch len(starts) {
	case 0:
		problems = append(problems, Problem{SeverityError, "", "workflow needs exactly one start node"})
	case 1:
	default:
		problems = append(problems, Problem{SeverityError, "",
			fmt.Sprintf("workflow needs exactly one start node, found %d (%s)", len(starts), strings.Join(starts, ", "))})
	}
	if len(terminals) == 0 {
		if app.App.Mode == ModeAdvancedChat {
			problems = append(problems, Problem{SeverityError, "", "advanced-chat workflow needs at least one answer node"})
		} else {
			problems = append(problems, Problem{SeverityError, "", "workflow needs at least one end node"})
		}
	}
	if app.App.Mode == ModeWorkflow && len(terminals) > 0 {
		hasEnd := false
		for _, id := range terminals {
			if seen[id].Data.Type == NodeEnd {
				hasEnd = true
			}
		}
		if !hasEnd {
			problems = append(problems, Problem{SeverityError, "", "workflow mode needs an end node (answer is advanced-chat only)"})
		}
	}
	return problems
}

func validateEdges(app App, seen map[string]Node) []Problem {
	var problems []Problem
	type edgeKey struct{ source, handle, target, targetHandle string }
	known := make(map[edgeKey]bool, len(app.Workflow.Graph.Edges))

	for _, edge := range app.Workflow.Graph.Edges {
		if edge.TargetHandle != "" && edge.TargetHandle != "target" {
			problems = append(problems, Problem{SeverityWarning, edge.Target, fmt.Sprintf(
				"targetHandle %q is unusual; Dify expects \"target\"", edge.TargetHandle)})
		}
		source, hasSource := seen[edge.Source]
		if !hasSource {
			problems = append(problems, Problem{SeverityError, edge.Source,
				fmt.Sprintf("edge %q references unknown source node", edgeName(edge))})
		}
		if _, hasTarget := seen[edge.Target]; !hasTarget {
			problems = append(problems, Problem{SeverityError, edge.Target,
				fmt.Sprintf("edge %q references unknown target node", edgeName(edge))})
		}
		if edge.Handle() == "fail-branch" {
			problems = append(problems, Problem{SeverityWarning, edge.Source,
				"fail-branch edges are not implemented; use error_strategy instead"})
		}
		if hasSource {
			problems = append(problems, validateHandleForNode(source, edge)...)
		}
		key := edgeKey{edge.Source, edge.Handle(), edge.Target, edge.TargetHandle}
		if known[key] {
			problems = append(problems, Problem{SeverityWarning, edge.Source,
				fmt.Sprintf("duplicate edge %q", edgeName(edge))})
		}
		known[key] = true
	}

	if cyclic := findCycle(app, seen); len(cyclic) > 0 {
		problems = append(problems, Problem{SeverityError, cyclic[0],
			"graph contains a cycle: " + strings.Join(cyclic, " -> ")})
	}
	return problems
}

func validateHandleForNode(node Node, edge Edge) []Problem {
	handle := edge.Handle()
	if node.Data.Type == NodeIfElse {
		cases, err := ifElseCaseIDs(node)
		if err != nil {
			return []Problem{{SeverityError, node.ID, err.Error()}}
		}
		for _, id := range cases {
			if handle == id {
				return nil
			}
		}
		if handle == HandleTrue || handle == HandleFalse {
			return nil
		}
		return []Problem{{SeverityError, node.ID, fmt.Sprintf(
			"edge handle %q does not match any if-else case (%s)", handle, strings.Join(cases, ", "))}}
	}
	if handle != HandleSource {
		return []Problem{{SeverityWarning, node.ID, fmt.Sprintf(
			"node type %q has a single output port; handle %q will never be taken", node.Data.Type, handle)}}
	}
	return nil
}

func ifElseCaseIDs(node Node) ([]string, error) {
	raw, ok := node.Data.Extra["cases"]
	if !ok {
		return []string{HandleTrue, HandleFalse}, nil
	}
	list, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("cases must be a list, got %T", raw)
	}
	var ids []string
	for _, item := range list {
		entry, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("each case must be a mapping, got %T", item)
		}
		id := stringField(entry, "case_id")
		if id == "" {
			id = stringField(entry, "id")
		}
		if id == "" {
			return nil, fmt.Errorf("case without case_id")
		}
		ids = append(ids, id)
	}
	return append(ids, HandleFalse), nil
}

func edgeName(edge Edge) string {
	if edge.ID != "" {
		return edge.ID
	}
	return edge.Source + " -> " + edge.Target
}

func findCycle(app App, seen map[string]Node) []string {
	adjacency := make(map[string][]string, len(app.Workflow.Graph.Edges))
	for _, edge := range app.Workflow.Graph.Edges {
		if _, ok := seen[edge.Source]; !ok {
			continue
		}
		if _, ok := seen[edge.Target]; !ok {
			continue
		}
		adjacency[edge.Source] = append(adjacency[edge.Source], edge.Target)
	}

	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := make(map[string]int, len(seen))
	var stack []string
	var cycle []string

	var visit func(string) bool
	visit = func(id string) bool {
		color[id] = gray
		stack = append(stack, id)
		for _, next := range adjacency[id] {
			switch color[next] {
			case gray:
				for i := len(stack) - 1; i >= 0; i-- {
					if stack[i] == next {
						cycle = append(append([]string{}, stack[i:]...), next)
						return true
					}
				}
				cycle = []string{next, next}
				return true
			case white:
				if visit(next) {
					return true
				}
			}
		}
		stack = stack[:len(stack)-1]
		color[id] = black
		return false
	}

	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if color[id] == white && visit(id) {
			return cycle
		}
	}
	return nil
}

func validateReferences(app App, seen map[string]Node) []Problem {
	var problems []Problem
	for _, node := range app.Workflow.Graph.Nodes {
		if node.ID == "" {
			continue
		}
		for _, ref := range nodeReferences(node) {
			problems = append(problems, checkReference(node, ref, seen)...)
		}
	}
	return problems
}

// nodeReferences collects every variable reference inside a node: array-form
// selectors in structured fields and {{#node.field#}} templates in strings.
func nodeReferences(node Node) []Selector {
	var out []Selector
	seen := map[string]bool{}
	add := func(selector Selector) {
		if key := selector.String(); key != "" && !seen[key] {
			seen[key] = true
			out = append(out, selector)
		}
	}

	var walk func(value any)
	walk = func(value any) {
		switch v := value.(type) {
		case map[string]any:
			for key, item := range v {
				if key == "variable_selector" || key == "iterator_selector" || key == "output_selector" {
					if selector, err := SelectorOf(item); err == nil {
						add(selector)
					}
					continue
				}
				walk(item)
			}
		case []any:
			for _, item := range v {
				if selector, err := SelectorOf(item); err == nil && selector != nil {
					add(selector)
					continue
				}
				walk(item)
			}
		case string:
			for _, match := range templateRefPattern.FindAllStringSubmatch(v, -1) {
				parts := strings.Split(match[1], ".")
				if len(parts) >= 2 {
					add(Selector(parts))
				}
			}
		}
	}
	walk(node.Data.Extra)
	return out
}

func checkReference(node Node, selector Selector, seen map[string]Node) []Problem {
	nodeID := selector.Node()
	switch nodeID {
	case "sys", "env", "conversation", "rag":
		return nil
	}
	if _, ok := seen[nodeID]; !ok {
		return []Problem{{SeverityError, node.ID, fmt.Sprintf(
			"reference %q points at an unknown node", selector.String())}}
	}
	return nil
}

func validateReachability(app App, seen map[string]Node) []Problem {
	start, ok := app.StartNode()
	if !ok {
		return nil
	}
	reachable := map[string]bool{start.ID: true}
	queue := []string{start.ID}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, edge := range app.Workflow.Graph.Edges {
			if edge.Source != current || reachable[edge.Target] {
				continue
			}
			if _, exists := seen[edge.Target]; !exists {
				continue
			}
			reachable[edge.Target] = true
			queue = append(queue, edge.Target)
		}
	}

	var problems []Problem
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if !reachable[id] {
			problems = append(problems, Problem{SeverityWarning, id,
				"node is unreachable from the start node"})
		}
	}
	return problems
}

func stringField(source map[string]any, key string) string {
	text, _ := source[key].(string)
	return strings.TrimSpace(text)
}
