# Repository Guidelines

## Project Structure & Module Organization
- `cmd/rivulet/` - CLI entrypoint (`main.go`, agent loop, `run`, `sample` subcommands).
- `agent/` - agent harness control loop.
- `engine/` - core scheduler, executor, retry, and pause logic.
- `model/` - workflow, node, review, and file types.
- `plugin/` - `NodeHandler` interface, `Deps`, and `Register`/`New` registry.
- `nodes/` - built-in node handlers: `echo`, `eval`, `files`, `fs`, `http`, `llmroute`, `logic`, `merge`, `ollama`, `openai`, `review`, `wasm`. The `llm` package is a shared cache/lib, not a handler. The `python` package exists but is not imported by default.
- `format/` - n8n JSON parser.
- `infra/` - local storage, paths, queue, and workflow support.
- `data/` - example workflows, scripts, and files.
- `Manifield/` - a separate Go workspace (`go.work`) containing the productized AI frontend/backend, HTTP API server, and research backend. Do not modify alongside core Rivulet changes; it has its own build constraints.
- Tests live next to packages (e.g. `format/n8n/parser_test.go`).

## Build, Test, and Development Commands
- `make build` - build CLI to `bin/rivulet`.
- `make test` - run `go test ./... -race -count=1`.
- `make lint` - run `golangci-lint`; gracefully warns if not installed.
- `make run` - execute the sample n8n workflow (`go run ./cmd/rivulet run --file data/workflows/n8n_workflow.json`).
- Run a single test: `go test ./path/to/package -run TestName -race`.
- Module path: `github.com/Tsinling0525/rivulet` (Go 1.22).

## Node Registration (Critical)
- Every node handler implements `plugin.NodeHandler` and registers itself via `init()` using `plugin.Register(nodeType, factory)`.
- All registered nodes MUST be imported as blank imports in `cmd/rivulet/main.go`. Without the import, the `init()` never runs and the node is unavailable.
- Node type strings are namespaced: `http:get`, `python:script`, `merge.concat`, `llm:route`, etc.

## Coding Conventions
- Nodes go under `nodes/<name>/`, one package per node type. Node type string and package dir name are usually the same.
- Filenames: lowercase with underscores if needed.
- Keep changes minimal and scoped. Prefer `make` targets; do not reformat unrelated files.

## Testing
- Table-driven tests preferred. Files as `*_test.go` next to the package under test.

## Agent-Specific Instructions
- Agent tool scoping: file tools (`read_file`, `edit_file`, `write_file`) are scoped to the selected workspace directory.
- `--approve never` runs the agent in dry-run mode: mutating tools report intent without changing files or running commands.
- Agent runs write JSONL traces under `.rivulet/runs/` by default; `--trace off` disables this.
- Use `read_file` with line numbers, then `replace_lines`, for operations where exact text replacement would be brittle.

## Configuration
- `RIV_DATA_DIR` overrides the data root (default: `data/`).
- Python nodes execute local scripts from `data/scripts/` with no sandboxing; only use with trusted scripts.
- OpenAI-backed nodes read `OPENAI_API_KEY` unless configured otherwise.
- File attachments are read from `data/files/<workflowID>/`.
