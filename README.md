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
├── runtime/            # scoped capability composition and lifecycle
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

## Agent CLI MVP

Rivulet includes a minimal Claude Code-style agent loop:

```text
goal -> plan -> tool call -> observation -> reflection -> stop/replan
```

Set an OpenAI key, then run one goal:

```bash
export OPENAI_API_KEY=...
go run ./cmd/rivulet agent --once "inspect this repo and run the tests"
```

To use DeepSeek instead:

```bash
export DEEPSEEK_API_KEY=...
go run ./cmd/rivulet agent --provider deepseek --once "inspect this repo and run the tests"
```

Or start the interactive loop:

```bash
go run ./cmd/rivulet agent
```

Useful flags:

```bash
go run ./cmd/rivulet agent --provider deepseek --cwd . --model deepseek-v4-flash --max-steps 48 --approve always --trace on
```

The MVP tools are `list_files`, `read_file`, `edit_file`, `replace_lines`,
`write_file`, and `shell`.
File tools are scoped to the selected workspace directory.
Use `read_file` with line numbers, then `replace_lines`, when exact text replacement
would be brittle.
Use `--approve never` to dry-run mutating tools: `edit_file`, `replace_lines`,
`write_file`, and `shell` report what they would do without changing files or running
commands.
Agent runs write JSONL traces under `.rivulet/runs/` by default. Use `--trace off`
to disable trace files.

See [the runtime architecture](docs/architecture.md) for the capability graph,
lifecycle ownership model, agent-loop contract, event migration, and security
boundaries.

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
- `memory:write`
- `memory:update`
- `memory:query`

Memory nodes keep a small per-user graph in local storage. `memory:write` stores
propositions, `memory:update` records a change and marks dependent memories for
review, and `memory:query` returns active matches plus stale or uncertain warnings.
See `data/workflows/template_memory.json` for a minimal write-and-query flow.

## Configuration

- `RIV_DATA_DIR` overrides the data root. The default is `data/`.
- Python nodes execute local scripts from `data/scripts/`.
- File attachments are read from `data/files/<workflowID>/`.
- OpenAI-backed nodes read `OPENAI_API_KEY` unless configured otherwise.
