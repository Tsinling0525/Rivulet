# Rivulet

Rivulet is evolving into an all-in-one AI product for personal and operator-facing work across multiple domains: research, content creation, and personal tracking.

The workflow engine is still the foundation, but it is no longer the main product abstraction. Workflows, runs, nodes, traces, and review gates are orchestration primitives under the hood. The frontend is organized around user-facing capability hubs:

- **Research** - paper search, summaries, citations, and research history.
- **Create** - video generation, scripts, storyboards, captions, and exports.
- **Track** - meal logging, nutrition analysis, confidence flags, and trends.
- **Library** - durable outputs, source files, exports, and generated artifacts.
- **Automations** - advanced workflow catalog, schedules, templates, and fallback workspace.
- **System** - runs, reviews, traces, model calls, plugins, and settings.

See [docs/frontend-information-architecture.md](docs/frontend-information-architecture.md) for the frontend IA, migration notes, workspace model, and UX principles.

## Current Status

Rivulet currently ships as:

- A Go workflow engine with plugin-based nodes.
- An API server that executes and persists workflows, runs, reviews, checkpoints, and schedules.
- A static React console served by the API from `apps/frontend`.
- Built-in AI workflow metadata, model-call observability, and human review gates.
- Example workflows and scripts under `data/`.

The frontend is intentionally lightweight: `apps/frontend/index.html` loads `apps/frontend/app.js` and `apps/frontend/styles.css` directly through the Go server. There is no frontend build step yet.

## Product Architecture

```text
User-facing product
├── Home
├── Research
├── Create
├── Track
└── Library

Advanced surfaces
├── Automations
│   ├── Workflow catalog
│   ├── Fallback workflow workspace
│   └── Schedules
└── System
    ├── Runs
    ├── Reviews
    ├── Traces
    └── Settings

Engine layer
├── Workflows
├── Nodes
├── Runs
├── Events
├── Review checkpoints
└── Artifacts / files
```

The product-facing pages should use task language. The engine-facing pages can use workflow, run, node, and trace language.

## Repository Layout

```text
Rivulet/
├── apps/
│   ├── backend/
│   │   ├── cmd/
│   │   │   ├── api/       # HTTP API server
│   │   │   ├── flowd/     # daemon/service entrypoint
│   │   │   └── rivulet/   # CLI
│   │   ├── engine/        # scheduler, executor, retry, pause
│   │   ├── format/        # n8n import/parser support
│   │   ├── infra/         # stores, API deps, paths, queue, metrics
│   │   ├── model/         # workflow, node, AI, review, file types
│   │   ├── nodes/         # built-in node handlers
│   │   └── plugin/        # node interfaces and registry
│   └── frontend/          # static React console served by the API
├── data/
│   ├── workflows/         # example workflow JSON
│   ├── scripts/           # local scripts for script nodes
│   └── files/             # workflow file inputs/outputs
├── docs/
└── Makefile
```

## Quick Start

Build the CLI:

```bash
make build
```

Start the API and frontend:

```bash
make api
```

Open the console:

```text
http://localhost:8080/
```

Use a custom API port:

```bash
RIV_API_PORT=3000 make api
```

Run tests:

```bash
make test
```

Build binaries:

```bash
make build        # bin/rivulet
make daemon-build # bin/flowd
make api-build    # bin/rivulet-api
```

## Frontend Console

The frontend is served directly by the Go API.

```text
/                  static React console
/app/app.js        React application
/app/styles.css    console styles
```

Current navigation:

```text
Home
Research
Create
Track
Library
Automations
System
```

The current React app calls these existing backend endpoints:

- `GET /dashboard/metrics`
- `GET /workflows`
- `GET /workflows/files`
- `GET /runs`
- `GET /reviews`
- `POST /reviews/:id/approve`
- `POST /reviews/:id/reject`

If the local environment has no persisted workflows, runs, or reviews, the frontend renders sample fallback data so the product shape is still visible.

## Workflow Engine

Workflows remain the orchestration layer behind the product UI.

Core concepts:

- **Workflow** - saved definition with nodes, edges, metadata, and active version.
- **Node** - processing unit registered through the plugin system.
- **Run** - one execution of a workflow with input, result, events, status, and duration.
- **Event** - observable execution signal, including AI model-call events.
- **Review** - persisted human approval request created by `review:gate`.
- **Checkpoint** - paused execution state that can resume after approval.
- **Artifact/File** - durable input or output stored under the data directory.

## AI Workflows

AI workflows are first-class workflow definitions. They can declare purpose, model inventory, risk level, review requirements, and frontend workspace type.

```json
{
  "workflow": {
    "id": "paper-search",
    "name": "Paper Search + Summary",
    "kind": "ai_workflow",
    "ai": {
      "purpose": "Find papers and summarize relevant claims",
      "models": ["gpt-5-mini", "text-embedding"],
      "risk_level": "medium",
      "human_review_required": true,
      "workspaceType": "paper"
    },
    "nodes": [],
    "connections": {},
    "settings": {}
  },
  "data": {}
}
```

Supported MVP `workspaceType` values:

- `default` - generic fallback workspace for advanced automation/debug use.
- `paper` - research and paper summarization workspace.
- `video` - video production workspace.

Unknown or missing workspace types fall back to `default`.

AI nodes emit `ai_model_call` events with metadata such as provider, model, prompt hash, prompt preview, token usage, latency, status, and error details when available.

## Human Review

Use `review:gate` to pause execution and require human approval before downstream nodes continue.

```json
{
  "id": "review",
  "name": "Review Draft",
  "type": "review:gate",
  "parameters": {
    "output_field": "output",
    "context_fields": ["prompt", "model"]
  }
}
```

Behavior:

- Creates a pending review request.
- Persists a checkpoint.
- Pauses the run.
- Approval resumes from the checkpoint.
- Rejection cancels the paused run.

Set `pass_through: true` only when downstream execution should continue while review annotations are recorded separately.

## Running Workflows

Run an example workflow from disk:

```bash
./bin/rivulet run --file data/workflows/n8n_workflow.json
```

Other examples:

```bash
./bin/rivulet run --file data/workflows/ollama_simple.json
./bin/rivulet run --file data/workflows/template_chatgpt_prompt.json
./bin/rivulet run --file data/workflows/image_to_latex.json
```

Start a workflow through the API:

```bash
curl -X POST http://localhost:8080/workflow/start \
  -H 'Content-Type: application/json' \
  -d '{
    "workflow": {
      "id": "echo-test",
      "name": "Echo Test",
      "nodes": [
        {
          "id": "echo1",
          "name": "Echo Node",
          "type": "echo",
          "typeVersion": 1.0,
          "position": [100, 100],
          "parameters": {
            "label": "Hello World"
          }
        }
      ],
      "connections": {},
      "settings": {}
    },
    "data": {
      "echo1": [{"message": "test"}]
    }
  }'
```

Check server health:

```bash
curl http://localhost:8080/health
```

## API Surface

Current API endpoints:

```text
GET    /health
GET    /dashboard/metrics

POST   /workflow/start

GET    /workflows/files
GET    /workflows
POST   /workflows
GET    /workflows/:id
POST   /workflows/:id/versions
POST   /workflows/:id/activate
GET    /workflows/:id/versions/:version
POST   /workflows/:id/prompts/:node_id/rollback

GET    /runs
GET    /runs/:id
POST   /runs/:id/replay

GET    /schedules
POST   /schedules
GET    /schedules/:id
POST   /schedules/:id/pause
POST   /schedules/:id/resume
DELETE /schedules/:id

GET    /reviews
GET    /reviews/:id
POST   /reviews/:id/approve
POST   /reviews/:id/reject

POST   /instances
GET    /instances
GET    /instances/:id
POST   /instances/:id/stop
GET    /instances/:id/logs
POST   /instances/:id/enqueue

POST   /api/chat/ollama
```

Persisted workflow versions, run history, interval schedules, human review requests, and paused execution checkpoints are stored under `data/store/`. Managed instances are currently in-memory.

## Built-In Nodes

Current built-in node types include:

- `echo` - echo a label into the item.
- `http:get` - fetch a URL into `body` and `status`.
- `http:request` - send JSON or multipart requests with optional polling.
- `files:load` - load attached files into item fields.
- `fs:write` - write a field to disk.
- `logic:if` - route items to `true` or `false`.
- `merge.concat` - pass-through merge node.
- `ollama` - call a local Ollama model.
- `chatgpt` - call OpenAI Responses API by default, with legacy Chat Completions compatibility when configured.
- `llm:route` - choose a model route based on task complexity and optional semantic cache.
- `eval:node` - score upstream AI output with deterministic criteria or a judge model.
- `review:gate` - pause for human approval.
- `wasm:node` - run a WASI WebAssembly module as a node.
- `python:script` - run a local Python script over an attached file.

### Dynamic LLM Routing

```json
{
  "id": "router1",
  "type": "llm:route",
  "name": "Cost Aware LLM",
  "parameters": {
    "prompt": "Answer this task: {{.task}}",
    "difficulty_field": "task_type",
    "complexity_threshold": 0.55,
    "semantic_cache": {
      "enabled": true,
      "threshold": 0.92
    },
    "routes": {
      "simple": {
        "provider": "ollama",
        "model": "llama3.2",
        "endpoint": "http://localhost:11434/api/generate"
      },
      "complex": {
        "provider": "openai",
        "model": "gpt-4o",
        "api_key_env": "OPENAI_API_KEY"
      }
    }
  }
}
```

Semantic cache entries are stored in `data/store/semantic_cache.json` unless `RIV_SEMANTIC_CACHE_PATH` is set.

### Python File Processing

The `python:script` node runs local scripts against files stored under `data/files/<workflow_id>/`.

```json
{
  "id": "py1",
  "type": "python:script",
  "name": "ToLaTeX",
  "parameters": {
    "script": "data/scripts/img_to_latex.py",
    "file_id_field": "file_id",
    "output_field": "latex",
    "python_bin": "python3"
  }
}
```

Input items should include the configured file ID field:

```json
{
  "data": {
    "py1": [{ "file_id": "sample-image" }]
  }
}
```

File layout:

```text
data/files/<workflow_id>/
├── sample-image
└── sample-image.json
```

## Plugin System

Custom nodes implement `plugin.NodeHandler` and register themselves in `init()`.

```go
package mynode

import (
	"context"

	"github.com/Tsinling0525/rivulet/model"
	"github.com/Tsinling0525/rivulet/plugin"
)

type MyNode struct{}

func (n *MyNode) Init(ctx context.Context, deps plugin.Deps) error {
	return nil
}

func (n *MyNode) Process(ctx context.Context, wf model.Workflow, node model.Node, in model.Items) (model.Items, error) {
	out := make(model.Items, 0, len(in))
	for _, item := range in {
		out = append(out, model.Item{
			"processed": true,
			"input":     item,
		})
	}
	return out, nil
}

func init() {
	plugin.Register("my:node", func() plugin.NodeHandler {
		return &MyNode{}
	})
}
```

When adding nodes:

1. Create a package under `apps/backend/nodes/<name>/`.
2. Implement `plugin.NodeHandler`.
3. Register the node in `init()`.
4. Add tests next to the package.
5. Import the package from the API/CLI entrypoint if it should be built in.

## Configuration

Environment variables:

```text
RIV_API_PORT                 API port, default 8080
RIV_DATA_DIR                 Data root for workflows, scripts, files, and store
RIV_FRONTEND_DIR             Static frontend directory, default apps/frontend
RIV_SEMANTIC_CACHE_PATH      Optional semantic cache path
OPENAI_API_KEY               Required for OpenAI-backed nodes
```

Default data paths:

```text
data/workflows
data/scripts
data/files/<workflow_id>
data/store
```

## Development

Use `rg` for code search and keep changes scoped.

Common commands:

```bash
make api          # run API + frontend
make run          # run CLI server entrypoint
make test         # go test ./apps/backend/... -race -count=1
make lint         # golangci-lint if installed
make build        # build CLI
make daemon-build # build flowd
make api-build    # build API server
```

Optional manual run:

```bash
go run ./apps/backend/cmd/rivulet run --file data/workflows/n8n_workflow.json
```

## Design Notes

The current frontend should keep this separation:

- Domain pages own product language and task interaction.
- Library owns user-facing outputs.
- Automations owns workflow definitions and fallback workspace.
- System owns runs, reviews, traces, model calls, and settings.
- The workflow engine remains stable underneath those surfaces.

The next frontend infrastructure step is to introduce a build system such as Vite so React dependencies are bundled locally instead of imported from a CDN at runtime.
