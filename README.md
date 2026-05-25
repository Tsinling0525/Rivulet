# Rivulet

Rivulet is the lightweight CLI workflow engine. The larger AI product surface has been
moved into `Manifield/`, including the HTTP API server, research backend, static
frontend, frontend docs, and product-oriented README.

## Current Shape

```text
Rivulet/
├── agent/             # agent harness control loop
├── cmd/rivulet/       # CLI entrypoint
├── engine/            # scheduler, executor, retry, pause
├── format/            # n8n import/parser support
├── infra/             # local state, files, queues, stores
├── model/             # workflow, node, review, file types
├── nodes/             # built-in node handlers
├── plugin/            # node interfaces and registry
├── data/              # example workflows, scripts, and files
├── Manifield/         # extracted AI frontend/backend product
└── Makefile
```

## Commands

```bash
make build
make test
make run
```

`make run` executes the sample n8n workflow through the CLI. You can run any workflow
file directly:

```bash
go run ./cmd/rivulet run --file data/workflows/n8n_workflow.json
./bin/rivulet run --file data/workflows/n8n_workflow.json
```

The CLI also has a built-in echo sample:

```bash
go run ./cmd/rivulet sample
```

## Built-in Nodes

- `echo`
- `http:get`
- `http:request`
- `logic:if`
- `merge.concat`
- `python:script`
- `files:load`
- `fs:write`
- `ollama`
- `chatgpt`
- `llm:route`
- `eval:node`
- `review:gate`
- `wasm`

## Configuration

- `RIV_DATA_DIR` overrides the data root. The default is `data/`.
- Python nodes execute local scripts from `data/scripts/`.
- File attachments are read from `data/files/<workflowID>/`.
- OpenAI-backed nodes read `OPENAI_API_KEY` unless configured otherwise.
