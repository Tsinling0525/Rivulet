# Rivulet

A lightweight, n8n-inspired workflow engine written in Go. Rivulet provides a node-based workflow system with plugin architecture, state management, and event-driven execution.

## 🚀 Features

- **Node-based Workflows** - Visual workflow composition with nodes and edges
- **Plugin Architecture** - Extensible node system for custom functionality
- **State Management** - Persistent node state across workflow executions
- **Event-driven** - Observable execution flow with event bus
- **Type-safe** - Strong typing with Go's type system
- **High Performance** - Compiled language efficiency for workflow execution
- **Concurrent Execution** - Parallel node processing capabilities

## 🏗️ Architecture

```
Rivulet/
├── cmd/flowd/          # Main workflow daemon
├── engine/             # Core execution engine
├── model/              # Data structures and types
├── plugin/             # Plugin system interfaces
├── nodes/              # Node implementations
└── infra/              # Infrastructure components
```

## 🎯 Quick Start

### Basic Workflow

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/yourorg/rivulet/engine"
    "github.com/yourorg/rivulet/model"
    "github.com/yourorg/rivulet/plugin"
)

func main() {
    // Setup dependencies
    deps := plugin.Deps{
        State: &memState{},
        Bus:   &nullBus{},
    }
    
    // Create workflow engine
    eng := engine.New(deps)

    // Define workflow
    wf := model.Workflow{
        ID:   "wf1",
        Name: "EchoFlow",
        Nodes: []model.Node{
            {
                ID:      "n1",
                Type:    "echo",
                Name:    "Echo",
                Timeout: 2 * time.Second,
                Config:  map[string]any{"label": "hello"},
            },
        },
    }

    // Execute workflow
    result, err := eng.Run(context.Background(), "exec-001", wf, map[model.ID]model.Items{
        "n1": {{"msg": "hello world"}},
    })
    
    if err != nil {
        panic(err)
    }
    
    fmt.Println(result)
}
```

### Run API Server (n8n-compatible)

```bash
go run cmd/api/main.go
```

Endpoints:

- Create workflow (n8n JSON):

```bash
curl -X POST http://localhost:8080/workflows \
  -H 'Content-Type: application/json' \
  -d @examples/n8n_workflow.json
```

- Start existing workflow:

```bash
curl -X POST http://localhost:8080/workflows/<id>/start \
  -H 'Content-Type: application/json' \
  -d '{"data":{}}'
```

### Running the Example

```bash
go run cmd/flowd/main.go
```

## 🔌 Built-in Nodes

- `echo` – echoes a label into the item
- `http:get` – fetch URL into `body` + `status` (templated URL)
- `logic:if` – routes to ports `true`/`false` based on template expression
- `merge.concat` – pass-through node (engine performs fan-in)
- `python:script` – run local Python script over an attached file and put stdout (e.g., LaTeX) into item

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
├── cmd/flowd/          # Main application
├── engine/             # Workflow execution engine
│   ├── executor.go     # Node execution logic
│   └── scheduler.go    # Workflow scheduling
├── model/              # Data models
│   └── types.go        # Core types and interfaces
├── plugin/             # Plugin system
│   ├── node.go         # Node interfaces
│   └── registry.go     # Plugin registry
├── nodes/              # Built-in nodes
│   └── echo/           # Example echo node
└── infra/              # Infrastructure
    └── state.go        # State management implementations
```

### Adding New Nodes
1. Create a new package in `nodes/`
2. Implement the `NodeHandler` interface
3. Register the node in `init()`
4. Add configuration options to `model.Node.Config`

### Building
```bash
go build -o rivulet cmd/flowd/main.go
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
