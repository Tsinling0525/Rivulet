# Rivulet

A lightweight, n8n-inspired workflow engine written in Go. Rivulet provides a node-based workflow system with plugin architecture, state management, and event-driven execution.

Rivulet treats AI workflows as a first-class workflow kind, not just generic automation with an LLM HTTP call. Workflows can declare `kind: "ai_workflow"` and AI metadata such as purpose, model inventory, risk level, and whether human review is required. AI nodes emit `ai_model_call` run events with provider/model, prompt hash, prompt preview, token usage when available, latency, and status.

## 🚀 Features

- **Node-based Workflows** - Visual workflow composition with nodes and edges
- **Plugin Architecture** - Extensible node system for custom functionality
- **AI Workflow Identity** - First-class AI metadata for purpose, models, risk, and review policy
- **AI Observability** - Model-call events capture provider/model, prompt hash, tokens, latency, and status
- **Human Review Gates** - `review:gate` nodes pause AI workflows for auditable approve/resume or reject decisions
- **State Management** - Persistent node state across workflow executions
- **Event-driven** - Observable execution flow with event bus
- **Type-safe** - Strong typing with Go's type system
- **High Performance** - Compiled language efficiency for workflow execution
- **Concurrent Execution** - Parallel node processing capabilities

## 🏗️ Architecture

```
Rivulet/
├── apps/
│   ├── backend/        # Go services (daemon, API server, CLI)
│   │   ├── cmd/        # Entrypoints: flowd/, api/, rivulet/
│   │   ├── engine/     # Core execution engine
│   │   ├── infra/      # Infrastructure components
│   │   ├── model/      # Data structures and types
│   │   ├── nodes/      # Built-in nodes
│   │   └── plugin/     # Plugin system interfaces
│   └── frontend/       # Static dashboard UI served by the API
├── data/               # Example workflows, scripts, files
└── go.work             # Go workspace definition
```

## 🎯 Quick Start

### 1. Install and Build

```bash
# Clone the repository
git clone https://github.com/Tsinling0525/rivulet.git
cd rivulet

# Build the CLI
make build
```

### 2. Start the API Server

```bash
# Start server on default port 8080
./bin/rivulet server

# Or specify custom port
RIV_API_PORT=3000 ./bin/rivulet server
```

The server provides n8n-compatible workflow execution endpoints and serves the dashboard UI at [http://localhost:8080/](http://localhost:8080/). The dashboard consumes the `/dashboard/metrics` API to visualise live workflow stats.

### 3. Run a Workflow from File

Try the included example workflows:

```bash
# Run echo workflow
./bin/rivulet run --file data/workflows/n8n_workflow.json

# Run Ollama AI workflow (requires Ollama installed)
./bin/rivulet run --file data/workflows/ollama_simple.json

# Run OpenAI chat workflow (requires OPENAI_API_KEY)
./bin/rivulet run --file data/workflows/template_chatgpt_prompt.json

# Run Python file processing workflow
./bin/rivulet run --file data/workflows/image_to_latex.json
```

### 4. Execute via API

Start a workflow directly with the API:

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
            "label": "Hello World!"
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

AI workflows can declare intent and review policy directly on the workflow:

```json
{
  "workflow": {
    "id": "support-triage",
    "name": "Support Triage",
    "kind": "ai_workflow",
    "ai": {
      "purpose": "Classify support tickets and draft replies",
      "models": ["gpt-5-mini"],
      "risk_level": "medium",
      "human_review_required": true
    },
    "nodes": [
      {
        "id": "draft",
        "name": "Draft Reply",
        "type": "chatgpt",
        "parameters": {
          "model": "gpt-5-mini",
          "prompt": "Draft a reply for {{.message}}",
          "human_review_required": true
        }
      }
    ],
    "connections": {},
    "settings": {}
  },
  "data": {
    "draft": [{ "message": "I need help with my invoice." }]
  }
}
```

To create an explicit human review step, add a `review:gate` node after an AI node. By default it records a pending review, persists an execution checkpoint, and pauses the run. Approving the review resumes downstream execution from the checkpoint; rejecting the review marks the paused run as cancelled. Set `pass_through: true` only when you want downstream nodes to continue with review annotations while the decision is audited separately.

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

### 5. Check Health

```bash
curl http://localhost:8080/health
```

### 6. Example Workflow Files

The `data/workflows/` directory contains example workflows:

- **n8n_workflow.json** - Simple echo workflow with connections
- **ollama_simple.json** - AI workflow using local Ollama LLM
- **template_chatgpt_prompt.json** - AI workflow using the `chatgpt` node
- **image_to_latex.json** - Python script workflow for file processing

### 7. Development Mode

For development with auto-restart:

```bash
# Start the API + dashboard
make run

# Run tests
make test

# Run workflow once
go run ./apps/backend/cmd/rivulet run --file data/workflows/n8n_workflow.json
```

### 8. Python File Processing

Rivulet includes a powerful Python script node for file processing. Here's how to use it:

#### Setup File Processing Workflow

The `image_to_latex.json` example demonstrates file processing:

```json
{
  "workflow": {
    "nodes": [
      {
        "id": "convert",
        "type": "python:script",
        "parameters": {
          "script": "data/scripts/img_to_latex.py",
          "file_id_field": "file_id",
          "output_field": "latex"
        }
      }
    ]
  },
  "data": {
    "start": [{"file_id": "sample-image"}]
  }
}
```

#### File Structure

Files are stored with metadata in `data/files/<workflow_id>/`:

```bash
data/files/image_to_latex_workflow/
├── sample-image          # The actual file
└── sample-image.json     # File metadata
```

### 9. Dashboard Metrics

- Visit `http://localhost:8080/` after running `./bin/rivulet server` to open the FlowTracker dashboard powered by `apps/frontend/index.html`.
- The UI is served directly by the Go API (configurable via `RIV_FRONTEND_DIR`) so the backend and frontend ship together in the monorepo.
- The dashboard view calls `/dashboard/metrics`, which aggregates execution stats from `infra.InstanceManager`—success/fail counts, queue depth, and per-instance timings.
- The workflows view calls `/workflows/files`, `/instances`, `/instances/:id`, `/instances/:id/logs`, `/instances/:id/enqueue`, `/runs`, `/schedules`, and `/reviews` so you can create instances, enqueue sample data, inspect recent persisted runs, replay a prior execution, manage interval schedules, and approve/resume or reject AI review requests from the browser.
- Extend the cards by enhancing `infra.DashboardMetrics()` and updating the frontend HTML, or replace the static assets with a compiled SPA that targets the same endpoint.

### 10. Current API Surface

The API currently exposes:

- `GET /health`
- `POST /workflow/start` for one-shot execution of an n8n-style payload
- `GET /workflows/files` to list workflow JSON files under `data/workflows`
- `GET /workflows`, `POST /workflows`, `GET /workflows/:id`
- `POST /workflows/:id/versions`, `POST /workflows/:id/activate`, `GET /workflows/:id/versions/:version`
- `GET /runs`, `GET /runs/:id`, `POST /runs/:id/replay`
- `GET /schedules`, `POST /schedules`, `GET /schedules/:id`
- `POST /schedules/:id/pause`, `POST /schedules/:id/resume`, `DELETE /schedules/:id`
- `GET /reviews`, `GET /reviews/:id`
- `POST /reviews/:id/approve`, `POST /reviews/:id/reject`; approval resumes a paused checkpoint when one exists, rejection cancels the paused run
- `POST /instances`, `GET /instances`, `GET /instances/:id`
- `POST /instances/:id/stop`, `GET /instances/:id/logs`, `POST /instances/:id/enqueue`
- `GET /dashboard/metrics`

Persisted workflow versions, run history, interval schedules, human review requests, and paused execution checkpoints are stored on disk under `data/store/`. Managed instances themselves remain in-memory.

#### Python Script Example

The Python script (`data/scripts/img_to_latex.py`) receives the file path as an argument:

```python
#!/usr/bin/env python3
import os
import sys

def main():
    if len(sys.argv) < 2:
        sys.stderr.write("Usage: img_to_latex.py <image_path>\n")
        sys.exit(1)
    
    img_path = sys.argv[1]
    name = os.path.basename(img_path)
    
    # Generate LaTeX that includes the image
    tex = f"""\\documentclass{{article}}
\\usepackage{{graphicx}}
\\begin{{document}}
\\includegraphics[width=\\linewidth]{{{name}}}
\\end{{document}}"""
    
    print(tex)  # Output goes to the "latex" field

if __name__ == "__main__":
    main()
```

#### How It Works

1. **Input**: Workflow item contains `file_id` field
2. **File Lookup**: Engine finds file in `data/files/<workflow_id>/<file_id>`
3. **Script Execution**: Python script runs with file path as argument
4. **Output**: Script's stdout becomes the specified output field (`latex`)
5. **Next Node**: Processed data flows to connected nodes

## 🔌 Built-in Nodes

- `echo` – echoes a label into the item
- `http:get` – fetch URL into `body` + `status` (templated URL)
- `http:request` – send JSON or multipart HTTP requests with optional polling
- `files:load` – load attached files into item fields
- `fs:write` – write a field to disk
- `logic:if` – routes to ports `true`/`false` based on template expression
- `merge.concat` – pass-through node (engine performs fan-in)
- `ollama` – render a prompt and call a local Ollama model
- `chatgpt` – render a prompt and call the OpenAI Responses API by default, with legacy Chat Completions compatibility when explicitly configured
- `eval:node` – score upstream AI output with deterministic criteria or an OpenAI-compatible judge model, emitting `pass`/`fail` ports
- `python:script` – run local Python script over an attached file and put stdout (e.g., LaTeX) into item

Eval node config example:

```json
{
  "id": "eval1",
  "type": "eval:node",
  "name": "Score AI Output",
  "parameters": {
    "target_field": "output",
    "pass_threshold": 0.8,
    "criteria": [
      { "name": "mentions total", "type": "contains", "value": "total", "weight": 1 },
      { "name": "no errors", "type": "not_contains", "value": "error", "weight": 1 }
    ]
  }
}
```

Add a `judge` object when scoring should be delegated to a stronger model:

```json
{
  "judge": {
    "provider": "openai",
    "model": "gpt-5",
    "api_key_env": "OPENAI_API_KEY"
  }
}
```

Python node config example:

```json
{
  "id": "py1",
  "type": "python:script",
  "name": "ToLaTeX",
  "parameters": {
    "script": "data/scripts/ocr_to_latex.py",
    "file_id_field": "file_id",
    "output_field": "latex",
    "python_bin": "python3"
  }
}
```

## 🔌 Plugin System

### Creating Custom Nodes

```go
package mynode

import (
    "context"
    "github.com/yourorg/rivulet/model"
    "github.com/yourorg/rivulet/plugin"
)

type MyNode struct{ deps plugin.Deps }

func (n *MyNode) Init(ctx context.Context, deps plugin.Deps) error {
    n.deps = deps
    return nil
}

func (n *MyNode) Process(ctx context.Context, wf model.Workflow, node model.Node, in model.Items) (model.Items, error) {
    // Process input items
    out := make(model.Items, len(in))
    for i, item := range in {
        // Transform data
        out[i] = model.Item{
            "processed": true,
            "data":      item["input"],
        }
    }
    return out, nil
}

// Register the node
func init() {
    plugin.Register("mynode", func() plugin.NodeHandler {
        return &MyNode{}
    })
}
```

### Node Configuration

```go
node := model.Node{
    ID:          "node1",
    Type:        "mynode",
    Name:        "My Custom Node",
    Concurrency: 1,                    // Parallel execution count
    Timeout:     30 * time.Second,     // Execution timeout
    Config: map[string]any{
        "api_key": "secret123",
        "endpoint": "https://api.example.com",
    },
    Credentials: "my_credentials",     // Reference to stored credentials
}
```

## 🔄 Workflow Definition

### Nodes and Edges

```go
workflow := model.Workflow{
    ID:   "data-pipeline",
    Name: "Data Processing Pipeline",
    Nodes: []model.Node{
        {ID: "fetch", Type: "http", Name: "Fetch Data"},
        {ID: "transform", Type: "transform", Name: "Transform Data"},
        {ID: "store", Type: "database", Name: "Store Result"},
    },
    Edges: []model.Edge{
        {FromNode: "fetch", FromPort: "main", ToNode: "transform", ToPort: "main"},
        {FromNode: "transform", FromPort: "main", ToNode: "store", ToPort: "main"},
    },
}
```

### Data Flow

```go
// Input data for each node
inputData := map[model.ID]model.Items{
    "fetch": {{"url": "https://api.example.com/data"}},
    "transform": {{"data": "raw_data_here"}},
    "store": {{"processed_data": "transformed_data"}},
}

// Execute workflow
result, err := engine.Run(ctx, "exec-123", workflow, inputData)
```

## 🏛️ Core Components

### Engine
Topological executor with:
- Per-node worker pools (`Concurrency` or `engine.Options`)
- Fan-in strategies (`concat`, `latest`, `wait_all`)
- Port-aware routing (`Edge.FromPort` → `Edge.ToPort`)
- Retry policy with exponential backoff and jitter

### Plugin System
Extensible interface for creating custom nodes with:
- **NodeHandler** - Core node processing interface
- **StateStore** - Persistent state management
- **EventBus** - Event emission and monitoring

### Model
Type-safe data structures:
- **Workflow** - Complete workflow definition
- **Node** - Individual workflow node
- **Edge** - Connection between nodes
- **Items** - Data flowing through nodes
 - **FileMeta** - Attached file metadata (ID, Name, Size, MediaType, CreatedAt)

## 🚀 Advanced Features

### State Management
```go
type StateStore interface {
    SaveNodeState(ctx context.Context, execID string, nodeID model.ID, state map[string]any) error
    LoadNodeState(ctx context.Context, execID string, nodeID model.ID) (map[string]any, error)
}
```

### Event Bus
```go
type EventBus interface {
    Emit(ctx context.Context, event string, fields map[string]any) error
}
```

### Concurrency Control
```go
node := model.Node{
    Concurrency: 5,  // Process 5 items in parallel
    Timeout:     60 * time.Second,
}
```

## 📦 Files, Paths and Attachments

- File attachments via `plugin.FileStore` with in-memory implementation `infra.NewMemFiles()`
- Default data directories (configurable via `RIV_DATA_DIR`):
  - Workflows: `data/workflows`
  - Scripts: `data/scripts`
  - Files: `data/files/<workflowID>`

The API server passes a `FileStore` to nodes so they can read/write files during execution.

## 🔧 Development

### Project Structure
```
├── apps/
│   ├── backend/
│   │   ├── cmd/flowd/      # Service-only entrypoint
│   │   ├── cmd/api/        # HTTP API server
│   │   ├── cmd/rivulet/    # CLI wrapper
│   │   ├── engine/         # Workflow execution engine
│   │   ├── infra/          # Infrastructure utilities
│   │   ├── model/          # Data models and types
│   │   ├── nodes/          # Built-in node handlers
│   │   └── plugin/         # Plugin system interfaces/registry
│   └── frontend/           # Static dashboard UI
├── data/                   # Example workflows, scripts, files
└── go.work                 # Go workspace definition
```

### Adding New Nodes
1. Create a new package under `apps/backend/nodes/`
2. Implement the `NodeHandler` interface
3. Register the node in `init()`
4. Add configuration options to `model.Node.Config`

### Building
```bash
make build        # build CLI into bin/rivulet
make daemon-build # build service entrypoint into bin/flowd
make api-build    # build API server into bin/rivulet-api
```

## 🎯 Use Cases

- **Data Pipelines** - ETL workflows and data processing
- **API Orchestration** - Multi-service API workflows
- **Automation** - Business process automation
- **Integration** - System integration workflows
- **Microservices** - Service orchestration and coordination

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch
3. Implement your changes
4. Add tests for new functionality
5. Submit a pull request

## 📄 License

MIT License - see LICENSE file for details

---

**Rivulet** - Lightweight, n8n-inspired workflow engine for Go applications.
