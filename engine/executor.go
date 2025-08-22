package engine

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Tsinling0525/rivulet/model"
	"github.com/Tsinling0525/rivulet/plugin"
)

type Engine struct {
	Deps    plugin.Deps
	Options map[model.ID]NodeRuntimeOptions
}

func New(deps plugin.Deps) *Engine {
	return &Engine{Deps: deps, Options: map[model.ID]NodeRuntimeOptions{}}
}

// Fan-in behavior when multiple predecessors feed a node
type FanInStrategy string

const (
	FanInConcat  FanInStrategy = "concat"   // concatenate all incoming items
	FanInLatest  FanInStrategy = "latest"   // use the last predecessor only
	FanInWaitAll FanInStrategy = "wait_all" // require all predecessors, then concat
)

// Per-node runtime knobs
type NodeRuntimeOptions struct {
	Workers   int
	FanIn     FanInStrategy
	QueueSize int
	Retry     RetryPolicy
}

// Optional advanced node: can emit to multiple ports
// Nodes can implement this without importing engine
type portedProcessor interface {
	ProcessPorted(ctx context.Context, wf model.Workflow, node model.Node, in model.Items) (map[model.Port]model.Items, error)
}

// Internal helper
func successorsWithPorts(wf model.Workflow) map[model.ID][]model.Edge {
	succ := make(map[model.ID][]model.Edge)
	for _, e := range wf.Edges {
		succ[e.FromNode] = append(succ[e.FromNode], e)
	}
	return succ
}

func predecessorsWithPorts(wf model.Workflow) map[model.ID][]model.Edge {
	pred := make(map[model.ID][]model.Edge)
	for _, e := range wf.Edges {
		pred[e.ToNode] = append(pred[e.ToNode], e)
	}
	return pred
}

// chunk splits items into n nearly equal chunks
func chunk(items model.Items, n int) []model.Items {
	if n <= 1 || len(items) == 0 {
		return []model.Items{items}
	}
	if n > len(items) {
		n = len(items)
	}
	size := (len(items) + n - 1) / n
	out := make([]model.Items, 0, n)
	for i := 0; i < len(items); i += size {
		j := i + size
		if j > len(items) {
			j = len(items)
		}
		out = append(out, items[i:j])
	}
	return out
}

func (e *Engine) Run(ctx context.Context, execID string, wf model.Workflow, inputs map[model.ID]model.Items) (map[model.ID]model.Items, error) {
	order, _, _ := topo(wf)
	succ := successorsWithPorts(wf)
	pred := predecessorsWithPorts(wf)

	// inbound buffers per node/port
	inbound := make(map[model.ID]map[model.Port]model.Items)
	for id := range inputs {
		if inbound[id] == nil {
			inbound[id] = make(map[model.Port]model.Items)
		}
		inbound[id][model.PortMain] = append(inbound[id][model.PortMain], inputs[id]...)
	}

	results := map[model.ID]model.Items{}

	for _, nodeID := range order {
		var node model.Node
		for _, n := range wf.Nodes {
			if n.ID == nodeID {
				node = n
				break
			}
		}
		handler, ok := plugin.New(node.Type)
		if !ok {
			return nil, fmt.Errorf("unknown node type: %s", node.Type)
		}
		if err := handler.Init(ctx, e.Deps); err != nil {
			return nil, err
		}

		// Build input with fan-in strategy on PortMain
		opts := e.Options[nodeID]
		if opts.FanIn == "" {
			opts.FanIn = FanInConcat
		}
		workers := node.Concurrency
		if workers <= 0 {
			workers = opts.Workers
		}
		if workers <= 0 {
			workers = 1
		}

		// Collect predecessor provided items on ToPort=main
		var in model.Items
		if inbound[nodeID] != nil {
			src := inbound[nodeID][model.PortMain]
			switch opts.FanIn {
			case FanInConcat, FanInWaitAll:
				// For WaitAll, ensure all predecessors sent something (if there are predecessors)
				if opts.FanIn == FanInWaitAll {
					if len(pred[nodeID]) > 0 && len(src) == 0 {
						return nil, fmt.Errorf("wait_all: missing inputs for node %s", nodeID)
					}
				}
				in = append(in, src...)
			case FanInLatest:
				if len(src) > 0 {
					in = append(in, src...)
				}
			default:
				in = append(in, src...)
			}
		}

		runCtx := ctx
		if node.Timeout > 0 {
			var cancel context.CancelFunc
			runCtx, cancel = context.WithTimeout(ctx, node.Timeout)
			defer cancel()
		}

		e.Deps.Bus.Emit(ctx, "node_started", map[string]any{"exec": execID, "node": node.ID})

		// Per-node worker pool over chunks
		chunks := chunk(in, workers)
		outByPortTotal := make(map[model.Port]model.Items)
		var mu sync.Mutex
		wg := sync.WaitGroup{}
		var procErr error
		for _, ch := range chunks {
			wg.Add(1)
			go func(batch model.Items) {
				defer wg.Done()
				// Check for ported processor
				if pp, ok := handler.(portedProcessor); ok {
					pout, err := func() (map[model.Port]model.Items, error) {
						pol := opts.Retry.normalized()
						for attempt := 0; ; attempt++ {
							if err := runCtx.Err(); err != nil {
								return nil, err
							}
							res, err := pp.ProcessPorted(runCtx, wf, node, batch)
							if err == nil || attempt >= pol.MaxRetries {
								return res, err
							}
							time.Sleep(backoff(attempt, pol.BaseDelay, pol.MaxDelay, pol.Jitter))
						}
					}()
					if err != nil {
						mu.Lock()
						if procErr == nil {
							procErr = err
						}
						mu.Unlock()
						return
					}
					mu.Lock()
					for p, items := range pout {
						outByPortTotal[p] = append(outByPortTotal[p], items...)
					}
					mu.Unlock()
					return
				}
				// Fallback single-port with retry
				out, err := func() (model.Items, error) {
					pol := opts.Retry.normalized()
					for attempt := 0; ; attempt++ {
						if err := runCtx.Err(); err != nil {
							return nil, err
						}
						o, err := handler.Process(runCtx, wf, node, batch)
						if err == nil || attempt >= pol.MaxRetries {
							return o, err
						}
						time.Sleep(backoff(attempt, pol.BaseDelay, pol.MaxDelay, pol.Jitter))
					}
				}()
				if err != nil {
					mu.Lock()
					if procErr == nil {
						procErr = err
					}
					mu.Unlock()
					return
				}
				mu.Lock()
				outByPortTotal[model.PortMain] = append(outByPortTotal[model.PortMain], out...)
				mu.Unlock()
			}(ch)
		}
		wg.Wait()
		if procErr != nil {
			return nil, procErr
		}

		// Emit event counts for main port
		mainCount := len(outByPortTotal[model.PortMain])
		e.Deps.Bus.Emit(ctx, "node_completed", map[string]any{"exec": execID, "node": node.ID, "count": mainCount})

		// Record flat results for convenience (main port)
		results[node.ID] = append(results[node.ID], outByPortTotal[model.PortMain]...)

		// Route to successors by ports
		for _, edge := range succ[node.ID] {
			items := outByPortTotal[edge.FromPort]
			if len(items) == 0 {
				continue
			}
			if inbound[edge.ToNode] == nil {
				inbound[edge.ToNode] = make(map[model.Port]model.Items)
			}
			inbound[edge.ToNode][edge.ToPort] = append(inbound[edge.ToNode][edge.ToPort], items...)
		}
	}

	e.Deps.Bus.Emit(ctx, "execution_completed", map[string]any{"exec": execID, "at": time.Now().UTC()})
	return results, nil
}
