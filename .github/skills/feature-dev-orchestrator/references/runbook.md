# Runbook

## Standard Autonomous Flow
1. `go run ./cmd/feature-dev reconcile`
2. `go run ./cmd/feature-dev execute-loop --json`
3. Read JSON and branch by stop reason.
4. Apply code changes in assigned repository only.
5. Run task verification command/tests.
6. Return to step 2.

## Planning Heuristics For Dependencies
- Add a dependency edge only when one task cannot be verified without another.
- Prefer narrow tasks that end in a concrete verification command.
- Separate infra/setup tasks from behavior tasks when verification differs.
- Keep cross-repo dependencies explicit.

## Task Quality Checklist
- Unique task id (for example T001).
- Repository name matches `.feature/repositories.json`.
- Dependencies reference valid task ids.
- Verify command is runnable inside assigned repository.
- Description states observable completion criteria.

## Hard Stop Conditions
Stop and request user input only when:
- requirements are ambiguous and block implementation choices
- repository ownership for a task is unknown
- verification command is missing and cannot be inferred safely
- action would require destructive data loss
