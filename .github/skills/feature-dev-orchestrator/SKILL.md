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

### 2) Deep Planning and Task Graph (Agent-Owned)
Do not modify production source code during planning. Allowed paths: `.feature/**`, `PLAN.md`, planning/review artifacts.

Deep planning phase (required):
1. Read `PLAN.md` and extract requirements and acceptance criteria.
2. Run discovery and inspect each relevant repository (structure, modules, APIs, tests).
3. Build an ownership matrix: requirement → primary repository → proposed task(s), with rationale.
4. Design small verifiable subtasks with one primary repository each and explicit dependencies only when truly blocking.
5. Write [`.feature/tasks/tasks.json`](.github/skills/feature-dev-orchestrator/assets/task-dag-template.json) including:
   - `id`, `title`, `repository`, `dependencies`, `verification`
   - `repository_rationale`, `ownership_confidence`, `requirement_ids`, `planned_verification`
6. Validate locally: `go run ./cmd/feature-dev graph`
7. Submit for review: `go run ./cmd/feature-dev plan submit --from-tasks`
8. Present for human review:
   - `go run ./cmd/feature-dev review --json`
   - `go run ./cmd/feature-dev graph`
   - Show the user `.feature/tasks/tasks.json` task list, repo assignments, dependencies, and validation warnings.
9. Stop and wait for explicit user approval (for example: "approve", "go ahead with implementation").
10. Persist approval: `go run ./cmd/feature-dev approve`

If the user requests changes:
- `go run ./cmd/feature-dev replan --reason "..."`
- Revise `.feature/tasks/tasks.json`, resubmit with `plan submit --from-tasks`, and request approval again.

Planning rules:
1. Analyze `PLAN.md` and relevant repositories deeply before writing tasks.
2. Assign each subtask to the correct repository with rationale.
3. Do not modify production code during planning.
4. Run `feature-dev plan submit --from-tasks` then `feature-dev review`.
5. Present `.feature/tasks/tasks.json` to the user for review.
6. Wait for explicit approval.
7. Do not start implementation until `feature-dev status --json` reports `workflow_status: APPROVED`.

### 3) Reconcile and Execute (After Approval Only)
Run:
- `go run ./cmd/feature-dev reconcile`
- `go run ./cmd/feature-dev execute-loop --json`

Execution rules:
1. Confirm workflow status is `APPROVED`.
2. Run `feature-dev ready`.
3. Select a `READY` task.
4. Load only task-specific context.
5. Run `feature-dev start <task>` or continue via execute-loop.
6. Implement the task in its assigned repository.
7. Run verification and continue until all tasks are `DONE`.

Interpret stop reasons:
- `plan_not_approved`: run review flow and obtain approval before coding.
- `awaiting_code_changes`: implement active task in assigned repository, run verify/tests, rerun execute-loop.
- `verify_failure_budget_reached`: inspect artifacts/output, fix root cause, rerun reconcile, rerun execute-loop.
- `no_executable_task`: inspect dependencies, missing repo assignment, or unmet prerequisites; repair tasks; rerun reconcile and execute-loop.
- `max_steps_reached`: rerun execute-loop immediately.

Continue until all tasks are DONE.

### 4) Command-Driven Skill Routing
- Run `go run ./cmd/feature-dev agent-hint --json` to produce routing hints.
- If `skill` is `feature-dev-orchestrator`, follow this skill's planning plus execute-loop flow.
- Use `suggested_next_command` and `reason` to choose the immediate next step.

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

## Suggested Invocation
Use this prompt when starting feature work:

```text
Use feature-dev-orchestrator to implement PLAN.md.

Do deep planning first: inspect relevant repositories, assign each subtask to the
correct repo with rationale, and write .feature/tasks/tasks.json with a valid DAG.
Do not modify production code during planning.

Then run:
  feature-dev plan submit --from-tasks
  feature-dev review --json
  feature-dev graph

Show me the tasks.json task list, repo assignments, dependencies, and any validation
warnings. Wait for my explicit approval before implementation.

After I approve, run feature-dev approve and then execute-loop --json.
```

Short form:

`/feature-dev-orchestrator Implement PLAN.md using feature-dev orchestration.`

For detailed branch behavior and loop policy, use [Runbook](./references/runbook.md).
