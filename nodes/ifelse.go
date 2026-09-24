package nodes

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tsinling0525/rivulet/dsl"
	"github.com/Tsinling0525/rivulet/expr"
)

// IfElse evaluates ordered cases and selects an outgoing branch handle, matching
// Dify's if-else node: the first matching case_id wins, otherwise "false".
type IfElse struct{}

func (IfElse) Type() string { return dsl.NodeIfElse }

type condition struct {
	ID                 string       `yaml:"id"`
	VariableSelector   dsl.Selector `yaml:"variable_selector"`
	ComparisonOperator string       `yaml:"comparison_operator"`
	Value              any          `yaml:"value"`
	VarType            string       `yaml:"varType"`
}

type ifElseCase struct {
	ID              string      `yaml:"id"`
	CaseID          string      `yaml:"case_id"`
	LogicalOperator string      `yaml:"logical_operator"`
	Conditions      []condition `yaml:"conditions"`
}

type ifElseConfig struct {
	Cases []ifElseCase `yaml:"cases"`
}

func (IfElse) Validate(node dsl.Node) []dsl.Problem {
	var cfg ifElseConfig
	if problems := decodeExtra(node, &cfg); problems != nil {
		return problems
	}
	var problems []dsl.Problem
	if len(cfg.Cases) == 0 {
		return []dsl.Problem{problem(node, "cases must contain at least one case")}
	}
	for index, item := range cfg.Cases {
		if caseID(item) == "" {
			problems = append(problems, problem(node, "cases[%d] has no case_id", index))
		}
		if len(item.Conditions) == 0 {
			problems = append(problems, problem(node, "case %q has no conditions", caseID(item)))
		}
		operator := strings.ToLower(strings.TrimSpace(item.LogicalOperator))
		if operator != "" && operator != "and" && operator != "or" {
			problems = append(problems, problem(node, "case %q has logical_operator %q; use and or or",
				caseID(item), item.LogicalOperator))
		}
		for conditionIndex, cond := range item.Conditions {
			if !cond.VariableSelector.Valid() {
				problems = append(problems, problem(node, "case %q conditions[%d] has no variable_selector",
					caseID(item), conditionIndex))
			}
			if _, ok := comparisonOperators[strings.ToLower(strings.TrimSpace(cond.ComparisonOperator))]; !ok {
				problems = append(problems, problem(node, "case %q conditions[%d] uses unknown comparison_operator %q",
					caseID(item), conditionIndex, cond.ComparisonOperator))
			}
		}
	}
	return problems
}

func (IfElse) Run(_ context.Context, req Request) (Output, error) {
	var cfg ifElseConfig
	if err := req.Node.Data.Decode(&cfg); err != nil {
		return Output{}, nodeError(req.Node, "invalid configuration: %v", err)
	}

	for _, item := range cfg.Cases {
		matched, err := evalCase(req, item)
		if err != nil {
			return Output{}, nodeError(req.Node, "case %q: %v", caseID(item), err)
		}
		if matched {
			req.Log("case %q matched", caseID(item))
			return Output{Fields: map[string]any{"result": true}, Branch: caseID(item)}, nil
		}
	}
	return Output{Fields: map[string]any{"result": false}, Branch: dsl.HandleFalse}, nil
}

func caseID(item ifElseCase) string {
	if id := strings.TrimSpace(item.CaseID); id != "" {
		return id
	}
	if id := strings.TrimSpace(item.ID); id != "" {
		return id
	}
	return dsl.HandleTrue
}

func evalCase(req Request, item ifElseCase) (bool, error) {
	operator := strings.ToLower(strings.TrimSpace(item.LogicalOperator))
	if operator == "" {
		operator = "and"
	}
	results := make([]bool, 0, len(item.Conditions))
	for _, cond := range item.Conditions {
		value, _ := req.Resolve(cond.VariableSelector)
		matched, err := compare(value, cond.ComparisonOperator, cond.Value)
		if err != nil {
			return false, err
		}
		if operator == "or" && matched {
			return true, nil
		}
		if operator == "and" && !matched {
			return false, nil
		}
		results = append(results, matched)
	}
	if operator == "or" {
		return false, nil
	}
	return len(results) > 0, nil
}

type comparisonFunc func(value, expected any) (bool, error)

var comparisonOperators = map[string]comparisonFunc{
	"contains":     func(v, e any) (bool, error) { return strings.Contains(expr.Stringify(v), expr.Stringify(e)), nil },
	"not contains": func(v, e any) (bool, error) { return !strings.Contains(expr.Stringify(v), expr.Stringify(e)), nil },
	"start with":   func(v, e any) (bool, error) { return strings.HasPrefix(expr.Stringify(v), expr.Stringify(e)), nil },
	"end with":     func(v, e any) (bool, error) { return strings.HasSuffix(expr.Stringify(v), expr.Stringify(e)), nil },
	"is":           func(v, e any) (bool, error) { return looseEqual(v, e), nil },
	"is not":       func(v, e any) (bool, error) { return !looseEqual(v, e), nil },
	"=":            func(v, e any) (bool, error) { return numericCompare(v, e, func(a, b float64) bool { return a == b }) },
	"!=":           func(v, e any) (bool, error) { return numericCompare(v, e, func(a, b float64) bool { return a != b }) },
	">":            func(v, e any) (bool, error) { return numericCompare(v, e, func(a, b float64) bool { return a > b }) },
	"<":            func(v, e any) (bool, error) { return numericCompare(v, e, func(a, b float64) bool { return a < b }) },
	">=":           func(v, e any) (bool, error) { return numericCompare(v, e, func(a, b float64) bool { return a >= b }) },
	"<=":           func(v, e any) (bool, error) { return numericCompare(v, e, func(a, b float64) bool { return a <= b }) },
	"empty":        func(v, _ any) (bool, error) { return isBlank(v), nil },
	"not empty":    func(v, _ any) (bool, error) { return !isBlank(v), nil },
	"null":         func(v, _ any) (bool, error) { return v == nil, nil },
	"not null":     func(v, _ any) (bool, error) { return v != nil, nil },
	"in":           func(v, e any) (bool, error) { return containsValue(e, v) },
	"not in":       func(v, e any) (bool, error) { in, err := containsValue(e, v); return !in, err },
}

func compare(value any, operator string, expected any) (bool, error) {
	fn, ok := comparisonOperators[strings.ToLower(strings.TrimSpace(operator))]
	if !ok {
		return false, fmt.Errorf("unknown comparison_operator %q", operator)
	}
	return fn(value, expected)
}

func looseEqual(value, expected any) bool {
	if a, ok := toFloat(value); ok {
		if b, ok := toFloat(expected); ok {
			return a == b
		}
	}
	if value == nil || expected == nil {
		return value == nil && expected == nil
	}
	return expr.Stringify(value) == expr.Stringify(expected)
}

func numericCompare(value, expected any, cmp func(a, b float64) bool) (bool, error) {
	a, ok := toFloat(value)
	if !ok {
		return false, fmt.Errorf("value %v is not numeric", value)
	}
	b, ok := toFloat(expected)
	if !ok {
		return false, fmt.Errorf("comparison value %v is not numeric", expected)
	}
	return cmp(a, b), nil
}

func containsValue(container, value any) (bool, error) {
	switch items := container.(type) {
	case []any:
		for _, item := range items {
			if looseEqual(item, value) {
				return true, nil
			}
		}
		return false, nil
	case []string:
		for _, item := range items {
			if looseEqual(item, value) {
				return true, nil
			}
		}
		return false, nil
	case string:
		return strings.Contains(items, expr.Stringify(value)), nil
	case nil:
		return false, errors.New("comparison value is empty; \"in\" needs a list")
	default:
		return false, fmt.Errorf("comparison value must be a list, got %T", container)
	}
}
