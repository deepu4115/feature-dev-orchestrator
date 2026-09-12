# Feature Development Orchestrator — Workspace-Native Cursor/Copilot Architecture Plan

## 1. Vision

Build a **workspace-native feature-development orchestration workflow that runs inside Cursor or GitHub Copilot coding agents** and supports both:

- a workspace containing a single Git repository
- a workspace containing multiple Git repositories

The top-level orchestration boundary is the **Workspace**.

A repository is an **execution, context, Git, and verification boundary** inside that workspace.

The system does not create another coding agent, agent broker, or universal interoperability protocol. Instead, it gives the coding agent already running in the IDE a disciplined workflow for turning a `PLAN.md` into verified code across one or more repositories while minimizing unnecessary context, model calls, rework, and cross-repository conflicts.

Core rule:

> **Workspace = orchestration boundary. Repository = execution and context boundary.**

The workflow should:

1. Understand the feature contract.
2. Discover repositories in the current workspace.
3. Analyze repositories once and reuse what it learns.
4. Decompose work into dependency-aware tasks.
5. Allow task dependencies to cross repository boundaries.
6. Determine the minimum sufficient context for each task.
7. Execute each task inside its target repository.
8. Verify changes using repository-aware commands.
9. Persist workspace-level state so the feature can resume in a new IDE-agent session.
10. Use parallelism only when it reduces total development work.

The workspace coordinates the feature.

Each repository remains the source of truth for its own code and Git history.

---

# 2. Core Idea

The coding agent itself is the execution runtime.

```text
Developer
   |
   v
Cursor / Copilot
   |
   v
Workspace Orchestration Instructions
   |
   +-- PLAN.md
   +-- workspace config
   +-- repository discovery
   +-- repository rules
   +-- orchestration skill/prompt
   +-- deterministic scripts
   +-- .feature/ workspace state
   |
   v
Discover -> Plan -> Execute -> Verify -> Persist -> Continue
```

The project therefore does **not** need:

- Universal Agent Protocol
- Agent adapters
- Agent capability negotiation
- Generic agent execution service
- Cursor/Copilot/Codex broker
- External orchestration daemon
- Custom model routing in V1

Portability comes from **plain workspace artifacts, repository-scoped context, and deterministic tools**, not from a custom agent protocol.

---

# 3. Workspace Model

A **Workspace** is the directory opened in Cursor/Copilot for feature development.

A workspace may contain one Git repository:

```text
workspace/
├── .git/
├── src/
├── pom.xml
├── PLAN.md
└── .feature/
```

Or many Git repositories:

```text
workspace/
├── repo-a/
│   ├── .git/
│   └── ...
├── repo-b/
│   ├── .git/
│   └── ...
├── repo-c/
│   ├── .git/
│   └── ...
├── PLAN.md
└── .feature/
```

The parent workspace directory does **not** need to be a Git repository.

For multi-repository workspaces, the recommended setup is:

```text
workspace/
├── repo-a/.git
├── repo-b/.git
└── repo-c/.git
```

rather than requiring:

```text
workspace/.git
```

Nested repositories, submodules, or monorepo-like layouts may exist, but the orchestrator must model each discovered Git repository explicitly.

---

# 4. Problem Statement

Normal IDE-agent feature development often becomes inefficient because:

- the agent repeatedly rediscovers repository architecture
- large amounts of workspace context are loaded unnecessarily
- long conversations accumulate stale context
- implementation starts before dependencies are understood
- tasks in different repositories are planned independently even when they depend on each other
- verification is executed from the wrong directory or with the wrong build tool
- one repository is over-contextualized with unrelated code from another repository
- parallel tasks can conflict through shared interfaces or generated artifacts
- interrupted sessions are difficult to resume reliably
- expensive model reasoning is used for deterministic operations
- developers cannot easily see why work was ordered a certain way

The orchestrator addresses these problems by turning feature development into a **stateful workspace workflow** followed by Cursor/Copilot.

---

# 5. Product Positioning

Do not position this as another AI coding agent.

Position it as:

> **A workspace-native orchestration system that makes Cursor/Copilot develop features across one or many repositories with less context, less rework, and stronger verification.**

The orchestration layer determines:

- which repositories are part of the workspace
- which repositories are relevant to the feature
- what needs to be done
- which repository owns each task
- task dependencies
- cross-repository dependencies
- what can safely run independently
- relevant files and symbols inside each repository
- what context the current task needs
- when deterministic tooling is sufficient
- when model reasoning is justified
- what verification must run and where
- what information must survive the current conversation

Central principle:

> **Use intelligence only where intelligence is needed.**

---

# 6. Design Goals

Priority order:

1. Correctness
2. Workspace-level consistency
3. Minimum unnecessary model work
4. Minimum rework
5. Minimum sufficient repository-scoped context
6. Reliable resumability
7. Useful parallelism
8. Explainability
9. Wall-clock performance

Latency is acceptable when additional deterministic analysis prevents expensive model calls or cross-repository rework.

---

# 7. V1 Scope

## In scope

- Cursor/Copilot-native workflow
- single-repository workspaces
- multi-repository workspaces
- automatic repository discovery
- explicit repository registry
- workspace-level `PLAN.md`
- repository-specific plans when needed
- repository analysis
- cross-repository feature decomposition
- workspace-level task DAG
- cross-repository task dependencies
- task state machine
- repository-scoped context
- repository knowledge caches
- deterministic scheduling
- selective parallelism
- Git-aware execution per repository
- optional Git worktrees
- repository-aware build/test/static-analysis execution
- acceptance-criteria verification
- cross-repository verification
- focused retry/rework
- durable workspace-level state
- resume across sessions
- dry-run planning
- execution artifacts
- context/token estimates
- explainable scheduling decisions
- security boundaries
- configurable workflow

## Out of scope for V1

- Universal Agent Protocol
- agent adapters
- agent broker/service
- direct vendor API integrations
- model-provider abstraction
- web UI
- desktop UI
- distributed execution
- autonomous cloud workers
- proprietary LLM
- new IDE
- organization-wide task queues
- complex dashboards

---

# 8. High-Level Architecture

```text
Developer
   |
   v
Cursor / Copilot Coding Agent
   |
   +---------------- Workspace Instructions -----------------+
   |                                                         |
   | AGENTS.md / Cursor Rules / Copilot Instructions         |
   | Feature Orchestrator Skill / Prompt                     |
   |                                                         |
   +---------------------------+-----------------------------+
                               |
                               v
                            PLAN.md
                               |
                               v
                    +----------------------+
                    | Workspace Discovery  |
                    | Git Repositories     |
                    +----------+-----------+
                               |
                               v
                    +----------------------+
                    | Planning Workflow    |
                    | Cross-Repo Analysis  |
                    +----------+-----------+
                               |
                               v
                    +----------------------+
                    | Workspace Task DAG   |
                    | .feature/            |
                    +----------+-----------+
                               |
                 +-------------+-------------+
                 |                           |
                 v                           v
        +------------------+        +------------------+
        | Context Engine   |        | Scheduler        |
        | repo-scoped      |        | deps/conflicts   |
        +--------+---------+        +--------+---------+
                 |                           |
                 +-------------+-------------+
                               |
                               v
                    +----------------------+
                    | Current IDE Agent    |
                    | Executes in Repo     |
                    +----------+-----------+
                               |
                               v
                    +----------------------+
                    | Repo-Aware Verify    |
                    | build/test/diff/etc. |
                    +----------+-----------+
                               |
                    fail ------+------ pass
                      |                 |
                      v                 v
               Focused Rework     Persist Result
                                        |
                                        v
                                  Next Ready Task
                                        |
                                        v
                             Cross-Repo Final Review
```

The **IDE agent is the intelligence layer**.

The workspace orchestrator coordinates repositories.

Each task executes within exactly one primary repository unless explicitly marked as workspace-level.

---

# 9. Workspace Discovery

Repository discovery is the first deterministic step.

The orchestrator should scan the workspace for Git repositories.

Conceptually:

```text
Workspace Root
   |
   v
Find Git roots
   |
   v
Normalize paths
   |
   v
Apply include/exclude rules
   |
   v
Assign repository IDs
   |
   v
Persist repository registry
```

Example discovered workspace:

```yaml
repositories:
  devices:
    path: ./devices
    git_root: ./devices

  analytics:
    path: ./analytics
    git_root: ./analytics

  common:
    path: ./common
    git_root: ./common
```

Repository discovery should handle:

- workspace root itself being a Git repository
- child Git repositories
- nested repositories
- ignored directories
- generated/build directories
- duplicate paths
- optional explicit registration

Repository IDs must remain stable across sessions where possible.

---

# 10. Repository Discovery Rules

Discovery should prefer deterministic rules.

Example:

```yaml
workspace:
  repository_discovery:
    mode: auto

    include:
      - "./*"

    exclude:
      - "./.feature"
      - "./.idea"
      - "./node_modules"
      - "./target"
      - "./build"
      - "./.worktrees"
```

Support modes:

```text
auto
explicit
hybrid
```

## Auto

Discover Git repositories automatically.

## Explicit

Only repositories listed in configuration participate.

## Hybrid

Discover automatically but allow explicit include/exclude overrides.

Recommended default:

```text
hybrid
```

---

# 11. Workspace Configuration

Example:

```yaml
schema_version: "1.0"

workspace:
  id: device-feature-workspace

  repository_discovery:
    mode: hybrid

repositories:
  devices:
    path: ./devices

  analytics:
    path: ./analytics

  common:
    path: ./common

execution:
  strategy: balanced
  parallelism: auto
  max_parallel_tasks: 2

context:
  progressive_expansion: true
  cache_repository_knowledge: true

verification:
  repository_aware: true
  cross_repository_final_check: true

git:
  use_worktrees: auto
  auto_commit: false
```

The configuration should work equally well when only one repository exists.

---

# 12. Workspace-Level State

Feature orchestration state belongs at the workspace level.

Recommended structure:

```text
workspace/
├── PLAN.md
├── .feature/
│   ├── config.yaml
│   ├── workflow.yaml
│   ├── repositories.json
│   │
│   ├── tasks/
│   ├── state/
│   ├── context/
│   ├── artifacts/
│   ├── reviews/
│   └── logs/
│
├── repo-a/
├── repo-b/
└── repo-c/
```

This means one feature can span multiple repositories while having one coherent workflow state.

Do not create unrelated workspace-level orchestration state separately inside every repository for the same feature.

---

# 13. Single-Repository Workspace

A single repository is simply a workspace with one repository.

Example:

```text
repo-a/
├── .git/
├── PLAN.md
├── .feature/
├── src/
└── pom.xml
```

Repository registry:

```yaml
repositories:
  repo-a:
    path: .
```

No separate architecture is required.

All workspace abstractions remain valid.

This avoids maintaining separate single-repo and multi-repo workflows.

---

# 14. Multi-Repository Workspace

Example:

```text
workspace/
├── PLAN.md
├── .feature/
├── devices/
│   ├── .git/
│   └── ...
├── analytics/
│   ├── .git/
│   └── ...
└── common/
    ├── .git/
    └── ...
```

A feature may touch:

```text
common
   |
   v
devices
   |
   v
analytics
```

The orchestrator must understand these relationships as one feature DAG even though code changes live in separate Git repositories.

---

# 15. Workspace PLAN.md

`PLAN.md` is the workspace-level feature contract.

Recommended structure:

```markdown
# Feature

## Objective

## Background

## Requirements

## Repositories

## Constraints

## Existing Architecture

## Cross-Repository Dependencies

## Acceptance Criteria

## Testing Requirements

## Non-Goals

## Risks

## Notes
```

The `Repositories` section is optional.

If omitted, the orchestrator discovers relevant repositories from the workspace.

Example:

```markdown
## Repositories

- devices
- analytics
- common
```

The plan should remain useful without understanding the orchestration system.

---

# 16. Repository-Specific Requirements

Some feature requirements may clearly belong to one repository.

Example:

```markdown
## Requirements

### common
- Add shared DTO

### devices
- Consume shared DTO
- Add transformation logic

### analytics
- Add reporting support
```

The orchestrator should preserve these hints.

It may also infer repository ownership based on existing architecture.

Repository ownership should never be guessed when repository evidence contradicts the plan.

---

# 17. Core User Experience

Typical workflow:

```text
1. Developer opens a Cursor workspace
2. Workspace contains one or more Git repositories
3. Developer creates/provides PLAN.md
4. Developer asks Cursor/Copilot to run feature orchestration
5. Agent discovers repositories
6. Agent analyzes relevant repositories
7. Agent creates workspace-level DAG
8. Developer reviews dry-run when desired
9. Agent executes repository-scoped tasks
10. Repository-aware checks verify each task
11. Cross-repository checks verify integration
12. Results are persisted at workspace level
13. A new session can resume
```

Example invocation:

```text
Run the feature orchestrator for PLAN.md.
```

Possible explicit prompts:

```text
Plan PLAN.md
Show workspace feature status
Execute next task
Continue feature
Verify feature
Explain T003
Show repository dependencies
```

---

# 18. Task Model

Every independently verifiable implementation unit becomes a task.

Each implementation task must identify its primary repository.

Example:

```yaml
schema_version: "1.0"

id: T003
title: Implement DeviceHealthService

repository: devices
working_directory: ./devices

status: READY

goal: >
  Implement the service responsible for retrieving device health.

dependencies:
  - T001

expected_files:
  - src/main/java/.../DeviceHealthService.java
  - src/test/java/.../DeviceHealthServiceTest.java

relevant_symbols:
  - Device
  - DeviceRepository

acceptance_criteria:
  - Device health can be retrieved
  - Unknown devices are handled
  - Unit tests pass

constraints:
  - Do not modify existing public APIs

verification:
  - ./mvnw -pl device test

context_refs:
  - repo.devices.architecture
  - repo.devices.symbol.DeviceRepository
  - task.T001.result
```

The `repository` field is mandatory for implementation tasks.

---

# 19. Workspace-Level Tasks

Some tasks may not belong to a single repository.

Examples:

- cross-repository planning
- contract compatibility validation
- end-to-end verification
- integration review
- release-note generation

These tasks may use:

```yaml
repository: workspace
```

Workspace tasks should not directly modify source code unless explicitly allowed.

---

# 20. Cross-Repository Task Dependencies

Dependencies may cross repositories.

Example:

```text
common:T001
     |
     +-------------+
     |             |
     v             v
devices:T002   analytics:T003
     |
     v
devices:T004
```

Task schema:

```yaml
id: T002
repository: devices

dependencies:
  - T001
```

`T001` may belong to `common`.

The scheduler must evaluate dependencies at the workspace level, not separately inside each repository.

---

# 21. Cross-Repository Dependency Types

Support different dependency strengths.

Example:

```yaml
dependencies:
  - task: T001
    type: hard

  - task: T004
    type: validation
```

Potential dependency types:

```text
hard
soft
validation
contract
generated-artifact
```

## Hard

The upstream task must be DONE before execution.

## Soft

Can proceed using current assumptions but may need later reconciliation.

## Validation

Implementation can proceed, but final verification depends on upstream completion.

## Contract

Depends on an interface/schema/API contract.

## Generated artifact

Depends on generated files, schemas, clients, or build outputs from another repository.

V1 may initially implement only `hard` and `validation` while retaining an extensible schema.

---

# 22. Task Granularity

Tasks should be:

- independently understandable
- owned primarily by one repository
- independently verifiable where possible
- small enough for focused context
- large enough to avoid excessive orchestration overhead

Avoid mixing unrelated repository changes into one task.

Bad:

```text
T001
- modify common DTO
- update devices service
- update analytics report
```

Better:

```text
T001 common: Add shared DTO
T002 devices: Consume shared DTO
T003 analytics: Consume shared DTO
```

This makes dependencies and verification explicit.

---

# 23. Task State Machine

```text
PLANNED
   |
   v
READY
   |
   v
RUNNING
   |
   +------> BLOCKED
   |
   v
IMPLEMENTED
   |
   v
VERIFYING
   |
   +------> REWORK
   |           |
   |           v
   |        RUNNING
   |
   +------> FAILED
   |
   v
DONE
```

Supported states:

```text
PLANNED
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

Every transition is persisted at workspace level.

---

# 24. Workspace Task DAG

All tasks belong to one workspace-level DAG.

The DAG may contain tasks from multiple repositories.

Example:

```text
                       T001(common)
                      /            \
                     v              v
             T002(devices)     T003(analytics)
                     \              /
                      v            v
                     T004(workspace)
```

Repository boundaries do not create separate DAGs.

They add execution constraints to the same DAG.

---

# 25. Planning Pipeline

```text
PLAN.md
   |
   v
Discover repositories
   |
   v
Parse requirements
   |
   v
Identify relevant repositories
   |
   v
Inspect repository caches
   |
   v
Analyze affected architecture
   |
   v
Generate repository-scoped tasks
   |
   v
Resolve cross-repository dependencies
   |
   v
Build workspace DAG
   |
   v
Estimate write/conflict surfaces
   |
   v
Estimate task context
   |
   v
Generate execution plan
```

Only stages requiring semantic reasoning should consume model reasoning.

---

# 26. Repository Knowledge Cache

Each repository gets its own cache.

Recommended structure:

```text
.feature/context/
├── workspace.md
│
├── devices/
│   ├── repository.md
│   ├── architecture.md
│   ├── conventions.md
│   ├── symbols.json
│   └── fingerprints.json
│
├── analytics/
│   ├── repository.md
│   ├── architecture.md
│   ├── conventions.md
│   ├── symbols.json
│   └── fingerprints.json
│
└── common/
    ├── repository.md
    ├── architecture.md
    ├── conventions.md
    ├── symbols.json
    └── fingerprints.json
```

Repository context should not be merged into one large global context artifact.

---

# 27. Workspace Knowledge

Workspace-level knowledge should contain only cross-repository information.

Example:

```text
.feature/context/workspace.md
```

Potential content:

- repository registry
- repository responsibilities
- high-level repository relationships
- known shared contracts
- generated-code relationships
- cross-repository build flow
- cross-repository integration points

Avoid duplicating full repository architecture here.

---

# 28. Repository Context Isolation

Context should default to the task's repository.

For:

```yaml
repository: devices
```

the initial context should primarily come from:

```text
repo.devices.*
```

Only load another repository when:

- a dependency result references it
- a shared interface is required
- a generated artifact is consumed
- verification requires it
- the task explicitly spans a contract boundary

This prevents accidental loading of an entire multi-repository workspace.

---

# 29. Context Engine

Do not repeatedly provide:

```text
PLAN.md
+ all repositories
+ all previous conversations
+ every completed task
```

Instead construct the **minimum sufficient repository-scoped context**.

Example:

```yaml
task: T003
repository: devices

goal:
  Implement DeviceHealthService.

requirements:
  - Return device health
  - Handle missing devices

files:
  - DeviceHealthService.java
  - DeviceRepository.java
  - DeviceHealthServiceTest.java

symbols:
  - Device
  - DeviceRepository

cross_repo_dependencies:
  - task.T001.result

constraints:
  - Do not modify public API

verification:
  cwd: ./devices
  commands:
    - ./mvnw test -Dtest=DeviceHealthServiceTest
```

---

# 30. Cross-Repository Context

When a task depends on another repository, prefer concise artifacts over raw source.

Example:

```text
T001(common) produces:
  task.T001.result
  contract.SharedDeviceDto
```

Then `T002(devices)` receives:

```yaml
context_refs:
  - task.T001.result
  - contract.SharedDeviceDto
  - repo.devices.architecture
```

It should not automatically load the entire `common` repository.

---

# 31. Progressive Context Expansion

Use progressive expansion per repository.

```text
Level 0
Task + requirements + dependency results

Level 1
Relevant symbols/snippets in target repo

Level 2
Relevant files in target repo

Level 3
Adjacent files/tests/configuration in target repo

Level 4
Cross-repository contract/source context

Level 5
Broader target-repository context

Level 6
Broader workspace exploration
```

Start at the lowest level likely to be sufficient.

Workspace-wide exploration is the last resort.

---

# 32. Context References

Use repository-qualified references.

Example:

```yaml
context_refs:
  - workspace.architecture
  - repo.devices.architecture
  - repo.devices.symbol.DeviceRepository
  - repo.common.contract.DeviceHealthDto
  - task.T001.result
```

Fingerprints:

```text
repo.devices.architecture@sha256:...
repo.common.contract.DeviceHealthDto@sha256:...
```

Repository qualification prevents collisions between similarly named classes or modules.

---

# 33. Same Symbol Names Across Repositories

Different repositories may contain identical symbol names.

Example:

```text
devices: DeviceService
analytics: DeviceService
```

Therefore symbol references must always be repository-qualified.

Bad:

```text
symbol.DeviceService
```

Good:

```text
repo.devices.symbol.DeviceService
repo.analytics.symbol.DeviceService
```

This is required for reliable context retrieval.

---

# 34. Deterministic vs Agent Decisions

## Deterministic

Prefer scripts/tools for:

- repository discovery
- repository path normalization
- DAG traversal
- cycle detection
- ready-task calculation
- state transitions
- Git status/diff per repository
- changed-file detection
- file overlap
- build execution
- test execution
- static analysis
- formatting checks
- artifact validation
- fingerprints
- cache invalidation
- repository-aware command execution
- basic token/context-size accounting

## Agent reasoning

Use Cursor/Copilot for:

- feature decomposition
- repository responsibility inference
- architectural interpretation
- semantic cross-repository dependencies
- implementation
- ambiguity resolution
- complex failure diagnosis
- risk assessment
- code review
- deciding whether unexpected edits are justified

---

# 35. Workspace Scheduler

The scheduler operates on the workspace DAG.

It asks:

```text
What is the next cheapest safe unit of workspace progress?
```

A task is ready when:

- all hard dependencies are DONE
- its target repository exists
- required repository state exists
- it is not blocked
- execution will not conflict with active work
- required upstream contracts are available

The task's repository determines where execution occurs.

---

# 36. Repository-Aware Scheduling

Two tasks in different repositories are not automatically safe to parallelize.

Example:

```text
T002 devices
T003 analytics
```

They may still conflict if both depend on:

```text
common:T001 contract not finalized
```

The scheduler must consider:

- repository overlap
- shared contract dependencies
- generated artifacts
- shared external resources
- build/test coupling
- ordering constraints

Different Git repositories reduce file conflicts, but not necessarily semantic conflicts.

---

# 37. Parallelism

Parallelism is not the primary objective.

Run tasks concurrently only when:

- dependencies permit it
- target repositories do not create unsafe coupling
- write surfaces are separate
- shared contracts are stable
- verification can remain isolated
- expected wall-clock savings justify extra coordination

Example:

```text
T002 -> devices
T003 -> analytics
```

Potentially parallel if both consume a completed stable contract.

But:

```text
T001 -> common contract
T002 -> devices consumes contract
```

must be serialized when `T002` depends on `T001`.

---

# 38. Parallelism Inside Cursor/Copilot

V1 must work perfectly with **one active coding agent**.

If the IDE supports background/subagents safely, the scheduler may use them as an optimization.

Example:

```yaml
execution:
  parallelism: auto
  max_parallel_tasks: 2
```

Each parallel task must still have:

- one target repository
- isolated Git state
- independent verification
- explicit dependency state

---

# 39. Git Model

Each repository owns its own Git state.

Example:

```text
workspace/
├── devices/.git
├── analytics/.git
└── common/.git
```

The orchestrator must never treat the workspace as if it has one global Git diff unless the workspace itself is a real Git repository.

Git commands must execute against the task's repository.

Example:

```text
git -C ./devices status
git -C ./devices diff
git -C ./analytics status
```

---

# 40. Repository Git Identity

Persist Git metadata per repository.

Example:

```yaml
repositories:
  devices:
    path: ./devices
    branch: feature/device-health
    head: abc123

  analytics:
    path: ./analytics
    branch: feature/device-health
    head: def456
```

This supports safe resume and reconciliation.

---

# 41. Branch Strategy

Repositories may use different branches.

The orchestrator should not assume identical branch names.

Example:

```text
devices    -> feature/device-health
analytics  -> feature/device-health-report
common     -> feature/shared-health-dto
```

Branch policy should be configurable.

V1 should avoid automatic branch creation unless explicitly enabled.

---

# 42. Worktrees

Worktrees remain repository-scoped.

Example:

```text
workspace/.worktrees/
├── devices-T002/
└── analytics-T003/
```

A worktree must map back to its source repository.

Do not create worktrees merely because multiple repositories exist.

Use them only when parallel execution needs isolation.

---

# 43. Conflict Estimation

Conflict estimation should include two dimensions.

## Intra-repository

```text
file overlap
symbol overlap
directory overlap
shared tests/configuration
```

## Cross-repository

```text
contract dependency
generated artifact dependency
version dependency
build-order dependency
shared schema
integration-test dependency
```

Classification:

```text
LOW
MEDIUM
HIGH
```

---

# 44. Repository-Aware Verification

Every task must define verification relative to its repository.

Example:

```yaml
verification:
  repository: devices
  working_directory: ./devices

  commands:
    - ./mvnw test -Dtest=DeviceHealthServiceTest
```

The orchestrator must not assume commands run from the workspace root.

---

# 45. Verification Command Discovery

Verification commands should be inferred from repository-native configuration.

Examples:

## Maven

```text
./mvnw test
mvn test
```

## Gradle

```text
./gradlew test
```

## Node

```text
npm test
pnpm test
yarn test
```

## Go

```text
go test ./...
```

## Rust

```text
cargo test
```

## Perl

Use repository-defined test tooling.

Never assume all repositories use the same language or build system.

---

# 46. Mixed-Technology Workspaces

A workspace may contain:

```text
devices      Java / Maven
analytics    Java / Gradle
tools        Python
legacy       Perl
frontend     Node
```

The orchestration model remains the same.

Only repository analyzers and verification commands vary.

This is a primary reason to keep repositories as explicit execution boundaries.

---

# 47. Task Verification Pipeline

For a repository-scoped task:

```text
Implementation
   |
   v
Repository Git diff
   |
   v
Scope validation
   |
   v
Repository compile/build
   |
   v
Focused repository tests
   |
   v
Static analysis
   |
   v
Acceptance criteria
   |
   v
Task DONE
```

Stop early when deterministic verification fails.

---

# 48. Cross-Repository Verification

After dependent repository tasks complete, the orchestrator may need integration verification.

Example:

```text
common:T001 DONE
devices:T002 DONE
analytics:T003 DONE
      |
      v
workspace:T004
Cross-repository verification
```

Potential checks:

- shared schema compatibility
- generated client compatibility
- API contract consistency
- dependent repository build
- integration tests
- version alignment
- configuration compatibility

---

# 49. Verification Levels

Use layered verification.

## Level 1 — Task repository

Focused checks for the changed repository.

## Level 2 — Affected repositories

Run dependent repository checks when interfaces changed.

## Level 3 — Workspace feature

Final cross-repository verification.

Do not run every repository's full test suite after every small task.

---

# 50. Contract Change Propagation

When a task changes a shared contract, mark dependent repository tasks as affected.

Example:

```text
common:T001 changes DTO
      |
      +------> devices:T002
      |
      +------> analytics:T003
```

If the contract changes again after downstream work:

```text
T001 fingerprint changed
```

dependent task results may become stale.

The orchestrator must detect this.

---

# 51. Artifact-Based Communication

Completed tasks should become concise workspace-level artifacts.

Example:

```text
.feature/artifacts/
├── T001-result.md
├── T002-result.md
└── T003-result.md
```

Each artifact identifies its repository.

Example:

```yaml
task_id: T002
repository: devices

status: SUCCESS

changed_files:
  - src/main/java/.../DeviceService.java

summary: >
  Consumed the shared DeviceHealthDto contract.

dependencies_used:
  - T001
```

Future tasks consume artifacts instead of prior conversations.

---

# 52. Cross-Repository Contracts

Important shared interfaces may have dedicated artifacts.

Example:

```text
.feature/context/contracts/
├── DeviceHealthDto.md
├── PolicySchema.md
└── ReportingApi.md
```

These should summarize:

- owning repository
- source files
- current fingerprint
- consumers
- relevant compatibility constraints

This is more efficient than loading entire upstream repositories.

---

# 53. Session Independence

The workflow must not depend on chat history.

A fresh Cursor/Copilot session should recover by reading:

```text
PLAN.md
.feature/config.yaml
.feature/repositories.json
.feature/state/workflow.json
.feature/state/tasks.json
relevant task file
relevant dependency artifacts
target repository cache
current Git state of affected repositories
```

No previous chat transcript is required.

---

# 54. Workspace Reconciliation

On resume:

```text
Persisted workspace state
        +
Current repository registry
        +
Git state of each affected repository
        +
Repository fingerprints
        |
        v
Workspace reconciliation
```

Detect:

- repository removed
- repository path changed
- branch changed
- repository HEAD changed
- task code reverted
- contract changed
- generated artifact changed
- interrupted RUNNING task
- stale context cache

Do not blindly trust persisted state.

---

# 55. Repository Reconciliation Scope

Only reconcile repositories relevant to the active feature when possible.

Avoid scanning every repository deeply on every resume.

Example:

```text
Feature repositories:
- common
- devices

Unrelated repositories:
- docs
- experiments
- old-tools
```

The unrelated repositories should not add context or verification cost.

---

# 56. Dry Run

Dry-run should be workspace-aware.

Example:

```text
Feature Analysis
────────────────────────

Workspace repositories:        5
Relevant repositories:         3

  common
  devices
  analytics

Tasks:                          9
Cross-repo dependencies:        4
Ready initially:                1
Parallel candidates:            2
High-risk contract edges:       1

Execution:

Group 1:
  T001 [common]

Group 2:
  T002 [devices]
  T003 [analytics]

Group 3:
  T004 [workspace verification]

Recommendation:
  Selective parallelism

Reason:
  devices and analytics can proceed independently
  after the common contract is finalized.
```

---

# 57. Explainability

Scheduling decisions should mention repositories explicitly.

Example:

```text
T003 [analytics] waits for T001 [common].

Reason:
- T003 consumes DeviceHealthDto.
- T001 creates that contract.
- The contract fingerprint does not yet exist.

Decision:
BLOCKED_BY_DEPENDENCY
```

Another:

```text
T002 [devices] and T003 [analytics] can run in parallel.

Reason:
- separate Git repositories
- no direct dependency between tasks
- both consume completed T001 contract
- verification is repository-local

Decision:
PARALLEL_CANDIDATE
```

---

# 58. Failure Classification

Classify failure before retrying.

```text
COMPILATION
TEST
STATIC_ANALYSIS
SCOPE_VIOLATION
MISSING_CONTEXT
ARCHITECTURE
ENVIRONMENT
MERGE_CONFLICT
CROSS_REPO_CONTRACT
REPOSITORY_MISSING
REPOSITORY_STATE
AGENT_EXECUTION
UNKNOWN
```

Repository identity is part of failure metadata.

Example:

```yaml
failure:
  type: TEST
  repository: devices
```

---

# 59. Intelligent Retry

Do not repeat the original request blindly.

Examples:

```text
Compilation failure in devices
-> compiler output + changed files + devices symbols

Test failure in analytics
-> failing test + analytics implementation

Cross-repo contract failure
-> contract artifact + upstream result + downstream failure

Missing context
-> expand target repository context first

Repository state mismatch
-> reconcile Git state before model reasoning

Environment failure
-> do not rewrite correct code
```

---

# 60. Cross-Repository Rework

A failure in one repository may invalidate downstream work.

Example:

```text
T001(common) contract changes during rework
      |
      +---- invalidate verification of T002(devices)
      |
      +---- invalidate verification of T003(analytics)
```

Do not automatically discard downstream code.

Instead mark:

```text
verification_stale: true
```

Then re-verify affected tasks.

---

# 61. Rework Budget

Example:

```yaml
rework:
  max_attempts_per_task: 3
  expand_context_after: 1
  require_user_after: 3
```

Cross-repository failures may trigger re-planning earlier if the dependency model was incorrect.

---

# 62. Workspace Security Model

Because the agent can operate across multiple repositories:

- never assume every workspace directory is trusted
- do not traverse unrelated directories unnecessarily
- never load secrets from one repository into another task context
- preserve repository-specific denied paths
- scope commands to repository roots
- require confirmation for destructive operations
- avoid destructive Git operations by default
- make network use explicit
- never silently weaken verification

---

# 63. Repository-Specific Security

A repository may override workspace defaults.

Example:

```yaml
repositories:
  devices:
    security:
      denied_paths:
        - credentials/
        - src/test/resources/private/

  analytics:
    security:
      denied_paths:
        - secrets/
```

The effective policy should be:

```text
workspace policy
+
repository policy
```

with the stricter rule winning.

---

# 64. Guardrails Against Cross-Repository Drift

The agent should not:

- modify a repository not relevant to the active feature
- move implementation between repositories without justification
- load unrelated repositories into context
- assume identical build commands across repositories
- assume same branch names
- assume same dependency versions
- mark downstream tasks valid after an upstream contract changed
- treat workspace root as one Git repository unless it actually is
- mix same-named symbols from different repositories
- execute verification from the wrong working directory

---

# 65. Existing Repository Instructions

Each repository may already contain:

- `AGENTS.md`
- Cursor rules
- Copilot instructions
- module-specific instructions
- build/test documentation

The orchestrator must treat repository-local instructions as constraints for tasks executed in that repository.

Workspace instructions coordinate.

Repository instructions specialize.

---

# 66. Instruction Hierarchy

Recommended hierarchy:

```text
Workspace orchestration instructions
        |
        v
Repository-local instructions
        |
        v
Task artifact
```

For a task in `devices`:

```text
workspace instructions
+
devices repository instructions
+
T003 task
+
minimum context
```

Do not load instructions from unrelated repositories.

---

# 67. Skill / Instruction Design

The main orchestrator instruction should behave like a state machine.

Conceptually:

```text
workspace-feature-orchestrator
├── DISCOVER
├── PLAN
├── EXECUTE
├── VERIFY
├── REWORK
├── REVIEW
└── RESUME
```

Each stage should load only the instructions relevant to that stage.

---

# 68. Progressive Instruction Loading

Keep the always-loaded instruction small.

Example:

```text
Core workspace instructions:
~300-800 tokens

Repository discovery instructions:
loaded only during discovery

Planning instructions:
loaded during planning

Execution instructions:
loaded for active task

Verification instructions:
loaded during verification

Recovery instructions:
loaded only after failure

Final review instructions:
loaded once
```

Large static orchestration prompts should be avoided.

---

# 69. Deterministic Helper Tools

Possible commands:

```text
feature workspace discover
feature workspace status
feature repositories

feature plan PLAN.md
feature graph
feature ready

feature context T003
feature start T003
feature verify T003
feature complete T003
feature fail T003

feature repo-status devices
feature reconcile
feature verify-workspace
```

These tools manage mechanics.

They should **not call another LLM**.

---

# 70. Repository-Aware Helper Execution

Helper commands should accept repository IDs rather than raw paths where possible.

Example:

```text
feature repo-status devices
```

Internally:

```text
repository ID
   |
   v
repository registry
   |
   v
normalized root
   |
   v
git/build/test command
```

This prevents path ambiguity.

---

# 71. Why Keep Helper Scripts?

Some behavior should not depend on whether the agent remembers an instruction correctly.

Examples:

- repository discovery
- repository ID mapping
- DAG readiness
- cycle detection
- state transition validation
- Git diff per repository
- fingerprint calculation
- verification execution
- artifact validation
- cross-repository dependency invalidation

This gives the workflow deterministic foundations.

---

# 72. Language and Framework Awareness

The orchestration core remains language-independent.

Repository-specific analyzers may detect:

```text
Maven / Gradle
npm / pnpm / yarn
Go modules
Cargo
Python tooling
Perl test tooling
```

Different repositories in the same workspace can use different stacks.

The workflow model does not change.

---

# 73. Java / Spring Boot Repository Optimization

For Java/Spring repositories, cache:

- Maven/Gradle modules
- package structure
- Spring components
- controllers
- services
- repositories
- configuration classes
- interfaces/implementations
- test classes
- dependency relationships

Verification can expand:

```text
single test
test class
module tests
affected modules
full repository build
```

---

# 74. Perl Repository Optimization

For Perl repositories, cache:

- packages/modules
- script entrypoints
- imported modules
- configuration/XML dependencies
- test files
- important global/shared modules

Avoid assuming Java-style symbol semantics.

---

# 75. Mixed Repository Dependency Example

Example workspace:

```text
workspace/
├── java-service/
├── perl-collector/
└── shared-schema/
```

Feature:

```text
T001 [shared-schema]
  Add response field
      |
      +----------------+
      |                |
      v                v
T002 [java-service]  T003 [perl-collector]
Consume field        Produce field
      |                |
      +--------+-------+
               v
        T004 [workspace]
        Integration verification
```

This is a first-class supported workflow.

---

# 76. Metrics

Measure what can be measured reliably.

Workspace metrics:

```text
repositories discovered
repositories touched
cross-repo dependencies
feature duration
tasks completed
parallel tasks
cross-repo verification failures
rework count
```

Repository metrics:

```text
build failures
test failures
changed files
context files loaded
context size
cache hits
cache invalidations
unexpected changed files
```

Where available:

```text
input tokens
output tokens
cost
model
```

Never make correctness depend on those metrics.

---

# 77. Optimization Objective

Conceptually minimize:

```text
Total Workspace Development Work =
    useful model reasoning
  + repeated repository context
  + implementation effort
  + cross-repository coordination
  + rework
  + review
  + merge/conflict resolution
  + verification
  + unnecessary model interactions
```

Subject to:

```text
Correctness
+ PLAN.md acceptance criteria
+ repository constraints
+ cross-repository compatibility
+ security
+ maintainability
```

---

# 78. Context Optimization Principle

Do not optimize only for smallest prompt.

Optimize:

```text
Useful information
------------------
Context consumed
```

For multi-repository workspaces:

> **Minimum sufficient repository-scoped context.**

The existence of more repositories must not automatically increase task context.

---

# 79. Parallelism Optimization Principle

Do not optimize:

```text
maximum simultaneous repositories
```

Optimize:

```text
maximum useful independent progress
```

Different repositories are easier to isolate physically, but may still be tightly coupled semantically.

---

# 80. Human Interaction

Ask the developer when:

- PLAN.md materially conflicts with repository architecture
- repository ownership is genuinely ambiguous
- a required repository is missing
- architecture choice has significant product implications
- destructive operation is required
- security boundary must be crossed
- repeated rework budget is exhausted
- workspace state cannot be reconciled safely

Do not ask questions that repository discovery or repository inspection can answer.

---

# 81. Suggested Installation Model

A workspace should need a small bootstrap.

Example:

```text
workspace/
├── .feature-orchestrator/
│   ├── ORCHESTRATOR.md
│   ├── discovery.md
│   ├── planning.md
│   ├── execution.md
│   ├── verification.md
│   ├── recovery.md
│   └── schemas/
│
├── .feature/
├── PLAN.md
├── repo-a/
└── repo-b/
```

Cursor/Copilot entrypoint instructions reference `ORCHESTRATOR.md`.

---

# 82. Optional Per-Repository Bootstrap

If repository-specific orchestration hints are needed:

```text
repo-a/
└── .feature-repo.yaml
```

Example:

```yaml
id: devices

verification:
  build: ./mvnw test

context:
  important_paths:
    - src/main/java
    - src/test/java
```

This file is optional.

The workspace registry remains authoritative for active feature orchestration.

---

# 83. V1 Milestone 1 — Workspace Runtime

Implement:

- workspace orchestration entrypoint
- Cursor/Copilot workspace instructions
- `.feature/` workspace state
- repository discovery
- repository registry
- task state machine
- basic resume

Goal:

> A new IDE-agent session can understand the workspace and continue a feature from persisted state.

---

# 84. V1 Milestone 2 — Workspace Planning Engine

Add:

- PLAN.md parsing workflow
- relevant repository identification
- repository inspection
- repository-scoped task decomposition
- cross-repository dependency DAG
- task artifacts
- dry-run summary
- DAG validation

Goal:

> PLAN.md reliably becomes one inspectable workspace execution plan.

---

# 85. V1 Milestone 3 — Repository-Scoped Context Engine

Add:

- repository knowledge cache
- workspace knowledge summary
- relevant-file discovery
- repository-qualified symbol references
- cross-repository contract artifacts
- progressive context expansion
- fingerprints
- cache invalidation

Goal:

> Tasks stop rediscovering repositories and stop loading unrelated workspace context.

---

# 86. V1 Milestone 4 — Deterministic Workspace Tooling

Add:

- repository discovery CLI/scripts
- state CLI/scripts
- ready-task calculation
- Git diff detection per repository
- conflict calculation
- repository-aware verification execution
- artifact validation
- workspace reconciliation

Goal:

> Mechanical orchestration no longer depends on model reasoning.

---

# 87. V1 Milestone 5 — Execution Workflow

Add:

- repository-aware ready-task selection
- task execution instructions
- result artifacts
- selective parallelism
- optional repository worktrees
- checkpoints
- cross-repository dependency invalidation

Goal:

> Cursor/Copilot can execute the complete workspace DAG safely.

---

# 88. V1 Milestone 6 — Verification and Recovery

Add:

- repository-local verification
- dependent-repository verification
- cross-repository verification
- failure classification
- progressive retry
- rework budgets
- risk-based review
- final workspace feature review

Goal:

> The workflow can recover from realistic multi-repository failures without broad restarts.

---

# 89. V1 Definition of Done

V1 succeeds when a developer can:

```text
1. Open a workspace containing one or more Git repositories
2. Add the orchestrator
3. Create PLAN.md
4. Ask Cursor/Copilot to plan the feature
5. Automatically discover repositories
6. Inspect generated workspace DAG
7. See which repository owns each task
8. Execute repository-scoped tasks
9. Respect cross-repository dependencies
10. Run repository-aware verification
11. Stop the IDE-agent session
12. Start a fresh session
13. Resume from workspace state
14. Reconcile repository Git state
15. Run cross-repository final verification
16. See what changed in each repository and why
```

No Universal Agent Protocol, external broker, or custom agent API is required.

---

# 90. Recommended Implementation Architecture

```text
feature-orchestrator/
├── instructions/
│   ├── ORCHESTRATOR.md
│   ├── discovery.md
│   ├── planning.md
│   ├── execution.md
│   ├── verification.md
│   └── recovery.md
│
├── schemas/
│   ├── workspace.schema.json
│   ├── repository.schema.json
│   ├── workflow.schema.json
│   ├── task.schema.json
│   └── result.schema.json
│
├── cli/
│   ├── workspace
│   ├── repository
│   ├── state
│   ├── dag
│   ├── git
│   ├── verify
│   └── context
│
├── repository/
│   ├── discovery
│   ├── scanner
│   ├── fingerprint
│   └── analyzers
│
└── templates/
```

There is intentionally no:

```text
protocol/
adapters/
agents/
model-router/
```

---

# 91. Technology Choice

The deterministic helper tooling should be portable and fast.

## Go

Strong default:

- single binary
- fast startup
- easy installation
- cross-platform
- good Git/process/filesystem support
- straightforward concurrency

## TypeScript

Good alternative when IDE ecosystem integration becomes important.

## Python

Useful for rapid prototyping but less attractive if zero-runtime installation is a priority.

Recommended default:

> **Go for deterministic helper tooling; Markdown/YAML/JSON for workspace and task contracts.**

The coding intelligence remains Cursor/Copilot.

---

# 92. What Should Not Be Built

Avoid premature infrastructure.

Do not build:

- a custom agent RPC protocol
- an adapter per AI vendor
- an agent gateway
- an LLM proxy
- a separate conversation-memory system
- a global workspace vector database before repository-local search proves insufficient
- a distributed scheduler
- a cloud control plane
- complex token accounting dependent on undocumented IDE behavior
- a worktree per task
- an LLM-based solution for deterministic repository discovery/DAG/state operations

---

# 93. Key Risks

## Repository discovery mistakes

Nested repositories or unusual layouts may be misclassified.

**Mitigation:** explicit registry overrides and dry-run output.

## Context leakage across repositories

The agent may load unrelated repositories.

**Mitigation:** repository-qualified references and target-repository-first context.

## Cross-repository contract drift

Upstream changes may invalidate downstream work.

**Mitigation:** contract fingerprints and verification invalidation.

## Verification in wrong directory

Commands may accidentally run from workspace root.

**Mitigation:** repository-aware execution wrapper.

## Instruction bloat

Large orchestration instructions may consume more context than they save.

**Mitigation:** progressive instruction loading.

## Agent ignores state

The IDE agent may improvise instead of following workflow.

**Mitigation:** deterministic state commands and concise mandatory entrypoint rules.

## Over-decomposition

Too many tiny tasks create orchestration overhead.

**Mitigation:** repository-scoped coherent task boundaries.

---

# 94. Product Principles

### 1. Workspace-first

The workspace is the feature orchestration boundary.

### 2. Repository-scoped execution

Every implementation task has one primary repository.

### 3. Cross-repository DAG

Dependencies can span repositories.

### 4. Repository-scoped context

Do not load unrelated repositories.

### 5. Repository-aware verification

Run checks using the target repository's tools and working directory.

### 6. IDE-agent native

Use the coding agent already available in Cursor/Copilot.

### 7. Repository portable

Keep durable workflow knowledge in plain workspace artifacts.

### 8. Deterministic where possible

Use scripts for mechanics.

### 9. Minimum sufficient context

Load only what the current task needs.

### 10. Durable state

Never depend on conversation history for recovery.

### 11. Selective parallelism

Concurrency is an optimization, not a goal.

### 12. Artifact-based communication

Persist concise results rather than conversations.

### 13. Explainability

Important scheduling, repository, and scope decisions should have reasons.

### 14. Simple before universal

Prove the workspace workflow before adding broader integrations.

---

# 95. Key Differentiator

Do not compete on:

> "We can run more agents."

Compete on:

> **"We coordinate feature development across one or many repositories so the coding agent does the minimum necessary work to reliably complete the feature."**

Every orchestration decision should answer:

```text
1. Which repository owns this work?
2. What is the next necessary task?
3. What repository context does it actually need?
4. Which cross-repository dependencies must be satisfied?
5. Can deterministic evidence replace additional reasoning?
6. Is parallel execution genuinely beneficial?
7. What repository-aware verification proves completion?
```

---

# 96. Long-Term Direction

Once the workspace-native Cursor/Copilot workflow is proven, it can become portable to other coding environments because the durable contract already consists of:

```text
PLAN.md
workspace registry
Markdown instructions
repository-scoped task artifacts
workspace state
Git repositories
verification commands
repository knowledge
cross-repository contracts
```

A future coding environment only needs to understand and follow those artifacts.

Do not add an interoperability protocol until a concrete integration requires one.

---

# 97. Immediate Engineering Target

The first target should be:

> **A workspace-native orchestration package that lets Cursor/Copilot take a PLAN.md, discover one or many Git repositories, build a cross-repository task DAG, execute repository-scoped tasks with minimum sufficient context, persist workspace-level state across sessions, use deterministic tooling for mechanical decisions, recover intelligently from failures, and perform repository-aware plus cross-repository verification.**

Build in this order:

```text
Workspace Instructions
        ↓
Repository Discovery
        ↓
Workspace State
        ↓
PLAN.md
        ↓
Repository Analysis
        ↓
Cross-Repo Task DAG
        ↓
Repository-Scoped Context
        ↓
Execution
        ↓
Repository Verification
        ↓
Cross-Repo Verification
        ↓
Recovery
        ↓
Optimization
```

Do **not** start with multi-agent infrastructure.

---

# 98. Success Criterion

The architecture is successful when a developer can open either:

```text
a single Git repository
```

or:

```text
a Cursor workspace containing multiple Git repositories
```

provide `PLAN.md`, invoke the orchestration workflow, and reliably reach verified feature completion **without an external agent platform, without assuming one global Git repository, and without carrying the entire workspace or feature conversation in model context**.
