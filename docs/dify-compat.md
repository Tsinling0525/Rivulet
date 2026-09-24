# Dify DSL compatibility

Rivulet implements a subset of Dify's workflow DSL (version `0.6.0`) with Dify's own
field names and semantics, so files can be exported from Dify, run locally, and edited
back. This page is the honest inventory: what is implemented, which fields are accepted
but ignored, and what will never be implemented.

## Envelope

| DSL | Support |
|---|---|
| `version` | Read; a missing version warns, a different major version warns |
| `kind: app` | Required |
| `app.name`, `app.description`, `app.icon`, `app.icon_type` | Read; name is used in output |
| `app.mode: workflow` | Supported (start → end) |
| `app.mode: advanced-chat` | Supported (start → answer) |
| `app.mode: chat/completion/agent-chat/rag-pipeline` | Warning-free rejection: `app.mode must be ...` |
| `workflow.graph.nodes` / `edges` / `viewport` | Read; `viewport`, `position`, `type: custom`, `zIndex`, `selected` ignored for execution |
| `workflow.features` | Accepted and ignored (opening statement, suggestions, TTS, moderation, ...) |
| `workflow.environment_variables` | Resolved into the `env` pool as `{{#env.NAME#}}` |
| `workflow.conversation_variables` | Accepted and ignored (no memory yet) |
| `dependencies` | Accepted and ignored (no plugin system) |
| `parentId`, `iteration-start`/`loop-start` containers | Error: containers are not supported |

## Nodes

### Implemented

| Type | Honoured fields | Output fields |
|---|---|---|
| `start` | `variables[]` (`variable`, `required`); optional unset inputs resolve to `""` | one field per declared variable (+ undeclared inputs pass through with a log line) |
| `end` | `outputs[]` (`variable`, `value_selector`, `value_type`) | merged into the run result; an unresolved selector emits `null` |
| `answer` | `answer` (rendered) | `answer` |
| `llm` | `model.provider`, `model.name`, `model.endpoint`, `model.completion_params` (`temperature`, `max_tokens`, others pass through), `prompt_template[]` roles `system`/`user`/`assistant`, `retry_config` | `text`, `usage` |
| `code` | `code`, `code_language: python3`, `variables[]`, `outputs` (declared keys are checked for presence, not coerced) | one field per key of the returned dict |
| `if-else` | `cases[]` with `case_id`, `logical_operator` (`and`/`or`), `conditions[]` (`variable_selector`, `comparison_operator`, `value`) | `result` plus the selected `sourceHandle` |
| `template-transform` | `template` (expressions only), `variables[]` | `output` |
| `http-request` | `method`, `url`, `headers`, `params`, `body.type` (`json`/`raw`/`form-data`/`x-www-form-urlencoded`), `authorization` (`no-auth`/`api-key`/`custom`), `timeout` | `status_code`, `body` (decoded JSON when possible), `headers` |
| `variable-aggregator` | `variables[][]` (first resolvable wins), `output_type` (accepted, values are not coerced) | `output` |

### Accepted but not implemented (validation warnings)

| Field | Behavior |
|---|---|
| `llm.context` / `context.variable_selector` | No retrieval happens; no context is injected |
| `llm.vision` | Image inputs are ignored |
| `llm.structured_output` | The node returns raw text |
| `llm.memory` | No conversation memory |
| `fail-branch` edges + `error_strategy: fail-branch` | Not implemented; use `error_strategy` on the node instead |
| `start` variables typed `file` | Passed through as a path string; no upload/download |
| `code_language: javascript` | Error; only `python3` runs |

### Not implemented (validation errors)

`iteration`, `loop`, `iteration-start`, `loop-start`, `tool`, `knowledge-retrieval`,
`agent`, `parameter-extractor`, `question-classifier`, `document-extractor`,
`list-operator`, `assigner`/`variable-assigner`, `datasource`, and the `trigger-*` nodes
(schedule, webhook, plugin).

Two of these have practical replacements today:

- **`tool`** → call the API directly with an `http-request` node (add `authorization:
  api-key` for bearer tokens).
- **`question-classifier`** → chained `if-else` nodes with `contains`/`is` conditions.

## Comparison operators

`contains`, `not contains`, `start with`, `end with`, `is`, `is not`, `=`, `!=`, `>`,
`<`, `>=`, `<=`, `empty`, `not empty`, `null`, `not null`, `in`, `not in`.

Numeric operators coerce numeric-looking values and error on non-numeric operands;
`is`/`is not` compare numerically when both sides are numbers and by rendered value
otherwise; `in`/`not in` accept a list (`[]`), and for a string container they degrade to
substring matching.

## Deliberate differences from Dify

| Area | Dify | Rivulet |
|---|---|---|
| Templates | Jinja2 (filters, loops, conditionals) | Plain `{{ name }}` and `{{#node.field#}}` substitution; no filters, loops, or conditionals. An unknown variable is an error, not an empty string |
| Execution | Queue-based engine with parallel workers in a Python service | Go scheduler with edge-driven readiness, bounded parallelism (default 4), deterministic step indices |
| Code sandbox | Sandboxed code execution service | Local `python3` subprocess with the caller's privileges; trusted input only |
| Persistence | Postgres, run history, workspaces, users | None; stdin/stdout and an optional `--trace` file |
| Cost/tokens | Tracked per run | `usage` is passed through from the provider when present |
