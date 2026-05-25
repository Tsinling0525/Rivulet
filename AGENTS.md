# Repository Guidelines

## Project Structure & Module Organization
- `cmd/rivulet/` - CLI entrypoint.
- `agent/` - agent harness control loop.
- `engine/` - core scheduler/executor; `model/` - types; `plugin/` - node interfaces/registry.
- `nodes/` - built-in nodes such as `echo`, `http`, `python`, `llm`, `merge`, and `logic`.
- `infra/` - local storage, paths, queue, and workflow support.
- `data/` - example workflows, scripts, and files.
- `Manifield/` - extracted AI frontend/backend product surface.
- Tests live next to packages, for example `format/n8n/parser_test.go`.

## Build, Test, and Development Commands
- `make run` - run the sample workflow through the CLI.
- `make build` - build CLI to `bin/rivulet`.
- `make test` - run `go test ./...` with `-race`.
- `make lint` - run `golangci-lint` if installed.
- Examples: `go run ./cmd/rivulet run --file data/workflows/n8n_workflow.json`, `./bin/rivulet run --file data/workflows/n8n_workflow.json`.

## Coding Style & Naming Conventions
- Go 1.22; format with `gofmt`/`goimports`. Tabs for indent; 100-col soft limit.
- Package names: short, lowercase; exported identifiers in `CamelCase`; unexported in `camelCase`.
- Filenames: lowercase with underscores if needed.
- Node type strings use namespaced style, for example `http:get`, `python:script`, `merge.concat`.
- Keep functions small; handle errors explicitly; avoid panics in library code.

## Testing Guidelines
- Use `testing`; files as `*_test.go`; functions `TestXxx`.
- Prefer table-driven tests. Add unit tests for new behavior and edge cases.
- Run locally with `make test`; optional coverage with `go test ./... -cover`.

## Commit & Pull Request Guidelines
- Commits: imperative, concise subject (<=72 chars), body explains what/why.
- Group related changes; avoid mixing refactors with features.
- PRs must include description, rationale, logs if behavior changes, and linked issues.
- CI checklist before opening PR: `make lint` and `make test` green; update docs/examples when applicable.

## Security & Configuration Tips
- Environment: `RIV_DATA_DIR` controls the data root.
- Python nodes execute local scripts from `data/scripts/`; validate inputs and avoid untrusted code.
- OpenAI-backed nodes read `OPENAI_API_KEY` unless explicitly configured otherwise.

## Agent-Specific Instructions
- Keep changes minimal and scoped; follow directory conventions above.
- Prefer Make targets; do not reformat unrelated files.
- When adding nodes, place under `nodes/<name>/`, implement `plugin.NodeHandler`, and register in `init()`.
