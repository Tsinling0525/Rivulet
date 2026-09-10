# Repository Guidelines

## Project Structure & Module Organization
- `cmd/rivulet/` - CLI entrypoint: `main.go` (dispatch), `agent_cmd.go` (agent flags/composition root), `agent_openai.go` (OpenAI-compatible client), `agent_tools.go` (coding tools + approval), `agent_trace.go` (JSONL traces, secret redaction), `trigger.go` (n8n/Dify bridge).
- `agent/` - agent harness: planner/tool/reflector policy, tool registry, verification loop.
- `runtime/` - scoped capability composition (`Context`, `Scope`, `ProvideInScope`, `Require`).
- `docs/architecture.md` - capability model, lifecycle ownership, security invariants.
- Tests live next to packages (`agent/harness_test.go`, `cmd/rivulet/trigger_test.go`).

Module: `github.com/Tsinling0525/rivulet`, Go 1.22, **standard library only** — `go.mod` has no `require` block and `go.sum` does not exist. Adding a dependency needs a stated reason.

## What this repo is not
There is no workflow engine, node registry, DAG scheduler, or workflow store, and there must not be one again.
Workflow design, scheduling, retries, and long-running automation belong to n8n or Dify.
The single integration point is `rivulet trigger`, which POSTs a payload to an external webhook/API URL and prints the response.
Do not reintroduce a node handler API behind this boundary; if something needs a workflow, express it in n8n/Dify and trigger it.

## Build, Test, and Development Commands
- `make build` - build the CLI to `bin/rivulet`.
- `make test` - `go test ./... -race -count=1`.
- `make vet` - `go vet ./...`.
- `make lint` - `golangci-lint`; warns gracefully if not installed.
- `make run` - interactive agent loop in the repo root.
- Run a single test: `go test ./cmd/rivulet -run TestSendTrigger -race`.

## Coding Conventions
- Go 1.22, `gofmt`/`goimports`, tabs, 100-column soft limit, explicit error handling, no panics in library code.
- Keep changes minimal and scoped; prefer the `make` targets; do not reformat unrelated files.
- Table-driven tests preferred. Files as `*_test.go` next to the package under test.
- Testable cores take explicit inputs: `sendTrigger(ctx, client, opts, out)` and `resolveTriggerBody(data, file, stdin)` are separated from flag parsing for exactly this reason.

## Agent-Specific Instructions
- File tools (`list_files`, `read_file`, `edit_file`, `replace_lines`, `write_file`) are confined to `--cwd`.
- `--approve never` is a dry run: mutating tools report intent without changing files or running commands. Preserve this invariant when adding tools.
- Agent runs write JSONL traces under `.rivulet/runs/` by default; `--trace off` disables them, and trace records must stay redacted for API keys, tokens, secrets, and passwords.
- `runtime.ProvideInScope` owns cleanup: capabilities registered in a scope must be disposed when the scope closes.
- Use `read_file` with line numbers, then `replace_lines`, when exact text replacement would be brittle.

## Configuration
- `OPENAI_API_KEY` / `DEEPSEEK_API_KEY` - model credentials for the agent.
- `RIVULET_AGENT_PROVIDER`, `RIVULET_AGENT_MODEL`, `RIVULET_AGENT_ENDPOINT` - agent defaults.
- `RIVULET_TRIGGER_URL` - default URL for `rivulet trigger`.
- Credentials stay in the environment: they are never capability values, trace fields, or CLI output.
