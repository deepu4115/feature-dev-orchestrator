# Issues Faced

## Summary

During planning and execution of the Ecommerce workspace plan, feature-dev required substantial manual repair and allowed inconsistent task state. It accepted placeholder planning artifacts, allowed multiple tasks to be `RUNNING`, lost transition history for active tasks, moved tasks to `BLOCKED` without reasons, used invalid repository verification commands, and failed to finalize a plan with an otherwise complete task graph.

## Reproduction Evidence

The workspace `.feature/state/task-summaries.jsonl` contains this sequence:

```text
18:36:06  T007  RUNNING  execute_next_started
18:38:39  T011  RUNNING  execute_next_started
18:39:44  T011  IMPLEMENTED
18:39:50  T011  VERIFYING
18:39:50  T011  DONE
```

There is no `IMPLEMENTED`, `VERIFYING`, `DONE`, `FAILED`, or `BLOCKED` event for `T007` after it entered `RUNNING`.

The persisted task file later contained:

```json
{
  "id": "T007",
  "status": "BLOCKED"
}
```

The task explanation reported only `task status is BLOCKED`; it did not provide a block reason or transition event.

## Confirmed Issues

### 1. Scaffold generated placeholder artifacts that passed too far into planning

`plan scaffold --from PLAN.md --json` generated placeholder requirements and tasks such as `repo-a`, `repo-b`, `R002`, `go test ./...`, and `npm test`, even though the workspace contained six Maven repositories.

The scaffold reported completeness/decomposition/acceptance gates but left traceability invalid. `task preview` then caught unknown repositories and unknown requirement IDs. A scaffold should either generate repository-aware artifacts or fail before creating misleading executable tasks.

### 2. Planning schemas were not sufficiently discoverable

The exact contracts for `requirements.json`, `assumptions.json`, `risks.json`, `impact.json`, `repo-analysis.json`, and `tasks.json` were not available through the initial workflow output. In particular:

- draft bundle files required wrapper objects such as `{ "requirements": [] }`;
- `tasks.json` required a top-level array;
- `verification` required an array of objects rather than a string;
- tasks required `verification_working_directory` and valid repository IDs.

The CLI schema command was needed to reconstruct these shapes after scaffold/preview failures. Templates should be valid and repository-aware from the start.

### 3. Validation errors arrived after invalid artifacts were created

The workflow produced errors such as unknown repository, unknown requirement ID, missing requirements file, and draft bundle load errors only after later planning commands. Structural validation should happen immediately during scaffold and task creation, with field-level remediation and canonical examples.

### 4. Clarification behavior was noisy for structural failures

The workflow drifted toward conceptual clarification questions while the actual problem was malformed JSON or missing wrapper structure. Structural/schema failures should be resolved before domain clarification is requested.

### 5. Multiple tasks can be RUNNING

`execute-next` selects a task based on its own dependency readiness. It does not first check whether another task in the workflow is already `RUNNING`.

As a result, an unrelated task such as `T011`, which did not depend on `T007`, could start while `T007` was still active.

Expected invariant:

```text
At most one task may be RUNNING in a single workflow execution.
```

### 6. No atomic task claim or execution lease

Task status persistence is per-task. There is no workflow-level active-task lease, compare-and-swap operation, or atomic claim that prevents two execution calls from claiming different tasks.

This makes repeated, overlapping, or concurrent `execute-next` / `execute-loop` calls unsafe.

### 7. Readiness is confused with schedulability

Dependency readiness answers whether a task's prerequisites are complete. It does not answer whether the workflow is available to start another task.

The scheduler currently treats a dependency-ready task as executable even when another task is already `RUNNING`.

The scheduler should apply both checks:

```text
schedulable(task) = dependency_ready(task) && no_active_task(workflow)
```

### 8. Missing audit event for T007's final state

The task history contains the initial `RUNNING` event for `T007`, but no event explaining how it became `BLOCKED`.

Every status mutation must append an event containing at least:

- task id
- previous status
- next status
- timestamp
- reason
- command or actor
- workflow revision

A persisted status must never appear without a matching transition record.

### 9. BLOCKED state has no actionable reason

`task explain T007 --json` returned only that the task status was `BLOCKED`. It did not identify who or what blocked the task, whether the block was recoverable, or what command could resolve it.

A blocked task should include structured metadata such as:

```json
{
  "blocked_reason": "...",
  "blocked_by": ["..."],
  "blocked_at": "...",
  "recovery_command": "..."
}
```

### 10. Reconcile does not detect orphaned active tasks

When a task is `RUNNING` with no later completion, verification, failure, cancellation, or lease information, reconciliation has no reliable way to determine whether the task is still active, abandoned, or corrupted.

Reconciliation should detect and report orphaned `RUNNING` tasks instead of allowing unrelated work to proceed silently.

### 11. Reconcile does not recover blocked tasks whose dependencies are complete

After `T007` was manually restored to `READY`, execution proceeded correctly. Later `T009` became `BLOCKED` even though its dependencies `T007` and `T012` were both `DONE`.

`task explain T009 --json` reported only `task status is BLOCKED`, and `reconcile` changed zero tasks. There is no supported CLI command to recover a stale `BLOCKED` task to `READY`.

The same recovery problem occurred with the original `T007` state. Normal recovery required manual status editing, which conflicts with the CLI-as-source-of-truth model.

### 12. Verification commands were not validated against repository capabilities

`T015` was configured with `./mvnw test`, but `user-service` has no Maven wrapper. The code verification with `mvn test` passed, while feature-dev marked the task `REWORK` with exit code 127 and immediately started `T018`.

Task planning should validate that configured commands exist in the assigned repository before approval, or provide a repository-aware Maven command fallback.

### 13. Verification failure incorrectly allowed another task to start

The T015 sequence was:

```text
T015 RUNNING
T015 IMPLEMENTED
T015 REWORK  verification_failed exit 127
T018 RUNNING
```

The task requiring rework remained unresolved while another task started. A verification failure should retain execution ownership, or explicitly enter a recoverable paused state before scheduling anything else.

### 14. Illegal transition after successful rework

After correcting T015's verifier to `mvn test`, the task successfully reached `DONE`, but a subsequent `execute-loop --task T015 --json` failed with:

```text
illegal transition: DONE -> READY
```

The CLI attempted to re-queue a task that had already completed. Explicit task targeting should reject completed tasks without mutating state, or treat the command as a no-op with a clear message.

### 15. Task history does not record the complete execution command context

The audit events record task, status, timestamp, and a generic message, but not the command invocation, verifier command, revision, actor, or reason. This made it difficult to explain why T007/T009 became blocked and why T015 entered rework.

### 16. Finalization did not provide a complete recovery path

After 23 of 24 tasks were complete, `finalize --json` reported that cross-repository verification and finalization were pending. The remaining task was stale `BLOCKED`, but the CLI did not provide a recovery command or structured action to unblock it.

Finalization should enumerate every remaining task, its exact blocker, and an executable recovery command. It should also distinguish an actual domain blocker from corrupted task state.

### 17. Cross-repository verification was advisory but not operationally declared

The plan repeatedly reported cross-repository dependency warnings and `cross_repo_verify_not_declared`. The documented `workspace-verify` artifact was not recognized by the CLI version, leaving no supported place to declare integration verification or an explicit deferral.

The CLI should provide a versioned workspace verification schema and make the cross-repository gate actionable before approval.

### 18. Review-pending lifecycle is unclear and can appear to skip READY

After plan approval, tasks intentionally remain in `REVIEW_PENDING` until `reconcile` promotes dependency-ready tasks to `READY`. Execution should then move only `READY` tasks to `RUNNING`.

The valid lifecycle is:

```text
REVIEW_PENDING -> READY -> RUNNING -> IMPLEMENTED -> VERIFYING -> DONE
```

However, task execution history showed `execute_next_started` events with `RUNNING` without a visible preceding `REVIEW_PENDING -> READY` event. This makes it impossible to determine from the task summary whether reconciliation occurred or whether execution bypassed the required `READY` state.

The CLI must reject a direct `REVIEW_PENDING -> RUNNING` transition. The promotion event should be visible consistently in task summaries, status output, and persisted task state, including the command, timestamp, reason, and workflow revision.

Approval should be documented as approval of the plan, not automatic readiness of every task. `reconcile` should be the explicit and auditable boundary between review-pending tasks and executable tasks.

## Impact

- Task execution order no longer reflects the recorded workflow state.
- Dependent tasks can remain blocked behind a task that was never actually completed.
- The task history cannot reconstruct the true execution sequence.
- Users cannot safely resume execution after an interrupted or overlapping command.
- `status`, `task explain`, and `execute-loop` can disagree with the audit trail.
- A plan can appear partially progressed while its active task was skipped.
- Verification can fail because of an invalid command rather than a code defect, but still trigger task rework and unrelated scheduling.
- Finalization can remain incomplete with no supported state-recovery command.

## Recommended Fixes

1. Add a workflow-wide active-task guard before scheduling any task.
2. Make task claiming atomic and reject a second claim while an active task exists.
3. Add an execution lease or explicit recovery state for interrupted tasks.
4. Validate the invariant that no more than one task is `RUNNING`.
5. Make every status mutation append an audit event transactionally with the state update.
6. Store structured block reasons and expose them through `task explain --json` and `status --json`.
7. Make `reconcile` fail or pause when it finds an orphaned `RUNNING` task.
8. Add regression tests for:
   - starting T007 and attempting to start unrelated T011;
   - concurrent execute-loop calls;
   - a task status mutation without an audit event;
   - orphaned `RUNNING` task recovery;
   - blocked tasks with actionable recovery metadata.
9. Validate verification commands against repository files and tool availability during plan preview.
10. Add a supported `unblock`/`resume` recovery operation that records the reason and actor.
11. Reject explicit execution of `DONE` tasks without attempting a `DONE -> READY` transition.
12. Add a versioned cross-repository verification artifact and enforce its declaration or documented deferral.
13. Emit structured command, revision, actor, reason, and verifier fields in every task event.
14. Make the `REVIEW_PENDING -> READY -> RUNNING` lifecycle explicit in CLI output and documentation.
15. Reject and audit any direct `REVIEW_PENDING -> RUNNING` transition.
16. Ensure reconcile promotion events are written to the same audit stream consumed by task summaries and status.

## Acceptance Criteria

- A second task cannot transition to `RUNNING` while any task is already `RUNNING`.
- Concurrent scheduler calls produce one successful claim and one deterministic refusal.
- Every persisted status transition has exactly one corresponding audit event.
- A task cannot become `BLOCKED` without a reason and recovery metadata.
- Reconciliation reports orphaned active tasks and does not schedule new work until recovery is explicit.
- `task explain` identifies the blocker and provides a valid next command.
- The T007/T011 sequence is covered by an automated regression test.
- Invalid repository commands are rejected before approval or reported as planning errors.
- Verification failure does not start another task while the failed task requires rework.
- A completed task cannot be re-queued by an explicit execute command.
- A task with completed dependencies can be recovered from `BLOCKED` through a documented CLI command.
- Finalization provides actionable output for every remaining task and cross-repository gate.
- Approved tasks remain visibly `REVIEW_PENDING` until reconcile promotes them to `READY`.
- Execution cannot start a task directly from `REVIEW_PENDING`.
- Every review-pending promotion is visible in the persisted audit history.
