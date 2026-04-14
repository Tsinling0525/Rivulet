# Repository Guidelines

## Project Structure & Module Organization
- `apps/backend/cmd/` – entrypoints: `flowd/` (daemon), `api/` (HTTP server), `rivulet/` (CLI).
- `apps/backend/engine/` – core scheduler/executor; `apps/backend/model/` – types; `apps/backend/plugin/` – node interfaces/registry.
- `apps/backend/nodes/` – built-in nodes (e.g., `echo`, `http`, `python`, `llm`, `merge`, `logic`).
- `apps/backend/infra/` – storage, API deps, paths, queue; `data/` – example workflows/scripts/files.
- Tests live next to packages (e.g., `format/n8n/parser_test.go`).

## Build, Test, and Development Commands
- `make run` – run the CLI server entrypoint (`go run ./apps/backend/cmd/rivulet server`).
- `make api` – start API server (default `:8080`).
- `make build` – build CLI to `bin/rivulet`.
- `make daemon-build` – build service entrypoint to `bin/flowd`.
- `make api-build` – build API to `bin/rivulet-api`.
- `make test` – run `go test ./apps/backend/...` with `-race`.
- `make lint` – run `golangci-lint` (install if missing).
- Examples: `go run ./apps/backend/cmd/rivulet server`, `./bin/rivulet run --file data/workflows/n8n_workflow.json`.

## Coding Style & Naming Conventions
- Go 1.22; format with `gofmt`/`goimports`. Tabs for indent; 100-col soft limit.
- Package names: short, lowercase; exported identifiers in `CamelCase`; unexported in `camelCase`.
- Filenames: lowercase with underscores if needed.
- Node type strings use namespaced style (e.g., `http:get`, `python:script`, `merge.concat`).
- Keep functions small; handle errors explicitly; avoid panics in library code.

## Testing Guidelines
- Use `testing` package; files as `*_test.go`; functions `TestXxx`.
- Prefer table-driven tests. Add unit tests for new behavior and edge cases.
- Run locally: `make test`; optional coverage: `go test ./... -cover`.

## Commit & Pull Request Guidelines
- Commits: imperative, concise subject (≤72 chars), body explains what/why.
- Group related changes; avoid mixing refactors with features.
- PRs must include: description, rationale, screenshots/logs if UI/API behavior changes, and linked issues.
- CI checklist before opening PR: `make lint` and `make test` green; update `README.md`/examples when applicable.

## Security & Configuration Tips
- Environment: `RIV_API_PORT` (API port), `RIV_DATA_DIR` (data root: `data/workflows`, `data/scripts`, `data/files/<workflowID>`).
- UI hosting: `RIV_FRONTEND_DIR` (override path serving static dashboard assets; defaults to `apps/frontend`).
- Python node executes local scripts from `data/scripts/`; validate inputs and avoid untrusted code.

## Agent-Specific Instructions
- Keep changes minimal and scoped; follow directory conventions above.
- Prefer Make targets; don’t reformat unrelated files.
- When adding nodes, place under `apps/backend/nodes/<name>/`, implement `plugin.NodeHandler`, and register in `init()`.
