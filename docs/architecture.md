# Architecture

Rivulet is a tiny workflow system: it loads a Dify-style workflow DSL file, validates it,
executes it locally, and reports outputs plus a per-node trace. There is no server, no
database, and no UI — that boundary is deliberate and permanent (see the decision record
at the end).

```text
*.dify.yml
  |
  +-- dsl.Load / dsl.Parse        envelope: version, kind, app, workflow.graph
  +-- dsl.Validate                structure, cycles, references, unsupported types
  +-- workflow.Validate           dsl checks + per-node handler validation
  |
  +-- workflow.Run
  |     scheduler  -> edge-driven: a node runs once all incoming edges resolve
  |     per node   -> pool snapshot, handler.Run, fields published to the pool
  |     policy     -> retry_config, error_strategy
  |     trace      -> Step{Index, NodeID, Status, Branch, Retries, Timings, Outputs}
  |
  +-- Result{Outputs, Answers, Steps} -> human table | --json | --trace FILE
```

## Packages

| Package | Responsibility |
|---|---|
| `dsl` | DSL types matching Dify 0.6.0, parsing, and static validation |
| `expr` | The run pool, `{{#node.field#}}`/`{{ name }}` rendering, dotted lookups |
| `nodes` | The nine node handlers and the explicit registry |
| `workflow` | Scheduler/executor (`engine.go`) and node policy (`policy.go`) |
| `llmclient` | Shared OpenAI-compatible model client |
| `agent`, `runtime` | Coding-agent harness and scoped capability composition |
| `cmd/rivulet` | CLI: flags, input coercion, model overrides, output formatting |

Dependency direction is one-way: `cmd` → `workflow` → (`nodes`, `expr`, `dsl`) → `llmclient`.
Nothing in `dsl`, `expr`, or `workflow` imports `cmd`, and `nodes` never imports `workflow`.

## Execution model

**Edges carry the branch decision.** Every edge has a `sourceHandle`: `"source"` for the
single-output nodes, a case ID or `"false"` for `if-else`. When a node completes, the
scheduler marks each outgoing edge as traversed or ruled out, and decrements the
target's pending count.

**A node runs when all incoming edges are resolved.** If at least one edge was
traversed, the node runs. If every incoming edge was ruled out, the node is `skipped`,
and its own outgoing edges are ruled out in turn — which is how an entire untaken branch
chain ends up as `skipped` rather than hanging the run or executing pointlessly.

**Independent nodes run in parallel**, bounded by `--concurrency` (default 4). Each node
receives a snapshot of the variable pool, so a running node can never observe a write
from a node that started later. This is why the pool is copied rather than shared: two
http nodes fanning out from the same start node must both see a stable view.

**Failures follow Dify's `error_strategy`:**

- `terminated` (default) — the run aborts with the failing node's ID and error.
- `continue-on-error` / `remove-abnormal-output` — the step is recorded as `failed`, the
  downstream nodes still run, and references to that node resolve to its `default_value`
  entries, or to an empty string through a wildcard entry. A workflow that depends on a
  flaky endpoint can therefore keep going and report partial results.

**`retry_config`** is honoured per node: `max_retries` (clamped to 10), `retry_interval`
in milliseconds, and optional exponential backoff with `multiplier`/`max_interval`.

## Variable resolution

Two forms, both from Dify:

- `{{#node_id.field#}}` inside text fields (prompts, templates, URLs, bodies, answers).
- `[node_id, field]` in structured fields (`value_selector`, `variable_selector`).

The field part may be dotted to walk nested objects. Reserved prefixes are `sys`
(`timestamp`, `user_id`), `env` (the DSL `environment_variables` block), and
`conversation` (accepted, unused until chat memory exists).

An unknown reference is a **hard error**, not an empty string: silently empty variables
hide DSL typos, and a workflow that renders `Hello ` because a node ID was mistyped is
worse than one that refuses to run. The single exception is a node that failed under
`continue-on-error`, which carries an explicit empty wildcard.

## Node contract

```go
type Handler interface {
    Type() string
    Validate(node dsl.Node) []dsl.Problem
    Run(ctx context.Context, req Request) (Output, error)
}
```

`Request` carries the node, a pool snapshot, the start-node inputs, the model override,
a shared HTTP client, and a work directory. `Output` is a field map plus an optional
`Branch` (the selected outgoing handle). Handlers are ordinary code: helpers inside a
handler are not capabilities, and there is no plugin indirection for every function.

`registry.go`-style magic is deliberately absent: `nodes.Registry()` lists the handlers,
so a missing node type is a visible error at validate *and* run time instead of an
implicit "not registered because a blank import was deleted".

## Static validation

`rivulet validate` runs both layers:

- **Graph** (`dsl.Validate`): `kind: app`, a supported `app.mode`, unique node IDs, no
  `parentId` containers, exactly one start node, at least one end node (or answer node in
  advanced-chat), edges referencing real nodes, no cycles (DFS), if-else handles matching
  declared cases, every reference pointing at a real node, and reachability warnings.
- **Nodes** (`workflow.Validate`): each handler's `Validate`, e.g. an `llm` node without a
  prompt, a `code` node without code or with `code_language: javascript`, an `http-request`
  node with an unsupported method or body type.

Dify node types that Rivulet does not implement produce an **error** (`dsl.Unsupported`),
never a silent no-op. Dify features that are accepted but not implemented — `vision`,
`context`/knowledge retrieval, `structured_output`, `fail-branch` edges, triggers —
produce **warnings** so a file exported from Dify still validates while being honest
about what will not happen. See [dify-compat.md](dify-compat.md).

## Lifecycle and ownership

`runtime.Scope` owns effects for the agent CLI: capabilities provided through
`runtime.ProvideInScope` are disposed in reverse order when the scope closes, and tool
registry registrations return idempotent disposers. The workflow runner needs no such
machinery — its only long-lived resources are the HTTP client and the temp scripts that
`code` nodes create and remove around each execution.

## Security invariants

- `code` nodes execute a local `python3` process with the caller's privileges; the runner
  writes to a temp file next to the workflow and deletes it afterwards. Only run DSL files
  you trust.
- `http-request` sends credentials only when the DSL (or an input) provides them; an empty
  key means no auth header rather than a literal `Bearer ` header.
- The agent CLI keeps approval gating (`--approve never` is a dry run), workspace
  confinement for file tools, and redaction of keys, tokens, secrets, and passwords in
  traces. Credentials stay in the environment, never in traces or CLI output.
- No secret is ever written into a DSL file by Rivulet: the DSL contains selectors, not
  values, unless the author hardcodes one.

## Decision record

Rivulet previously carried, in sequence, a custom workflow engine with 17 node types and
a node registry, a productized frontend/backend, and finally a coding-agent CLI. On
2026-09-10 the workflow stack, the product surface, and their stores were removed, and
workflows were delegated to n8n/Dify through an HTTP trigger. That reversal was itself
the wrong answer: it left the repository with no reason to exist beyond a thinner copy of
existing tools.

The current position is:

- **A tiny workflow system, with Dify as the reference model.** Not "an n8n clone", not "a
  platform", not "an agent framework with a workflow attachment".
- **Local-first.** One binary, no server, no database, no browser. Deploying a Dify
  instance to run a five-node graph locally is the problem this solves.
- **Dify-shaped on purpose.** Node types, DSL field names, variable reference syntax,
  branch handles, and error/retry semantics follow Dify so files are portable in both
  directions and the documentation is short.
- **Deliberately incomplete.** Nine node types and a documented list of what will never
  be implemented beat a partial reimplementation of the platform.
- **The churn rule.** Every previous identity was *added* rather than *replacing* the
  last, which is how the repository ended up describing four architectures at once. From
  here: a new identity must delete the old one or live in its own repository, and the
  docs must describe exactly one architecture.

## Remaining work

1. Extract model-provider contracts from the command layer and inject them into the agent
   policy implementations (agent side, unchanged by this rewrite).
2. Durable session event store and snapshot folding for agent runs, retaining the JSONL
   trace and `RunResult` APIs.
3. Chat memory / `conversation_variables` for advanced-chat mode (currently accepted and
   unused).
4. File inputs and outputs for `start`, `http-request`, and `end` (`file` typed variables,
   multipart uploads).
5. Optional `iteration` container support, if a real workflow needs it — the scheduler's
   edge model is the prerequisite, not a rewrite.
