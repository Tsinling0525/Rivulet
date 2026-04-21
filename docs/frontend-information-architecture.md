# Rivulet Frontend Information Architecture

## Current Product Direction

Rivulet is now organized as an all-in-one AI product rather than a workflow-engine dashboard. Workflows remain an orchestration layer and automation primitive, but they are no longer the primary top-level product abstraction.

Implemented top-level navigation:

```text
Home
Research
Create
Track
Library
Automations
System
```

Current product route model:

```text
/
/research
/create
/track
/library
/automations/workflows
/system/runs
```

Product-facing surfaces:

- `Research`: paper search, summaries, citation outputs, and research history.
- `Create`: video generation, scripts, storyboards, captions, and media review.
- `Track`: meal logging, nutrition estimates, confidence flags, and personal history.
- `Library`: durable user-facing outputs and source files.

Advanced/operator surfaces:

- `Automations`: workflow catalog, schedules, templates, and the generic fallback workspace.
- `System`: runs, reviews, traces, model-call observability, plugins, and settings.

Naming shift:

```text
Workflow        -> Automation, except in advanced workflow catalog
Run             -> Task or activity in product pages
Execution trace -> System trace
Asset           -> Library item or output
Review gate     -> Approval
Node            -> Step, except in System and Automations
```

The older workflow-first IA below is retained as historical implementation context for the engine-facing surfaces. Product work should prioritize the current navigation above.

This document defines the MVP information architecture, page model, low-fidelity wireframes, reusable workspace pattern, and component system for evolving Rivulet from a simple chat playground into a personal AI workflow console.

Rivulet should feel like an operator workspace for durable AI workflows. Chat is available where it helps, but the primary objects are workflows, runs, reviews, events, and artifacts.

## Product Model

Rivulet has three distinct user-facing concepts:

| Concept | Meaning | Example |
| --- | --- | --- |
| Workflow definition | The saved workflow, its nodes, AI metadata, review policy, and default configuration. | Paper search workflow with `kind: "ai_workflow"` and medium risk. |
| Workflow execution | One run of a workflow with specific inputs, events, state transitions, model calls, and status. | Paper search run started at 10:03 with query "agentic coding papers". |
| Output artifact | Durable output created by a run and available after the run completes. | `summary.md`, `citations.bib`, `video.mp4`, `meal_log.json`. |

The frontend should keep these concepts visibly separate. A user should know whether they are editing what a workflow does, watching one execution, or using an artifact produced by a past execution.

## Top-Level IA

```text
Rivulet
├─ Home
├─ Workflows
│  ├─ Workflow Index
│  └─ Workflow Workspace
│     ├─ Current Run
│     ├─ Run History
│     ├─ Workflow Assets
│     └─ Workflow Settings
├─ Runs
│  └─ Run Detail / Execution Trace
├─ Review
│  └─ Human Review Queue
├─ Assets
│  └─ Global Artifacts / Files / History
└─ Settings
   ├─ Models
   ├─ Plugins / Nodes
   ├─ Data Storage
   ├─ Runtime
   └─ Review Policies
```

### Shared Platform Pages

Shared platform pages operate across all workflows:

- `Home`: cross-workflow operational overview.
- `Workflows`: browse, create, open, and run workflows.
- `Runs`: all workflow executions and trace debugging.
- `Review`: centralized human review queue.
- `Assets`: global artifact browser and history.
- `Settings`: model inventory, plugins, storage, runtime, and review policy.

### Workflow-Specific Workspaces

Each workflow has a workspace adapted to its domain:

- Paper search and summary
- Video generation
- Meal logging and nutrition analysis
- Future custom AI workflows

The workspace keeps the same layout and operational controls, but customizes inputs, execution labels, artifact views, exports, and review criteria.

## Route Model

Suggested MVP routes:

```text
/                         Home
/workflows                Workflow index
/workflows/:workflowID    Workflow workspace
/workflows/:workflowID/history
/workflows/:workflowID/assets
/runs                     Run index
/runs/:runID              Run detail / execution trace
/review                   Human review queue
/assets                   Global assets and history
/settings                 Settings
```

Optional later routes:

```text
/workflows/:workflowID/definition
/workflows/:workflowID/settings
/settings/models
/settings/plugins
/settings/review
```

For MVP, keep route count small and use tabs inside pages instead of deeply nested pages.

## Why This IA

A single chat page hides important workflow concepts:

- A workflow may run without chat.
- A workflow may produce several artifacts, not one message.
- A workflow may pause for human review.
- AI model calls need observability.
- Runs need traceability after completion.
- Workflow definitions need durable configuration.

Rivulet's frontend should treat chat as one interaction layer inside a workflow workspace. The core product should expose workflow configuration, execution state, review gates, event traces, and artifacts.

## App Shell

```text
┌──────────────────────────────────────────────────────────────┐
│ Top Bar: Rivulet        Quick Launch    Search        User   │
├───────────────┬──────────────────────────────────────────────┤
│ Sidebar       │ Page Content                                 │
│ Home          │                                              │
│ Workflows     │                                              │
│ Runs          │                                              │
│ Review        │                                              │
│ Assets        │                                              │
│ Settings      │                                              │
└───────────────┴──────────────────────────────────────────────┘
```

Shell responsibilities:

- Persistent navigation.
- Global quick launcher.
- Global search.
- Clear active page state.
- Status indicator for backend connectivity.
- Compact badge count for pending reviews.

## Core Pages

### Home / Dashboard

Purpose: show current operating state and offer fast entry points.

```text
┌──────────────────────────────────────────────────────────────┐
│ Home                                      Quick Launch        │
├──────────────────────────────────────────────────────────────┤
│ ┌────────────┐ ┌──────────────┐ ┌────────────┐ ┌──────────┐ │
│ │ Active Runs│ │ Needs Review │ │ Failed Runs│ │ Artifacts│ │
│ │     3      │ │      2       │ │     1      │ │    18    │ │
│ └────────────┘ └──────────────┘ └────────────┘ └──────────┘ │
│                                                              │
│ Recent Workflows                                             │
│ ┌────────────────┐ ┌────────────────┐ ┌────────────────┐     │
│ │ Paper Summary  │ │ Video Generator│ │ Meal Logger    │     │
│ │ Last run: ok   │ │ Needs review   │ │ Last run: ok   │     │
│ │ Risk: Medium   │ │ Risk: High     │ │ Risk: Low      │     │
│ └────────────────┘ └────────────────┘ └────────────────┘     │
│                                                              │
│ Recent Activity                                              │
│ 10:08  Paper Summary completed                               │
│ 10:04  Video Generator paused for review                     │
│ 09:51  Meal Logger created nutrition summary                 │
└──────────────────────────────────────────────────────────────┘
```

MVP content:

- Active runs count.
- Pending reviews count.
- Failed runs count.
- Recent workflow cards.
- Recent activity stream.
- Quick launch workflow action.

Avoid making the Home page a marketing page. It should be an operational dashboard.

### Workflows Index

Purpose: browse and launch durable workflow definitions.

```text
┌──────────────────────────────────────────────────────────────┐
│ Workflows                                      New Workflow  │
├──────────────────────────────────────────────────────────────┤
│ Search workflows...                                          │
│ Filters: All | AI | Automation | Review Required | Failed    │
│ Sort: Recently run                                           │
│                                                              │
│ ┌─────────────────────┐ ┌─────────────────────┐              │
│ │ Paper Search        │ │ Video Generator     │              │
│ │ AI workflow         │ │ AI workflow         │              │
│ │ Purpose: Research   │ │ Purpose: Media      │              │
│ │ Risk: Medium        │ │ Risk: High          │              │
│ │ Review: Required    │ │ Review: Required    │              │
│ │ Last run: Completed │ │ Last run: Paused    │              │
│ │ [Open] [Run]        │ │ [Open] [Resume]     │              │
│ └─────────────────────┘ └─────────────────────┘              │
│                                                              │
│ ┌─────────────────────┐                                      │
│ │ Meal Logger         │                                      │
│ │ AI workflow         │                                      │
│ │ Purpose: Nutrition  │                                      │
│ │ Risk: Low           │                                      │
│ │ Review: Optional    │                                      │
│ │ Last run: Completed │                                      │
│ │ [Open] [Run]        │                                      │
│ └─────────────────────┘                                      │
└──────────────────────────────────────────────────────────────┘
```

Workflow card fields:

- Name.
- Kind.
- Purpose.
- Risk level.
- Human review policy.
- Model inventory summary.
- Last run status.
- Primary action.
- Secondary action.

### Single Workflow Workspace

Purpose: configure inputs, run the workflow, inspect execution, and collect artifacts.

```text
┌──────────────────────────────────────────────────────────────┐
│ Paper Search + Summary      Status: Ready   Run   More       │
│ Purpose: Find papers and summarize them                      │
│ Risk: Medium   Review: Required   Models: gpt-5-mini, embed  │
├────────────────┬─────────────────────────────┬───────────────┤
│ Inputs         │ Execution                   │ Outputs       │
│                │                             │               │
│ Query          │ Tabs: Steps | Chat | Logs   │ Current Run   │
│ [___________]  │                             │ summary.md    │
│                │ Step Timeline               │ citations.bib │
│ Sources        │ 1. Search papers            │ results.json  │
│ [x] arXiv      │ 2. Rank results             │               │
│ [x] Web        │ 3. Summarize                │ Preview       │
│                │ 4. Await review             │ ┌───────────┐ │
│ Date range     │                             │ │ Summary   │ │
│ [Last 5 years] │ Selected Step Detail        │ │ preview   │ │
│                │ model.call, tokens, status  │ └───────────┘ │
│ Model          │                             │               │
│ [gpt-5-mini]   │ Operator Notes / Chat       │ Export        │
│                │ [message input]             │ MD PDF JSON   │
│ [Run]          │                             │               │
└────────────────┴─────────────────────────────┴───────────────┘
```

Workspace header fields:

- Workflow name.
- Purpose.
- Risk level.
- Review policy.
- Model inventory.
- Current run status.
- Actions: run, stop, resume, save preset, more.

Center panel modes:

- `Steps`: default view for operators.
- `Chat`: optional conversational support for the workflow.
- `Logs`: raw event stream and debugging.

### Run Detail / Execution Trace

Purpose: inspect one execution after or during a run.

```text
┌──────────────────────────────────────────────────────────────┐
│ Run: paper-summary / 2026-04-21 10:03                       │
│ Status: Completed   Duration: 38s   Workflow: Paper Search   │
├────────────────┬─────────────────────────────────────────────┤
│ Run Metadata   │ Execution Trace                             │
│                │                                             │
│ Workflow       │ 10:03 run.started                           │
│ Started by     │ 10:03 node.started search                   │
│ Started at     │ 10:04 node.completed search                 │
│ Duration       │ 10:04 ai_model_call gpt-5-mini ok           │
│ Risk level     │ 10:05 artifact.created summary.md           │
│ Review policy  │ 10:06 review.approved                       │
│                │ 10:06 run.completed                         │
│ Inputs         │                                             │
│ Models Used    │ Selected Event Detail                       │
│ Artifacts      │ ┌─────────────────────────────────────────┐ │
│ Reviews        │ │ provider/model                          │ │
│                │ │ prompt hash                             │ │
│                │ │ prompt preview                          │ │
│                │ │ input/output tokens                     │ │
│                │ │ latency                                 │ │
│                │ │ status/error                            │ │
│                │ └─────────────────────────────────────────┘ │
└────────────────┴─────────────────────────────────────────────┘
```

Required event detail for AI calls:

- Provider.
- Model.
- Prompt hash.
- Prompt preview when available.
- Token usage.
- Latency.
- Status.
- Error.
- Linked node.
- Linked artifacts.

### Human Review Page

Purpose: process pending decisions across all workflows.

```text
┌──────────────────────────────────────────────────────────────┐
│ Human Review Queue                                           │
├────────────────┬─────────────────────────────┬───────────────┤
│ Queue          │ Review Item                 │ Decision      │
│                │                             │               │
│ Filters        │ Workflow: Video Generator   │ Action        │
│ [Pending]      │ Run: video-gen / 10:04      │ ( ) Approve   │
│ [High Risk]    │ Node: review-script         │ ( ) Reject    │
│ [Mine]         │                             │ ( ) Request   │
│                │ Proposed Output             │     edits     │
│ Items          │ ┌─────────────────────────┐ │               │
│ > Video Script │ │ Script preview          │ │ Comment       │
│   Meal Advice  │ └─────────────────────────┘ │ [__________]  │
│   Paper Claims │                             │               │
│                │ AI Metadata                 │ [Submit]      │
│                │ model, prompt hash, risk    │               │
│                │                             │               │
│                │ Source Inputs               │               │
│                │ Execution Context           │               │
└────────────────┴─────────────────────────────┴───────────────┘
```

Review decision actions:

- Approve.
- Reject.
- Request edits.

Review page should write an auditable decision event. It should not silently mutate artifacts without a trace.

### Assets / History Page

Purpose: browse durable outputs independently from execution traces.

```text
┌──────────────────────────────────────────────────────────────┐
│ Assets                                                       │
├──────────────────────────────────────────────────────────────┤
│ Search assets...                                             │
│ Filters: Workflow | Type | Date | Status | Reviewed          │
│                                                              │
│ ┌────────────────────┐ ┌────────────────────┐                │
│ │ summary.md         │ │ video.mp4          │                │
│ │ Paper Search       │ │ Video Generator    │                │
│ │ Markdown           │ │ Video              │                │
│ │ Approved           │ │ Pending review     │                │
│ └────────────────────┘ └────────────────────┘                │
│                                                              │
│ ┌────────────────────┐                                      │
│ │ meal_log.json      │                                      │
│ │ Meal Logger        │                                      │
│ │ Structured data    │                                      │
│ │ Exportable         │                                      │
│ └────────────────────┘                                      │
│                                                              │
│ Selected Asset Preview                                      │
│ Metadata | Source Run | Review State | Export | Delete       │
└──────────────────────────────────────────────────────────────┘
```

Asset metadata:

- Name.
- Type.
- Workflow ID.
- Run ID.
- Created at.
- Review state.
- Export formats.
- Storage path or file ID.

### Settings Page

Purpose: configure shared platform behavior.

```text
┌──────────────────────────────────────────────────────────────┐
│ Settings                                                     │
├────────────────┬─────────────────────────────────────────────┤
│ Sections       │ Models                                      │
│                │ ┌─────────────────────────────────────────┐ │
│ Models         │ │ Provider | Model | Enabled | Workflows  │ │
│ Plugins        │ └─────────────────────────────────────────┘ │
│ Storage        │                                             │
│ Runtime        │ Review Policies                             │
│ Review         │ [x] Require review for high-risk workflows  │
│ API            │ [x] Log prompt hashes                       │
│                │ [ ] Require review before external export   │
│                │                                             │
│                │ Storage                                     │
│                │ RIV_DATA_DIR                                │
│                │ Workflow files, scripts, artifacts          │
└────────────────┴─────────────────────────────────────────────┘
```

MVP settings sections:

- Models.
- Plugins and nodes.
- Storage.
- Runtime.
- Review policies.
- API/server.

## Reusable Workflow Workspace Pattern

All workflow workspaces use a three-panel layout:

```text
┌────────────────┬─────────────────────────────┬───────────────┐
│ Left Panel     │ Center Panel                │ Right Panel   │
│ Inputs         │ Execution                   │ Outputs       │
│ Configuration  │ Steps                       │ Artifacts     │
│ Presets        │ Chat                        │ Exports       │
│ Parameters     │ Logs                        │ Review State  │
└────────────────┴─────────────────────────────┴───────────────┘
```

### Shared Across Workflows

- Workflow header.
- Run, stop, and resume actions.
- Current run status.
- Risk level badge.
- Review required badge.
- Model inventory summary.
- Step timeline.
- Event log.
- Model metadata drawer.
- Human review state.
- Artifact list.
- Export menu.
- Run history link.
- Error display.

### Customized Per Workflow Type

- Input fields.
- Domain-specific presets.
- Step labels.
- Output preview.
- Artifact types.
- Export formats.
- Review criteria.
- Suggested chat prompts.

### Panel Behavior

Left panel:

- Structured inputs.
- Runtime parameters.
- Workflow-specific presets.
- Review toggle if allowed.
- Run action.

Center panel:

- Step status.
- Node events.
- Chat or operator notes.
- Logs and trace diagnostics.
- Selected event detail.

Right panel:

- Current output artifacts.
- Artifact previews.
- Approval state.
- Export actions.
- Links to source run and review decision.

## Example Workflow Adaptations

### Paper Search + Summary

```text
┌────────────────┬─────────────────────────────┬───────────────┐
│ Inputs         │ Execution                   │ Outputs       │
│ Query          │ Search papers               │ Paper list    │
│ Sources        │ Rank results                │ Summary       │
│ Date range     │ Fetch abstracts/full text   │ Citations     │
│ Summary depth  │ Summarize                   │ Export MD/PDF │
│ Citation style │ Await review                │ BibTeX        │
└────────────────┴─────────────────────────────┴───────────────┘
```

Custom inputs:

- Search query.
- Sources: arXiv, Semantic Scholar, web, uploaded PDFs.
- Date range.
- Max papers.
- Summary depth.
- Citation style.
- Model selection.

Artifacts:

- `paper_results.json`.
- `summary.md`.
- `citations.bib`.
- `review_notes.md`.

Review criteria:

- Citation quality.
- Unsupported claims.
- Source relevance.
- Summary accuracy.

### Video Generation

```text
┌────────────────┬─────────────────────────────┬───────────────┐
│ Inputs         │ Execution                   │ Outputs       │
│ Source content │ Generate outline            │ Script        │
│ Style          │ Draft script                │ Storyboard    │
│ Length         │ Create storyboard           │ Preview video │
│ Voice          │ Render video                │ Captions      │
│ Aspect ratio   │ Await review                │ Export MP4    │
└────────────────┴─────────────────────────────┴───────────────┘
```

Custom inputs:

- Source content.
- Uploaded files.
- Target duration.
- Style preset.
- Voice.
- Aspect ratio.
- Caption format.
- Review before render toggle.

Artifacts:

- `outline.md`.
- `script.md`.
- `storyboard.json`.
- `captions.srt`.
- `video.mp4`.

Review criteria:

- Script approval before expensive rendering.
- Safety or brand compliance.
- Final video approval before export.

### Meal Logging + Nutrition

```text
┌────────────────┬─────────────────────────────┬───────────────┐
│ Inputs         │ Execution                   │ Outputs       │
│ Meal text      │ Parse meal                  │ Nutrition     │
│ Photo upload   │ Estimate portions           │ Daily log     │
│ Portion hints  │ Analyze nutrition           │ Trends        │
│ Goals          │ Generate notes              │ CSV/JSON      │
│ Date/time      │ Flag uncertainty            │               │
└────────────────┴─────────────────────────────┴───────────────┘
```

Custom inputs:

- Meal description.
- Photo upload.
- Portion estimates.
- Meal time.
- Nutrition goals.
- Dietary constraints.

Artifacts:

- `meal_log.json`.
- `nutrition_summary.md`.
- `daily_totals.csv`.
- `uncertainty_flags.json`.

Review criteria:

- Low-confidence portion estimates.
- Medical-risk advice.
- Uncertain ingredient detection.

## Component System

### Navigation And Shell

- `AppShell`
- `TopBar`
- `SidebarNav`
- `GlobalSearch`
- `QuickActionLauncher`
- `BackendStatusIndicator`

### Workflow Components

- `WorkflowCard`
- `WorkflowKindBadge`
- `RiskLevelBadge`
- `ReviewRequiredBadge`
- `WorkflowHeader`
- `WorkflowInputPanel`
- `ParameterField`
- `PresetSelector`
- `RunButton`
- `RunControls`

### Execution Components

- `RunStatusBadge`
- `ExecutionStepList`
- `ExecutionStepItem`
- `EventTimeline`
- `EventDetailPanel`
- `ModelCallEvent`
- `PromptMetadataDrawer`
- `TokenUsageMeter`
- `LatencyIndicator`
- `ErrorEventPanel`

### Review Components

- `ReviewQueueList`
- `ReviewItemPreview`
- `ReviewDecisionPanel`
- `ReviewStateBadge`
- `ReviewHistory`

### Artifact Components

- `ArtifactList`
- `ArtifactCard`
- `ArtifactViewer`
- `MarkdownArtifactViewer`
- `JsonArtifactViewer`
- `VideoArtifactViewer`
- `ExportMenu`
- `SourceRunLink`

### Common States

- `EmptyState`
- `ErrorState`
- `LoadingState`
- `InlineNotice`
- `FilterBar`
- `SearchInput`
- `StatusBadge`

## MVP Build Sequence

Implement in this order:

1. App shell with sidebar, top bar, and routes.
2. Home dashboard using existing metrics where available.
3. Workflow index with workflow cards.
4. Reusable three-panel workflow workspace.
5. Run detail page with event timeline.
6. Human review queue.
7. Artifact browser.
8. Settings page for models, plugins, storage, runtime, and review policy.

Do not build a separate bespoke UI for every workflow first. Build the shared workspace pattern, then adapt per workflow through configuration and domain-specific panels.

## MVP Data Requirements

The frontend needs these API shapes or equivalents:

```text
Workflow
- id
- name
- kind
- purpose
- ai.models
- ai.risk_level
- ai.human_review_required
- ai.workspaceType
- last_run
- status

Run
- id
- workflow_id
- status
- started_at
- completed_at
- duration_ms
- inputs
- events
- artifacts
- reviews

RunEvent
- id
- run_id
- node_id
- type
- timestamp
- status
- message
- metadata

AIModelCallEvent metadata
- provider
- model
- prompt_hash
- prompt_preview
- input_tokens
- output_tokens
- total_tokens
- latency_ms
- status
- error

Review
- id
- workflow_id
- run_id
- node_id
- status
- output_ref
- context
- decision
- reviewer
- decided_at

Artifact
- id
- workflow_id
- run_id
- name
- type
- mime_type
- created_at
- review_state
- storage_ref
- export_formats
```

`ai.workspaceType` selects the workflow interaction UI. Supported MVP values are `default`, `paper`, and `video`; unknown values should fall back to `default`.

## UX Principles

### Chat Is A Layer

Chat should support workflow execution, not replace workflow UI. A user should be able to run, inspect, review, and export without sending a chat message.

### AI Runs Must Be Observable

Model calls should be visible as events. The UI should expose prompt hash, prompt preview when available, model, provider, token usage, latency, status, and errors.

### Reviewability Is A First-Class Flow

Human review should have a central queue, clear decisions, and auditable history. Approving or rejecting output should become part of the run trace.

### Definitions, Executions, And Artifacts Stay Separate

Workflow definitions describe what can run. Runs describe what happened. Artifacts are durable outputs. The UI should not blur those concepts.

### Operator Console Over Chat Playground

Prefer clear panels, tables, timelines, badges, and artifact viewers. Keep the UI minimal, dense, and legible.

### Workflow-Specific Without Fragmenting The Product

Each workflow can customize inputs and outputs, but all workflows should share navigation, status, events, review, artifacts, and history.

## Visual Direction

Use a clean operator-console style:

- Neutral background.
- White or near-white work surfaces.
- Compact badges.
- Strong table and timeline layout.
- Low decoration.
- Clear typography hierarchy.
- Consistent status colors.
- No marketing hero on the app surface.

Suggested status colors:

```text
Ready       neutral
Running     blue
Completed   green
Paused      amber
Needs review amber
Failed      red
Cancelled   gray
```

Suggested risk colors:

```text
Low      green
Medium   amber
High     red
Unknown  gray
```

## Open Implementation Questions

- Should workflow definitions be edited in the MVP UI, or only launched and inspected?
- Should artifacts be stored and served through the API by ID, or referenced by local path under `RIV_DATA_DIR`?
- Should chat messages be persisted as run events, operator notes, or separate conversation records?
- Should review decisions support "request edits" in the backend, or only approve/reject at first?
- Should global search index workflows, runs, artifacts, and review items from the beginning?

For MVP, the recommended default is browse and run saved workflows first, then add workflow editing after the execution and review surfaces are reliable.
