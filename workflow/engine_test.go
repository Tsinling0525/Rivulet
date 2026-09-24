package workflow

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tsinling0525/rivulet/dsl"
)

func mustApp(t *testing.T, document string) dsl.App {
	t.Helper()
	app, err := dsl.Parse([]byte(document))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return app
}

func stepByID(t *testing.T, result Result, nodeID string) Step {
	t.Helper()
	for _, step := range result.Steps {
		if step.NodeID == nodeID {
			return step
		}
	}
	t.Fatalf("no step for node %q in %v", nodeID, stepIDs(result))
	return Step{}
}

func stepIDs(result Result) []string {
	out := make([]string, 0, len(result.Steps))
	for _, step := range result.Steps {
		out = append(out, fmt.Sprintf("%s:%s", step.NodeID, step.Status))
	}
	return out
}

const linearWorkflow = `
version: "0.6.0"
kind: app
app: {name: Linear, mode: workflow}
workflow:
  graph:
    nodes:
      - id: start_node
        data:
          type: start
          variables:
            - variable: name
              required: true
      - id: template_node
        data:
          type: template-transform
          template: "Hello {{ arg1 }}"
          variables:
            - variable: arg1
              value_selector: [start_node, name]
      - id: end_node
        data:
          type: end
          outputs:
            - variable: greeting
              value_selector: [template_node, output]
    edges:
      - {source: start_node, target: template_node}
      - {source: template_node, target: end_node}
`

func TestRunLinearWorkflow(t *testing.T) {
	result, err := Run(context.Background(), mustApp(t, linearWorkflow), map[string]any{"name": "Ada"}, Options{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.Outputs["greeting"] != "Hello Ada" {
		t.Fatalf("outputs = %#v", result.Outputs)
	}
	if len(result.Steps) != 3 {
		t.Fatalf("steps = %v", stepIDs(result))
	}
	for index, step := range result.Steps {
		if step.Status != StatusCompleted {
			t.Fatalf("step %d status = %s (%s)", index, step.Status, step.Error)
		}
		if step.Index != index+1 {
			t.Fatalf("step index = %d, want %d", step.Index, index+1)
		}
	}
	if stepByID(t, result, "template_node").Outputs["output"] != "Hello Ada" {
		t.Fatalf("template outputs = %#v", stepByID(t, result, "template_node").Outputs)
	}
}

func TestRunRejectsMissingRequiredInput(t *testing.T) {
	_, err := Run(context.Background(), mustApp(t, linearWorkflow), nil, Options{})
	if err == nil || !strings.Contains(err.Error(), "missing required input") {
		t.Fatalf("unexpected error: %v", err)
	}
}

const branchWorkflow = `
version: "0.6.0"
kind: app
app: {name: Branch, mode: workflow}
workflow:
  graph:
    nodes:
      - id: start_node
        data: {type: start, variables: [{variable: count, required: true}]}
      - id: cond_node
        data:
          type: if-else
          cases:
            - case_id: "big"
              conditions:
                - variable_selector: [start_node, count]
                  comparison_operator: ">"
                  value: 5
      - id: big_node
        data:
          type: template-transform
          template: "many"
      - id: small_node
        data:
          type: template-transform
          template: "few"
      - id: end_node
        data:
          type: end
          outputs:
            - variable: result
              value_selector: [aggregate_node, output]
      - id: aggregate_node
        data:
          type: variable-aggregator
          output_type: string
          variables:
            - [big_node, output]
            - [small_node, output]
    edges:
      - {source: start_node, target: cond_node}
      - {source: cond_node, sourceHandle: "big", target: big_node}
      - {source: cond_node, sourceHandle: "false", target: small_node}
      - {source: big_node, target: aggregate_node}
      - {source: small_node, target: aggregate_node}
      - {source: aggregate_node, target: end_node}
`

func TestRunTakesOnlyTheMatchingBranch(t *testing.T) {
	result, err := Run(context.Background(), mustApp(t, branchWorkflow), map[string]any{"count": 9}, Options{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.Outputs["result"] != "many" {
		t.Fatalf("outputs = %#v", result.Outputs)
	}
	if status := stepByID(t, result, "small_node").Status; status != StatusSkipped {
		t.Fatalf("small_node status = %s, want skipped", status)
	}
	if status := stepByID(t, result, "big_node").Status; status != StatusCompleted {
		t.Fatalf("big_node status = %s, want completed", status)
	}
	if branch := stepByID(t, result, "cond_node").Branch; branch != "big" {
		t.Fatalf("branch = %q", branch)
	}
}

func TestRunTakesElseBranch(t *testing.T) {
	result, err := Run(context.Background(), mustApp(t, branchWorkflow), map[string]any{"count": 1}, Options{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.Outputs["result"] != "few" {
		t.Fatalf("outputs = %#v", result.Outputs)
	}
	if status := stepByID(t, result, "big_node").Status; status != StatusSkipped {
		t.Fatalf("big_node status = %s, want skipped", status)
	}
}

const skippedChainWorkflow = `
version: "0.6.0"
kind: app
app: {name: Chain, mode: workflow}
workflow:
  graph:
    nodes:
      - id: start_node
        data: {type: start, variables: [{variable: flag, required: true}]}
      - id: cond_node
        data:
          type: if-else
          cases:
            - case_id: "yes"
              conditions:
                - variable_selector: [start_node, flag]
                  comparison_operator: is
                  value: "on"
      - id: unused_first
        data: {type: template-transform, template: "unused"}
      - id: unused_second
        data: {type: template-transform, template: "unused too"}
      - id: used
        data: {type: template-transform, template: "used"}
      - id: end_node
        data:
          type: end
          outputs:
            - variable: result
              value_selector: [used, output]
    edges:
      - {source: start_node, target: cond_node}
      - {source: cond_node, sourceHandle: "yes", target: used}
      - {source: cond_node, sourceHandle: "false", target: unused_first}
      - {source: unused_first, target: unused_second}
      - {source: used, target: end_node}
      - {source: unused_second, target: end_node}
`

func TestRunSkipsWholeUntakenChain(t *testing.T) {
	result, err := Run(context.Background(), mustApp(t, skippedChainWorkflow), map[string]any{"flag": "on"}, Options{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	for _, id := range []string{"unused_first", "unused_second"} {
		if status := stepByID(t, result, id).Status; status != StatusSkipped {
			t.Fatalf("%s status = %s, want skipped", id, status)
		}
	}
	if result.Outputs["result"] != "used" {
		t.Fatalf("outputs = %#v", result.Outputs)
	}
}

func TestRunExecutesIndependentNodesConcurrently(t *testing.T) {
	var inFlight, maxInFlight int32
	release := make(chan struct{})
	var once sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := atomic.AddInt32(&inFlight, 1)
		for {
			previous := atomic.LoadInt32(&maxInFlight)
			if current <= previous || atomic.CompareAndSwapInt32(&maxInFlight, previous, current) {
				break
			}
		}
		if current == 2 {
			once.Do(func() { close(release) })
		}
		select {
		case <-release:
		case <-time.After(2 * time.Second):
		}
		atomic.AddInt32(&inFlight, -1)
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer server.Close()

	document := fmt.Sprintf(`
version: "0.6.0"
kind: app
app: {name: Parallel, mode: workflow}
workflow:
  graph:
    nodes:
      - id: start_node
        data: {type: start}
      - id: left_node
        data: {type: http-request, url: "%[1]s/left"}
      - id: right_node
        data: {type: http-request, url: "%[1]s/right"}
      - id: end_node
        data: {type: end}
    edges:
      - {source: start_node, target: left_node}
      - {source: start_node, target: right_node}
      - {source: left_node, target: end_node}
      - {source: right_node, target: end_node}
`, server.URL)

	result, err := Run(context.Background(), mustApp(t, document), nil, Options{Concurrency: 2})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if atomic.LoadInt32(&maxInFlight) < 2 {
		t.Fatalf("max in-flight requests = %d, want both nodes to run at once", maxInFlight)
	}
	if len(result.Steps) != 4 {
		t.Fatalf("steps = %v", stepIDs(result))
	}
}

func TestRunHonoursConcurrencyLimit(t *testing.T) {
	var inFlight, maxInFlight int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := atomic.AddInt32(&inFlight, 1)
		for {
			previous := atomic.LoadInt32(&maxInFlight)
			if current <= previous || atomic.CompareAndSwapInt32(&maxInFlight, previous, current) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		atomic.AddInt32(&inFlight, -1)
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer server.Close()

	document := fmt.Sprintf(`
version: "0.6.0"
kind: app
app: {name: Serial, mode: workflow}
workflow:
  graph:
    nodes:
      - id: start_node
        data: {type: start}
      - id: a
        data: {type: http-request, url: "%[1]s/a"}
      - id: b
        data: {type: http-request, url: "%[1]s/b"}
      - id: c
        data: {type: http-request, url: "%[1]s/c"}
      - id: end_node
        data: {type: end}
    edges:
      - {source: start_node, target: a}
      - {source: start_node, target: b}
      - {source: start_node, target: c}
      - {source: a, target: end_node}
      - {source: b, target: end_node}
      - {source: c, target: end_node}
`, server.URL)

	if _, err := Run(context.Background(), mustApp(t, document), nil, Options{Concurrency: 1}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := atomic.LoadInt32(&maxInFlight); got != 1 {
		t.Fatalf("max in-flight = %d, want 1 with --concurrency 1", got)
	}
}

func TestRunTerminatedFailureAborts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	document := fmt.Sprintf(`
version: "0.6.0"
kind: app
app: {name: Failing, mode: workflow}
workflow:
  graph:
    nodes:
      - id: start_node
        data: {type: start}
      - id: http_node
        data: {type: http-request, url: "%s"}
      - id: end_node
        data: {type: end}
    edges:
      - {source: start_node, target: http_node}
      - {source: http_node, target: end_node}
`, server.URL)

	_, err := Run(context.Background(), mustApp(t, document), nil, Options{})
	if err == nil || !strings.Contains(err.Error(), "http_node") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunContinuesOnErrorWithDefaults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	document := fmt.Sprintf(`
version: "0.6.0"
kind: app
app: {name: Tolerant, mode: workflow}
workflow:
  graph:
    nodes:
      - id: start_node
        data: {type: start}
      - id: http_node
        data:
          type: http-request
          url: "%s"
          error_strategy: continue-on-error
          default_value: {status_code: -1}
      - id: template_node
        data:
          type: template-transform
          template: "status={{#http_node.status_code#}} body={{#http_node.body#}}"
      - id: end_node
        data:
          type: end
          outputs:
            - variable: message
              value_selector: [template_node, output]
    edges:
      - {source: start_node, target: http_node}
      - {source: http_node, target: template_node}
      - {source: template_node, target: end_node}
`, server.URL)

	result, err := Run(context.Background(), mustApp(t, document), nil, Options{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if status := stepByID(t, result, "http_node").Status; status != StatusFailed {
		t.Fatalf("http_node status = %s, want failed", status)
	}
	if result.Outputs["message"] != "status=-1 body=" {
		t.Fatalf("outputs = %#v", result.Outputs)
	}
}

func TestRunRetriesFailingNode(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&attempts, 1) < 3 {
			http.Error(w, "flaky", http.StatusBadGateway)
			return
		}
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer server.Close()

	document := fmt.Sprintf(`
version: "0.6.0"
kind: app
app: {name: Retry, mode: workflow}
workflow:
  graph:
    nodes:
      - id: start_node
        data: {type: start}
      - id: http_node
        data:
          type: http-request
          url: "%s"
          retry_config:
            enabled: true
            max_retries: 3
            retry_interval: 1
      - id: end_node
        data: {type: end}
    edges:
      - {source: start_node, target: http_node}
      - {source: http_node, target: end_node}
`, server.URL)

	result, err := Run(context.Background(), mustApp(t, document), nil, Options{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	step := stepByID(t, result, "http_node")
	if step.Status != StatusCompleted || step.Retries != 2 {
		t.Fatalf("step = %+v, want completed after 2 retries", step)
	}
	if atomic.LoadInt32(&attempts) != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
}

func TestRunFailsAfterRetriesExhausted(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		http.Error(w, "always broken", http.StatusInternalServerError)
	}))
	defer server.Close()

	document := fmt.Sprintf(`
version: "0.6.0"
kind: app
app: {name: Exhausted, mode: workflow}
workflow:
  graph:
    nodes:
      - id: start_node
        data: {type: start}
      - id: http_node
        data:
          type: http-request
          url: "%s"
          retry_config: {enabled: true, max_retries: 2, retry_interval: 1}
      - id: end_node
        data: {type: end}
    edges:
      - {source: start_node, target: http_node}
      - {source: http_node, target: end_node}
`, server.URL)

	if _, err := Run(context.Background(), mustApp(t, document), nil, Options{}); err == nil {
		t.Fatal("expected the run to fail")
	}
	if atomic.LoadInt32(&attempts) != 3 {
		t.Fatalf("attempts = %d, want 3 (initial + 2 retries)", attempts)
	}
}

func TestRunUnknownNodeType(t *testing.T) {
	document := `
version: "0.6.0"
kind: app
app: {name: Unknown, mode: workflow}
workflow:
  graph:
    nodes:
      - id: start_node
        data: {type: start}
      - id: mystery
        data: {type: mystery-node}
      - id: end_node
        data: {type: end}
    edges:
      - {source: start_node, target: mystery}
      - {source: mystery, target: end_node}
`
	_, err := Run(context.Background(), mustApp(t, document), nil, Options{})
	if err == nil || !strings.Contains(err.Error(), "unknown node type") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunRecordsBranchAndLogLines(t *testing.T) {
	var logs []string
	app := mustApp(t, branchWorkflow)
	result, err := Run(context.Background(), app, map[string]any{"count": 7}, Options{
		Logf: func(format string, args ...any) { logs = append(logs, fmt.Sprintf(format, args...)) },
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(logs) == 0 || !strings.Contains(strings.Join(logs, "\n"), "case \"big\" matched") {
		t.Fatalf("logs = %v", logs)
	}
	if result.Steps[0].StartedAt.IsZero() || result.Steps[0].FinishedAt.IsZero() {
		t.Fatal("step timestamps were not recorded")
	}
}

func TestValidateCombinesGraphAndNodeProblems(t *testing.T) {
	document := `
version: "0.6.0"
kind: app
app: {name: Invalid, mode: workflow}
workflow:
  graph:
    nodes:
      - id: start_node
        data: {type: start}
      - id: code_node
        data: {type: code, code: "x", code_language: javascript}
      - id: end_node
        data: {type: end}
    edges:
      - {source: start_node, target: code_node}
      - {source: code_node, target: end_node}
`
	problems := Validate(mustApp(t, document), nil)
	if !dsl.HasErrors(problems) {
		t.Fatalf("expected errors, got %v", problems)
	}
	if !strings.Contains(fmt.Sprint(problems), "javascript") {
		t.Fatalf("problems = %v, want the node-level problem included", problems)
	}
}

func TestValidateReportsUnimplementedNodeType(t *testing.T) {
	document := `
version: "0.6.0"
kind: app
app: {name: Unimplemented, mode: workflow}
workflow:
  graph:
    nodes:
      - id: start_node
        data: {type: start}
      - id: tool_node
        data: {type: tool}
      - id: end_node
        data: {type: end}
    edges:
      - {source: start_node, target: end_node}
`
	problems := Validate(mustApp(t, document), nil)
	if !dsl.HasErrors(problems) || !strings.Contains(fmt.Sprint(problems), "tool") {
		t.Fatalf("problems = %v", problems)
	}
}

func TestRunRespectsCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Run(ctx, mustApp(t, linearWorkflow), map[string]any{"name": "Ada"}, Options{}); err == nil {
		t.Fatal("expected a cancelled context to abort the run")
	}
}

func TestRetryPolicyDelay(t *testing.T) {
	policy := retryPolicy{interval: 10 * time.Millisecond, multiplier: 2, maxDelay: 25 * time.Millisecond}
	if got := policy.delay(1); got != 10*time.Millisecond {
		t.Fatalf("first delay = %s", got)
	}
	if got := policy.delay(2); got != 20*time.Millisecond {
		t.Fatalf("second delay = %s", got)
	}
	if got := policy.delay(3); got != 25*time.Millisecond {
		t.Fatalf("delay should be capped: %s", got)
	}
	if got := (retryPolicy{}).delay(1); got != 0 {
		t.Fatalf("zero interval should not sleep, got %s", got)
	}
}

func TestEdgeMatchesBranch(t *testing.T) {
	tests := []struct {
		handle string
		branch string
		want   bool
	}{
		{dsl.HandleSource, "", true},
		{dsl.HandleSource, dsl.HandleSource, true},
		{dsl.HandleTrue, "", false},
		{dsl.HandleTrue, dsl.HandleTrue, true},
		{dsl.HandleFalse, dsl.HandleTrue, false},
	}
	for _, tc := range tests {
		edge := dsl.Edge{Source: "a", Target: "b", SourceHandle: tc.handle}
		if got := edgeMatches(edge, tc.branch); got != tc.want {
			t.Fatalf("edgeMatches(handle=%q, branch=%q) = %v, want %v", tc.handle, tc.branch, got, tc.want)
		}
	}
}
