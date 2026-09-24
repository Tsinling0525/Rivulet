// Package workflow executes a parsed DSL document.
//
// The scheduler follows Dify's semantics: every edge carries a source handle,
// a node runs once all incoming edges are resolved, nodes whose incoming edges
// were all ruled out are skipped, and independent nodes run in parallel.
package workflow

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/Tsinling0525/rivulet/dsl"
	"github.com/Tsinling0525/rivulet/expr"
	"github.com/Tsinling0525/rivulet/llmclient"
	"github.com/Tsinling0525/rivulet/nodes"
)

// Options configures a run.
type Options struct {
	// Model overrides the model configuration declared by llm nodes.
	Model *llmclient.Config
	// HTTPClient is shared by http-request nodes.
	HTTPClient *http.Client
	// WorkDir is where code nodes write their temporary scripts.
	WorkDir string
	// Concurrency is the maximum number of nodes running at once (default 4).
	Concurrency int
	// Logf receives per-node progress lines.
	Logf func(format string, args ...any)
	// Registry overrides the node registry (used by tests).
	Registry map[string]nodes.Handler
}

// Status is the outcome of one node execution.
type Status string

// Node execution statuses.
const (
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusSkipped   Status = "skipped"
)

// Step is one node execution record.
type Step struct {
	Index      int
	NodeID     string
	NodeType   string
	Title      string
	Status     Status
	Branch     string
	Retries    int
	StartedAt  time.Time
	FinishedAt time.Time
	Outputs    map[string]any
	Error      string
}

// Duration is the wall-clock time the node took.
func (s Step) Duration() time.Duration { return s.FinishedAt.Sub(s.StartedAt) }

// Result is a completed run.
type Result struct {
	Outputs map[string]any
	Answers []string
	Steps   []Step
}

const defaultConcurrency = 4

// Run validates nothing: call Validate first for a friendly report. Run returns
// an error for unknown node types, terminated node failures, or cancellation.
func Run(ctx context.Context, app dsl.App, inputs map[string]any, opts Options) (Result, error) {
	registry := opts.Registry
	if registry == nil {
		registry = nodes.Registry()
	}
	concurrency := opts.Concurrency
	if concurrency <= 0 {
		concurrency = defaultConcurrency
	}

	graph := app.Workflow.Graph
	nodesByID := make(map[string]dsl.Node, len(graph.Nodes))
	for _, node := range graph.Nodes {
		nodesByID[node.ID] = node
	}

	outgoing := make(map[string][]dsl.Edge, len(nodesByID))
	incoming := make(map[string][]dsl.Edge, len(nodesByID))
	for _, edge := range graph.Edges {
		outgoing[edge.Source] = append(outgoing[edge.Source], edge)
		incoming[edge.Target] = append(incoming[edge.Target], edge)
	}

	pool := expr.NewPool()
	pool.SetPool(expr.PoolEnvironment, app.EnvironmentValues())
	pool.SetPool(expr.PoolSystem, map[string]any{
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"user_id":   "rivulet-cli",
	})

	var (
		mu       sync.Mutex
		steps    []Step
		index    int
		outputs  = map[string]any{}
		answers  []string
		internal = make(chan nodeResult)
	)

	pending := make(map[string]int, len(nodesByID))
	traversed := make(map[string]int, len(nodesByID))
	for id := range nodesByID {
		pending[id] = len(incoming[id])
	}

	var ready []string
	for id := range nodesByID {
		if pending[id] == 0 {
			ready = append(ready, id)
		}
	}
	sort.Strings(ready)

	// resolveEdge records the outcome of an edge and returns nodes that became
	// runnable or permanently skipped.
	resolveEdge := func(edge dsl.Edge, active bool) (newlyReady []string, newlySkipped []string) {
		if active {
			traversed[edge.Target]++
		}
		pending[edge.Target]--
		if pending[edge.Target] > 0 {
			return nil, nil
		}
		if traversed[edge.Target] > 0 {
			return []string{edge.Target}, nil
		}
		// Every incoming edge was ruled out: the node never runs, and its own
		// outgoing edges are ruled out in turn.
		skipped := []string{edge.Target}
		queue := []string{edge.Target}
		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]
			for _, next := range outgoing[current] {
				pending[next.Target]--
				if pending[next.Target] > 0 {
					continue
				}
				if traversed[next.Target] > 0 {
					newlyReady = append(newlyReady, next.Target)
					continue
				}
				skipped = append(skipped, next.Target)
				queue = append(queue, next.Target)
			}
		}
		return newlyReady, skipped
	}

	markSkipped := func(ids []string) {
		now := time.Now()
		mu.Lock()
		defer mu.Unlock()
		sort.Strings(ids)
		for _, id := range ids {
			index++
			node := nodesByID[id]
			steps = append(steps, Step{
				Index: index, NodeID: id, NodeType: node.Data.Type,
				Title: titleOf(node), Status: StatusSkipped, StartedAt: now, FinishedAt: now,
			})
		}
	}

	inflight := 0
	var runErr error

	for {
		if runErr != nil {
			break
		}
		for len(ready) > 0 && inflight < concurrency {
			id := ready[0]
			ready = ready[1:]
			node := nodesByID[id]
			inflight++

			mu.Lock()
			index++
			stepIndex := index
			mu.Unlock()

			go func(node dsl.Node, stepIndex int) {
				started := time.Now()
				result := executeNode(ctx, node, stepIndex, registry, pool.Snapshot(), inputs, opts)
				result.step.StartedAt = started
				result.step.FinishedAt = time.Now()
				result.step.Index = stepIndex
				internal <- result
			}(node, stepIndex)
		}

		if inflight == 0 {
			break
		}

		result := <-internal
		inflight--

		mu.Lock()
		steps = append(steps, result.step)
		mu.Unlock()

		active := result.step.Status == StatusCompleted
		if result.step.Status == StatusFailed {
			strategy := errorStrategyOf(nodesByID[result.step.NodeID])
			if strategy == errorStrategyTerminated {
				runErr = fmt.Errorf("node %s (%s) failed: %s",
					result.step.NodeID, result.step.NodeType, result.step.Error)
				break
			}
			// continue-on-error: downstream nodes still run, and references to
			// this node resolve to its declared defaults (or empty).
			pool.Set(result.step.NodeID, failureDefaults(nodesByID[result.step.NodeID]))
			logf(opts, "node %s failed (continuing): %s", result.step.NodeID, result.step.Error)
			active = true
		}
		if result.step.Status == StatusCompleted {
			fields := result.step.Outputs
			pool.Set(result.step.NodeID, fields)
			switch nodesByID[result.step.NodeID].Data.Type {
			case dsl.NodeEnd:
				for key, value := range fields {
					outputs[key] = value
				}
			case dsl.NodeAnswer:
				if answer, ok := fields["answer"].(string); ok {
					answers = append(answers, answer)
				}
			}
		}

		branch := result.branch
		var newlyReady, newlySkipped []string
		for _, edge := range outgoing[result.step.NodeID] {
			edgeActive := active && edgeMatches(edge, branch)
			nextReady, skipped := resolveEdge(edge, edgeActive)
			newlyReady = append(newlyReady, nextReady...)
			newlySkipped = append(newlySkipped, skipped...)
		}
		markSkipped(newlySkipped)
		sort.Strings(newlyReady)
		ready = append(ready, newlyReady...)
	}

	if runErr != nil {
		return Result{}, runErr
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}

	sort.SliceStable(steps, func(i, j int) bool { return steps[i].Index < steps[j].Index })
	return Result{Outputs: outputs, Answers: answers, Steps: steps}, nil
}

type nodeResult struct {
	step   Step
	branch string
}

func executeNode(
	ctx context.Context,
	node dsl.Node,
	index int,
	registry map[string]nodes.Handler,
	pool expr.Pool,
	inputs map[string]any,
	opts Options,
) nodeResult {
	step := Step{
		Index:    index,
		NodeID:   node.ID,
		NodeType: node.Data.Type,
		Title:    titleOf(node),
	}

	handler, ok := registry[node.Data.Type]
	if !ok {
		step.Status = StatusFailed
		step.Error = fmt.Sprintf("unknown node type %q", node.Data.Type)
		return nodeResult{step: step}
	}

	request := nodes.Request{
		Node:       node,
		Mode:       "",
		Pool:       pool,
		Inputs:     inputs,
		Model:      opts.Model,
		HTTPClient: opts.HTTPClient,
		WorkDir:    opts.WorkDir,
		Logf:       opts.Logf,
	}

	retry := retryPolicyOf(node)
	var lastErr error
	for attempt := 0; attempt <= retry.maxRetries; attempt++ {
		if attempt > 0 {
			step.Retries = attempt
			logf(opts, "retrying node %s (attempt %d/%d)", node.ID, attempt+1, retry.maxRetries+1)
			if err := sleepWithContext(ctx, retry.delay(attempt)); err != nil {
				step.Status = StatusFailed
				step.Error = err.Error()
				return nodeResult{step: step}
			}
		}
		output, err := handler.Run(ctx, request)
		if err == nil {
			step.Status = StatusCompleted
			step.Outputs = output.Fields
			step.Branch = output.Branch
			return nodeResult{step: step, branch: output.Branch}
		}
		lastErr = err
	}

	step.Status = StatusFailed
	step.Error = lastErr.Error()
	return nodeResult{step: step}
}

func titleOf(node dsl.Node) string {
	if title := node.Data.Title; title != "" {
		return title
	}
	return node.Data.Type
}

func edgeMatches(edge dsl.Edge, branch string) bool {
	if branch == "" || branch == dsl.HandleSource {
		return edge.Handle() == dsl.HandleSource
	}
	return edge.Handle() == branch
}

func logf(opts Options, format string, args ...any) {
	if opts.Logf == nil {
		return
	}
	opts.Logf(format, args...)
}

func sleepWithContext(ctx context.Context, duration time.Duration) error {
	if duration <= 0 {
		return nil
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// Validate runs static checks: graph structure plus per-node configuration.
func Validate(app dsl.App, registry map[string]nodes.Handler) []dsl.Problem {
	if registry == nil {
		registry = nodes.Registry()
	}
	problems := dsl.Validate(app)
	for _, node := range app.Workflow.Graph.Nodes {
		if node.ID == "" || node.Data.Type == "" {
			continue
		}
		if reason, ok := dsl.Unsupported[node.Data.Type]; ok {
			_ = reason // already reported by dsl.Validate
			continue
		}
		handler, ok := registry[node.Data.Type]
		if !ok {
			problems = append(problems, dsl.Problem{
				Severity: dsl.SeverityError,
				NodeID:   node.ID,
				Message:  fmt.Sprintf("node type %q is not implemented by this runner", node.Data.Type),
			})
			continue
		}
		problems = append(problems, handler.Validate(node)...)
	}
	return problems
}
