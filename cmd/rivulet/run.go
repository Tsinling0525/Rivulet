package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Tsinling0525/rivulet/dsl"
	"github.com/Tsinling0525/rivulet/llmclient"
	"github.com/Tsinling0525/rivulet/nodes"
	"github.com/Tsinling0525/rivulet/workflow"
)

type workflowCLIOptions struct {
	File        string
	Inputs      map[string]string
	InputFile   string
	Provider    string
	Model       string
	Endpoint    string
	APIKey      string
	Concurrency int
	TracePath   string
	JSON        bool
}

func runWorkflowCLI(args []string) error {
	return runWorkflowCLIWithIO(args, os.Stdout, os.Stdin)
}

func runWorkflowCLIWithIO(args []string, out io.Writer, stdin io.Reader) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	file := fs.String("file", "", "Workflow DSL file to run (.yml or .json)")
	inputFile := fs.String("input-file", "", "JSON object of workflow inputs")
	provider := fs.String("provider", getenvDefault("RIVULET_MODEL_PROVIDER", ""),
		"Override the llm provider: "+strings.Join(nodes.ProviderNames(), ", "))
	model := fs.String("model", getenvDefault("RIVULET_MODEL", ""), "Override the llm model name")
	endpoint := fs.String("endpoint", getenvDefault("RIVULET_MODEL_ENDPOINT", ""), "Override the llm endpoint")
	apiKey := fs.String("api-key", getenvDefault("RIVULET_MODEL_API_KEY", ""), "Override the llm API key")
	concurrency := fs.Int("concurrency", 0, "Maximum nodes running at once (default 4)")
	trace := fs.String("trace", "", "Write a JSON run trace to this path")
	asJSON := fs.Bool("json", false, "Print the run result as JSON")
	inputs := newKeyValueFlags()
	fs.Var(inputs, "input", "Workflow input as key=value (repeatable)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if *concurrency < 0 {
		return fmt.Errorf("--concurrency must not be negative, got %d", *concurrency)
	}

	app, err := loadApp(*file)
	if err != nil {
		return err
	}
	resolved, err := resolveInputs(*inputFile, inputs.values, stdin)
	if err != nil {
		return err
	}

	opts := workflowCLIOptions{
		File:        *file,
		Inputs:      inputs.values,
		InputFile:   *inputFile,
		Provider:    *provider,
		Model:       *model,
		Endpoint:    *endpoint,
		APIKey:      *apiKey,
		Concurrency: *concurrency,
		TracePath:   *trace,
		JSON:        *asJSON,
	}
	return executeWorkflow(app, resolved, opts, out)
}

func executeWorkflow(app dsl.App, inputs map[string]any, opts workflowCLIOptions, out io.Writer) error {
	modelOverride, err := buildModelOverride(opts)
	if err != nil {
		return err
	}

	workDir := filepath.Dir(opts.File)
	if workDir == "" {
		workDir = "."
	}
	started := time.Now()
	result, err := workflow.Run(context.Background(), app, inputs, workflow.Options{
		Model:       modelOverride,
		WorkDir:     workDir,
		Concurrency: opts.Concurrency,
		Logf: func(format string, args ...any) {
			if !opts.JSON {
				fmt.Fprintf(out, "  · "+format+"\n", args...)
			}
		},
	})
	if err != nil {
		return err
	}

	if opts.TracePath != "" {
		if err := writeWorkflowTrace(opts.TracePath, app, result); err != nil {
			return err
		}
	}

	if opts.JSON {
		return printJSONResult(out, app, result)
	}
	printHumanResult(out, app, result, time.Since(started), opts.TracePath)
	if len(app.Workflow.Graph.Nodes) > 0 {
		for _, step := range result.Steps {
			if step.Status == workflow.StatusFailed {
				return fmt.Errorf("workflow finished with failed node %s", step.NodeID)
			}
		}
	}
	return nil
}

func loadApp(path string) (dsl.App, error) {
	if strings.TrimSpace(path) == "" {
		return dsl.App{}, errors.New("--file is required")
	}
	app, err := dsl.Load(path)
	if err != nil {
		return dsl.App{}, err
	}
	if problems := workflow.Validate(app, nil); dsl.HasErrors(problems) {
		return dsl.App{}, fmt.Errorf("%s is not valid:\n%s", path, formatProblems(problems))
	}
	return app, nil
}

func buildModelOverride(opts workflowCLIOptions) (*llmclient.Config, error) {
	provider := strings.TrimSpace(opts.Provider)
	override := llmclient.Config{
		Model:    strings.TrimSpace(opts.Model),
		Endpoint: strings.TrimSpace(opts.Endpoint),
		APIKey:   strings.TrimSpace(opts.APIKey),
	}
	if provider == "" {
		if override.Model == "" && override.Endpoint == "" && override.APIKey == "" {
			return nil, nil
		}
		return &override, nil
	}
	endpoint, keyEnv, ok := nodes.ProviderDefaults(provider)
	if !ok {
		return nil, fmt.Errorf("unsupported provider %q; use one of %s", provider, strings.Join(nodes.ProviderNames(), ", "))
	}
	if override.Endpoint == "" {
		override.Endpoint = endpoint
	}
	if override.APIKey == "" {
		override.APIKey = strings.TrimSpace(os.Getenv(keyEnv))
	}
	if provider == "ollama" && override.APIKey == "" {
		override.APIKey = "ollama"
	}
	return &override, nil
}

func resolveInputs(path string, inline map[string]string, stdin io.Reader) (map[string]any, error) {
	inputs := map[string]any{}
	if strings.TrimSpace(path) != "" {
		var raw []byte
		var err error
		if path == "-" {
			raw, err = io.ReadAll(stdin)
		} else {
			raw, err = os.ReadFile(path)
		}
		if err != nil {
			return nil, fmt.Errorf("read input file: %w", err)
		}
		if err := json.Unmarshal(raw, &inputs); err != nil {
			return nil, fmt.Errorf("input file must be a JSON object: %w", err)
		}
	}
	for key, value := range inline {
		inputs[key] = coerceInputValue(value)
	}
	return inputs, nil
}

// coerceInputValue keeps numeric and boolean inputs usable in if-else
// comparisons without forcing the caller to write JSON.
func coerceInputValue(value string) any {
	trimmed := strings.TrimSpace(value)
	switch trimmed {
	case "true":
		return true
	case "false":
		return false
	case "null":
		return nil
	}
	var decoded any
	if err := json.Unmarshal([]byte(trimmed), &decoded); err == nil {
		switch decoded.(type) {
		case float64, []any, map[string]any:
			return decoded
		}
	}
	return value
}

func validateWorkflowCLI(args []string) error {
	return validateWorkflowCLIWithIO(args, os.Stdout)
}

func validateWorkflowCLIWithIO(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	file := fs.String("file", "", "Workflow DSL file to validate")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if strings.TrimSpace(*file) == "" {
		return errors.New("--file is required")
	}
	app, err := dsl.Load(*file)
	if err != nil {
		return err
	}
	problems := workflow.Validate(app, nil)
	printProblems(out, app, problems)
	if dsl.HasErrors(problems) {
		return errors.New("workflow has validation errors")
	}
	return nil
}

func printProblems(out io.Writer, app dsl.App, problems []dsl.Problem) {
	if len(problems) == 0 {
		fmt.Fprintf(out, "ok: %s (%s, %d nodes, %d edges)\n",
			displayName(app), app.App.Mode, len(app.Workflow.Graph.Nodes), len(app.Workflow.Graph.Edges))
		return
	}
	errorsOnly := dsl.Errors(problems)
	fmt.Fprintf(out, "%s (%s): %d error(s), %d warning(s)\n",
		displayName(app), app.App.Mode, len(errorsOnly), len(problems)-len(errorsOnly))
	for _, problem := range problems {
		fmt.Fprintf(out, "  %s\n", problem)
	}
}

func formatProblems(problems []dsl.Problem) string {
	var builder strings.Builder
	for _, problem := range problems {
		if problem.Severity != dsl.SeverityError {
			continue
		}
		fmt.Fprintf(&builder, "  %s\n", problem)
	}
	return strings.TrimRight(builder.String(), "\n")
}

func printHumanResult(out io.Writer, app dsl.App, result workflow.Result, elapsed time.Duration, tracePath string) {
	fmt.Fprintf(out, "%s (%s)\n", displayName(app), app.App.Mode)
	for _, step := range result.Steps {
		line := fmt.Sprintf("[%d] %-20s %-9s %6s", step.Index, truncateText(step.NodeType, 20), step.Status, step.Duration().Round(time.Millisecond))
		if step.Branch != "" {
			line += "  branch=" + step.Branch
		}
		if step.Retries > 0 {
			line += fmt.Sprintf("  retries=%d", step.Retries)
		}
		if step.Status == workflow.StatusCompleted && len(step.Outputs) > 0 {
			line += "  fields=" + strings.Join(sortedKeys(step.Outputs), ",")
		}
		if step.Error != "" {
			line += "  error=" + step.Error
		}
		fmt.Fprintln(out, line)
	}
	fmt.Fprintf(out, "finished in %s\n", elapsed.Round(time.Millisecond))

	if len(result.Answers) > 0 {
		fmt.Fprintln(out, "answers:")
		for _, answer := range result.Answers {
			fmt.Fprintln(out, "  "+answer)
		}
	}
	if len(result.Outputs) > 0 {
		fmt.Fprintln(out, "outputs:")
		keys := make([]string, 0, len(result.Outputs))
		for key := range result.Outputs {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			fmt.Fprintf(out, "  %s: %s\n", key, formatValue(result.Outputs[key]))
		}
	}
	if tracePath != "" {
		fmt.Fprintln(out, "trace: "+tracePath)
	}
}

type workflowTrace struct {
	App     string          `json:"app"`
	Mode    string          `json:"mode"`
	Version string          `json:"version"`
	Outputs map[string]any  `json:"outputs"`
	Answers []string        `json:"answers,omitempty"`
	Steps   []workflow.Step `json:"steps"`
}

func writeWorkflowTrace(path string, app dsl.App, result workflow.Result) error {
	trace := workflowTrace{
		App:     app.App.Name,
		Mode:    app.App.Mode,
		Version: app.Version,
		Outputs: result.Outputs,
		Answers: result.Answers,
		Steps:   result.Steps,
	}
	encoded, err := json.MarshalIndent(trace, "", "  ")
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, append(encoded, '\n'), 0o644)
}

func printJSONResult(out io.Writer, app dsl.App, result workflow.Result) error {
	answers := result.Answers
	if answers == nil {
		answers = []string{}
	}
	outputs := result.Outputs
	if outputs == nil {
		outputs = map[string]any{}
	}
	payload := map[string]any{
		"app":     app.App.Name,
		"mode":    app.App.Mode,
		"outputs": outputs,
		"answers": answers,
		"steps":   result.Steps,
	}
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(out, string(encoded))
	return nil
}

func listNodesCLI(args []string) error {
	return listNodesCLIWithIO(args, os.Stdout)
}

func listNodesCLIWithIO(args []string, out io.Writer) error {
	if len(args) > 0 {
		return fmt.Errorf("nodes takes no arguments, got %v", args)
	}
	fmt.Fprintln(out, "implemented node types:")
	registry := nodes.Registry()
	for _, nodeType := range nodes.Types() {
		fmt.Fprintf(out, "  %-22s %s\n", nodeType, describeNodeType(nodeType, registry))
	}
	fmt.Fprintln(out, "\nDify node types that are deliberately not implemented:")
	unsupported := make([]string, 0, len(dsl.Unsupported))
	for nodeType := range dsl.Unsupported {
		unsupported = append(unsupported, nodeType)
	}
	sort.Strings(unsupported)
	for _, nodeType := range unsupported {
		fmt.Fprintf(out, "  %-22s %s\n", nodeType, dsl.Unsupported[nodeType])
	}
	return nil
}

func describeNodeType(nodeType string, registry map[string]nodes.Handler) string {
	switch nodeType {
	case dsl.NodeStart:
		return "workflow inputs (required/optional, typed)"
	case dsl.NodeEnd:
		return "declares workflow outputs from variable selectors"
	case dsl.NodeAnswer:
		return "renders a chatflow answer string"
	case dsl.NodeLLM:
		return "one chat completion via an OpenAI-compatible endpoint"
	case dsl.NodeCode:
		return "python3 snippet; main(**inputs) -> dict"
	case dsl.NodeIfElse:
		return "ordered cases with and/or conditions, else branch is \"false\""
	case dsl.NodeTemplate:
		return "text template over mapped variables"
	case dsl.NodeHTTP:
		return "http request with params, headers, body and auth"
	case dsl.NodeAggregate:
		return "picks the value from whichever branch ran"
	default:
		if handler, ok := registry[nodeType]; ok {
			return handler.Type()
		}
		return ""
	}
}

func displayName(app dsl.App) string {
	if strings.TrimSpace(app.App.Name) != "" {
		return app.App.Name
	}
	return "workflow"
}

func formatValue(value any) string {
	switch v := value.(type) {
	case string:
		if newline := strings.IndexByte(v, '\n'); newline >= 0 {
			return v[:newline] + "..."
		}
		return v
	case nil:
		return "null"
	default:
		encoded, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(encoded)
	}
}

func sortedKeys(source map[string]any) []string {
	keys := make([]string, 0, len(source))
	for key := range source {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// keyValueFlags collects repeatable key=value flags.
type keyValueFlags struct {
	values map[string]string
}

func newKeyValueFlags() *keyValueFlags {
	return &keyValueFlags{values: map[string]string{}}
}

func (f *keyValueFlags) String() string {
	keys := make([]string, 0, len(f.values))
	for key := range f.values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+f.values[key])
	}
	return strings.Join(parts, ",")
}

func (f *keyValueFlags) Set(value string) error {
	key, val, ok := strings.Cut(value, "=")
	if !ok || strings.TrimSpace(key) == "" {
		return fmt.Errorf("expected key=value, got %q", value)
	}
	f.values[strings.TrimSpace(key)] = val
	return nil
}
