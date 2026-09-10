# CLAUDE.md

This file provides guidance to Claude Code when working with this repository.

## Commands

```bash
make run       # interactive agent loop in the repo root
make build     # build CLI binary -> bin/rivulet
make test      # go test ./... -race -count=1
make vet       # go vet ./...
make lint      # golangci-lint run ./...
```

Single test:

```bash
go test ./cmd/rivulet -run TestSendTrigger -race
go test ./agent -run TestHarness -race
```

## Architecture

Rivulet is a coding-agent CLI. It is **not** a workflow engine: n8n and Dify own
workflow design, scheduling, and automation. Rivulet's only integration with them is
`rivulet trigger`, an HTTP call to an external webhook/API.

Three packages, standard library only:

**Entry point**: `cmd/rivulet/`

1. `main.go` - dispatches the `agent` and `trigger` subcommands, prints usage.
2. `agent_cmd.go` - agent flags, provider selection, and the composition root that
   provides `agent.ToolResolver` and `agent.AgentLoop` into a `runtime.Scope`.
3. `agent_openai.go` - OpenAI-compatible chat/responses client used by the planner
   and reflector (`openai` and `deepseek` providers).
4. `agent_tools.go` - `list_files`, `read_file`, `edit_file`, `replace_lines`,
   `write_file`, `shell`, confined to `--cwd` and gated by the approval mode.
5. `agent_trace.go` - JSONL run traces under `.rivulet/runs/`, with redaction.
6. `trigger.go` - `rivulet trigger --url ... [--data|--file] [--header ...]`.

**Harness**: `agent/`

```text
goal -> plan -> one tool call -> observation -> reflection -> stop/replan
```

`Planner` and `Reflector` are interfaces; `Harness` is the only implementation.
`VerificationHarness` wraps a loop with a grader and retries with feedback.
Tool failures become observations so the reflector decides to stop or replan.

**Capabilities**: `runtime/` - `NewScope`, `NewContext`, `ProvideInScope`, `Require`.
Effects registered in a scope are disposed in reverse order when it closes.

## Conventions

- Standard library only. `go.mod` has no `require` block; do not add dependencies
  without a reason.
- Never reintroduce a node registry, DAG scheduler, or workflow store. Workflows
  belong in n8n/Dify.
- Keep the model client, tools, and harness constructor-injected so tests can replace
  them; only the composition root touches `os.Getenv`.
- Credentials are read from the environment and must never reach a trace file or CLI
  output; `agent_trace.go` redacts keys, tokens, secrets, and passwords.
- Go 1.22, `gofmt`/`goimports`, tabs, 100-column soft limit, explicit error handling,
  no panics in library code.
- Tests are table-driven when practical and live next to the package they test.
