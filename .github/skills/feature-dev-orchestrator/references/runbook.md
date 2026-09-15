# Runbook

## Deep Planning Phase (Required Before Implementation)
1. Read `PLAN.md` and extract requirements and acceptance criteria.
2. `go run ./cmd/feature-dev discover` and read `.feature/repositories.json`.
3. Inspect each relevant repository: entrypoints, modules, APIs, tests, ownership boundaries.
4. Build an ownership matrix: requirement → primary repository → task(s), with rationale.
5. Write `.feature/tasks/tasks.json` with:
   - task id, title, repository, dependencies, verification command
   - `repository_rationale`, `ownership_confidence`, `requirement_ids`, `planned_verification`
6. Do not modify production source during planning.
7. `go run ./cmd/feature-dev graph`
8. `go run ./cmd/feature-dev plan submit --from-tasks`
9. `go run ./cmd/feature-dev review --json`
10. Present `.feature/tasks/tasks.json`, repo assignments, DAG, and warnings to the user.
11. Wait for explicit approval language such as:
   - approve
   - approved
   - looks good, proceed
   - go ahead with implementation
12. `go run ./cmd/feature-dev approve`
13. Only then begin implementation.

If the user requests changes:
1. `go run ./cmd/feature-dev replan --reason "..."`
2. Revise `.feature/tasks/tasks.json`.
3. `go run ./cmd/feature-dev plan submit --from-tasks`
4. Repeat review and approval.

Useful inspection commands:
- `go run ./cmd/feature-dev task preview --json`
- `go run ./cmd/feature-dev plan-diff`
- `go run ./cmd/feature-dev status --json`
- `go run ./cmd/feature-dev agent-hint --json`

## Standard Autonomous Flow (Post-Approval)
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
- Repository rationale and ownership confidence recorded.
- Requirement IDs traced on tasks when possible.
- Dependencies reference valid task ids.
- Verify command is runnable inside assigned repository.
- Description states observable completion criteria.

## Hard Stop Conditions
Stop and request user input only when:
- requirements are ambiguous and block implementation choices
- repository ownership for a task is unknown
- verification command is missing and cannot be inferred safely
- action would require destructive data loss
- plan revision exists but workflow is not `APPROVED`

Do not infer approval from ambiguous responses such as "looks interesting" or "probably fine".
