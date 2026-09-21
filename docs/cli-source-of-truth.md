# CLI as source of truth

Task status, leases, and audit history must be mutated only through `feature-dev` commands.

Do not hand-edit `.feature/tasks/tasks.json` or `.feature/state/workflow.json` during a live run.
If state was edited manually, run:

```text
feature-dev reconcile --dry-run --json
feature-dev reconcile --repair-invariants --json
```

Recovery commands:

- `feature-dev task unblock <id> --reason "..."`
- `feature-dev task resume <id>`
- `feature-dev task recover <id>`

Every status change appends an event to `.feature/state/task-summaries.jsonl`.
Command invocations may also be recorded in `.feature/state/command-journal.jsonl`.
