---
name: feature-dev-orchestrator
description: 'Use for workspace-native feature orchestration with feature-dev CLI and a feature plan such as PLAN.md, including task DAG planning, repository assignment, dependency validation, PLAN coverage gates, and autonomous execute-loop cycles with minimal manual intervention in Copilot/Cursor. Trigger when user says: use feature-dev command, implement PLAN.md, feature-dev workflow, execute-loop orchestration, or reconcile and continue.'
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
6. **No LLM inside feature-dev** — you (the calling agent) read `PLAN.md` prose and write `requirements.json` / `tasks.json`; the CLI validates coverage and blocks approval on gaps.

## PLAN.md Template

Use [assets/PLAN-template.md](assets/PLAN-template.md) as the starting point. Key conventions for maximum coverage:

| Convention | Why |
|------------|-----|
| One bullet = one capability | Enables `completeness` gate and clean task decomposition |
| `<!-- req: R001 -->` markers | Stable IDs across PLAN → requirements.json → tasks.json |
| Separate `## Requirements` and `## Acceptance Criteria` | CLI parses both; AC items need task linkage |
| Repo subsections under Requirements | Hints repository ownership for task assignment |
| No bundled "including X, Y, Z" in one bullet | Triggers `decomposition_hints`; split into separate bullets instead |
| Verification note: no `cd repo &&` in commands | Verifier already runs from repository root |

## Three-Link Traceability

Every plan must satisfy all three links before approval:

```
PLAN.md bullets  →  requirements.json  →  tasks.json (requirement_ids)  →  verification
     completeness        decomposition              traceability
```

Check with:

```bash
go run ./cmd/feature-dev plan coverage --from PLAN.md --json
```

Gates in output: `completeness`, `decomposition`, `traceability`, `acceptance_coverage`. Fix `agent_actions` until `valid: true`.

## Procedure

### Recommended agent flow

Default end-to-end path. Run from workspace root; use `go run ./cmd/feature-dev` or `feature-dev` if installed.

```bash
go run ./cmd/feature-dev init && go run ./cmd/feature-dev discover
go run ./cmd/feature-dev plan scaffold --from PLAN.md --json
# Agent reads PLAN.md + coverage gaps; edits .feature/plans/draft/* and tasks.json
go run ./cmd/feature-dev plan coverage --from PLAN.md --json   # repeat until valid
go run ./cmd/feature-dev schema show tasks --json               # if unsure about JSON shape
go run ./cmd/feature-dev task preview --json                    # always before submit
go run ./cmd/feature-dev plan submit --from-tasks
go run ./cmd/feature-dev review --json                          # includes coverage_matrix
# Wait for explicit user approval
go run ./cmd/feature-dev approve
go run ./cmd/feature-dev reconcile && go run ./cmd/feature-dev execute-loop --json
# On awaiting_code_changes: edit code → implement <task-id> → execute-loop again
# When all tasks DONE:
go run ./cmd/feature-dev finalize --json
```

Alternative bootstrap (manual scaffold):

```bash
go run ./cmd/feature-dev plan draft init
go run ./cmd/feature-dev task init
```

Rules:
- Always run `plan coverage --json` after editing requirements/tasks and before submit.
- Always run `task preview --json` before `plan submit --from-tasks`.
- **Structural** errors (bad JSON shape) → `schema show <artifact> --json`, `task validate --json`, or `repair tasks --dry-run --json` (workflow stays `PLANNING`).
- **Coverage** errors (missing PLAN items, orphan requirements) → fix bundle using `agent_actions` from `plan coverage --json`; do not use `plan clarify` for structural issues.
- **Domain** errors (repo ownership, assumptions) → `plan clarify --json`, ask user, fix, resubmit.
- During execution: after code changes run `implement <task-id>` before the next `execute-loop`.
- If a task is stuck, run `task explain <task-id> --json` for blocking reasons.
- Each cycle: `go run ./cmd/feature-dev agent-hint --json` and follow `suggested_next_command`.

### 1) Bootstrap
Run:
- `go run ./cmd/feature-dev init`
- `go run ./cmd/feature-dev discover`
- `go run ./cmd/feature-dev doctor --json`
- `go run ./cmd/feature-dev status --json`
- `go run ./cmd/feature-dev agent-hint --json`

If doctor reports issues, fix initialization/discovery first.

### 2) Deep Planning (Agent-Owned, No Production Code)
Allowed paths during planning: `.feature/**`, `PLAN.md`, planning/review artifacts only.

#### 2a) Understand and discover
- Read `PLAN.md` using [PLAN-template.md](assets/PLAN-template.md) conventions.
- Extract requirements and acceptance criteria as **separate list items** with stable IDs.
- Confirm repositories via `discover` and `.feature/repositories.json`.

#### 2b) Scaffold planning artifacts (CLI-first)
Preferred one-command scaffold:

```bash
go run ./cmd/feature-dev plan scaffold --from PLAN.md --json
```

This runs `plan draft init`, `task init`, initial `plan coverage`, and writes `.feature/plans/draft/manifest.json` with `next_agent_steps`.

Or manually:
- `go run ./cmd/feature-dev plan draft init`
- `go run ./cmd/feature-dev task init`

Use `go run ./cmd/feature-dev schema show <artifact> --json` for contracts (`tasks`, `requirements`, `assumptions`, `risks`, `impact`, `repo-analysis`). Do not reverse-engineer JSON from Go source.

`tasks.json` must be a **top-level JSON array**. Each task needs:
- `verification`: `[{"command": "mvn test"}]` (array of objects, not a string)
- `verification_working_directory`: `repository_root` (default) — **do not** use `cd repo &&` in commands
- `requirement_ids`: links to `requirements.json` entries

`requirements.json` entries should include:
- `id`, `description`
- `source_section`, `source_ref` (copied from PLAN.md for coverage matching)

#### 2c) Repository and impact analysis
- Inspect each relevant repository (structure, modules, APIs, tests).
- Edit `.feature/plans/draft/repo-analysis.json` with evidence paths per repo.
- Edit `.feature/plans/draft/impact.json` with affected repos and change areas.

#### 2d) Requirements, assumptions, risks
- Edit `.feature/plans/draft/requirements.json` — one entry per PLAN bullet (R001, R002, …).
- Edit `.feature/plans/draft/assumptions.json` (confidence, impact, evidence).
- Edit `.feature/plans/draft/risks.json` (level, type, mitigation).
- Optional: `.feature/plans/draft/workspace-verify.json` for cross-repo integration commands.
- If integration env is unavailable, document `cross_repo_verification_deferral` in risks.

Reference templates under [assets/](assets/) match the embedded CLI templates.

#### 2e) Task graph (decompose requirements into tasks)
- Edit `.feature/tasks/tasks.json`:
  - **One task per requirement or per repo-slice** for compound features
  - `id`, `title`, `repository`, `dependencies`, `verification`, `requirement_ids`
  - `repository_rationale`, `ownership_confidence`, `planned_verification`
  - `verification_working_directory`: `repository_root` | `workspace_root` | `custom`
- Run `go run ./cmd/feature-dev graph`
- Run `go run ./cmd/feature-dev plan coverage --from PLAN.md --json` until `valid: true`
- Run `go run ./cmd/feature-dev task preview --json` (or `plan validate --json`)

#### 2f) Submit, clarify, review, approve
- `go run ./cmd/feature-dev plan submit --from-tasks`
- On **structural** failure → `schema show`, `task validate --json`, or `repair tasks --dry-run --json`
- On **coverage** failure → fix `requirements.json` / `tasks.json` per `plan coverage` `agent_actions`; rerun coverage + preview
- On **domain** failure → `plan clarify --json` → present questions → wait for user → resubmit
- On pass → `review --json` (check `coverage_matrix`) + `graph`
- Present tasks.json-first plus requirements, assumptions, risks, impact, and warnings
- Wait for explicit approval → `approve` (blocks if coverage incomplete unless `--defer-unmapped "reason"`)

If the user requests changes:
- `go run ./cmd/feature-dev replan --reason "..."`
- Revise draft bundle + `tasks.json`, rerun coverage, resubmit

Planning rules:
1. Do not modify production code during planning.
2. All required draft bundle files must exist before submit passes validation.
3. Do not guess repo ownership, requirement deferrals, or risk mitigations when validation fails — ask the user.
4. Do not start implementation until `workflow_status: APPROVED`.
5. Split compound PLAN bullets into separate requirements/tasks — do not rely on one vague task for multiple capabilities.

### 2g) Agent-hint routing

| reason | Agent action |
|--------|----------------|
| `repositories_not_discovered` | run discover, then planning |
| `no_tasks_defined` | `plan scaffold --from PLAN.md` or `plan draft init` + `task init` |
| `planning_bundle_incomplete` | `plan draft init`, then `task preview --json` |
| `schema_fix_required` | `schema show <artifact> --json`, fix JSON shape, `task preview --json` |
| `plan_coverage_incomplete` | `plan coverage --from PLAN.md --json`, fix per `agent_actions` |
| `planning_in_progress` | complete draft bundle + tasks, coverage + preview, then submit |
| `tasks_need_submit` | `plan submit --from-tasks` |
| `task_plan_invalid` / `plan_needs_clarification` | `plan clarify --json`, ask user, fix, resubmit |
| `tasks_awaiting_approval` | present review (include `coverage_matrix`), wait for approval |
| `plan_rejected` / `plan_replanning` | replan, revise, resubmit |
| `plan_approved_ready` | reconcile + execute-loop |
| `task_awaiting_implementation` | code in assigned repo, then `implement <task-id>` + execute-loop |
| `ready_for_orchestration` | reconcile + execute-loop |
| `tasks_blocked_or_waiting` | `task explain <id> --json`, then reconcile |
| `awaiting_final_verification` | `finalize --json` |
| `feature_completed` | summarize outcome (`status --json` shows `completion_ready: true`) |

### 3) Reconcile and Execute (After Approval Only)
Run:
- `go run ./cmd/feature-dev reconcile` (or `--dry-run --json` to inspect)
- `go run ./cmd/feature-dev execute-loop --json`

After a task completes, `reconcile` promotes dependency-ready tasks from `REVIEW_PENDING` → `READY` automatically.

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
5. Run `go run ./cmd/feature-dev finalize --json` when all tasks are DONE.
6. Inspect `cross_repo_verify` in finalize output for skipped integration gates and residual risk.
7. If a task won't start, run `task explain <task-id> --json`.

Interpret stop reasons:
- `plan_not_approved`: run review flow and obtain approval before coding.
- `awaiting_code_changes`: edit code in assigned repo, run `implement <task-id>`, then rerun execute-loop.
- `awaiting_final_verification`: run finalize.
- `feature_completed`: summarize and stop.
- `verify_failure_budget_reached`: inspect artifacts, fix code, run `implement <task-id>` if needed, rerun loop.
- `no_executable_task`: run `task explain`, reconcile, or finalize if all tasks DONE.

### 4) Diagnostics and Repair

| Command | When |
|---------|------|
| `doctor --json` | Workspace health check (config, tasks, bundle, coverage, stale artifacts) |
| `plan coverage --from PLAN.md --json` | PLAN → requirements → tasks gap analysis |
| `task validate --json` | Syntax/schema check on tasks.json only |
| `repair tasks --dry-run --json` | Diagnose tasks.json issues without writing |
| `repair tasks --apply --json` | Safe repair with backup under `.feature/backups/` |
| `task explain <id> --json` | Why a task is not ready |
| `reconcile --dry-run --json` | Preview reconcile/promotion without saving |
| `status --json` | Unified status (`completed`, `completion_ready`, `finalization`) |

### 5) Command-Driven Skill Routing
- Run `go run ./cmd/feature-dev agent-hint --json` each cycle.
- Follow `suggested_next_command` and `reason`.
- Trust `status --json` and `agent-hint` over stale `last-validation.json` after finalize.

### 6) Cycle Output Contract
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
- Do not bundle multiple PLAN requirements into one task without explicit user deferral.

## Suggested Invocation

```text
Use feature-dev-orchestrator to implement PLAN.md.

Start from assets/PLAN-template.md conventions (one bullet per capability, req markers).

Run plan scaffold --from PLAN.md, then fill requirements.json and tasks.json so that
plan coverage --json passes completeness, decomposition, and traceability gates.
Create at least one task per requirement with requirement_ids and repository-scoped
verification (no cd repo && prefix). Do not modify production code during planning.

Then run:
  feature-dev plan coverage --from PLAN.md --json
  feature-dev task preview --json
  feature-dev plan submit --from-tasks
  feature-dev review --json
  feature-dev graph

If coverage fails, fix requirements.json and tasks.json per agent_actions — do not clarify.
If domain validation fails, run plan clarify --json, ask me targeted questions, and resubmit.

Show me tasks.json, coverage_matrix, repo assignments, dependencies, and warnings.
Wait for my explicit approval before implementation.

After I approve, run feature-dev approve and execute-loop --json.
When stopped with awaiting_code_changes, implement in the assigned repo, run
feature-dev implement <task-id>, then execute-loop --json again.
When all tasks are DONE, run feature-dev finalize --json.
```

Short form:

`/feature-dev-orchestrator Implement PLAN.md using feature-dev orchestration.`

For detailed branch behavior and loop policy, use [Runbook](./references/runbook.md).
