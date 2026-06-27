# Agent Harness

This package is the first non-DAG agent control loop for Rivulet.

The minimal loop is:

```text
goal -> plan -> one tool call -> observation -> reflection -> stop/replan
```

`Planner` and `Reflector` are interfaces so LLM-backed implementations can be added
without coupling the harness to a specific provider. Tool failures are captured as
observations, letting the reflector decide whether to stop or replan.

## Verification Loop

`VerificationHarness` wraps the agent loop with a grader:

```text
goal -> agent run -> grade -> pass
                  \-> feedback -> retry
```

The grader can be deterministic, LLM-backed, or human-backed. When a grade fails, the
feedback is appended to the next attempt's goal so the inner agent loop can correct
itself without coupling planner implementations to a specific evaluation system.

This maps the first two loop-engineering layers directly into Rivulet:

- Agent loop: `Harness`
- Verification loop: `VerificationHarness`

Event-driven loops are handled outside this package by workflow scheduling and trigger
infrastructure. Hill-climbing loops can consume persisted run events and verification
grades to propose prompt, tool, or rubric changes.
