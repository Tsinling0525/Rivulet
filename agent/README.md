# Agent Harness

This package is the first non-DAG agent control loop for Rivulet.

The minimal loop is:

```text
goal -> plan -> one tool call -> observation -> reflection -> stop/replan
```

`Planner` and `Reflector` are interfaces so LLM-backed implementations can be added
without coupling the harness to a specific provider. Tool failures are captured as
observations, letting the reflector decide whether to stop or replan.
