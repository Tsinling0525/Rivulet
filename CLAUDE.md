# CLAUDE.md

This file provides guidance to Claude Code when working with this repository.

## Commands

```bash
make run       # Run the sample workflow through the CLI
make build     # Build CLI binary -> bin/rivulet
make test      # go test ./... -race -count=1
make lint      # golangci-lint run ./...
```

Single test:

```bash
go test ./engine/... -run TestXxx -race
```

Manual workflow execution:

```bash
./bin/rivulet run --file data/workflows/n8n_workflow.json
go run ./cmd/rivulet run --file data/workflows/n8n_workflow.json
```

The larger AI HTTP API and frontend product now lives under `Manifield/`.

## Architecture

Rivulet is a CLI workflow orchestration engine with n8n-compatible workflow JSON.
Workflows are DAGs of typed nodes executed in topological order.

### Backend

**Entry point**: `cmd/rivulet/`

**Core execution path:**
1. `cmd/rivulet/run.go` - reads workflow JSON, converts n8n format, runs the engine.
2. `format/n8n/` - converts n8n-style workflow requests to internal models.
3. `engine/executor.go` - topological sort, node execution, retry/timeout, event emission.
4. `plugin/registry.go` and `plugin/node.go` - `NodeHandler` interface and registry.
5. `nodes/<name>/` - built-in node implementations.
6. `infra/` - local files, stores, queues, state, and test helpers.

**Plugin contract:**

```go
func (n *Node) Init(ctx context.Context, deps plugin.Deps) error
func (n *Node) Process(ctx context.Context, wf model.Workflow, node model.Node, in model.Items) (model.Items, error)
func init() { plugin.Register("type:name", func() plugin.NodeHandler { return &Node{} }) }
```

When adding a node, create `nodes/<name>/`, implement `NodeHandler`,
register it in `init()`, and import it in `cmd/rivulet/main.go`.

### Data Layout

```text
data/
  workflows/    example workflow JSON definitions
  scripts/      Python scripts used by python:script nodes
  files/<wfid>/ workflow file inputs/outputs
  store/        local persisted state used by supporting infra
```

### Key Environment Variables

| Variable | Default | Purpose |
|---|---|---|
| `RIV_DATA_DIR` | `data/` | Root for data paths |
| `OPENAI_API_KEY` | - | Required for OpenAI-backed nodes |

## Conventions

- Node type strings are namespaced lowercase, for example `http:get`, `python:script`, `llm:route`, `merge.concat`.
- Workflow format is n8n-compatible JSON with top-level `workflow` and `data` keys.
- Tests are table-driven when practical and live next to the package they test.
- Style is Go 1.22, `gofmt`/`goimports`, tabs, 100-col soft limit, explicit error handling, no panics in library code.
