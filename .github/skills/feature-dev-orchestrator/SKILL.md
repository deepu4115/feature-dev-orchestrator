---
name: feature-dev-orchestrator
description: 'Use for workspace-native feature orchestration with feature-dev CLI and a feature plan such as PLAN.md, including task DAG planning, repository assignment, dependency validation, and autonomous execute-loop cycles with minimal manual intervention in Copilot/Cursor. Trigger when user says: use feature-dev command, implement PLAN.md, feature-dev workflow, execute-loop orchestration, or reconcile and continue.'
argument-hint: 'Provide feature goal, constraints, and repos in scope; choose quick or thorough planning.'
user-invocable: true
---

# Feature Dev Orchestrator Skill

Use this skill when you want the agent to run feature work through the feature-dev CLI with deterministic task state, repository boundaries, and low-touch execution.

## When To Use
- Multi-repository feature implementation.
- Single-repository work where deterministic resume is required.
- Agent-driven planning of tasks and dependencies before coding.
- Repeated execute-loop cycles with bounded verification retries.
- User prompt includes phrases like "use feature-dev command" or "run feature-dev workflow".
- User provides a feature plan such as `PLAN.md` and asks the agent to implement it.

## Required Principles
1. The CLI is the source of truth for task transitions.
2. Never manually edit task status fields during normal flow.
3. Run orchestration commands from workspace root.
4. Perform code edits only in the repository assigned to the active task.
5. Always verify task code before returning to orchestration.

## Procedure

### 1) Bootstrap
Run:
- `go run ./cmd/feature-dev init`
- `go run ./cmd/feature-dev discover`
- `go run ./cmd/feature-dev doctor`
- `go run ./cmd/feature-dev status --json`
- `go run ./cmd/feature-dev agent-hint --json`

If doctor reports issues, fix initialization/discovery first.

### 2) Deep Planning (Agent-Owned, No Production Code)
Allowed paths during planning: `.feature/**`, `PLAN.md`, planning/review artifacts only.

#### 2a) Understand and discover
- Read `PLAN.md`; extract requirements and acceptance criteria.
- Confirm repositories via `discover` and `.feature/repositories.json`.

#### 2b) Repository and impact analysis
- Inspect each relevant repository (structure, modules, APIs, tests).
- Write `.feature/plans/draft/repo-analysis.json` with evidence paths per repo.
- Write `.feature/plans/draft/impact.json` with affected repos and change areas.

#### 2c) Requirements, assumptions, risks
- Write `.feature/plans/draft/requirements.json` (authoritative R001… list).
- Write `.feature/plans/draft/assumptions.json` (confidence, impact, evidence).
- Write `.feature/plans/draft/risks.json` (level, type, mitigation).
- Optional: `.feature/plans/draft/workspace-verify.json` for cross-repo integration commands.

Use templates under [assets/](assets/).

#### 2d) Task graph
- Write [`.feature/tasks/tasks.json`](assets/task-dag-template.json) with:
  - `id`, `title`, `repository`, `dependencies`, `verification`
  - `repository_rationale`, `ownership_confidence`, `requirement_ids`, `planned_verification`
- Run `go run ./cmd/feature-dev graph`
- Run `go run ./cmd/feature-dev task preview --json`

#### 2e) Submit, clarify, review, approve
- `go run ./cmd/feature-dev plan submit --from-tasks`
- On failure → `go run ./cmd/feature-dev plan clarify --json` → present questions → **wait for user** → update draft bundle → resubmit
- On pass → `go run ./cmd/feature-dev review --json` + `graph`
- Present tasks.json-first plus requirements, assumptions, risks, impact, and warnings
- Wait for explicit approval → `go run ./cmd/feature-dev approve`
- On rejection → `go run ./cmd/feature-dev reject --reason "..."` or `replan` → revise → resubmit

If the user requests changes:
- `go run ./cmd/feature-dev replan --reason "..."`
- Revise draft bundle + `tasks.json`, resubmit with `plan submit --from-tasks`

Planning rules:
1. Do not modify production code during planning.
2. All required draft bundle files must exist before submit passes strict validation.
3. Do not guess repo ownership, requirement deferrals, or risk mitigations when validation fails — ask the user.
4. Do not start implementation until `workflow_status: APPROVED`.

### 2f) Agent-hint routing

| reason | Agent action |
|--------|----------------|
| `repositories_not_discovered` | run discover, then planning |
| `no_tasks_defined` | write tasks.json + draft bundle |
| `planning_bundle_incomplete` | complete draft bundle files |
| `tasks_need_submit` | `plan submit --from-tasks` |
| `task_plan_invalid` / `plan_needs_clarification` | `plan clarify --json`, ask user, fix, resubmit |
| `tasks_awaiting_approval` | present review, wait for approval |
| `plan_rejected` / `plan_replanning` | replan, revise, resubmit |
| `plan_approved_ready` | reconcile + execute-loop |
| `task_awaiting_implementation` | code in assigned repo, then `implement <task-id>` + execute-loop |
| `ready_for_orchestration` | reconcile + execute-loop |
| `awaiting_final_verification` | `finalize --json` |
| `feature_completed` | summarize outcome |

### 3) Reconcile and Execute (After Approval Only)
Run:
- `go run ./cmd/feature-dev reconcile`
- `go run ./cmd/feature-dev execute-loop --json`

#### Task status lifecycle (agent-driven via CLI)
Each task moves through:

`READY` → `RUNNING` → `IMPLEMENTED` → `VERIFYING` → `DONE`

| Transition | Command | Who |
|------------|---------|-----|
| → `RUNNING` | `execute-loop` / `execute-next` / `task start` | CLI |
| → `IMPLEMENTED` | `implement <task-id>` | **Agent** (after code changes) |
| → `VERIFYING` → `DONE` | `execute-loop` / `execute-next` (on verify pass) | CLI |

After coding, always run:

```bash
go run ./cmd/feature-dev implement <task-id>
go run ./cmd/feature-dev execute-loop --json
```

Do not call `verify` on a `RUNNING` task — it requires `IMPLEMENTED` or `VERIFYING`.
`execute-loop` verifies and marks `DONE` automatically when tests pass.

#### Per-task execution cycle
1. `execute-loop --json` starts the next ready task (`RUNNING`) or verifies an `IMPLEMENTED` task.
2. If stop reason is `awaiting_code_changes`, edit code in the task's assigned repository only.
3. Run `go run ./cmd/feature-dev implement <task-id>` to mark coding complete.
4. Run `go run ./cmd/feature-dev execute-loop --json` again — verify on pass → `DONE`.
5. Repeat until all tasks are `DONE`, then `finalize --json`.

Execution rules:
1. Confirm workflow status is `APPROVED`, `EXECUTING`, or `VERIFYING`.
2. Do not run execute-loop while `CLARIFICATION_NEEDED` or `REVIEW_PENDING`.
3. Implement only the assigned task in its assigned repository.
4. After code changes, run `implement <task-id>` before rerunning execute-loop.
5. Run `go run ./cmd/feature-dev finalize --json` when all tasks are DONE (traceability + cross-repo verify → `COMPLETED`).
6. If finalize fails, inspect the report, fix gaps, rerun finalize.

Interpret stop reasons:
- `plan_not_approved`: run review flow and obtain approval before coding.
- `awaiting_code_changes`: edit code in assigned repo, run `implement <task-id>`, then rerun execute-loop.
- `awaiting_final_verification`: run finalize.
- `feature_completed`: summarize and stop.
- `verify_failure_budget_reached`: inspect artifacts, fix code, run `implement <task-id>` if needed, rerun loop.
- `no_executable_task`: inspect dependencies or run finalize if all tasks DONE.

### 4) Command-Driven Skill Routing
- Run `go run ./cmd/feature-dev agent-hint --json` each cycle.
- Follow `suggested_next_command` and `reason`.

### 5) Cycle Output Contract
For every cycle, report:
1. current status/stop reason
2. active task id and repository
3. code changes performed
4. verification command and result
5. next command to run

## Guardrails
- Avoid destructive git operations.
- Keep edits scoped to current task.
- Do not fabricate completion without CLI transition.
- Do not infer approval from ambiguous phrases ("sounds good", "maybe", "probably fine").
- Valid approval phrases: "approve", "approved", "go ahead with implementation", "looks good, proceed".

## Suggested Invocation

```text
Use feature-dev-orchestrator to implement PLAN.md.

Do deep planning first: inspect relevant repositories, write the planning bundle under
.feature/plans/draft/ (requirements, assumptions, risks, impact, repo-analysis), and
write .feature/tasks/tasks.json with a valid DAG. Do not modify production code during planning.

Then run:
  feature-dev plan submit --from-tasks
  feature-dev review --json
  feature-dev graph

If validation fails, run feature-dev plan clarify --json, ask me targeted questions,
update the planning bundle, and resubmit.

Show me the tasks.json task list, repo assignments, dependencies, assumptions, risks,
and any validation warnings. Wait for my explicit approval before implementation.

After I approve, run feature-dev approve and then execute-loop --json.
When execute-loop stops with awaiting_code_changes, implement the task in its assigned
repository, run feature-dev implement <task-id>, then rerun execute-loop --json.
When all tasks are DONE, run feature-dev finalize --json.
```

Short form:

`/feature-dev-orchestrator Implement PLAN.md using feature-dev orchestration.`

For detailed branch behavior and loop policy, use [Runbook](./references/runbook.md).
