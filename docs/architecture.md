# Runtime architecture

Rivulet has exactly one execution surface: the coding-agent CLI. Workflow
orchestration is deliberately **not** in this repository — n8n and Dify own workflow
design, scheduling, retries, and automation, and Rivulet reaches them only through the
`trigger` HTTP boundary. The guiding principle is unchanged: contracts belong to stable
packages, concrete providers are chosen at a composition root.

## Current runtime model

```text
CLI composition root (cmd/rivulet)
  |
  +-- coding agent capability context
  |     +-- ToolResolver -> coding tool registry (approval-gated, cwd-confined)
  |     +-- AgentLoop    -> Harness (planner / tool / reflector policy)
  |
  +-- trigger client
        +-- HTTP request -> external n8n webhook / Dify app API
```

`runtime.Context` is a composition-time capability registry, not an application-wide
service locator. Components call `runtime.Require` at a composition boundary, then
receive normal constructor dependencies. The migrated capabilities are
`agent.ToolResolver` and `agent.AgentLoop`.

## Capability graph and contracts

```text
AgentLoop
  requires (by its implementation): Planner, Reflector, ToolResolver
  emits: ExecutionEvent

ToolResolver
  provider: Registry
  consumers: Harness and future agent-loop implementations

TriggerClient (cmd/rivulet/trigger.go)
  requires: target URL, method, body source, headers, timeout
  emits: HTTP request; prints status, body, and non-2xx failures
```

`AgentLoop` is the policy boundary:

```go
type AgentLoop interface {
    Run(context.Context, string) (RunResult, error)
}
```

`agent.Harness` is the current plan/tool/reflect implementation, and
`agent.VerificationHarness` wraps any loop with a grader plus feedback retries. ReAct,
plan-and-execute, reviewer, or deterministic loops can implement the same contract
without owning model clients, tools, or storage.

## Lifecycle model

```text
create scope
  -> provide capabilities / perform registrations
  -> record each returned cleanup as an effect
  -> run scope-owned workers
  -> close scope
       -> cancel workers
       -> wait for workers
       -> dispose effects in reverse registration order
```

`runtime.Scope` owns effects and goroutines. `runtime.ProvideInScope` removes a
provided capability when the scope closes. Tool registry registrations return
idempotent disposers that restore the preceding provider or remove the tool, so
temporary tool overlays cannot leak across sessions.

## Coupling identified in the audit

- `runAgentCLIWithIO` previously constructed the concrete model client, planner and
  reflector policy, mutable tool registry, and harness loop together, with no contract
  for substituting the loop. This is fixed: the loop is provided as `agent.AgentLoop`
  and resolved through the capability context.
- `agent.Harness` held mutable step state internally for a run but exposed no
  append-only execution stream or session boundary. Partially fixed by
  `RunResult.Events`; persistence is still open.
- `agent.Registry.Register` mutated shared registry state without a matching cleanup
  operation. Fixed with disposers.
- The trigger client is intentionally a thin transport. It must never grow workflow
  semantics (branching, retries, node types); those are the orchestrator's job.

## Agent execution events

The agent still returns the compatible mutable `RunResult`/`Steps` structure.
Alongside it, `RunResult.Events` appends structured records for run start, step
start/completion, successful completion, failure, and max-step exhaustion.
`ExecutionEventSink` lets a future session store persist the same stream.

```text
goal -> AgentLoop -> plan -> tool -> observation -> reflection
                  -> append ExecutionEvent -> RunResult
```

This is intentionally a staged migration, not a persistence rewrite.

## Security invariants

- Coding-agent mutations and shell commands pass through the CLI's approval mode;
  `--approve never` remains a dry run and must not execute a command or write a file.
- Workspace file tools resolve paths beneath the configured agent workspace.
- Trace files redact API keys, tokens, secrets, and passwords before writing.
- Credentials remain provider configuration (`OPENAI_API_KEY`, `DEEPSEEK_API_KEY`),
  never runtime capability values, execution events, or CLI output.

Approval and workspace confinement are runtime entry-point invariants, not optional
tool-plugin conventions. Future external tool providers must be wrapped by the same
enforcement path before registration.

## Decision record: the workflow engine was removed

The repository previously carried its own workflow stack: `engine/` (scheduler,
executor, retry, pause), `plugin/` (node interface and registry), `nodes/` (17 node
handlers), `format/n8n/` (n8n JSON parser), `model/` (workflow types), `memory/`,
`infra/` (local stores, queues, metrics, migrations), `data/` (example workflows,
scripts, files), and a productized frontend/backend surface under `Manifield/` and
`apps/`.

That stack was removed because:

- Maintaining a general DAG engine, a node registry with implicit `init()`
  registration, a checkpoint/review store, and a workflow UI duplicated what n8n and
  Dify already do better, and the duplication was the largest source of coupling in the
  audit above.
- The engine's value depended on the CLI importing every node package as a blank
  import, so node availability was implicit rather than composed.
- The agent harness is the part with a distinct reason to exist, and it never depended
  on the workflow packages: `agent/` uses only the standard library, and
  `cmd/rivulet/agent_*.go` depends on `agent/` plus `runtime/`.

Consequences: `go.mod` has no `require` block and no `go.sum`; the previously required
`github.com/tetratelabs/wazero` dependency is gone with the `wasm` node. Workflow
definitions, retries, and scheduling now live in n8n/Dify and are invoked with
`rivulet trigger`.

## Migration plan

1. **Completed:** typed capability availability, scoped effects, reversible agent-tool
   registration, an `AgentLoop` contract, and agent execution events.
2. **Completed:** removal of the in-repo workflow engine (`engine/`, `plugin/`,
   `nodes/`, `format/`, `model/`, `memory/`, `infra/`, `data/`, `Manifield/`, `apps/`),
   replaced by the `trigger` boundary to n8n/Dify.
3. Extract model-provider contracts from the command-layer OpenAI-compatible client,
   then inject them into agent policy implementations.
4. Add a durable session event store and snapshot folding while retaining the existing
   JSONL trace and `RunResult` APIs.
5. Centralize approval/sandbox enforcement around all privileged agent tool execution
   before supporting third-party tool providers.

## Intentional non-capabilities

Small helpers, prompt-formatting functions, retry math, and JSON parsing remain
ordinary code. Rivulet does not use a plugin abstraction for every function; a
capability is introduced only when an implementation, lifecycle, test boundary,
permission boundary, or consumer relationship is independent. The same restraint
applies to the trigger command: it is one HTTP call, not a workflow abstraction.
