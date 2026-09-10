package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Workflow orchestration does not live in this repository. The trigger command
// is the entire boundary: it hands a payload to an external orchestrator (an
// n8n webhook or a Dify app API) and reports what came back, so Rivulet never
// owns a DAG, a scheduler, or a node registry again.
const (
	defaultTriggerTimeoutSeconds = 30
	triggerResponseLimit         = 1 << 20
)

type triggerOptions struct {
	URL     string
	Method  string
	Body    string
	Headers []string
	Timeout time.Duration
}

// headerFlags collects repeatable --header "Name: value" arguments.
type headerFlags []string

func (h *headerFlags) String() string { return strings.Join(*h, ", ") }

func (h *headerFlags) Set(value string) error {
	*h = append(*h, value)
	return nil
}

func runTriggerCLI(args []string) error {
	fs := flag.NewFlagSet("trigger", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	url := fs.String("url", getenvDefault("RIVULET_TRIGGER_URL", ""), "External workflow URL to trigger (n8n webhook or Dify app API)")
	method := fs.String("method", "", "HTTP method (default: POST when a body is sent, GET otherwise)")
	data := fs.String("data", "", "Request body as a literal string")
	file := fs.String("file", "", "Read the request body from a file, or - for stdin")
	timeout := fs.Int("timeout", defaultTriggerTimeoutSeconds, "Request timeout in seconds")
	var headers headerFlags
	fs.Var(&headers, "header", "Additional request header, \"Name: value\" (repeatable)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if *timeout <= 0 {
		return fmt.Errorf("--timeout must be positive, got %d", *timeout)
	}

	body, err := resolveTriggerBody(*data, *file, os.Stdin)
	if err != nil {
		return err
	}
	opts := triggerOptions{
		URL:     strings.TrimSpace(*url),
		Method:  strings.TrimSpace(*method),
		Body:    body,
		Headers: headers,
		Timeout: time.Duration(*timeout) * time.Second,
	}
	if opts.URL == "" {
		return errors.New("--url is required (or set RIVULET_TRIGGER_URL)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()
	return sendTrigger(ctx, &http.Client{Timeout: opts.Timeout}, opts, os.Stdout)
}

func resolveTriggerBody(data, file string, stdin io.Reader) (string, error) {
	switch {
	case data != "" && file != "":
		return "", errors.New("use either --data or --file, not both")
	case file == "-":
		raw, err := io.ReadAll(stdin)
		if err != nil {
			return "", fmt.Errorf("read stdin: %w", err)
		}
		return string(raw), nil
	case file != "":
		raw, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("read body file: %w", err)
		}
		return string(raw), nil
	default:
		return data, nil
	}
}

func sendTrigger(ctx context.Context, client *http.Client, opts triggerOptions, out io.Writer) error {
	method := strings.ToUpper(opts.Method)
	if method == "" {
		if opts.Body != "" {
			method = http.MethodPost
		} else {
			method = http.MethodGet
		}
	}

	var body io.Reader
	if opts.Body != "" {
		body = strings.NewReader(opts.Body)
	}
	req, err := http.NewRequestWithContext(ctx, method, opts.URL, body)
	if err != nil {
		return err
	}
	if opts.Body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, header := range opts.Headers {
		name, value, ok := strings.Cut(header, ":")
		if !ok || strings.TrimSpace(name) == "" {
			return fmt.Errorf("invalid --header %q; want \"Name: value\"", header)
		}
		req.Header.Set(strings.TrimSpace(name), strings.TrimSpace(value))
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("trigger %s: %w", opts.URL, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, triggerResponseLimit+1))
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	truncated := len(raw) > triggerResponseLimit
	if truncated {
		raw = raw[:triggerResponseLimit]
	}

	fmt.Fprintf(out, "%s %s -> %s\n", method, opts.URL, resp.Status)
	if text := strings.TrimRight(string(raw), "\n"); text != "" {
		fmt.Fprintln(out, text)
	}
	if truncated {
		fmt.Fprintf(out, "(response truncated at %d bytes)\n", triggerResponseLimit)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("trigger failed with %s", resp.Status)
	}
	return nil
}
