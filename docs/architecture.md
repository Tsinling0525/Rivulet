# Runtime architecture

Rivulet has two currently separate execution surfaces: the workflow engine and
the coding-agent CLI. They share the principle that contracts belong to stable
packages, while concrete providers are selected at a composition root.

## Current runtime model

```text
CLI composition root
  |
  +-- coding agent capability context
  |     +-- ToolResolver -> coding tool registry
  |     +-- AgentLoop    -> Harness (planner / tool / reflector policy)
  |
  +-- workflow engine
        +-- plugin.Deps  -> state, events, files, reviews
        +-- node registry -> built-in node handlers
```

`runtime.Context` is deliberately a composition-time capability registry, not
an application-wide service locator. Components call `runtime.Require` at a
composition boundary, then receive normal constructor dependencies. The first
migrated capabilities are `agent.ToolResolver` and `agent.AgentLoop`.

The workflow engine remains on the existing `plugin.Deps` contract while it is
migrated in subsequent vertical slices. Its node handlers are independent
runtime units because they have an external extension boundary; helpers inside
individual nodes are intentionally ordinary code.

## Coupling identified in the audit

- `runAgentCLIWithIO` previously constructed the concrete model client,
  planner/reflector policy, mutable tool registry, and harness loop together,
  with no contract for substituting the loop.
- `agent.Harness` held mutable step state internally for a run but exposed no
  append-only execution stream or session boundary.
- `agent.Registry.Register` mutated shared registry state without a matching
  cleanup operation.
- `engine.Engine` directly calls the process-global `plugin.New`; package
  `init` registrations and CLI blank imports therefore decide node availability
  implicitly.
- `plugin.Deps` is a useful compatibility contract but combines state, events,
  files, and reviews into one bag, encouraging handlers to retain more runtime
  dependencies than they need.
- Workflow run events are recorded by an infra wrapper after dispatch, while
  the agent trace is a separate CLI concern; neither is yet a shared session
  event capability.

## Capability graph and contracts

```text
AgentLoop
  requires (by its implementation): Planner, Reflector, ToolResolver
  emits: ExecutionEvent

ToolResolver
  provider: Registry
  consumers: Harness and future agent-loop implementations

Workflow executor
  requires: StateStore, EventBus, FileStore, ReviewStore, NodeFactory
  emits: workflow execution events
```

`AgentLoop` is the policy boundary:

```go
type AgentLoop interface {
    Run(context.Context, string) (RunResult, error)
}
```

`agent.Harness` is the current plan/tool/reflect implementation. ReAct,
plan-and-execute, reviewer, or deterministic loops can implement the same
contract without owning model clients, tools, or storage.

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
provided capability when the scope closes. Tool registry registrations now
return idempotent disposers that restore the preceding provider or remove the
tool, so temporary tool overlays do not leak across sessions.

## Agent execution events

The agent still returns the compatible mutable `RunResult`/`Steps` structure.
Alongside it, `RunResult.Events` appends structured records for run start,
step start/completion, successful completion, failure, and max-step exhaustion.
`ExecutionEventSink` lets a future session store persist the same stream.

```text
goal -> AgentLoop -> plan -> tool -> observation -> reflection
                  -> append ExecutionEvent -> RunResult
```

This is intentionally a staged migration, not a persistence rewrite. A later
session-store capability can persist events and derive snapshots for replay,
forking, and trace inspection.

## Security invariants

- Coding-agent mutations and shell commands pass through the CLI's approval
  mode; `--approve never` remains dry-run and must not execute a command or
  write a file.
- Workflow review gates create and resolve review records through
  `ReviewStore`; a paused workflow may resume only through checkpoint resume.
- Workspace file tools resolve paths beneath the configured agent workspace.
- Credentials remain provider configuration (`OPENAI_API_KEY` or
  `DEEPSEEK_API_KEY`), not runtime capability values or execution events.

Approval and workspace confinement are runtime entry-point invariants, not
optional tool-plugin conventions. Future external tool providers must be
wrapped by the same enforcement path before registration.

## Migration plan

1. **Completed:** introduce typed capability availability, scoped effects,
   reversible agent-tool registration, an `AgentLoop` contract, and agent
   execution events.
2. Split workflow `plugin.Deps` into focused capability contracts and inject a
   `NodeFactory` into `engine.Engine`, preserving the current global registry
   as a compatibility provider.
3. Give workflow node registration scoped cleanup APIs and move CLI blank
   imports into an explicit built-in provider.
4. Extract model-provider contracts from command-layer OpenAI-compatible
   clients, then inject them into agent policy implementations.
5. Add a durable session event store and snapshot folding while retaining the
   existing JSONL trace and `RunResult` APIs.
6. Centralize approval/sandbox enforcement around all privileged agent tool
   execution before supporting third-party tool providers.

### First-slice change set

The completed slice changes `runtime/` (new context/scope), `agent/` (tool
cleanup, loop and event contracts), `cmd/rivulet/agent_cmd.go` (composition
root), and this documentation. It intentionally does not modify `engine/`,
`plugin/`, node APIs, configuration files, or workflow persistence.

## Intentional non-capabilities

Small helpers, prompt-formatting functions, retry math, JSON parsing, and
individual node implementation details remain ordinary code. Rivulet does not
use a plugin abstraction for every function; a capability is introduced only
when an implementation, lifecycle, test boundary, permission boundary, or
consumer relationship is independent.

## Risks and compatibility

The principal migration risk is introducing a second global registry while
moving away from `plugin` globals. The capability context therefore remains
per-runtime and scoped. The other risk is event/schema churn; the initial event
stream is additive and does not alter the CLI output, configuration format,
workflow behavior, or existing `Harness.Run` signature.
