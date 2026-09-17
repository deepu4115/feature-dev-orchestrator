# Suggested Feature-dev Orchestrator Enhancement Plan

## Purpose

This plan is based on an end-to-end feature-dev run against the Ecommerce workspace. The workflow successfully discovered repositories, validated a task DAG, executed eight tasks, ran Maven verification, and finalized traceability. It also exposed gaps that can cause an internally valid plan to omit user requirements or waste time repairing orchestration state.

The goal is to make feature-dev safer for autonomous agents while preserving the CLI as the source of truth.

## Observed Problems

### P1. PLAN.md completeness was not enforced

The source plan explicitly required multiple-address management, including add, edit, primary-address selection, and switching the primary address. That requirement was omitted while manually creating `requirements.json`, so no task referenced it.

The submitted plan still passed because traceability only checked the requirements that had been entered. The result was an internally consistent but incomplete plan.

**Impact:** The workflow reported completion while a documented acceptance criterion remained unimplemented.

### P2. Verification commands were interpreted from the wrong working directory

Tasks were initially generated with commands such as:

```text
cd user-service && mvn test -DskipTests=false
```

The verifier already executed the command from the assigned repository directory, so the command failed with `cd: user-service: No such file or directory`.

**Impact:** A passing build was reported as a verification failure because task authors did not know the working-directory contract.

### P3. Approved tasks remained `REVIEW_PENDING`

After replanning and approving revision 2, T001 completed successfully, but dependent tasks remained `REVIEW_PENDING` and the automatic ready queue was empty. The graph was valid, but the normal execute loop did not unlock the next task. Explicit commands such as `execute-next --task T002` were required.

**Impact:** Autonomous execution stopped despite a valid approved plan and satisfied dependencies.

### P4. Agent hints became stale or contradictory

After all eight tasks were complete and the workflow was `COMPLETED`, `agent-hint --json` still returned `schema_fix_required`. The status and execute-loop outputs were authoritative and correct, but the stale hint could send an agent into unnecessary recovery work.

**Impact:** Agents may follow an obsolete suggested command after the workflow has already succeeded.

### P5. Final status fields were ambiguous

Finalization returned `completed: true` and `workflow_status: COMPLETED`, while `status --json` still showed `completion_ready: false` and `approval_required: true`.

**Impact:** Consumers cannot reliably determine whether the feature is complete from one status response.

### P6. Cross-repository verification was under-specified

The task graph contained cross-repository dependencies, but task verification commands were repository-local. Finalization marked cross-repository verification as `SKIP` because no external integration environment was available.

**Impact:** The plan can finish with unit/build evidence while service-to-service behavior remains unverified, without a strongly visible final warning or explicit evidence requirement.

### P7. Structural validation happened after manual artifact editing

Malformed task JSON was only detected during preview/submission after several edits. The recovery required deleting and reconstructing the task file.

**Impact:** A single syntax error erased the visible task graph and created unnecessary recovery work.

## Enhancement Goals

1. Detect missing requirements before approval.
2. Make schema and execution contracts self-describing.
3. Keep task lifecycle transitions deterministic after replanning.
4. Ensure status, hints, and finalization agree.
5. Distinguish local verification from cross-repository integration evidence.
6. Reduce destructive or high-risk manual repair steps.

## Proposed Changes

## E1. Add PLAN-to-task completeness analysis

Add a CLI command such as:

```bash
feature-dev plan coverage --from PLAN.md --json
```

The command should identify requirements, explicit acceptance criteria, additional business requirements, risks, and demo-flow steps from the source plan, then compare them with `requirements.json` and `tasks.json`.

Suggested output:

```json
{
  "valid": false,
  "source": "PLAN.md",
  "covered_requirement_ids": ["R001", "R002"],
  "unmapped_source_items": [
    {
      "title": "Multiple address management",
      "source_section": "Additional Business Requirements",
      "suggested_requirement_id": "R012"
    }
  ],
  "actions": [
    "Add a requirement entry",
    "Map it to at least one task",
    "Define verification evidence"
  ]
}
```

Approval should be blocked when unmapped source items exist, unless the user explicitly records a deferral with a reason.

## E2. Generate a canonical planning bundle with contracts

Extend `plan draft init` and `task init` to generate:

- valid JSON files
- `.schema.json` or embedded schema metadata
- comments or README guidance explaining repository ownership and command working directories
- example populated entries
- a machine-readable manifest of required files

Add a single command:

```bash
feature-dev plan scaffold --from PLAN.md
```

This should create the draft bundle, extract candidate requirements, and create task placeholders without silently claiming coverage.

## E3. Validate structure before any domain clarification

Validation order should be:

1. JSON syntax
2. Root shape and field types
3. Required files
4. Repository and dependency references
5. Requirement traceability
6. PLAN-to-task completeness
7. Domain clarification questions

Errors should include the file, JSON path, expected type, actual type, and a direct repair command or example.

Example:

```text
.feature/tasks/tasks.json: $.0.verification
Expected: array of objects with command and optional name
Actual: string
Fix: feature-dev schema show tasks --json
```

The CLI must not enter clarification mode while structural validation is failing.

## E4. Make verification working-directory semantics explicit

Every task should expose:

```json
{
  "repository": "order-service",
  "verification_working_directory": "repository_root",
  "verification": [
    {"command": "mvn test -DskipTests=false"}
  ]
}
```

The CLI should reject commands containing a redundant repository-directory prefix when `verification_working_directory` is `repository_root`, or normalize them safely.

For cross-repository checks, require an explicit form:

```json
{
  "repository": "workspace-root",
  "verification_working_directory": "workspace_root",
  "command": "..."
}
```

This prevents shell commands from accidentally assuming a different directory.

## E5. Reconcile lifecycle state after plan approval and task completion

After approval or a task transitions to `DONE`, reconcile should derive readiness from:

- approved revision
- current task definitions
- dependency status
- task ownership
- implementation state

Tasks in `REVIEW_PENDING` should become `READY` when their dependencies are complete and the plan revision is approved. This transition must be CLI-owned and persisted.

Add a diagnostic command:

```bash
feature-dev task explain T002 --json
```

It should explain why a task is or is not ready:

```json
{
  "task": "T002",
  "status": "REVIEW_PENDING",
  "ready": true,
  "blocking_reasons": [],
  "suggested_next_command": "feature-dev task start T002"
}
```

## E6. Make agent hints derive from current authoritative state

`agent-hint` must be calculated from the same state used by `status`, `ready`, and `execute-loop`.

Rules:

- If workflow is `COMPLETED`, return `feature_completed` and no recovery command.
- If preview validation passes, never return `schema_fix_required`.
- If the ready queue is empty, report exact blocking reasons.
- Include the current revision and state timestamp.
- Treat hints as advisory, never as a contradictory state source.

## E7. Unify status and finalization semantics

Define explicit status fields:

```json
{
  "workflow_status": "COMPLETED",
  "completed": true,
  "completion_ready": true,
  "approval_required": false,
  "approved_plan_revision": 2,
  "finalization": "PASSED"
}
```

Once finalization succeeds:

- `completion_ready` must be `true`
- `approval_required` must be `false`
- `finalization` must be `PASSED`
- `agent-hint` must return `feature_completed`

## E8. Make cross-repository verification a first-class gate

A plan with cross-repository dependencies should declare one of:

- an executable integration command
- an environment-backed verification command
- an explicit documented deferral

Finalization should report:

```json
{
  "cross_repo_verify": {
    "status": "SKIPPED",
    "reason": "Consul, Config Server, MySQL, or Redis unavailable",
    "residual_risk": "Service-to-service purchase flow not exercised"
  }
}
```

A skipped gate should be visible in review and final output, not represented only as a generic `SKIP` value.

## E9. Add safe repair and dry-run commands

Provide commands that diagnose without mutating state:

```bash
feature-dev doctor --json
feature-dev plan validate --json
feature-dev task explain T002 --json
feature-dev reconcile --dry-run --json
feature-dev execute-loop --dry-run --json
```

For artifact repair, prefer generated patches or temporary validation over deleting files. A repair command should create a backup before rewriting an artifact.

## E10. Add a completeness quality gate

Introduce a final gate distinct from traceability:

```text
traceability: every task requirement ID maps to a known requirement
completeness: every source-plan requirement maps to a requirement ID and task
```

A plan can pass traceability while failing completeness, as happened with address management. Both must be displayed independently.

## Proposed Implementation Phases

### Phase 1: Safety and diagnostics

- Add `plan validate --json` with ordered structural validation.
- Add verification working-directory metadata.
- Fix stale agent-hint state handling.
- Align `status` and `finalize` completion fields.
- Add `task explain` and `execute-loop --dry-run`.

### Phase 2: Planning completeness

- Add PLAN-to-task coverage extraction.
- Add completeness quality gate.
- Block approval for unmapped requirements unless explicitly deferred.
- Add source-section references to requirement records.

### Phase 3: Execution reliability

- Reconcile `REVIEW_PENDING` tasks into `READY` after approval and satisfied dependencies.
- Add explicit cross-repository verification declarations.
- Improve skipped-gate reporting and residual-risk output.

### Phase 4: Authoring ergonomics

- Add one-command scaffold generation.
- Generate examples and schema guidance with every planning bundle.
- Add safe artifact repair and backup behavior.
- Add migration support for existing `.feature` bundles.

## Acceptance Criteria

The enhancement is successful when:

1. A PLAN.md containing an address requirement causes coverage validation to report that requirement if it is absent from the draft bundle.
2. Approval is blocked or an explicit deferral is required for unmapped source requirements.
3. A generated Maven verification command runs from the assigned repository without redundant `cd` prefixes.
4. After T001 is marked `DONE`, a dependency-satisfied T002 becomes `READY` after reconcile.
5. `agent-hint`, `status`, and `execute-loop` agree on the current workflow state.
6. After successful finalization, status reports `COMPLETED`, `completion_ready: true`, and no approval is required.
7. Cross-repository verification reports actionable skipped-gate evidence and residual risk.
8. Structural JSON errors are identified before clarification questions are generated.
9. A new agent can scaffold and validate a complete plan without reading orchestrator implementation source code.

## Suggested Test Matrix

| Scenario | Expected result |
|---|---|
| Missing `verification` array | Structural error with JSON path and schema example |
| Unknown requirement ID | Domain validation error naming task and ID |
| Requirement in PLAN.md absent from requirements.json | Completeness failure with source section |
| Approved plan with satisfied dependency | Dependent task becomes `READY` after reconcile |
| Verification command includes `cd repo` | Warning or normalized command based on working-directory metadata |
| Completed workflow queried through `agent-hint` | `feature_completed`, no repair command |
| Finalization with unavailable Consul/MySQL/Redis | Explicit skipped gate and residual risk |
| Cross-repository command without workspace owner | Validation error requiring explicit owner |

## Success Metrics

- Zero manual schema reverse-engineering steps in a clean planning run.
- Zero plans approved with unmapped PLAN.md acceptance criteria.
- Zero false verification failures caused by repository-directory prefixes.
- Zero cases where `status` and `agent-hint` disagree after finalization.
- Every skipped integration gate includes an actionable residual-risk explanation.

## Recommendation

Implement Phase 1 and Phase 2 before relying on feature-dev for autonomous multi-repository delivery. The most important correction is the completeness gate: internal task traceability is necessary, but it is not sufficient to prove that the source plan was fully implemented.
