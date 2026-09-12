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

### 2) Read Plan and Create Task Graph (Agent-Owned)
- Read the plan file named in the prompt. If the prompt says `PLAN.md`, read `PLAN.md` from the workspace root.
- Treat the plan as the feature intent and acceptance source; inspect relevant code in assigned repositories to fill in implementation details.
- Create or update `.feature/tasks/tasks.json`.
- Ensure each task includes:
  - id
  - title
  - repository
  - dependencies
  - description
  - verify command
- Use [Task DAG Template](./assets/task-dag-template.json) as the shape reference.

Validate graph:
- `go run ./cmd/feature-dev graph`

If graph validation fails, repair dependencies and rerun until valid.

### 3) Reconcile and Execute
Run:
- `go run ./cmd/feature-dev reconcile`
- `go run ./cmd/feature-dev execute-loop --json`

Interpret stop reasons:
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
- `/feature-dev-orchestrator Implement PLAN.md using feature-dev orchestration with minimal manual intervention.`
- `Use feature-dev command to implement PLAN.md. Read the plan, infer the task DAG and dependencies, then continue until all tasks are DONE.`

For detailed branch behavior and loop policy, use [Runbook](./references/runbook.md).
