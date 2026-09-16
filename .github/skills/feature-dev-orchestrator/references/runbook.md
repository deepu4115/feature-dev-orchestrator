# Runbook

## Deep Planning Phase (Required Before Implementation)
1. Read `PLAN.md` and extract requirements and acceptance criteria.
2. `go run ./cmd/feature-dev discover` and read `.feature/repositories.json`.
3. Inspect each relevant repository: entrypoints, modules, APIs, tests, ownership boundaries.
4. Write the planning bundle under `.feature/plans/draft/`:
   - `requirements.json` — authoritative requirement list
   - `assumptions.json` — confidence, impact, evidence
   - `risks.json` — level, type, mitigation
   - `impact.json` — repos and change areas
   - `repo-analysis.json` — evidence paths per repo
   - `workspace-verify.json` — optional cross-repo integration commands
5. Write `.feature/tasks/tasks.json` with task id, title, repository, dependencies, verification, planning metadata.
6. Do not modify production source during planning.
7. `go run ./cmd/feature-dev graph`
8. `go run ./cmd/feature-dev task preview --json`
9. `go run ./cmd/feature-dev plan submit --from-tasks`
10. If validation fails:
    - `go run ./cmd/feature-dev plan clarify --json`
    - Present questions to the user and **wait for answers**
    - Update draft bundle + tasks.json
    - Resubmit with `plan submit --from-tasks`
11. `go run ./cmd/feature-dev review --json`
12. Present tasks.json-first plus requirements, assumptions, risks, impact, and warnings.
13. Wait for explicit approval language such as: approve, approved, looks good proceed, go ahead with implementation.
14. `go run ./cmd/feature-dev approve`
15. Only then begin implementation.

If the user requests changes:
1. `go run ./cmd/feature-dev replan --reason "..."`
2. Revise draft bundle and `.feature/tasks/tasks.json`.
3. `go run ./cmd/feature-dev plan submit --from-tasks`
4. Repeat review and approval.

## Clarification Loop
When strict validation fails, workflow enters `CLARIFICATION_NEEDED`. The CLI writes `.feature/plans/draft/clarification-request.json` with targeted questions. Do not guess — ask the user, record answers in `clarification-responses.json` (or edit files directly), then resubmit.

## Standard Autonomous Flow (Post-Approval)
1. `go run ./cmd/feature-dev reconcile`
2. `go run ./cmd/feature-dev execute-loop --json`
3. Read JSON and branch by stop reason.
4. Apply code changes in assigned repository only.
5. Run task verification command/tests.
6. Return to step 2 until all tasks are DONE.
7. `go run ./cmd/feature-dev finalize --json` — runs traceability + cross-repo verify; auto-sets `COMPLETED` on pass.

## Agent-Hint Routing

| reason | Action |
|--------|--------|
| `planning_bundle_incomplete` | Complete draft bundle files |
| `plan_needs_clarification` | `plan clarify --json`, ask user, fix, resubmit |
| `tasks_awaiting_approval` | Present review, wait for approval |
| `plan_approved_ready` | reconcile + execute-loop |
| `awaiting_final_verification` | finalize --json |
| `feature_completed` | Summarize outcome |

## Planning Heuristics For Dependencies
- Add a dependency edge only when one task cannot be verified without another.
- Prefer narrow tasks that end in a concrete verification command.
- Separate infra/setup tasks from behavior tasks when verification differs.
- Keep cross-repo dependencies explicit.

## Task Quality Checklist
- Unique task id (for example T001).
- Repository name matches `.feature/repositories.json`.
- Repository rationale and ownership confidence recorded.
- Requirement IDs traced on tasks and in requirements.json.
- Dependencies reference valid task ids.
- Verify command is runnable inside assigned repository.

## Hard Stop Conditions
Stop and request user input when:
- requirements are ambiguous and block implementation choices
- repository ownership for a task is unknown
- verification command is missing and cannot be inferred safely
- plan validation fails (use clarification loop)
- workflow is `CLARIFICATION_NEEDED` or `REVIEW_PENDING`

Do not infer approval from ambiguous responses.
