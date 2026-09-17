Yes. I’d add this as a dedicated section in your implementation plan. It should treat **human approval as a hard workflow gate**, not just a conversational instruction.

# Human Review and Approval Gate Before Implementation

## 1. Objective

Introduce a mandatory **Human Review and Approval Gate** between feature planning and implementation.

The orchestrator may:

* analyze `PLAN.md`
* discover repositories
* inspect architecture
* generate tasks
* build the dependency DAG
* identify assumptions
* identify risks
* determine repository ownership
* propose expected change areas
* propose verification strategy

But it must **not modify production code** until the user explicitly approves the generated implementation plan.

Core rule:

> **AI proposes. Human validates. AI executes. Deterministic tooling verifies.**

This approval gate is required to prevent incorrect task decomposition, repository ownership mistakes, architectural drift, missing requirements, invalid assumptions, and unnecessary implementation work.

---

# 2. Updated Workflow

The feature lifecycle becomes:

```text
PLAN.md
   ↓
Workspace Discovery
   ↓
Repository Analysis
   ↓
Requirement Extraction
   ↓
Impact Analysis
   ↓
Assumption Identification
   ↓
Risk Analysis
   ↓
Task Generation
   ↓
Repository Assignment
   ↓
Dependency DAG Generation
   ↓
Verification Strategy
   ↓
Plan Validation
   ↓
──────────────────────────────
     HUMAN REVIEW REQUIRED
──────────────────────────────
   ↓
User Reviews Plan
   ↓
+------------------------------+
|                              |
| Approve                      | Request Changes
|                              |
v                              v
APPROVED                    REPLANNING
   |                           |
   |                           v
   |                    Generate Revision
   |                           |
   |                           v
   |                    REVIEW_PENDING
   |                           |
   +---------------------------+
   |
   v
Implementation
   ↓
Verification
   ↓
Rework if required
   ↓
Final Feature Verification
   ↓
DONE
```

Implementation is impossible while the feature plan is in:

```text
DRAFT
PLANNING
REVIEW_PENDING
REPLANNING
REJECTED
```

Only an approved plan revision can enter execution.

---

# 3. Workflow State Model

Introduce explicit workflow-level states.

```text
NEW
 |
 v
PLANNING
 |
 v
PLAN_GENERATED
 |
 v
REVIEW_PENDING
 |
 +-----------> REPLANNING
 |                 |
 |                 v
 |           PLAN_GENERATED
 |                 |
 |                 v
 |           REVIEW_PENDING
 |
 +-----------> REJECTED
 |
 v
APPROVED
 |
 v
EXECUTING
 |
 v
VERIFYING
 |
 +-----------> REWORK
 |                 |
 |                 v
 |            EXECUTING
 |
 v
COMPLETED
```

Supported states:

```text
NEW
PLANNING
PLAN_GENERATED
REVIEW_PENDING
REPLANNING
REJECTED
APPROVED
EXECUTING
VERIFYING
REWORK
BLOCKED
FAILED
COMPLETED
CANCELLED
```

---

# 4. Critical State Rule

The state engine must enforce:

```text
workflow.status == APPROVED
```

before any implementation task may move to:

```text
RUNNING
```

For example:

```text
T003 status = READY
workflow status = REVIEW_PENDING
```

must result in:

```text
Task T003 cannot start.

Reason:
Current feature plan has not been approved.

Plan revision: 4
Status: REVIEW_PENDING

Required action:
Approve revision 4 before implementation.
```

This check must exist in the deterministic Go state engine.

It must not depend only on Cursor/Copilot following instructions.

---

# 5. Task State Model

Task states remain separate from workflow states.

Before approval:

```text
DRAFT
   ↓
PLANNED
   ↓
REVIEW_PENDING
```

After workspace plan approval:

```text
REVIEW_PENDING
   ↓
READY
   ↓
RUNNING
   ↓
IMPLEMENTED
   ↓
VERIFYING
   ↓
DONE
```

Failure paths:

```text
RUNNING
   ↓
BLOCKED

VERIFYING
   ↓
REWORK
   ↓
RUNNING

VERIFYING
   ↓
FAILED
```

Recommended task states:

```text
DRAFT
PLANNED
REVIEW_PENDING
READY
RUNNING
BLOCKED
IMPLEMENTED
VERIFYING
REWORK
FAILED
DONE
CANCELLED
```

Tasks should normally be approved collectively as part of a plan revision rather than individually.

---

# 6. Plan Revision Model

Every generated plan must have a revision number.

Example:

```yaml
plan:
  id: feature-device-health
  revision: 3
  status: REVIEW_PENDING
```

When planning first completes:

```text
revision = 1
```

If the user requests a change:

```text
revision 1
    ↓
revision 2
```

If another change is requested:

```text
revision 2
    ↓
revision 3
```

Each revision is immutable after creation.

Do not silently modify an existing approved plan revision.

---

# 7. Approval Model

Approval must always reference an exact plan revision.

Example:

```yaml
approval:
  status: APPROVED
  approved_revision: 3
  approved_at: "2026-09-15T21:30:00+05:30"
```

The important invariant is:

```text
approved_revision == current_plan_revision
```

before implementation is permitted.

If:

```text
current plan revision = 4
approved revision = 3
```

then the workflow automatically becomes:

```text
REVIEW_PENDING
```

Implementation must stop.

---

# 8. Approval Invalidation

Any material modification to the implementation plan must invalidate approval.

Examples:

* task added
* task removed
* repository assignment changed
* dependency changed
* acceptance criterion changed
* architecture decision changed
* major assumption changed
* expected change area changed
* verification strategy materially changed
* new cross-repository dependency added
* risk level significantly changed

Flow:

```text
APPROVED revision 3
        ↓
material plan change
        ↓
revision 4 created
        ↓
approval invalidated
        ↓
REVIEW_PENDING
```

This protects the user from approving one plan while the agent executes another.

---

# 9. Non-Material Changes

Not every text change should invalidate approval.

Examples that may remain non-material:

* spelling correction
* formatting change
* additional explanatory text
* non-functional description improvement

The safe MVP approach is:

> Treat all structural plan changes as material.

More advanced semantic classification can come later.

---

# 10. Plan Artifact Structure

Recommended location:

```text
.feature/plans/
├── revision-001/
│   ├── plan.yaml
│   ├── summary.md
│   ├── dag.json
│   └── review.md
├── revision-002/
└── revision-003/
```

Current pointer:

```text
.feature/state/workflow.json
```

Example:

```json
{
  "current_plan_revision": 3,
  "approved_plan_revision": null,
  "workflow_status": "REVIEW_PENDING"
}
```

---

# 11. Plan Schema

Example:

```yaml
schema_version: "1.0"

feature:
  id: device-health
  title: Add Device Health API

plan:
  revision: 3
  status: REVIEW_PENDING
  generated_at: "..."

repositories:
  - id: common
    relevant: true

  - id: devices
    relevant: true

  - id: analytics
    relevant: true

requirements:
  - id: R001
    description: Add device health endpoint

tasks:
  - id: T001
    repository: common

  - id: T002
    repository: devices
    dependencies:
      - T001

assumptions:
  - id: A001
    statement: Shared DTO can remain backward compatible
    confidence: MEDIUM

risks:
  - id: RK001
    level: HIGH
    description: Shared DTO affects multiple repositories

verification:
  strategy: repository-aware

approval:
  required: true
  approved_revision: null
```

---

# 12. User Review Artifact

Generate:

```text
.feature/plans/revision-003/review.md
```

This should be optimized for human review.

Example structure:

```markdown
# Feature Plan Review

## Objective

## Relevant Repositories

## Requirements

## Proposed Tasks

## Dependency Graph

## Cross-Repository Dependencies

## Assumptions

## Risks

## Expected Change Areas

## Verification Plan

## Open Questions

## Plan Changes Since Previous Revision

## Approval Status
```

The user should not need to inspect raw YAML/JSON unless desired.

---

# 13. Review Summary

The review output should be concise enough to inspect but detailed enough to approve safely.

Example:

```text
Feature Plan
────────────────────────────────────────

Revision: 3
Status: REVIEW_PENDING

Repositories:
  common
  devices
  analytics

Tasks: 4

T001 [common]
Add DeviceHealthDto

T002 [devices]
Implement DeviceHealthService
Depends on: T001

T003 [analytics]
Add health reporting support
Depends on: T001

T004 [workspace]
Cross-repository verification
Depends on: T002, T003

Assumptions:
A001 MEDIUM
Shared DTO extension is backward compatible.

Risks:
HIGH
Shared contract modification affects 2 downstream repositories.

Verification:
common     build + unit tests
devices    focused module tests
analytics  focused module tests
workspace  contract/integration verification

Status:
USER APPROVAL REQUIRED
```

---

# 14. Dependency Graph Presentation

Human review must include the DAG.

Example:

```text
             T001 [common]
             Add DTO
               |
        +------+------+
        |             |
        v             v
 T002 [devices]  T003 [analytics]
        |             |
        +------+------+
               |
               v
       T004 [workspace]
       Final Verification
```

The user should immediately understand implementation order.

---

# 15. Requirements Traceability Before Approval

Before presenting the plan, every requirement should map to at least one task.

Example:

```text
R001
Add Device Health API
  ↓
T002
DeviceHealthController

R002
Support reporting
  ↓
T003
Analytics transformation

R003
Backward compatibility
  ↓
T001 + T004
```

Planning validation must detect orphan requirements.

Example error:

```text
Requirement R004 is not covered by any task.

Plan cannot enter REVIEW_PENDING.
```

---

# 16. Acceptance Criteria Mapping

Every acceptance criterion should also map to verification evidence.

Example:

```text
AC001
GET /devices/{id}/health works

Implemented by:
T002

Verified by:
DeviceHealthControllerIT
```

Before approval the verification evidence may be proposed rather than executed.

Example:

```yaml
acceptance_criteria:
  - id: AC001
    implementation_tasks:
      - T002
    planned_verification:
      - integration-test
```

---

# 17. Assumptions

The plan must explicitly surface assumptions before asking for approval.

Example:

```yaml
assumptions:
  - id: A001

    statement: >
      DeviceHealthDto can be extended without breaking
      existing consumers.

    confidence: MEDIUM

    evidence:
      - common/src/...
      - existing compatibility tests

    affected_tasks:
      - T001
      - T002
      - T003
```

The user may:

```text
ACCEPT
CHALLENGE
REJECT
REQUEST_MORE_EVIDENCE
```

an assumption.

---

# 18. High-Risk Assumptions

High-risk assumptions should prevent silent approval.

Example:

```text
A002
Confidence: LOW
Impact: HIGH

Assumption:
No database migration is required.
```

The review should highlight:

```text
⚠ LOW-CONFIDENCE HIGH-IMPACT ASSUMPTION
```

The user may still approve, but it must be visible.

---

# 19. Risk Analysis

Every plan should classify risks.

Suggested levels:

```text
LOW
MEDIUM
HIGH
CRITICAL
```

Potential categories:

```text
ARCHITECTURE
PUBLIC_API
CROSS_REPOSITORY
DATA_MODEL
DATABASE
SECURITY
COMPATIBILITY
PERFORMANCE
MIGRATION
BUILD
TESTING
DEPLOYMENT
```

Example:

```yaml
risks:
  - id: RK001
    type: CROSS_REPOSITORY
    level: HIGH

    description: >
      DeviceHealthDto is consumed by devices and analytics.

    mitigation:
      - finalize shared contract first
      - verify downstream repositories
```

---

# 20. Expected Change Areas

The plan should predict where code is expected to change.

Example:

```yaml
expected_change_areas:
  common:
    - shared DTO
    - serialization tests

  devices:
    - service
    - controller
    - integration tests

  analytics:
    - transformer
    - reporting tests
```

These are expectations, not hard file restrictions.

Later verification can flag unexpected scope.

---

# 21. Repository Ownership Review

Every implementation task must have one primary repository.

The review must show:

```text
T001 -> common
T002 -> devices
T003 -> analytics
```

If ownership is uncertain:

```text
T002 repository ownership confidence: LOW
```

the user should see it before implementation.

---

# 22. Verification Plan Review

The review must describe how each repository/task will be verified.

Example:

```text
common
  compile
  focused unit tests

devices
  compile
  DeviceHealthServiceTest
  DeviceHealthControllerIT

analytics
  AnalyticsTransformerTest

workspace
  contract compatibility
  affected repository build
```

Do not wait until implementation completes to think about verification.

---

# 23. Review Commands

Recommended CLI:

```text
feature-dev review
feature-dev review --json
feature-dev approve
feature-dev reject
feature-dev replan
feature-dev plan-diff
```

Potential convenience command:

```text
feature-dev review --open
```

may later open the review artifact in the editor.

Not required for MVP.

---

# 24. `feature-dev review`

Responsibilities:

1. load current plan revision
2. validate schema
3. validate DAG
4. validate repository references
5. validate requirement coverage
6. validate acceptance-criteria mapping
7. validate task ownership
8. validate verification strategy
9. display assumptions
10. display risks
11. display plan revision
12. display current approval state

It must never modify production code.

---

# 25. `feature-dev approve`

Example:

```bash
feature-dev approve
```

Expected behavior:

```text
Plan revision 3 approved.

Tasks unlocked:
  T001

Workflow:
  APPROVED

Next:
  feature-dev ready
```

Internally:

```text
validate current plan
        ↓
verify REVIEW_PENDING
        ↓
record approval
        ↓
set approved_revision
        ↓
workflow -> APPROVED
        ↓
unlock root READY tasks
```

---

# 26. Explicit Revision Approval

Support:

```bash
feature-dev approve --revision 3
```

If revision 4 exists:

```text
Cannot approve revision 3.

Current revision: 4
Requested revision: 3

Review the current plan before approval.
```

This prevents accidental stale approval.

---

# 27. `feature-dev reject`

Example:

```bash
feature-dev reject --reason "T003 should not be part of this feature"
```

Result:

```text
Workflow: REJECTED
Implementation remains locked.
```

The rejection reason should be persisted.

---

# 28. `feature-dev replan`

Example:

```bash
feature-dev replan
```

Flow:

```text
current revision
   ↓
preserve immutable revision
   ↓
workflow -> REPLANNING
   ↓
Cursor updates task plan
   ↓
feature-dev validates
   ↓
new revision created
   ↓
REVIEW_PENDING
```

---

# 29. Conversational Modification Flow

The user should not need to use CLI commands for every modification.

Example:

User:

```text
Split T002 into service implementation and controller implementation.
Remove analytics changes.
Add an integration test task.
```

Cursor:

1. modifies the plan proposal
2. creates revision 4
3. runs plan validation
4. presents revision 4
5. asks for approval

The Go engine remains authoritative about revision and approval state.

---

# 30. Plan Diff

For revision 4 versus revision 3:

```bash
feature-dev plan-diff
```

Example:

```text
Plan Changes
────────────────────────────

Revision:
3 -> 4

Added:
+ T005 DeviceHealthController integration tests

Removed:
- T003 Analytics reporting changes

Modified:
T002
  old: Service + Controller
  new: Service only

Dependencies:
T005 now depends on T004

Risk:
Cross-repository risk reduced HIGH -> MEDIUM
```

This is very important once plans become large.

---

# 31. Approval After Replanning

After any new plan revision:

```text
approved_revision = previous revision
current_revision = new revision
```

must result in:

```text
REVIEW_PENDING
```

Example:

```text
Revision 3: APPROVED
Revision 4: generated

Current workflow:
REVIEW_PENDING

Previous approval does not apply to revision 4.
```

---

# 32. Implementation Lock

The implementation lock must exist inside the Go engine.

Commands such as:

```text
feature-dev start T001
feature-dev start T002
```

must run:

```text
checkCurrentPlanApproved()
```

before changing task state.

Pseudo-code:

```go
func CanStartTask(workflow Workflow, task Task) error {
    if workflow.Status != Approved {
        return ErrPlanNotApproved
    }

    if workflow.ApprovedRevision != workflow.CurrentRevision {
        return ErrPlanRevisionNotApproved
    }

    if task.Status != Ready {
        return ErrTaskNotReady
    }

    return nil
}
```

This must not rely only on prompt instructions.

---

# 33. Source Modification Guard

During planning/review stages, Cursor instructions should prohibit production source modification.

Allowed:

```text
.feature/**
PLAN.md
planning artifacts
review artifacts
```

Disallowed:

```text
repo-a/src/**
repo-b/src/**
production configuration
tests intended as implementation
```

for normal planning.

This protects against the agent starting implementation while still planning.

---

# 34. Planning Git Snapshot

When planning finishes, capture repository state.

Example:

```yaml
plan_snapshot:
  repositories:
    common:
      branch: main
      head: abc123

    devices:
      branch: feature-x
      head: def456
```

Before implementation begins, verify the repositories have not materially changed.

If they have:

```text
Plan generated against stale repository state.
```

Depending on the change:

```text
reconcile
or
replan
```

may be required.

---

# 35. Approval Validity and Repository Changes

Not every repository change should invalidate plan approval.

Examples:

### Potentially safe

```text
documentation update
unrelated test change
unrelated repository
```

### Potentially unsafe

```text
planned target file changed
shared contract changed
branch changed
repository HEAD substantially moved
dependency changed
```

For MVP:

> Warn on relevant repository state drift and require reconciliation before execution.

Later this can become more precise using fingerprints.

---

# 36. Review Gate Configuration

Example:

```yaml
approval:
  before_implementation: required

  invalidate_on:
    task_structure_change: true
    dependency_change: true
    repository_change: true
    acceptance_criteria_change: true
    verification_change: true

  allow_approval_from_cli: true
```

Default:

```text
required
```

---

# 37. Future Approval Policies

Later support:

```text
required
risk_based
disabled
```

### Required

Every plan needs approval.

### Risk-based

Simple plans may execute automatically while complex plans require approval.

### Disabled

Useful only in controlled automation environments.

For V1:

> Support only `required`.

This keeps behavior predictable.

---

# 38. Approval Audit Trail

Persist:

```text
revision
timestamp
approval status
reason
plan fingerprint
repository snapshot
```

Example:

```yaml
approval_history:
  - revision: 2
    status: REJECTED
    reason: Analytics changes unnecessary

  - revision: 3
    status: APPROVED
```

Do not depend on chat history to know what happened.

---

# 39. Plan Fingerprint

Generate a deterministic fingerprint over the meaningful plan content.

Example:

```text
sha256:
tasks
dependencies
repositories
requirements
acceptance criteria
verification strategy
assumptions
```

Persist:

```yaml
plan_fingerprint: sha256:abc...
```

Approval records the same fingerprint.

Before execution:

```text
current fingerprint
==
approved fingerprint
```

must be true.

This gives stronger protection than revision numbers alone.

---

# 40. Cursor/Copilot Instructions

The orchestrator instruction should state:

```text
PLANNING RULES

1. Analyze PLAN.md and relevant repositories.
2. Generate the complete proposed implementation plan.
3. Create tasks, dependencies, assumptions, risks, and verification strategy.
4. Do not modify production code.
5. Run feature-dev review.
6. Present the plan to the user.
7. Wait for explicit approval.
8. If the user requests changes:
   a. update the plan
   b. create a new revision
   c. present the plan again
9. Do not start implementation until feature-dev reports APPROVED.
```

---

# 41. Execution Instructions

Once approved:

```text
EXECUTION RULES

1. Confirm workflow status is APPROVED.
2. Run feature-dev ready.
3. Select a READY task.
4. Load only task-specific context.
5. Run feature-dev start <task>.
6. Implement the task.
7. Run feature-dev verify <task>.
8. Repair failures before continuing.
9. Run feature-dev complete <task>.
10. Continue until all implementation tasks are DONE.
11. Run feature-dev verify-workspace.
```

---

# 42. Approval User Experience

The ideal interaction should be:

```text
User:
Implement PLAN.md.

Cursor:
I analyzed the workspace and generated implementation plan revision 1.

Repositories:
- common
- devices
- analytics

Tasks:
...

Dependencies:
...

Assumptions:
...

Risks:
...

Verification:
...

No source code has been changed.

Please review the plan.

User:
T003 isn't required.
Split T002 into two tasks.

Cursor:
Updated plan revision 2.

Changes:
- Removed T003
- Split T002 into T002 and T004

Updated DAG:
...

Status: REVIEW_PENDING

User:
Approved.

Cursor:
Plan revision 2 approved.

Starting T001...
```

---

# 43. Explicit Approval Semantics

Do not infer approval from ambiguous responses such as:

```text
looks interesting
probably fine
continue explaining
```

Approval should be recognized from clear intent such as:

```text
approve
approved
looks good, proceed
go ahead with implementation
yes, implement this plan
```

When unclear, implementation remains locked.

The deterministic CLI should still require approval state to be persisted.

---

# 44. Avoid Repeated Approval During Normal Execution

Once revision 3 is approved, individual normal implementation steps should not repeatedly ask the user.

Approval is for:

```text
the implementation strategy
```

not:

```text
every code edit
```

Reapproval is required only when the plan materially changes.

This prevents the workflow from becoming cumbersome.

---

# 45. Emergency Replanning During Implementation

Sometimes implementation disproves the approved plan.

Example:

```text
T003 reveals an undocumented repository dependency.
```

The agent should not silently expand scope.

Flow:

```text
T003 BLOCKED
   ↓
replanning required
   ↓
revision 4 generated
   ↓
REVIEW_PENDING
   ↓
user approves
   ↓
execution resumes
```

Completed tasks remain completed unless invalidated.

---

# 46. Replanning Scope

Replanning should preserve stable work.

Do not regenerate the entire DAG unnecessarily.

Example:

```text
T001 DONE
T002 DONE
T003 BLOCKED
T004 PLANNED
```

New revision may preserve:

```text
T001 DONE
T002 DONE
```

and change:

```text
T003
T004
```

Approval applies to the revised remaining plan.

---

# 47. Completed Task Invalidation

If replanning changes an assumption or contract used by an already completed task:

```text
T002 DONE
```

may become:

```text
T002 DONE
verification_stale: true
```

Prefer re-verification over automatically discarding implementation.

---

# 48. Review Quality Checks

Before presenting a plan for approval, automatically verify:

```text
[ ] every task has repository ownership
[ ] all dependencies exist
[ ] DAG has no cycles
[ ] all requirements map to tasks
[ ] all acceptance criteria have planned verification
[ ] all repositories exist
[ ] cross-repository dependencies are explicit
[ ] assumptions are listed
[ ] high-risk assumptions are highlighted
[ ] risks have mitigation
[ ] expected change areas exist
[ ] verification commands/strategy are defined
[ ] no implementation changes occurred during planning
```

If these fail:

```text
PLAN_GENERATED
```

must not transition to:

```text
REVIEW_PENDING
```

until corrected.

---

# 49. JSON Output

For agent integration:

```bash
feature-dev review --json
```

Example:

```json
{
  "revision": 3,
  "status": "REVIEW_PENDING",
  "approval_required": true,
  "tasks": 7,
  "repositories": 3,
  "assumptions": 4,
  "high_risk_assumptions": 1,
  "risks": {
    "high": 1,
    "medium": 2
  },
  "validation": {
    "dag": "PASS",
    "requirements_coverage": "PASS",
    "verification_mapping": "PASS"
  }
}
```

Structured output should be preferred for Cursor automation.

---

# 50. New CLI Command Set

Recommended planning/review commands:

```text
feature-dev plan PLAN.md

feature-dev review
feature-dev review --json

feature-dev plan-diff

feature-dev approve
feature-dev approve --revision <n>

feature-dev reject
feature-dev replan

feature-dev status
feature-dev graph
```

Execution commands remain:

```text
feature-dev ready
feature-dev start T001
feature-dev context T001
feature-dev verify T001
feature-dev complete T001
```

---

# 51. Implementation Milestone A — Workflow Approval State

Implement first:

```text
workflow states
plan revision
approval state
approval persistence
implementation lock
```

Tests:

```text
cannot start before approval
can start after approval
new revision invalidates approval
stale revision cannot be approved
approval survives restart
```

---

# 52. Implementation Milestone B — Review Artifact

Implement:

```text
review.md generation
task summary
DAG rendering
assumption summary
risk summary
verification summary
```

Definition of done:

> A developer can understand the entire proposed implementation without opening individual task files.

---

# 53. Implementation Milestone C — Plan Validation

Implement deterministic checks for:

```text
task repository validity
dependency validity
DAG cycles
requirement coverage
acceptance-criteria coverage
verification presence
plan fingerprint
```

Only valid plans can enter `REVIEW_PENDING`.

---

# 54. Implementation Milestone D — Revisioning

Implement immutable plan revisions:

```text
revision-001
revision-002
revision-003
```

Support:

```text
current revision
approved revision
previous revision
plan diff
```

---

# 55. Implementation Milestone E — Cursor Integration

Teach Cursor/Copilot:

```text
plan
review
wait
apply requested changes
re-review
approve
execute
```

Test that Cursor does not modify source during planning.

---

# 56. Test Scenarios

## Scenario 1 — Simple approval

```text
PLAN
↓
revision 1
↓
review
↓
approve
↓
implement
```

Expected:

```text
PASS
```

## Scenario 2 — Implementation attempted before approval

```text
revision 1
REVIEW_PENDING
feature-dev start T001
```

Expected:

```text
DENIED
```

## Scenario 3 — User requests changes

```text
revision 1
↓
review
↓
user modifies scope
↓
revision 2
↓
review
↓
approve
```

Expected:

```text
only revision 2 executable
```

## Scenario 4 — Approved plan modified

```text
revision 2 approved
↓
task added
↓
revision 3
```

Expected:

```text
approval invalidated
```

## Scenario 5 — Resume

```text
revision 3 approved
↓
close Cursor
↓
reopen
↓
reconcile
```

Expected:

```text
approval restored from state
no new approval required
```

## Scenario 6 — Mid-implementation replanning

```text
T001 DONE
T002 BLOCKED
↓
revision 4
↓
review/approve
```

Expected:

```text
T001 preserved
remaining DAG updated
```

---

# 57. Security Consideration

User approval must never authorize arbitrary future commands.

Approval means:

```text
I approve this implementation plan.
```

It does not mean:

```text
Execute any command without restriction.
```

Security policies remain independently enforced.

Destructive operations may still require separate confirmation.

---

# 58. Why This Gate Matters

Without this gate:

```text
PLAN.md
   ↓
AI interpretation
   ↓
AI implementation
```

A planning error becomes code immediately.

With the gate:

```text
PLAN.md
   ↓
AI interpretation
   ↓
structured proposed plan
   ↓
human review
   ↓
approved execution
```

This turns the model from:

```text
autonomous planner + executor
```

into:

```text
proposal engine
+
controlled executor
```

for the most consequential stage.

---

# 59. Recommended Default Behaviour

V1 default:

```yaml
approval:
  before_implementation: required
```

Do not provide an easy silent bypass initially.

For development/testing, optionally support:

```text
--auto-approve
```

only in explicit non-production/test mode.

Never make it the default.

---

# 60. Final Behaviour

The final orchestration lifecycle should be:

```text
PLAN.md
   ↓
Workspace Discovery
   ↓
Repository Analysis
   ↓
Impact Analysis
   ↓
Requirement Mapping
   ↓
Assumptions
   ↓
Risks
   ↓
Task Generation
   ↓
Cross-Repo DAG
   ↓
Verification Strategy
   ↓
Plan Validation
   ↓
Plan Revision Created
   ↓
REVIEW_PENDING
   ↓
Human Review
   ↓
Approve / Modify / Reject
   ↓
APPROVED
   ↓
Implementation
   ↓
Task Verification
   ↓
Rework if Needed
   ↓
Cross-Repo Verification
   ↓
Requirement Traceability Check
   ↓
Final Review
   ↓
COMPLETED
```

The central invariant is:

> **No production implementation begins until the exact current plan revision has been explicitly reviewed and approved by the user.**

I’d place this enhancement **between your Planning Engine milestone and Execution Workflow milestone**. It should become a core architectural requirement rather than an optional UX feature.
