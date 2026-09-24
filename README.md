# Rivulet

**A tiny workflow system.** Rivulet runs Dify-style workflow DSL files locally: one Go
binary, no server, no database, no browser. You write (or export from Dify) a
`*.dify.yml` graph, run it with `rivulet run`, and get the outputs plus a per-node trace.

```bash
rivulet run --file examples/hello.dify.yml --input name=world
```

```text
Hello (workflow)
[1] start                completed    0s  fields=name
[2] template-transform   completed    0s  fields=output
[3] end                  completed    0s  fields=greeting
finished in 0s
outputs:
  greeting: Hello world, this workflow ran locally.
```

## Positioning

This is the project's identity, and it is frozen:

- **What it is:** a tiny workflow system. Dify's workflow model — its DSL envelope,
  node types, variable references, branching, and error/retry semantics — is the
  reference design, implemented as a local CLI rather than a platform.
- **What it is not, permanently:** no web server or UI, no database or multi-tenancy,
  no plugin marketplace, no knowledge base or vector store, no scheduler or triggers,
  no user/auth model. Those are the parts that made earlier iterations of this project
  unmaintainable.
- **The one rule that keeps this true:** a new product identity must either delete the
  old one or live in its own repository, and the docs must describe exactly one
  architecture. Never two. Workflow orchestration belongs here; anything that starts
  to look like a platform does not.
- **The coding agent is a sibling, not the product.** `rivulet agent` is a workable
  Claude-Code-style loop that shares the model client (`llmclient`) and nothing else.
  It is kept because it works and is tested; it is not the reason this repository
  exists.

## Install and build

```bash
make build      # -> bin/rivulet
make test       # go test ./... -race -count=1
make vet        # go vet ./...
make run        # run examples/hello.dify.yml
make examples   # validate every examples/*.dify.yml
```

Requires Go 1.22+. The only dependency is `gopkg.in/yaml.v3` (DSL parsing).

## Commands

```bash
rivulet run --file app.dify.yml [--input k=v ...] [flags]
rivulet validate --file app.dify.yml
rivulet nodes
rivulet agent [--once "goal"] [flags]
```

### `rivulet run`

| Flag | Purpose |
|---|---|
| `--file` | Workflow DSL file (`.yml` or `.json`) |
| `--input k=v` | Workflow input; repeatable, overrides `--input-file` |
| `--input-file` | JSON object of inputs, or `-` for stdin |
| `--json` | Print the result (outputs, answers, per-node steps) as JSON |
| `--trace PATH` | Write a JSON run trace to `PATH` |
| `--concurrency N` | Maximum nodes running at once (default 4) |
| `--provider` | Override the llm provider: `openai`, `deepseek`, `ollama` |
| `--model` / `--endpoint` / `--api-key` | Override the llm model configuration |

CLI model flags win over the DSL, which is what makes it possible to run a workflow
against a local stub or a different vendor without editing the file. Inputs are parsed
as JSON when they look like JSON, so `--input count=7` arrives as a number.

### `rivulet validate`

Static checks: the DSL envelope, one start node, at least one end (or answer) node,
duplicate IDs, dangling edges, cycles, unreachable nodes, every `{{#node.field#}}`
reference pointing at a real node, if-else edge handles matching declared cases, and
per-node configuration. Errors exit 1; warnings (unreachable nodes, Dify features that
are accepted but not implemented) print and exit 0.

## Nodes

Nine node types, matching Dify's names and field shapes:

| Type | Behavior |
|---|---|
| `start` | Workflow inputs; typed, required/optional, optional inputs resolve to `""` |
| `end` | Declares outputs via `value_selector`; missing branches emit `null` |
| `answer` | Renders a chatflow answer string (advanced-chat mode) |
| `llm` | One chat completion over an OpenAI-compatible endpoint |
| `code` | `python3` snippet: `main(**inputs) -> dict` |
| `if-else` | Ordered cases (`and`/`or`) + `false` branch, selected by edge `sourceHandle` |
| `template-transform` | Text template over mapped variables |
| `http-request` | Method, URL, params, headers, JSON/raw/form bodies, api-key/custom auth |
| `variable-aggregator` | Picks whichever branch actually ran |

Variable references use Dify's two forms: `{{#node_id.field#}}` inside text and
`[node_id, field]` in structured fields (`value_selector`, `variable_selector`). Dotted
paths walk nested values, so `{{#code_node.result.items#}}` works. An unknown reference
is an error rather than an empty string — silent empties hide DSL typos.

`rivulet nodes` prints this table plus every Dify node type that is deliberately not
implemented (`iteration`, `tool`, `knowledge-retrieval`, `agent`, triggers, ...).
See [docs/dify-compat.md](docs/dify-compat.md) for the field-level compatibility notes.

## Examples

| File | Shows |
|---|---|
| `examples/hello.dify.yml` | start → template → end |
| `examples/branch.dify.yml` | if-else routing plus a variable aggregator |
| `examples/http-and-code.dify.yml` | HTTP fetch, `continue-on-error`, python3 reshaping |
| `examples/llm-chat.dify.yml` | advanced-chat: llm → answer, with retry config |

The first two run offline. `http-and-code` needs `--input url=...`; `llm-chat` needs
`OPENAI_API_KEY` (or `--provider deepseek` / `--endpoint` for anything else).

## Security

- `code` nodes execute a local `python3` process with your privileges. Only run
  workflow files you trust.
- `http-request` renders URLs, headers, and bodies from workflow variables. Credentials
  passed as inputs end up in outbound requests by design, so pass them with `--input`
  or environment variables rather than committing them to a DSL file.
- The agent CLI keeps its own invariants: mutations and shell commands are gated by
  `--approve` (`never` is a dry run), file tools are confined to `--cwd`, and trace
  files redact keys, tokens, secrets, and passwords.

See [docs/architecture.md](docs/architecture.md) for the scheduler, branch, and
lifecycle model.
