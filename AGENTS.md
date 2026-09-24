# Repository Guidelines

This repository holds a workflow engine described by `PRD.md` (企业级工作流流程引擎设计方案)
plus a local Dify-compatible workflow runner and a coding-agent CLI. Read `PRD.md` first: it is
the requirements source for engine behavior, and every engine change should be traceable to a
section in it.

## Project Structure & Module Organization

**Go surface — local workflow runner + coding agent**

- `cmd/rivulet/` — CLI entrypoint. `main.go` dispatches `run`, `validate`, `nodes`, `agent`.
  `run.go` (workflow commands, input coercion, output formatting), `agent_cmd.go` (agent flags and
  composition root), `agent_openai.go` (OpenAI-compatible planner/reflector client),
  `agent_tools.go` (coding tools + approval), `agent_trace.go` (JSONL traces, secret redaction).
- `dsl/` — Dify 0.6.0-shaped DSL types (`App`, `Workflow`, `Graph`, `Node`, `Edge`, `Selector`),
  parsing, and static validation (`Severity`/`Problem`).
- `expr/` — variable pool and `{{#node.field#}}` / `{{ name }}` rendering with dotted lookups.
- `nodes/` — the nine node handlers (`start`, `end`, `answer`, `llm`, `code`, `if-else`,
  `template-transform`, `http-request`, `variable-aggregator`) behind the `Handler` interface
  (`Request`/`Output`) and an explicit registry.
- `workflow/` — scheduler/executor (`engine.go`: `Options`, `Status`, `Step`, `Result`) and node
  policy (`policy.go`: retry/error strategy).
- `llmclient/` — shared OpenAI-compatible model client (`Config`, `Message`, `Result`).
- `agent/` — agent harness: `Tool`/`Registry`/`ToolResolver`, `AgentLoop`, `Harness`,
  `VerificationHarness`, plan/reflect/observation types.
- `runtime/` — scoped capability composition (`NewScope`, `NewContext`, `ProvideInScope`,
  `Require`).
- `docs/architecture.md` (runner architecture + decision record), `docs/dify-compat.md`
  (field-level Dify compatibility), `agent/README.md`.

**C++ surface — the PRD engine (`cpp/`)**

- `cpp/include/wf/` — `json` (self-written JSON), `expression` (`${...}` engine), `script`
  (`wfscript`), `types` (enums, state machines, durations), `model` (DSL + pre-publish
  validation), `entity` (instance/node/task/history/event/timer/branch records), `repository`
  (storage interface + in-memory + JSON snapshot), `services` (clock, organization, service
  invoker, notifier, event publisher), `engine`, `api` (PRD §18-shaped facade).
- `cpp/src/` — implementation, split by concern: `engine.cpp` (run loop, node execution,
  lifecycle, queries), `engine_tasks.cpp` (task operations, countersign evaluation),
  `engine_scheduler.cpp` (timers, timeouts, outbox delivery), `main.cpp` (`wf-cli`).
- `cpp/tests/` — 77+ cases on a small in-repo framework (`test.hpp`), one file per layer.
- `cpp/examples/` — JSON definitions (`expense_approval`, `leave_approval`, `order_delivery`,
  `shipment`, `contract_approval`) plus `org.json` seed data.
- `cpp/docs/PRD-mapping.md` — PRD section → file/symbol/status mapping. Update it with any
  behavior change.

Dependency direction is one-way on both surfaces: Go `cmd` → `workflow` → (`nodes`, `expr`,
`dsl`) → `llmclient`; `nodes` never imports `workflow`, and `dsl`/`expr`/`workflow` never import
`cmd`. C++ `main` → `api` → `engine` → (`repository`, `services`, `script`, `expression`,
`model`, `entity`, `types`) → `json`.

## Engine Model (must stay true to PRD.md)

- **Definition vs instance separation (§3.1).** A definition is a template with versions; an
  instance binds exactly one published version (`definitionVersion`) and never follows a later
  publish. Published versions are immutable — edit by publishing a new version, not by mutating.
- **State driven (§3.2, §6, §20).** Process, node and task statuses change only through legal
  transitions. Do not assign a status directly; go through the transition guard, and reject
  illegal moves instead of silently applying them.
- **Traceable execution (§3.3).** Every meaningful step writes a history record (who, when, which
  node, which action, input/output, where it went) and an event-outbox row. If you add an
  operation, add its audit path.
- **Idempotent (§3.4, §22).** Starting is deduplicated by `processCode + businessKey`; task
  completion uses a version-optimistic update; event delivery is idempotent per event id.
- **Validated before publish (§34.2).** A definition that fails validation must not be
  deployable: missing start/end, orphan or unreachable nodes, cycles, missing assignee rules,
  unconfigured service nodes, incomplete conditional branches, non-converging parallel branches,
  invalid durations.
- **Gateways.** Conditional gateways need at least one real condition plus a default branch.
  Parallel forks must converge on a join (parallel gateway with in-degree > 1, or `joinAll: true`);
  branch arrivals are counted, and a branch that parks keeps the instance running.
- **Countersign accounting (会签/或签/比例/依次).** Only real votes count: transfer children vote,
  delegation children do not (the vote stays with the original handler), pre-add-sign tasks block
  their parent without voting, and post-add-sign tasks must complete before the node closes.
- **Side effects go through interfaces.** External calls use `ServiceInvoker`; notifications use
  `Notifier`; listeners use `EventPublisher` through the event outbox with the retry ladder
  (§29.3). Never call out inline from engine code, and never add a network dependency to the
  core.

## What stays out (permanently)

The runner surface is local-first, and that boundary does not move: no web server or UI, no
database or multi-tenancy, no plugin marketplace, no knowledge base or vector store, no scheduler
or trigger endpoints, no user/auth model. Service definition (§5.2.2), instance (§5.2.3) and task
(§5.2.4) behavior lives in the engine and its API facade — not in a server.

The PRD's platform scope (HTTP layer, MySQL/Redis/MQ/ES, distributed locking and scheduling,
multi-tenant isolation, visual designer, dashboards — §25–§29, §33–§34) stays behind interfaces.
Add an implementation of `Repository`, `Clock`, `OrganizationService`, `ServiceInvoker`,
`Notifier` or `EventPublisher` — do not add infrastructure directly to the engine, and do not add
third-party libraries without a stated reason.

The one-architecture rule stands: a new identity must replace the old one or live in its own
repository, and the docs must describe exactly one architecture per surface.

## Build, Test, and Development Commands

Go (requires Go 1.22+):

- `make build` — build the CLI to `bin/rivulet`.
- `make test` — `go test ./... -race -count=1`.
- `make vet` / `make lint` — `go vet ./...` / `golangci-lint` (lint warns gracefully if absent).
- `make run` — run `examples/hello.dify.yml`; `make examples` — validate every `examples/*.dify.yml`.
- `make agent` — interactive coding-agent loop.
- Single test: `go test ./workflow -run TestEngine -race`.

C++ engine (requires CMake ≥ 3.16 and a C++17 compiler):

- `cmake -S cpp -B cpp/build -G Ninja && cmake --build cpp/build`
- `ctest --test-dir cpp/build --output-on-failure` (or run `cpp/build/wf-tests` directly).
- `cpp/build/wf-cli demo cpp/examples/expense_approval.json --demo-org` — end-to-end PRD example.
- `cpp/build/wf-cli validate <definition.json>` before anything else when editing the DSL.

Build artifacts never enter git: `bin/` is ignored; add `cpp/build/` to `.gitignore` when you
touch it.

## Coding Conventions

**Go**

- Go 1.22, `gofmt`/`goimports`, tabs, 100-column soft limit, explicit error handling, no panics
  in library code.
- One dependency only: `gopkg.in/yaml.v3` for DSL parsing. Adding another needs a stated reason
  in the change description.
- Table-driven tests preferred, `*_test.go` next to the package under test.
- Testable cores take explicit inputs and are split from flag parsing (`sendTrigger`-style
  separation): keep I/O and `os.Getenv` at the composition root, inject clients/policies.
- Keep changes minimal and scoped; prefer the `make` targets; do not reformat unrelated files.

**C++ (`cpp/`)**

- C++17, standard library only, 2-space indent, 100-column soft limit, RAII, no exceptions across
  the public API boundary except documented parse/deploy errors.
- Keep `-Wall -Wextra -Wpedantic` clean; a new warning is a defect.
- Header per concern in `include/wf/`, implementation split by concern in `src/`; public types in
  `namespace wf`, details in anonymous namespaces.
- Add a case to `cpp/tests/test_<layer>.cpp` for every behavior change and keep `README.md` /
  `docs/PRD-mapping.md` in sync.

## Engine-Specific Instructions (agent-facing)

- Every new engine capability needs: DSL parsing, pre-publish validation, execution, history +
  event records, and a test. Half-wired node types are worse than absent ones.
- Record every status change with history and an event; a mutation without an audit record is a
  bug.
- Reject illegal state transitions; never "repair" a broken instance silently.
- Secrets never live in DSL files, definitions, traces, or CLI output. DSL values can reference
  environment-provided inputs (`--input k=v`, `--input-file`, env vars) instead of embedding
  credentials, and trace records must stay redacted for API keys, tokens, secrets, and passwords.
- `code` nodes execute `python3` with the user's privileges and `http-request` sends whatever the
  workflow renders: only run workflow files you trust, and say so when documenting new node
  behavior.
- When a PRD requirement is deliberately not implemented, record it as such in
  `cpp/docs/PRD-mapping.md` with the reason, instead of leaving it ambiguous.

## Coding-Agent Instructions (the `rivulet agent` surface)

- File tools (`list_files`, `read_file`, `edit_file`, `replace_lines`, `write_file`) are confined
  to `--cwd`.
- `--approve never` is a dry run: mutating tools report intent without changing files or running
  commands. Preserve this invariant when adding tools.
- Agent runs write JSONL traces under `.rivulet/runs/` by default; `--trace off` disables them.
- `runtime.ProvideInScope` owns cleanup: capabilities registered in a scope must be disposed when
  the scope closes.
- Use `read_file` with line numbers, then `replace_lines`, when exact text replacement would be
  brittle.

## Configuration

- `OPENAI_API_KEY` / `DEEPSEEK_API_KEY` — model credentials for the agent and `llm` nodes.
- `RIVULET_AGENT_PROVIDER`, `RIVULET_AGENT_MODEL`, `RIVULET_AGENT_ENDPOINT` — agent defaults.
- Workflow inputs arrive via `--input k=v` / `--input-file`; CLI model flags override DSL values
  so a workflow can run against a local stub or another vendor without editing the file.
- The C++ engine has no environment configuration: everything is passed through its API
  (`EngineOptions`, `StartRequest`) or CLI flags (`--repo`, `--org`, `--demo-org`, `--tenant`).
- Credentials stay in the environment: they are never definition fields, capability values, trace
  fields, or CLI output.
