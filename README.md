# Rivulet

Rivulet is a small Go CLI for running an autonomous coding agent in a workspace.
It is deliberately not an orchestrator: workflow design, scheduling, and long-running
automation belong to [n8n](https://n8n.io) or [Dify](https://dify.ai). Rivulet triggers
those systems over HTTP and gets out of the way.

## What is in here

```text
Rivulet/
├── agent/          # agent harness: planner / tool / reflector policy, verification loop
├── cmd/rivulet/    # CLI entrypoint (agent + trigger) and the OpenAI-compatible client
├── runtime/        # scoped capability composition and lifecycle
└── docs/           # architecture notes
```

Three packages, standard library only, no runtime dependencies.

## Commands

```bash
make build   # -> bin/rivulet
make test    # go test ./... -race -count=1
make lint    # golangci-lint (optional)
make run     # interactive agent loop in the repo root
```

## Agent CLI

A minimal Claude Code-style loop:

```text
goal -> plan -> tool call -> observation -> reflection -> stop/replan
```

Set a key, then run one goal or start the interactive loop:

```bash
export OPENAI_API_KEY=...
go run ./cmd/rivulet agent --once "inspect this repo and run the tests"

export DEEPSEEK_API_KEY=...
go run ./cmd/rivulet agent --provider deepseek --once "inspect this repo and run the tests"

go run ./cmd/rivulet agent
```

Flags:

```bash
go run ./cmd/rivulet agent --provider deepseek --cwd . --model deepseek-v4-flash \
  --max-steps 48 --approve always --trace on
```

| Flag | Default | Purpose |
|---|---|---|
| `--provider` | `openai` | `openai` or `deepseek` (`RIVULET_AGENT_PROVIDER`) |
| `--cwd` | `.` | Workspace directory; file tools are confined to it |
| `--model` | provider default | Model name (`RIVULET_AGENT_MODEL`) |
| `--endpoint` | provider default | OpenAI-compatible endpoint (`RIVULET_AGENT_ENDPOINT`) |
| `--once` | - | Run a single goal and exit |
| `--max-steps` | `48` | Maximum loop steps per goal |
| `--approve` | `always` | `always`, or `never` for dry-run |
| `--trace` | `on` | `on` writes JSONL traces to `.rivulet/runs/` |

Tools: `list_files`, `read_file`, `edit_file`, `replace_lines`, `write_file`, `shell`.
Use `read_file` with line numbers and then `replace_lines` when exact text replacement
would be brittle. Secrets are redacted before they reach a trace file.

`--approve never` is a dry run: mutating tools report what they would do without
changing files or running commands.

## Triggering n8n or Dify

Rivulet does not execute workflows. It hands a payload to an external orchestrator:

```bash
# n8n webhook
rivulet trigger --url https://n8n.example.com/webhook/research \
  --data '{"topic":"rivulet","depth":2}'

# Dify app API
rivulet trigger --url https://api.dify.ai/v1/workflows/run \
  --header "Authorization: Bearer app-xxxx" \
  --data '{"inputs":{"topic":"rivulet"},"response_mode":"blocking","user":"cli"}'

# body from a file or stdin
rivulet trigger --url "$RIVULET_TRIGGER_URL" --file payload.json
cat payload.json | rivulet trigger --url "$RIVULET_TRIGGER_URL" --file -
```

| Flag | Default | Purpose |
|---|---|---|
| `--url` | `RIVULET_TRIGGER_URL` | Webhook or API endpoint |
| `--method` | `POST` with a body, else `GET` | HTTP method |
| `--data` | - | Request body as a literal string |
| `--file` | - | Body file, or `-` for stdin |
| `--header` | - | `"Name: value"`, repeatable (auth goes here) |
| `--timeout` | `30` | Seconds |

The response status and body are printed; any `4xx`/`5xx` exits non-zero with the body
still printed, so an n8n error node or a Dify failure is visible in the shell.

See [docs/architecture.md](docs/architecture.md) for the capability model, lifecycle
ownership, and the reasons the workflow engine was removed.
