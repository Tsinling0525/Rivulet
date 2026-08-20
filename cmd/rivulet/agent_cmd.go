package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Tsinling0525/rivulet/agent"
)

type agentCLIOptions struct {
	Provider    string
	CWD         string
	Model       string
	Endpoint    string
	Once        string
	MaxSteps    int
	ApproveMode string
	Trace       string
}

const defaultAgentMaxSteps = 48

func runAgentCLI(args []string) error {
	fs := flag.NewFlagSet("agent", flag.ExitOnError)
	provider := fs.String("provider", getenvDefault("RIVULET_AGENT_PROVIDER", "openai"), "Model provider: openai or deepseek")
	cwd := fs.String("cwd", ".", "Workspace directory")
	model := fs.String("model", getenvDefault("RIVULET_AGENT_MODEL", ""), "Model name")
	endpoint := fs.String("endpoint", getenvDefault("RIVULET_AGENT_ENDPOINT", ""), "Model API endpoint")
	once := fs.String("once", "", "Run one goal and exit")
	maxSteps := fs.Int("max-steps", defaultAgentMaxSteps, "Maximum agent loop steps per goal")
	approve := fs.String("approve", "always", "Approval mode: always or never")
	trace := fs.String("trace", "on", "Trace logging: on or off")
	if err := fs.Parse(args); err != nil {
		return err
	}

	opts := agentCLIOptions{
		Provider:    strings.TrimSpace(*provider),
		CWD:         *cwd,
		Model:       *model,
		Endpoint:    *endpoint,
		Once:        strings.TrimSpace(*once),
		MaxSteps:    *maxSteps,
		ApproveMode: *approve,
		Trace:       *trace,
	}
	return runAgentCLIWithIO(context.Background(), opts, os.Stdin, os.Stdout)
}

func runAgentCLIWithIO(ctx context.Context, opts agentCLIOptions, in io.Reader, out io.Writer) error {
	if opts.MaxSteps <= 0 {
		opts.MaxSteps = defaultAgentMaxSteps
	}
	cwd, err := filepath.Abs(opts.CWD)
	if err != nil {
		return err
	}
	info, err := os.Stat(cwd)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("cwd is not a directory: %s", cwd)
	}
	opts.CWD = cwd
	opts.ApproveMode = strings.ToLower(strings.TrimSpace(opts.ApproveMode))
	switch opts.ApproveMode {
	case "", "always":
		opts.ApproveMode = approveAlways
	case "never":
	default:
		return fmt.Errorf("unsupported approval mode %q; use always or never", opts.ApproveMode)
	}
	opts.Trace = strings.ToLower(strings.TrimSpace(opts.Trace))
	switch opts.Trace {
	case "", traceOn:
		opts.Trace = traceOn
	case traceOff:
	default:
		return fmt.Errorf("unsupported trace mode %q; use on or off", opts.Trace)
	}

	client, err := newAgentTextClient(opts)
	if err != nil {
		return err
	}
	harness := agent.Harness{
		Planner:   jsonPlanner{Client: client, CWD: opts.CWD},
		Reflector: jsonReflector{Client: client},
		Tools:     newCodingToolRegistry(opts.CWD, out, opts.ApproveMode),
		MaxSteps:  opts.MaxSteps,
	}

	if opts.Once != "" {
		return runAgentGoal(ctx, harness, opts.Once, out, opts)
	}

	fmt.Fprintln(out, "Rivulet agent MVP. Type a goal, or \"exit\" to quit.")
	scanner := bufio.NewScanner(in)
	for {
		fmt.Fprint(out, "rivulet> ")
		if !scanner.Scan() {
			break
		}
		goal := strings.TrimSpace(scanner.Text())
		switch strings.ToLower(goal) {
		case "", "exit", "quit", ":q":
			if goal == "" {
				continue
			}
			return nil
		default:
			if err := runAgentGoal(ctx, harness, goal, out, opts); err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
			}
		}
	}
	return scanner.Err()
}

func runAgentGoal(ctx context.Context, harness agent.Harness, goal string, out io.Writer, opts agentCLIOptions) error {
	fmt.Fprintf(out, "\nGoal: %s\n", goal)
	result, err := harness.Run(ctx, goal)
	if tracePath, traceErr := writeAgentTrace(opts.CWD, opts.Trace, result); traceErr != nil {
		fmt.Fprintf(out, "trace error: %v\n", traceErr)
	} else if tracePath != "" {
		fmt.Fprintf(out, "Trace: %s\n", tracePath)
	}
	for _, step := range result.Steps {
		fmt.Fprintf(out, "\n[%d] %s\n", step.Index, step.Plan.Summary)
		if step.Observation.Error != "" {
			fmt.Fprintf(out, "    error: %s\n", step.Observation.Error)
		} else if step.Observation.Summary != "" {
			fmt.Fprintf(out, "    %s\n", step.Observation.Summary)
		}
	}
	if result.FinalSummary != "" {
		fmt.Fprintf(out, "\nFinal: %s\n\n", result.FinalSummary)
	}
	return err
}

func getenvDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func newAgentTextClient(opts agentCLIOptions) (openAITextClient, error) {
	provider := strings.ToLower(strings.TrimSpace(opts.Provider))
	if provider == "" {
		provider = "openai"
	}

	switch provider {
	case "openai":
		apiKey := os.Getenv("OPENAI_API_KEY")
		if apiKey == "" {
			return openAITextClient{}, errors.New("OPENAI_API_KEY is not set")
		}
		model := firstNonEmpty(opts.Model, os.Getenv("OPENAI_MODEL"), "gpt-5-mini")
		endpoint := firstNonEmpty(opts.Endpoint, os.Getenv("OPENAI_ENDPOINT"), "https://api.openai.com/v1/responses")
		return openAITextClient{
			APIKey:          apiKey,
			Model:           model,
			Endpoint:        endpoint,
			MaxOutputTokens: 1400,
		}, nil
	case "deepseek":
		apiKey := os.Getenv("DEEPSEEK_API_KEY")
		if apiKey == "" {
			return openAITextClient{}, errors.New("DEEPSEEK_API_KEY is not set")
		}
		model := firstNonEmpty(opts.Model, os.Getenv("DEEPSEEK_MODEL"), "deepseek-v4-flash")
		endpoint := firstNonEmpty(opts.Endpoint, os.Getenv("DEEPSEEK_ENDPOINT"), "https://api.deepseek.com/chat/completions")
		return openAITextClient{
			APIKey:          apiKey,
			Model:           model,
			Endpoint:        endpoint,
			MaxOutputTokens: 4096,
			ResponseFormat:  "json_object",
			ExtraFields: map[string]any{
				"thinking": map[string]string{"type": "disabled"},
			},
		}, nil
	default:
		return openAITextClient{}, fmt.Errorf("unsupported provider %q; use openai or deepseek", provider)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
