---
name: feature-dev-orchestrator
description: 'Use for workspace-native feature orchestration with feature-dev CLI and a feature plan such as PLAN.md, including task DAG planning, repository assignment, dependency validation, PLAN coverage gates, single-active-task execution leases, recovery CLIs, and autonomous execute-loop cycles with minimal manual intervention in Copilot/Cursor. Trigger when user says: use feature-dev command, implement PLAN.md, feature-dev workflow, execute-loop orchestration, or reconcile and continue.'
argument-hint: 'Provide feature goal, constraints, and repos in scope; choose quick or thorough planning.'
user-invocable: true
---

# Feature Dev Orchestrator Skill

Use this skill when you want the agent to run feature work through the feature-dev CLI with deterministic task state, repository boundaries, and low-touch execution.

## When To Use
- Multi-repository feature implementation.
- Single-repository work where deterministic resume is required.
- Agent-driven planning of tasks and dependencies before coding.
- Repeated execute-loop cycles with bounded verification and explicit rework recovery.
- User prompt includes phrases like "use feature-dev command" or "run feature-dev workflow".
- User provides a feature plan such as `PLAN.md` and asks the agent to implement it.

## Required Principles
1. **CLI is the only source of truth** for task status, leases, and audit history. Never hand-edit `.feature/tasks/tasks.json` or `.feature/state/workflow.json` during a live run. See [docs/cli-source-of-truth.md](../../../docs/cli-source-of-truth.md).
2. Run orchestration commands from workspace root (`go run ./cmd/feature-dev` or installed `feature-dev`).
3. Perform code edits only in the repository assigned to the **active** task.
4. Always mark implementation complete with `implement <task-id>` before the next `execute-loop`.
5. **No LLM inside feature-dev** — you (the calling agent) read `PLAN.md` and write draft/tasks JSON; the CLI validates and blocks on gaps.
6. **At most one in-flight task** (`RUNNING` / `IMPLEMENTED` / `VERIFYING`). Never start a second task while another is active.
7. **Fix structural/schema problems before domain clarification.** Do not ask conceptual questions when JSON shape or repository IDs are wrong.

## Hard Invariants (do not violate)

| Invariant | Rule |
|-----------|------|
| Single active task | `schedulable = dependency_ready AND no_active_task AND lease free AND not scheduling_paused` |
| Atomic claim | Only one `execute-next` / `task start` may claim the workflow lease |
| Audited transitions | Every status change writes `.feature/state/task-summaries.jsonl` (prev/next, reason, actor, command, revision) |
| BLOCKED metadata | A task cannot be `BLOCKED` without `blocked_reason` + `recovery_command` |
| Verify failure ownership | On verification failure, stop with `awaiting_rework` — do **not** start another READY task |
| DONE is terminal for execute | `execute-next --task <DONE>` / `execute-loop --task <DONE>` must not re-queue; expect `already_done` |
| Lifecycle | `REVIEW_PENDING → READY → RUNNING → IMPLEMENTED → VERIFYING → DONE` (never skip READY in audit) |

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
| Prefer real tools in repos | e.g. `mvn test` if no `mvnw`; CLI rejects missing wrappers at plan validate |

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

Gates: `completeness`, `decomposition`, `traceability`, `acceptance_coverage`. Fix `agent_actions` until `valid: true`.

## Procedure

### Recommended agent flow

```bash
go run ./cmd/feature-dev init && go run ./cmd/feature-dev discover
go run ./cmd/feature-dev plan scaffold --from PLAN.md --json
# Scaffold seeds repo-aware tasks from repositories.json when present.
# It fails closed on placeholder repos (repo-a/repo-b) and invalid verify commands.
# Follow manifest next_agent_steps; use schema show for contracts.

# Agent reads PLAN.md + coverage gaps; edits .feature/plans/draft/* and tasks.json
go run ./cmd/feature-dev plan coverage --from PLAN.md --json   # until valid
go run ./cmd/feature-dev schema show tasks --json              # if unsure about shape
go run ./cmd/feature-dev schema show workspace-verify --json   # when cross-repo deps exist
go run ./cmd/feature-dev task preview --json                   # always before submit
go run ./cmd/feature-dev plan submit --from-tasks
go run ./cmd/feature-dev review --json                         # includes coverage_matrix
# Wait for explicit user approval
go run ./cmd/feature-dev approve
go run ./cmd/feature-dev reconcile && go run ./cmd/feature-dev execute-loop --json
# On awaiting_code_changes: edit code → implement <task-id> → execute-loop again
# On awaiting_rework: fix verifier/code → task resume <id> → execute-loop again
# When all tasks DONE:
go run ./cmd/feature-dev finalize --json
```

Alternative bootstrap (manual):

```bash
go run ./cmd/feature-dev plan draft init
go run ./cmd/feature-dev task init
```

### Planning rules (read carefully)

- Always run `plan coverage --json` after editing requirements/tasks and before submit.
- Always run `task preview --json` before `plan submit --from-tasks`.
- **Structural** errors (bad JSON, unknown repo/req IDs, missing verify binary) → `schema show <artifact> --json`, `task validate --json`, or `repair tasks --dry-run --json`. Workflow stays `PLANNING`. Do **not** `plan clarify` for these.
- **Coverage** errors → fix bundle using `agent_actions` from `plan coverage --json`.
- **Domain** errors (ownership, assumptions) → `plan clarify --json`, ask user, fix, resubmit.
- Cross-repo dependencies require either:
  - `.feature/plans/draft/workspace-verify.json` with `commands`, **or**
  - `cross_repo_verification_deferral` in risks.
  Missing both is a **validation error**, not a soft warning.
- Verification commands are checked against the assigned repo (e.g. `./mvnw` must exist). Prefer build-system defaults: Maven without wrapper → `mvn test`.

### Schema artifacts

Use `go run ./cmd/feature-dev schema show <artifact> --json` for contracts. Valid artifacts:

`tasks`, `requirements`, `assumptions`, `risks`, `impact`, `repo-analysis`, `workspace-verify`

Do not reverse-engineer JSON from Go source.

`tasks.json` must be a **top-level JSON array**. Each task needs:
- `verification`: `[{"command": "mvn test"}]` (array of objects, not a string)
- `verification_working_directory`: `repository_root` (default) — **do not** use `cd repo &&` in commands
- `requirement_ids`: links to `requirements.json` entries
- Valid `repository` IDs from `.feature/repositories.json`

### Approve vs reconcile (lifecycle)

```
REVIEW_PENDING → READY → RUNNING → IMPLEMENTED → VERIFYING → DONE
                 ↑
         promote (audited)
```

- **`approve`** approves the **plan**. It may unlock **dependency-ready** tasks to `READY` (audited `task_promoted_ready`). It does **not** mean every task is executable.
- **`reconcile`** is the safe resume boundary: promotes remaining dep-ready `REVIEW_PENDING|PLANNED|DRAFT → READY`, detects orphaned active tasks, and may pause scheduling.
- Execution may only start `READY` (or resume an already-active task). Direct `REVIEW_PENDING → RUNNING` is illegal.
- When start needs a promotion, the CLI writes **two** audit events: `→ READY` then `→ RUNNING`.

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
| `task_plan_invalid` / `plan_needs_clarification` | structural → schema/repair; domain → `plan clarify --json` |
| `tasks_awaiting_approval` | present review (include `coverage_matrix`), wait for approval |
| `plan_rejected` / `plan_replanning` | replan, revise, resubmit |
| `plan_approved_ready` | reconcile + execute-loop |
| `task_awaiting_implementation` | code in assigned repo, then `implement <task-id>` + execute-loop |
| `ready_for_orchestration` | reconcile + execute-loop |
| `tasks_blocked_or_waiting` | `task explain <id> --json`, then follow `recovery_command` / `suggested_next_command` (`unblock` / `resume` / `recover`) |
| `awaiting_final_verification` | `finalize --json` |
| `feature_completed` | summarize outcome (`status --json` shows `completion_ready: true`) |

### 3) Reconcile and Execute (After Approval Only)

```bash
go run ./cmd/feature-dev reconcile --dry-run --json   # inspect proposed transitions / orphans
go run ./cmd/feature-dev reconcile --json
go run ./cmd/feature-dev execute-loop --json
```

Useful reconcile flags:
- `--auto-unblock-stale` — promote lease-expired/stale `BLOCKED` tasks whose deps are DONE (off by default)
- `--repair-invariants` — fail if workspace invariants are violated after reconcile

#### Task status lifecycle

| Transition | Command | Who |
|------------|---------|-----|
| → `READY` | `approve` / `reconcile` / `task unblock` / `task resume` / `task recover` | CLI (audited) |
| → `RUNNING` | `execute-loop` / `execute-next` / `task start` | CLI (claims lease) |
| → `IMPLEMENTED` | `implement <task-id>` | **Agent** after code changes |
| → `VERIFYING` → `DONE` | `execute-loop` / `execute-next` on verify pass | CLI |
| → `REWORK` | verification failure | CLI; **stop** — do not schedule peers |
| → `BLOCKED` | reconcile orphan/missing repo | CLI with reason + recovery_command |

After coding:

```bash
go run ./cmd/feature-dev implement <task-id>
go run ./cmd/feature-dev execute-loop --json
```

Do not call `verify` on a `RUNNING` task — it requires `IMPLEMENTED` or `VERIFYING`.

#### Per-task execution cycle
1. `execute-loop --json` resumes the active task, or starts the next **schedulable** `READY` task (never a second RUNNING).
2. If stop reason is `awaiting_code_changes`, edit code in that task's repository only.
3. Run `implement <task-id>`, then `execute-loop --json` again (verify → `DONE` on pass).
4. If stop reason is `awaiting_rework`, fix code and/or verification command, then:
   ```bash
   go run ./cmd/feature-dev task resume <task-id>
   go run ./cmd/feature-dev execute-loop --json
   ```
5. Repeat until all tasks are `DONE`, then `finalize --json`.

#### Recovery commands (CLI-as-SoT — never hand-edit status)

| Situation | Command |
|-----------|---------|
| `BLOCKED` with complete deps / stale lease | `task unblock <id> --reason "..."` (`--force` only if intentional) |
| `REWORK` or `FAILED` after fix | `task resume <id>` |
| Orphaned / lease-expired active task | `task recover <id>` |
| Unsure why stuck | `task explain <id> --json` — follow `recovery_command` / `suggested_next_command` |
| Manual JSON edit already happened | `reconcile --dry-run --json` then `reconcile --repair-invariants --json` |

#### Interpret stop reasons / error codes

| Stop / code | Meaning | Next action |
|-------------|---------|-------------|
| `plan_not_approved` | Plan gate | review + approve |
| `awaiting_code_changes` | Active task needs code | edit → `implement` → execute-loop |
| `awaiting_rework` | Verify failed; peers will not start | fix → `task resume` → execute-loop |
| `awaiting_final_verification` | All tasks DONE | `finalize --json` |
| `feature_completed` | Done | summarize and stop |
| `no_executable_task` | Nothing schedulable | `task explain`, `reconcile`, or finalize |
| `scheduling_paused` | Orphan/active lease pause | `task recover <id>` then continue |
| `workflow_busy` | Another task holds the lease | finish/recover active task first |
| `already_done` | Targeted DONE task | do not re-queue; pick another task or finalize |
| `task_not_executable` | Bad target status | follow explain recovery |
| `verification_command_unavailable` | Bad plan verifier | fix command in tasks.json before approve |

#### Finalize incomplete work

If `finalize --json` returns `not_all_tasks_done`, inspect `remaining_tasks[]`:
- `id`, `status`, `blocker`, `block_kind`, `recovery_command`
- Run the given recovery command, then re-run finalize after all tasks are DONE.

`status --json` also surfaces `active_task_id`, `scheduling_paused_reason`, and `invariants`.

### 4) Diagnostics and Repair

| Command | When |
|---------|------|
| `doctor --json` | Workspace health |
| `plan coverage --from PLAN.md --json` | PLAN → requirements → tasks gaps |
| `task validate --json` | tasks.json syntax/schema |
| `repair tasks --dry-run --json` / `--apply --json` | Diagnose/repair tasks.json with backup |
| `task explain <id> --json` | Blockers + recovery_command |
| `reconcile --dry-run --json` | Proposed transitions, orphans, unblocks |
| `status --json` | Unified status + lease + invariants |
| `schema show <artifact> --json` | Canonical contracts |

### 5) Command-Driven Skill Routing
- Each cycle: `go run ./cmd/feature-dev agent-hint --json`.
- Follow `suggested_next_command` and `reason`.
- Prefer `status --json` / `agent-hint` over stale validation artifacts after finalize.

### 6) Cycle Output Contract
For every cycle, report:
1. current status/stop reason (and error_code if present)
2. active task id, repository, and lease/pause if any
3. code changes performed
4. verification command and result
5. next command to run (prefer explain/agent-hint recovery commands)

## Guardrails
- Avoid destructive git operations.
- Keep edits scoped to the current active task's repository.
- Do not fabricate completion without a CLI transition.
- Do not start a second task while one is `RUNNING`/`IMPLEMENTED`/`VERIFYING`.
- Do not continue execute-loop after `awaiting_rework` without `task resume`.
- Do not hand-edit task status; use `unblock` / `resume` / `recover`.
- Do not infer approval from ambiguous phrases ("sounds good", "maybe").
- Valid approval phrases: "approve", "approved", "go ahead with implementation", "looks good, proceed".
- Do not bundle multiple PLAN requirements into one task without explicit user deferral.
- Do not use placeholder repos (`repo-a`, `repo-b`) or unverified wrappers (`./mvnw` when missing).

## Suggested Invocation

```text
Use feature-dev-orchestrator to implement PLAN.md.

Start from assets/PLAN-template.md conventions (one bullet per capability, req markers).

Run plan scaffold --from PLAN.md, then fill requirements.json and tasks.json so that
plan coverage --json passes completeness, decomposition, and traceability gates.
Create at least one task per requirement with requirement_ids and repository-scoped
verification that exists in that repo (no cd repo && prefix; no missing mvnw).
Declare workspace-verify.json commands or cross_repo_verification_deferral when needed.
Do not modify production code during planning.

Then run:
  feature-dev plan coverage --from PLAN.md --json
  feature-dev task preview --json
  feature-dev plan submit --from-tasks
  feature-dev review --json
  feature-dev graph

If structural/coverage fails, fix JSON per schema show / agent_actions — do not clarify.
If domain validation fails, run plan clarify --json, ask me targeted questions, and resubmit.

Show me tasks.json, coverage_matrix, repo assignments, dependencies, and warnings.
Wait for my explicit approval before implementation.

After I approve, run feature-dev approve, then reconcile && execute-loop --json.
Keep at most one active task. On awaiting_code_changes: edit assigned repo,
feature-dev implement <task-id>, then execute-loop again.
On awaiting_rework: fix, feature-dev task resume <task-id>, then execute-loop again.
On BLOCKED/orphan: task explain, then unblock/recover as suggested — never hand-edit status.
When all tasks are DONE, run feature-dev finalize --json (use remaining_tasks recovery if incomplete).
```

Short form:

`/feature-dev-orchestrator Implement PLAN.md using feature-dev orchestration.`

For detailed branch behavior and loop policy, use [Runbook](./references/runbook.md).
