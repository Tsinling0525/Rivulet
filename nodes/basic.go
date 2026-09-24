package nodes

import (
	"context"
	"fmt"
	"strings"

	"github.com/Tsinling0525/rivulet/dsl"
	"github.com/Tsinling0525/rivulet/expr"
)

// Start exposes the workflow inputs. It is the entry point of every run.
type Start struct{}

func (Start) Type() string { return dsl.NodeStart }

type startVariable struct {
	Variable  string `yaml:"variable"`
	Label     string `yaml:"label"`
	Type      string `yaml:"type"`
	Required  bool   `yaml:"required"`
	MaxLength *int   `yaml:"max_length"`
}

type startConfig struct {
	Variables []startVariable `yaml:"variables"`
}

func (Start) Validate(node dsl.Node) []dsl.Problem {
	var cfg startConfig
	if problems := decodeExtra(node, &cfg); problems != nil {
		return problems
	}
	var problems []dsl.Problem
	for index, variable := range cfg.Variables {
		if strings.TrimSpace(variable.Variable) == "" {
			problems = append(problems, problem(node, "variables[%d] has no name", index))
		}
		if variable.Type == "file" {
			problems = append(problems, dsl.Problem{
				Severity: dsl.SeverityWarning,
				NodeID:   node.ID,
				Message:  fmt.Sprintf("input %q is typed \"file\", which Rivulet passes through as a path string", variable.Variable),
			})
		}
	}
	return problems
}

func (Start) Run(_ context.Context, req Request) (Output, error) {
	var cfg startConfig
	if err := req.Node.Data.Decode(&cfg); err != nil {
		return Output{}, nodeError(req.Node, "invalid configuration: %v", err)
	}

	fields := make(map[string]any, len(cfg.Variables))
	declared := make(map[string]bool, len(cfg.Variables))
	var missing []string
	for _, variable := range cfg.Variables {
		name := strings.TrimSpace(variable.Variable)
		declared[name] = true
		value, provided := req.Inputs[name]
		if !provided || isBlank(value) {
			if variable.Required {
				missing = append(missing, name)
				continue
			}
			// Optional inputs still resolve, as an empty string, so templates
			// that reference them do not fail on an unset value.
			if provided && !isBlank(value) {
				fields[name] = value
			} else {
				fields[name] = ""
			}
			continue
		}
		fields[name] = value
	}
	if len(missing) > 0 {
		return Output{}, nodeError(req.Node, "missing required input(s): %s", strings.Join(missing, ", "))
	}

	for name, value := range req.Inputs {
		if declared[name] {
			continue
		}
		req.Log("input %q is not declared by the start node; passing it through", name)
		fields[name] = value
	}
	return Output{Fields: fields}, nil
}

// End declares the workflow outputs.
type End struct{}

func (End) Type() string { return dsl.NodeEnd }

type endOutput struct {
	Variable      string       `yaml:"variable"`
	ValueSelector dsl.Selector `yaml:"value_selector"`
	ValueType     string       `yaml:"value_type"`
}

type endConfig struct {
	Outputs []endOutput `yaml:"outputs"`
}

func (End) Validate(node dsl.Node) []dsl.Problem {
	var cfg endConfig
	if problems := decodeExtra(node, &cfg); problems != nil {
		return problems
	}
	var problems []dsl.Problem
	if len(cfg.Outputs) == 0 {
		problems = append(problems, dsl.Problem{
			Severity: dsl.SeverityWarning,
			NodeID:   node.ID,
			Message:  "end node declares no outputs",
		})
	}
	for index, output := range cfg.Outputs {
		if strings.TrimSpace(output.Variable) == "" {
			problems = append(problems, problem(node, "outputs[%d] has no variable name", index))
		}
	}
	return problems
}

func (End) Run(_ context.Context, req Request) (Output, error) {
	var cfg endConfig
	if err := req.Node.Data.Decode(&cfg); err != nil {
		return Output{}, nodeError(req.Node, "invalid configuration: %v", err)
	}
	fields := make(map[string]any, len(cfg.Outputs))
	for _, output := range cfg.Outputs {
		name := strings.TrimSpace(output.Variable)
		if !output.ValueSelector.Valid() {
			return Output{}, nodeError(req.Node, "output %q has no value_selector", name)
		}
		value, ok := req.Resolve(output.ValueSelector)
		if !ok {
			// A skipped branch leaves the selector unresolved. Report it as nil
			// rather than failing the run; validate catches unknown node IDs.
			req.Log("output %q references %s, which did not run; emitting null", name, output.ValueSelector)
			fields[name] = nil
			continue
		}
		fields[name] = value
	}
	return Output{Fields: fields}, nil
}

// Answer renders a chatflow answer.
type Answer struct{}

func (Answer) Type() string { return dsl.NodeAnswer }

func (Answer) Validate(node dsl.Node) []dsl.Problem {
	if strings.TrimSpace(node.Data.StringField("answer")) == "" {
		return []dsl.Problem{problem(node, "answer must not be empty")}
	}
	return nil
}

func (Answer) Run(_ context.Context, req Request) (Output, error) {
	rendered, err := req.Render(req.Node.Data.StringField("answer"))
	if err != nil {
		return Output{}, nodeError(req.Node, "%v", err)
	}
	return Output{Fields: map[string]any{"answer": rendered}}, nil
}

// Template renders a text template from mapped variables (Dify's
// template-transform node). Expressions only: no loops or filters.
type Template struct{}

func (Template) Type() string { return dsl.NodeTemplate }

type templateVariable struct {
	Variable      string       `yaml:"variable"`
	ValueSelector dsl.Selector `yaml:"value_selector"`
}

type templateConfig struct {
	Template  string             `yaml:"template"`
	Variables []templateVariable `yaml:"variables"`
}

func (Template) Validate(node dsl.Node) []dsl.Problem {
	var cfg templateConfig
	problems := decodeExtra(node, &cfg)
	if problems != nil {
		return problems
	}
	if strings.TrimSpace(cfg.Template) == "" {
		problems = append(problems, problem(node, "template must not be empty"))
	}
	for index, variable := range cfg.Variables {
		if strings.TrimSpace(variable.Variable) == "" {
			problems = append(problems, problem(node, "variables[%d] has no name", index))
		}
	}
	return problems
}

func (Template) Run(_ context.Context, req Request) (Output, error) {
	var cfg templateConfig
	if err := req.Node.Data.Decode(&cfg); err != nil {
		return Output{}, nodeError(req.Node, "invalid configuration: %v", err)
	}
	variables := make(map[string]any, len(cfg.Variables))
	for _, variable := range cfg.Variables {
		value, ok := req.Resolve(variable.ValueSelector)
		if !ok {
			return Output{}, nodeError(req.Node, "template variable %q references %s, which produced no value",
				variable.Variable, variable.ValueSelector)
		}
		variables[variable.Variable] = value
	}
	rendered, err := expr.Render(cfg.Template, req.Pool, variables)
	if err != nil {
		return Output{}, nodeError(req.Node, "%v", err)
	}
	return Output{Fields: map[string]any{"output": rendered}}, nil
}

// VariableAggregator picks the value from whichever branch ran (Dify's
// variable-aggregator node).
type VariableAggregator struct{}

func (VariableAggregator) Type() string { return dsl.NodeAggregate }

type aggregateConfig struct {
	OutputType string         `yaml:"output_type"`
	Variables  []dsl.Selector `yaml:"variables"`
}

func (VariableAggregator) Validate(node dsl.Node) []dsl.Problem {
	var cfg aggregateConfig
	if problems := decodeExtra(node, &cfg); problems != nil {
		return problems
	}
	if len(cfg.Variables) == 0 {
		return []dsl.Problem{problem(node, "variables must list at least one branch")}
	}
	return nil
}

func (VariableAggregator) Run(_ context.Context, req Request) (Output, error) {
	var cfg aggregateConfig
	if err := req.Node.Data.Decode(&cfg); err != nil {
		return Output{}, nodeError(req.Node, "invalid configuration: %v", err)
	}
	var missing []string
	for _, selector := range cfg.Variables {
		value, ok := req.Resolve(selector)
		if !ok || value == nil {
			missing = append(missing, selector.String())
			continue
		}
		return Output{Fields: map[string]any{"output": value}}, nil
	}
	return Output{}, nodeError(req.Node, "no branch produced a value (checked %s)", strings.Join(missing, ", "))
}

func isBlank(value any) bool {
	switch v := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(v) == ""
	case []any:
		return len(v) == 0
	default:
		return false
	}
}
