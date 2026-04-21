# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
make api          # Start HTTP API server + frontend (default :8080)
make run          # Run CLI server entrypoint
make build        # Build CLI binary → bin/rivulet
make api-build    # Build API binary → bin/rivulet-api
make daemon-build # Build daemon binary → bin/flowd
make test         # go test ./apps/backend/... -race -count=1
make lint         # golangci-lint run ./apps/backend/...
```

Single test: `go test ./apps/backend/engine/... -run TestXxx -race`

Custom port: `RIV_API_PORT=3000 make api`

Manual workflow execution: `./bin/rivulet run --file data/workflows/ollama_simple.json`

## Architecture

Rivulet is a workflow orchestration engine (n8n-compatible) with an HTTP API, React frontend, and Go backend. Workflows are DAGs of typed nodes executed in topological order.

### Backend (`apps/backend/`)

**Entry points** (`cmd/`): `api/` (HTTP server), `rivulet/` (CLI), `flowd/` (daemon)

**Core execution path:**
1. `cmd/api/server/server.go` — Gin router, REST endpoints, wires all infra deps
2. `infra/` — Storage layer (JSON files under `data/store/`): `workflow_store`, `run_store`, `checkpoint_store`, `review_store`, `schedule_store`, `state`, `queue`, `instances`
3. `engine/executor.go` — Topological sort, node execution, retry/timeout, event emission
4. `plugin/registry.go` + `plugin/node.go` — `NodeHandler` interface; nodes self-register via `init()`
5. `nodes/<name>/` — Built-in node implementations: `echo`, `http`, `ollama`, `openai`, `python`, `logic`, `llmroute`, `review`, `wasm`, `eval`, `files`, `fs`, `merge`

**Model types** (`model/types.go`): `Workflow`, `Node`, `Edge`, `Item`, `Run`, `Review`, `FileMeta`

**Plugin contract** — every node must implement:
```go
func (n *Node) Init(ctx context.Context, deps plugin.Deps) error
func (n *Node) Process(ctx context.Context, wf model.Workflow, node model.Node, in model.Items) (model.Items, error)
func init() { plugin.Register("type:name", func() plugin.NodeHandler { return &Node{} }) }
```

When adding a node: create `apps/backend/nodes/<name>/`, implement `NodeHandler`, register in `init()`, import the package in the server entrypoint.

### Frontend (`apps/frontend/`)

No build step — React 18.2.0 loaded from esm.sh CDN. Static files served by the Go API under `/app/*`. Edit `app.js` and `styles.css` directly; changes are visible on reload.

Product-first navigation routes: `/research`, `/create`, `/track`, `/library`, `/automations/workflows`, `/system/runs`.

### Data Layout

```
data/
  workflows/    example workflow JSON definitions
  scripts/      Python scripts used by python:script nodes
  files/<wfid>/ workflow file inputs/outputs
  store/        persisted state (workflows, runs, reviews, schedules, checkpoints)
```

### Key Environment Variables

| Variable | Default | Purpose |
|---|---|---|
| `RIV_API_PORT` | `8080` | HTTP API port |
| `RIV_DATA_DIR` | `data/` | Root for all data paths |
| `RIV_FRONTEND_DIR` | `apps/frontend` | Static frontend directory |
| `OPENAI_API_KEY` | — | Required for openai nodes |

## Conventions

- **Node type strings**: namespaced lowercase — `http:get`, `python:script`, `llm:route`, `merge.concat`
- **AI workflow metadata**: workflows can declare `ai_workflow` kind with `purpose`, `models`, `risk_level`, `human_review_required`, `workspaceType` fields
- **Workflow format**: n8n-compatible JSON with top-level `workflow` (definition) and `data` (input items) keys
- **n8n import**: `format/n8n/` converts n8n export format to internal model
- **Tests**: table-driven, files as `*_test.go` next to the package they test
- **Style**: Go 1.22, `gofmt`/`goimports`, tabs, 100-col soft limit, explicit error handling, no panics in library code
