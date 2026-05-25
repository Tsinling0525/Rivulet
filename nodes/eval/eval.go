package eval

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/Tsinling0525/rivulet/model"
	"github.com/Tsinling0525/rivulet/plugin"
)

const defaultThreshold = 0.7

type Node struct {
	deps plugin.Deps
}

type criterion struct {
	Name          string   `json:"name,omitempty"`
	Description   string   `json:"description,omitempty"`
	Type          string   `json:"type,omitempty"`
	Field         string   `json:"field,omitempty"`
	Value         string   `json:"value,omitempty"`
	Values        []string `json:"values,omitempty"`
	Weight        float64  `json:"weight,omitempty"`
	CaseSensitive bool     `json:"case_sensitive,omitempty"`
}

type criterionResult struct {
	Name      string  `json:"name,omitempty"`
	Type      string  `json:"type,omitempty"`
	Score     float64 `json:"score"`
	Weight    float64 `json:"weight"`
	Passed    bool    `json:"passed"`
	Rationale string  `json:"rationale,omitempty"`
}

type evalResult struct {
	Mode        string            `json:"mode"`
	Score       float64           `json:"score"`
	Passed      bool              `json:"passed"`
	Threshold   float64           `json:"threshold"`
	TargetField string            `json:"target_field,omitempty"`
	Rationale   string            `json:"rationale,omitempty"`
	Judge       string            `json:"judge,omitempty"`
	Criteria    []criterionResult `json:"criteria,omitempty"`
}

type judgeConfig struct {
	Provider       string
	Model          string
	Endpoint       string
	APIKey         string
	APIKeyEnv      string
	PromptTemplate string
	Timeout        time.Duration
}

func (n *Node) Init(ctx context.Context, deps plugin.Deps) error {
	n.deps = deps
	return nil
}

func (n *Node) Process(ctx context.Context, wf model.Workflow, node model.Node, in model.Items) (model.Items, error) {
	outByPort, err := n.ProcessPorted(ctx, wf, node, in)
	if err != nil {
		return nil, err
	}
	return outByPort[model.PortMain], nil
}

func (n *Node) ProcessPorted(ctx context.Context, wf model.Workflow, node model.Node, in model.Items) (map[model.Port]model.Items, error) {
	cfg := node.Config
	targetField := stringConfig(cfg, "target_field", "output")
	threshold := floatConfig(cfg, "pass_threshold", defaultThreshold)
	criteria, err := parseCriteria(cfg["criteria"])
	if err != nil {
		return nil, err
	}
	judge, hasJudge, err := parseJudgeConfig(cfg["judge"])
	if err != nil {
		return nil, err
	}

	all := make(model.Items, 0, len(in))
	passed := model.Items{}
	failed := model.Items{}
	for _, item := range in {
		if item == nil {
			item = model.Item{}
		}
		var result evalResult
		if hasJudge {
			result, err = n.evaluateWithJudge(ctx, judge, criteria, targetField, threshold, item)
		} else {
			result, err = evaluateCriteria(criteria, targetField, threshold, item)
		}
		if err != nil {
			return nil, err
		}

		out := cloneItem(item)
		out["eval"] = result
		out["eval_score"] = result.Score
		out["eval_passed"] = result.Passed
		all = append(all, out)
		if result.Passed {
			passed = append(passed, out)
		} else {
			failed = append(failed, out)
		}
		n.emitResult(ctx, wf, node, result)
	}

	return map[model.Port]model.Items{
		model.PortMain:     all,
		model.Port("pass"): passed,
		model.Port("fail"): failed,
	}, nil
}

func evaluateCriteria(criteria []criterion, targetField string, threshold float64, item model.Item) (evalResult, error) {
	if len(criteria) == 0 {
		criteria = []criterion{{Name: "non_empty", Type: "non_empty", Field: targetField, Weight: 1}}
	}

	var totalWeighted float64
	var totalWeight float64
	results := make([]criterionResult, 0, len(criteria))
	for _, c := range criteria {
		if c.Weight <= 0 {
			c.Weight = 1
		}
		if c.Field == "" {
			c.Field = targetField
		}
		res, err := evaluateCriterion(c, item)
		if err != nil {
			return evalResult{}, err
		}
		totalWeighted += res.Score * c.Weight
		totalWeight += c.Weight
		res.Weight = c.Weight
		results = append(results, res)
	}

	score := 0.0
	if totalWeight > 0 {
		score = totalWeighted / totalWeight
	}
	return evalResult{
		Mode:        "criteria",
		Score:       clamp01(score),
		Passed:      score >= threshold,
		Threshold:   threshold,
		TargetField: targetField,
		Criteria:    results,
	}, nil
}

func evaluateCriterion(c criterion, item model.Item) (criterionResult, error) {
	fieldValue := stringValue(item[c.Field])
	checkValue := fieldValue
	values := c.Values
	if c.Value != "" {
		values = append([]string{c.Value}, values...)
	}
	if !c.CaseSensitive {
		checkValue = strings.ToLower(checkValue)
		for i := range values {
			values[i] = strings.ToLower(values[i])
		}
	}

	res := criterionResult{
		Name:   c.Name,
		Type:   c.Type,
		Weight: c.Weight,
	}

	switch c.Type {
	case "", "non_empty":
		res.Passed = strings.TrimSpace(fieldValue) != ""
	case "contains":
		res.Passed = len(values) > 0
		for _, value := range values {
			if !strings.Contains(checkValue, value) {
				res.Passed = false
				break
			}
		}
	case "contains_any":
		for _, value := range values {
			if strings.Contains(checkValue, value) {
				res.Passed = true
				break
			}
		}
	case "not_contains", "forbidden":
		res.Passed = true
		for _, value := range values {
			if strings.Contains(checkValue, value) {
				res.Passed = false
				break
			}
		}
	case "regex":
		if c.Value == "" {
			return res, fmt.Errorf("criterion %q requires value regex", c.Name)
		}
		re, err := regexp.Compile(c.Value)
		if err != nil {
			return res, err
		}
		res.Passed = re.MatchString(fieldValue)
	case "min_length":
		min, err := parseFloat(c.Value)
		if err != nil {
			return res, fmt.Errorf("criterion %q requires numeric value", c.Name)
		}
		res.Passed = float64(len(fieldValue)) >= min
	case "max_length":
		max, err := parseFloat(c.Value)
		if err != nil {
			return res, fmt.Errorf("criterion %q requires numeric value", c.Name)
		}
		res.Passed = float64(len(fieldValue)) <= max
	case "equals":
		res.Passed = len(values) > 0 && checkValue == values[0]
	case "one_of":
		for _, value := range values {
			if checkValue == value {
				res.Passed = true
				break
			}
		}
	case "starts_with":
		res.Passed = len(values) > 0 && strings.HasPrefix(checkValue, values[0])
	case "ends_with":
		res.Passed = len(values) > 0 && strings.HasSuffix(checkValue, values[0])
	case "json_valid":
		var raw any
		res.Passed = json.Unmarshal([]byte(fieldValue), &raw) == nil
	default:
		return res, fmt.Errorf("unsupported eval criterion type %q", c.Type)
	}

	if res.Passed {
		res.Score = 1
		res.Rationale = "criterion passed"
	} else {
		res.Score = 0
		res.Rationale = "criterion failed"
	}
	return res, nil
}

func (n *Node) evaluateWithJudge(ctx context.Context, cfg judgeConfig, criteria []criterion, targetField string, threshold float64, item model.Item) (evalResult, error) {
	output := stringValue(item[targetField])
	criteriaJSON, _ := json.Marshal(criteria)
	inputJSON, _ := json.Marshal(item)
	prompt := cfg.PromptTemplate
	if prompt == "" {
		prompt = defaultJudgePrompt
	}
	tpl, err := template.New("judge_prompt").Parse(prompt)
	if err != nil {
		return evalResult{}, err
	}
	var rendered bytes.Buffer
	data := map[string]any{
		"InputJSON":    string(inputJSON),
		"Output":       output,
		"CriteriaJSON": string(criteriaJSON),
	}
	if err := tpl.Execute(&rendered, data); err != nil {
		return evalResult{}, err
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	raw, err := callJudge(callCtx, cfg, rendered.String())
	if err != nil {
		return evalResult{}, err
	}
	result, err := parseJudgeResult(raw, threshold, targetField)
	if err != nil {
		return evalResult{}, err
	}
	result.Mode = "judge"
	result.Judge = cfg.Provider + "/" + cfg.Model
	result.Threshold = threshold
	result.TargetField = targetField
	return result, nil
}

func callJudge(ctx context.Context, cfg judgeConfig, prompt string) (string, error) {
	if cfg.Provider == "" {
		cfg.Provider = "openai"
	}
	if cfg.Provider != "openai" {
		return "", fmt.Errorf("unsupported judge provider %q", cfg.Provider)
	}
	if cfg.Model == "" {
		cfg.Model = "gpt-5-mini"
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = "https://api.openai.com/v1/responses"
	}
	if cfg.APIKey == "" {
		if cfg.APIKeyEnv == "" {
			cfg.APIKeyEnv = "OPENAI_API_KEY"
		}
		cfg.APIKey = os.Getenv(cfg.APIKeyEnv)
	}
	if cfg.APIKey == "" && !isLocalEndpoint(cfg.Endpoint) {
		return "", fmt.Errorf("%s is not set for judge provider", cfg.APIKeyEnv)
	}

	payload := map[string]any{
		"model": cfg.Model,
		"input": prompt,
	}
	if strings.Contains(cfg.Endpoint, "/chat/completions") {
		payload = map[string]any{
			"model":    cfg.Model,
			"messages": []map[string]string{{"role": "user", "content": prompt}},
		}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.Endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("judge error: status %s body=%s", resp.Status, strings.TrimSpace(string(raw)))
	}
	return extractJudgeText(cfg.Endpoint, raw)
}

func extractJudgeText(endpoint string, raw []byte) (string, error) {
	if strings.Contains(endpoint, "/chat/completions") {
		var parsed struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return "", err
		}
		if len(parsed.Choices) == 0 {
			return "", errors.New("judge returned no choices")
		}
		return parsed.Choices[0].Message.Content, nil
	}

	var parsed struct {
		OutputText string `json:"output_text"`
		Output     []struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", err
	}
	if parsed.OutputText != "" {
		return parsed.OutputText, nil
	}
	for _, output := range parsed.Output {
		for _, content := range output.Content {
			if content.Text != "" {
				return content.Text, nil
			}
		}
	}
	return "", errors.New("judge returned no text")
}

func parseJudgeResult(raw string, threshold float64, targetField string) (evalResult, error) {
	raw = stripJSONFence(raw)
	var parsed struct {
		Score     float64           `json:"score"`
		Passed    *bool             `json:"passed,omitempty"`
		Rationale string            `json:"rationale,omitempty"`
		Criteria  []criterionResult `json:"criteria,omitempty"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return evalResult{}, fmt.Errorf("judge response must be JSON: %w", err)
	}
	score := normalizeScore(parsed.Score)
	passed := score >= threshold
	if parsed.Passed != nil {
		passed = *parsed.Passed
	}
	for i := range parsed.Criteria {
		parsed.Criteria[i].Score = normalizeScore(parsed.Criteria[i].Score)
		if parsed.Criteria[i].Weight <= 0 {
			parsed.Criteria[i].Weight = 1
		}
	}
	return evalResult{
		Mode:        "judge",
		Score:       score,
		Passed:      passed,
		Threshold:   threshold,
		TargetField: targetField,
		Rationale:   parsed.Rationale,
		Criteria:    parsed.Criteria,
	}, nil
}

func (n *Node) emitResult(ctx context.Context, wf model.Workflow, node model.Node, result evalResult) {
	if n.deps.Bus == nil {
		return
	}
	_ = n.deps.Bus.Emit(ctx, "eval_result", map[string]any{
		"exec":           plugin.ExecutionIDFromContext(ctx),
		"workflow":       wf.ID,
		"workflow_kind":  wf.Kind,
		"node":           node.ID,
		"mode":           result.Mode,
		"score":          result.Score,
		"passed":         result.Passed,
		"threshold":      result.Threshold,
		"target_field":   result.TargetField,
		"judge":          result.Judge,
		"criteria_count": len(result.Criteria),
	})
}

func parseCriteria(raw any) ([]criterion, error) {
	if raw == nil {
		return nil, nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("criteria must be an array")
	}
	out := make([]criterion, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("criteria entries must be objects")
		}
		c := criterion{
			Name:          stringConfig(m, "name", ""),
			Description:   stringConfig(m, "description", ""),
			Type:          stringConfig(m, "type", "non_empty"),
			Field:         stringConfig(m, "field", ""),
			Value:         stringConfig(m, "value", ""),
			Values:        stringSlice(m["values"]),
			Weight:        floatConfig(m, "weight", 1),
			CaseSensitive: boolConfig(m, "case_sensitive", false),
		}
		if c.Name == "" {
			c.Name = c.Type
		}
		if c.Value == "" {
			c.Value = stringConfig(m, "contains", "")
		}
		if len(c.Values) == 0 {
			c.Values = stringSlice(m["contains_any"])
		}
		if c.Type == "not_contains" || c.Type == "forbidden" {
			if c.Value == "" {
				c.Value = stringConfig(m, "forbidden", "")
			}
			if len(c.Values) == 0 {
				c.Values = stringSlice(m["forbidden_values"])
			}
		}
		out = append(out, c)
	}
	return out, nil
}

func parseJudgeConfig(raw any) (judgeConfig, bool, error) {
	if raw == nil {
		return judgeConfig{}, false, nil
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return judgeConfig{}, false, fmt.Errorf("judge must be an object")
	}
	timeoutSeconds := floatConfig(m, "timeout_seconds", 60)
	return judgeConfig{
		Provider:       stringConfig(m, "provider", "openai"),
		Model:          stringConfig(m, "model", "gpt-5-mini"),
		Endpoint:       stringConfig(m, "endpoint", ""),
		APIKey:         stringConfig(m, "api_key", ""),
		APIKeyEnv:      stringConfig(m, "api_key_env", "OPENAI_API_KEY"),
		PromptTemplate: stringConfig(m, "prompt", ""),
		Timeout:        time.Duration(timeoutSeconds * float64(time.Second)),
	}, true, nil
}

func stringConfig(m map[string]any, key, def string) string {
	raw, ok := m[key]
	if !ok || raw == nil {
		return def
	}
	if v, ok := raw.(string); ok {
		if v == "" {
			return def
		}
		return v
	}
	return fmt.Sprint(raw)
}

func floatConfig(m map[string]any, key string, def float64) float64 {
	if v, ok := asFloat(m[key]); ok {
		return v
	}
	return def
}

func boolConfig(m map[string]any, key string, def bool) bool {
	if v, ok := m[key].(bool); ok {
		return v
	}
	return def
}

func stringSlice(raw any) []string {
	switch v := raw.(type) {
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	case []string:
		return append([]string(nil), v...)
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			out = append(out, fmt.Sprint(item))
		}
		return out
	default:
		return nil
	}
}

func stringValue(raw any) string {
	switch v := raw.(type) {
	case nil:
		return ""
	case string:
		return v
	case []byte:
		return string(v)
	default:
		data, err := json.Marshal(v)
		if err == nil {
			return string(data)
		}
		return fmt.Sprint(v)
	}
}

func parseFloat(value string) (float64, error) {
	var out float64
	_, err := fmt.Sscan(value, &out)
	return out, err
}

func asFloat(raw any) (float64, bool) {
	switch v := raw.(type) {
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case json.Number:
		out, err := v.Float64()
		return out, err == nil
	default:
		return 0, false
	}
}

func normalizeScore(score float64) float64 {
	if score > 1 && score <= 100 {
		score = score / 100
	}
	return clamp01(score)
}

func clamp01(score float64) float64 {
	if score < 0 {
		return 0
	}
	if score > 1 {
		return 1
	}
	return score
}

func cloneItem(item model.Item) model.Item {
	out := make(model.Item, len(item)+3)
	for key, value := range item {
		out[key] = value
	}
	return out
}

func stripJSONFence(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "```") {
		value = strings.TrimPrefix(value, "```json")
		value = strings.TrimPrefix(value, "```")
		value = strings.TrimSuffix(value, "```")
	}
	return strings.TrimSpace(value)
}

func isLocalEndpoint(endpoint string) bool {
	return strings.Contains(endpoint, "localhost") || strings.Contains(endpoint, "127.0.0.1")
}

const defaultJudgePrompt = `You are judging the quality of an upstream AI node output.
Return only JSON with this shape:
{"score":0.0,"passed":false,"rationale":"short reason","criteria":[{"name":"criterion","score":0.0,"passed":false,"rationale":"short reason"}]}

Criteria:
{{.CriteriaJSON}}

Input item JSON:
{{.InputJSON}}

Output to evaluate:
{{.Output}}
`

func init() { plugin.Register("eval:node", func() plugin.NodeHandler { return &Node{} }) }
