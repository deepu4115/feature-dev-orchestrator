# `feature-dev` — Suggested Implementation Plan

## 1. Objective

Build a reusable **workspace-native orchestration engine** named `feature-dev`, installed once and usable from any Cursor/Copilot workspace.

It must support both single-repository and multi-repository workspaces with:

- workspace-level feature planning and state
- automatic Git repository discovery
- repository-scoped execution and context
- cross-repository task dependencies
- repository-aware Git and verification
- resume across IDE-agent sessions
- deterministic orchestration mechanics
- Cursor/Copilot as the semantic reasoning and coding layer

> **Workspace = orchestration boundary. Repository = execution/context/Git/verification boundary.**

## 2. Recommended Stack

Use **Go** for the core CLI and deterministic engine.

Use:

- **Cobra** — CLI command structure
- **Markdown** — `PLAN.md`, tasks, results, reviews, agent instructions
- **YAML** — human-editable configuration
- **JSON** — machine-managed workflow state and repository registry
- **JSON Schema** — artifact/schema validation
- **Native Git CLI** — initially shell out using `git -C <repo> ...`
- **Native build tools** — Maven, Gradle, npm, Go, Perl tooling, etc.
- **ripgrep/filesystem scanning** — initial repository search
- **Tree-sitter later** — only if richer symbol analysis proves necessary

Do not make Cursor APIs, embeddings, an IDE extension, or an LLM API dependency of V1.

## 3. Responsibility Boundary

```text
Cursor/Copilot
    -> semantic understanding
    -> feature decomposition
    -> architecture reasoning
    -> implementation
    -> complex failure analysis
    -> selective review

feature-dev
    -> workspace/repository discovery
    -> task state
    -> DAG
    -> task readiness
    -> Git
    -> command execution
    -> verification mechanics
    -> fingerprints
    -> reconciliation
    -> deterministic scheduling
```

The guiding rule is:

> **Cursor reasons. `feature-dev` controls.**

Do not duplicate the IDE agent's intelligence inside the Go engine.

## 4. Architecture

```text
Reusable feature-dev Repository
            |
        go install
            |
            v
     feature-dev binary
            |
            v
   Cursor/Copilot Workspace
            |
     +------+-------+
     |              |
   PLAN.md       .feature/
                    |
             workspace state
                    |
          +---------+---------+
          |                   |
       repo-a               repo-b
        .git                 .git
```

The orchestrator source does not need to be copied into target workspaces.

## 5. Suggested Source Layout

```text
feature-dev/
├── cmd/
│   └── feature-dev/
│       └── main.go
├── internal/
│   ├── workspace/
│   ├── repository/
│   ├── task/
│   ├── dag/
│   ├── state/
│   ├── git/
│   ├── verify/
│   ├── context/
│   ├── reconcile/
│   ├── scheduler/
│   └── config/
├── instructions/
│   ├── ORCHESTRATOR.md
│   ├── discovery.md
│   ├── planning.md
│   ├── execution.md
│   ├── verification.md
│   └── recovery.md
├── schemas/
│   ├── workspace.schema.json
│   ├── repository.schema.json
│   ├── task.schema.json
│   ├── workflow.schema.json
│   └── result.schema.json
├── templates/
├── testdata/
├── go.mod
└── README.md
```

## 6. Runtime Workspace Layout

```text
workspace/
├── PLAN.md
├── .feature/
│   ├── config.yaml
│   ├── repositories.json
│   ├── workflow.yaml
│   ├── tasks/
│   ├── state/
│   ├── context/
│   │   ├── workspace.md
│   │   ├── repo-a/
│   │   └── repo-b/
│   ├── artifacts/
│   ├── reviews/
│   └── logs/
├── repo-a/
└── repo-b/
```

For a single repository, the repository root is simply the workspace root.

## 7. MVP CLI

Start with:

```text
feature-dev init
feature-dev discover
feature-dev repositories
feature-dev status
feature-dev graph
feature-dev ready
feature-dev start T001
feature-dev context T001
feature-dev verify T001
feature-dev complete T001
feature-dev fail T001
feature-dev reconcile
feature-dev verify-workspace
feature-dev doctor
feature-dev version
```

Support `--json` on commands used heavily by the agent.

Avoid a giant autonomous `feature-dev run` command initially.

## 8. Cursor Execution Loop

```text
feature-dev ready
        |
        v
 T003 [devices]
        |
        v
feature-dev context T003
        |
        v
Cursor implements T003
        |
        v
feature-dev verify T003
        |
   +----+----+
 FAIL       PASS
   |          |
repair     feature-dev complete T003
              |
              v
       feature-dev ready
```

This loop should work even after starting a fresh Cursor session.

## 9. Phase 1 — CLI and Workspace Bootstrap

Implement:

```text
feature-dev version
feature-dev doctor
feature-dev init
```

`init` should:

1. resolve the workspace root
2. create `.feature/`
3. create default configuration
4. initialize the repository registry
5. create state/context/artifact directories
6. be safe to run repeatedly

Tests should cover existing and new workspaces.

## 10. Phase 2 — Repository Discovery

Implement:

```text
feature-dev discover
feature-dev repositories
```

Flow:

```text
workspace root
   ↓
find Git roots
   ↓
normalize paths
   ↓
apply include/exclude rules
   ↓
assign stable repository IDs
   ↓
persist repositories.json
```

Support:

```text
auto
explicit
hybrid
```

discovery modes.

Test single repo, sibling repos, nested repos, ignored directories, removed repos, and renamed paths.

## 11. Phase 3 — Task Contract

Define the schema before implementing planning.

```yaml
schema_version: "1.0"

id: T003
title: Implement transformation
repository: devices
working_directory: ./devices
status: PLANNED

goal: >
  Implement the required transformation.

dependencies:
  - T001

expected_files: []
context_refs: []
acceptance_criteria: []

verification:
  commands: []
```

Every implementation task has one primary repository.

Workspace-level coordination tasks use:

```yaml
repository: workspace
```

## 12. Phase 4 — State Engine

Implement deterministic transitions:

```text
PLANNED
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
RUNNING -> BLOCKED
VERIFYING -> REWORK
VERIFYING -> FAILED
```

Reject illegal transitions and persist state atomically.

## 13. Phase 5 — DAG Engine

Implement:

- graph construction
- dependency validation
- missing dependency detection
- cycle detection
- ready-task calculation
- cross-repository dependencies

Example:

```text
T001 [common]
     |
 +---+---+
 |       |
 v       v
T002    T003
[devices] [analytics]
```

`feature-dev ready --json` should return a deterministic structured result.

## 14. Phase 6 — Git Layer

Use native Git initially:

```bash
git -C ./devices status
git -C ./devices diff
git -C ./devices rev-parse HEAD
git -C ./devices branch --show-current
```

Capture per-repository:

- root
- branch
- HEAD
- changed files
- untracked files
- diff
- merge/rebase state

Never assume the workspace itself is one Git repository.

## 15. Phase 7 — Verification Engine

Detect repository build systems:

```text
pom.xml       -> Maven
build.gradle  -> Gradle
package.json  -> Node
go.mod        -> Go
Cargo.toml    -> Rust
```

Prefer wrappers such as `./mvnw` and `./gradlew`.

Every verification execution must carry:

```text
repository ID
working directory
command
timeout
exit code
output
```

Start with focused verification rather than full-workspace builds.

## 16. Phase 8 — First Cursor MVP

At this point, integrate Cursor through instructions rather than an API.

Create a small `ORCHESTRATOR.md` telling Cursor to:

1. inspect feature state
2. run `feature-dev reconcile` when resuming
3. run `feature-dev ready`
4. load the returned task
5. run `feature-dev start <task>`
6. implement inside the task repository
7. run `feature-dev verify <task>`
8. repair focused failures
9. run `feature-dev complete <task>`
10. repeat
11. run final workspace verification

This is the first useful product milestone.

## 17. Phase 9 — Planning

Do **not** initially put an LLM planner inside Go.

Use:

```text
PLAN.md
   ↓
Cursor analyzes workspace
   ↓
Cursor creates proposed tasks
   ↓
feature-dev validates task schemas
   ↓
feature-dev validates DAG
```

The Go engine owns validity; Cursor owns semantic decomposition.

Later `feature-dev plan` can provide deterministic preparation, validation, and persistence.

## 18. Phase 10 — Repository Context Cache

Do not start with embeddings.

Generate repository-specific caches:

```text
.feature/context/devices/
├── repository.md
├── architecture.md
├── conventions.md
├── symbols.json
└── fingerprints.json
```

Cache:

- modules
- source/test roots
- build system
- important packages
- architectural layers
- important symbols
- configuration
- conventions

Let Cursor create semantic summaries when useful; let Go manage storage and invalidation.

## 19. Phase 11 — Context References

Use repository-qualified references:

```text
repo.devices.architecture
repo.devices.symbol.DeviceService
repo.analytics.symbol.DeviceService
repo.common.contract.DeviceDto
task.T001.result
```

Never use unqualified symbol references in multi-repository workspaces.

Implement:

```text
feature-dev context T003
```

to produce a compact context manifest.

## 20. Phase 12 — Progressive Context

Expand context only when necessary:

```text
Level 0: task + requirements + dependency results
Level 1: relevant symbols
Level 2: relevant target-repository files
Level 3: adjacent tests/configuration
Level 4: cross-repository contract
Level 5: broader target repository
Level 6: broader workspace
```

Workspace-wide context should be the last resort.

## 21. Phase 13 — Result Artifacts

After completion persist:

```text
.feature/artifacts/T003-result.md
```

Include:

- repository
- status
- changed files
- concise implementation summary
- decisions
- verification evidence
- concerns

Future tasks should consume result artifacts rather than conversation history.

## 22. Phase 14 — Reconciliation

Implement:

```text
feature-dev reconcile
```

Compare persisted state against:

- repository registry
- Git branch
- Git HEAD
- current changes
- task state
- repository fingerprints
- contract fingerprints

Detect:

- repository removed
- branch changed
- external code changes
- interrupted RUNNING task
- reverted completed work
- stale verification
- stale dependency results

This is essential for session-independent operation.

## 23. Phase 15 — Failure and Rework

Classify failures:

```text
COMPILATION
TEST
STATIC_ANALYSIS
SCOPE_VIOLATION
MISSING_CONTEXT
CROSS_REPO_CONTRACT
REPOSITORY_STATE
ENVIRONMENT
UNKNOWN
```

Return structured evidence to Cursor.

Example:

```json
{
  "task": "T003",
  "repository": "devices",
  "type": "TEST",
  "exit_code": 1
}
```

Retry using focused failure context rather than resending the entire task/workspace.

Set a rework limit, for example three attempts.

## 24. Phase 16 — Cross-Repository Contracts

Introduce explicit contract artifacts only where useful:

```text
.feature/context/contracts/
├── DeviceDto.md
└── ReportingApi.md
```

Track:

- owning repository
- source files
- fingerprint
- consuming tasks/repositories
- compatibility constraints

If an upstream contract changes, mark downstream verification stale instead of blindly discarding downstream code.

## 25. Phase 17 — Workspace Verification

Implement:

```text
feature-dev verify-workspace
```

Determine affected repositories from the DAG.

Do not automatically test every repository.

Example:

```text
common changed
   ↓
devices depends on common
analytics depends on common
```

Verify only the affected dependency closure plus final feature-level acceptance criteria.

## 26. Phase 18 — Scheduling

First make sequential execution reliable.

Then add:

```text
dependency analysis
repository overlap
file/symbol overlap
contract dependencies
verification coupling
```

Classify tasks as:

```text
SERIALIZE
PARALLEL_CANDIDATE
```

The Go engine identifies safe candidates.

Cursor/Copilot decides how to use available agent capabilities.

Do not couple the engine to a Cursor multi-agent API.

## 27. Testing Strategy

### Unit tests

Cover:

- workspace path resolution
- repository IDs
- task state transitions
- DAG construction
- cycle detection
- ready-task calculation
- fingerprints
- config parsing
- verification detection

### Integration tests

Use real temporary Git repositories.

Fixtures:

```text
testdata/
├── single-java/
├── multi-java/
├── java-perl/
├── cross-contract/
├── nested-repositories/
├── dirty-repository/
└── broken-workspace/
```

### End-to-end

Simulate:

```text
init
discover
create tasks
ready
start
modify source
verify
complete
close/resume
reconcile
verify-workspace
```

Prefer real Git repositories over mocking everything.

## 28. Agent-Friendly Output

Human-readable output is default.

Important commands should support:

```text
--json
```

Example:

```bash
feature-dev ready --json
```

```json
{
  "ready": [
    {
      "task": "T003",
      "repository": "devices"
    }
  ]
}
```

This is more reliable than asking the agent to parse decorative terminal output.

## 29. Security

From the beginning:

- restrict operations to registered repository roots
- reject path traversal
- exclude secrets/credentials from context
- avoid destructive Git operations
- require confirmation for destructive commands
- validate verification commands before execution
- log significant executed commands
- never silently weaken tests or acceptance criteria

Repository-specific policies can extend workspace policy.

## 30. Versioning and Distribution

Expose:

```text
feature-dev version
```

Keep tool version and schema version separate.

Workspace configuration may contain:

```yaml
orchestrator:
  minimum_version: "0.1.0"
```

During development:

```bash
go install ./cmd/feature-dev
```

Later publish binaries for:

```text
darwin-arm64
darwin-amd64
linux-amd64
linux-arm64
```

An optional Homebrew/internal package distribution can come later.

## 31. Development Milestones

### M0 — Skeleton

```text
CLI
version
doctor
init
```

### M1 — Workspace

```text
repository discovery
registry
single/multi-repo support
```

### M2 — Workflow mechanics

```text
task schema
state engine
DAG
ready calculation
```

### M3 — Repository mechanics

```text
Git
verification detection
verification runner
```

### M4 — Cursor MVP

```text
orchestration instructions
Cursor-generated tasks
execute/verify/complete loop
```

**The system should already be useful here.**

### M5 — Context

```text
repository cache
qualified references
fingerprints
progressive context
```

### M6 — Recovery

```text
reconciliation
failure classification
rework
resume
```

### M7 — Cross-repository intelligence

```text
contracts
dependency invalidation
workspace verification
```

### M8 — Optimization

```text
conflict estimation
selective parallelism
metrics
context optimization
```

## 32. MVP Definition of Done

The first useful MVP should allow:

```text
1. Install feature-dev globally.
2. Open a Cursor workspace.
3. Run feature-dev init.
4. Discover one or more Git repositories.
5. Give Cursor PLAN.md.
6. Cursor creates task artifacts.
7. feature-dev validates the DAG.
8. feature-dev ready returns the next task.
9. Cursor implements it.
10. feature-dev verify runs in the correct repository.
11. feature-dev complete persists completion.
12. Cross-repository dependency ordering works.
13. Close Cursor.
14. Open a fresh session.
15. feature-dev reconcile restores reliable state.
16. Continue until the feature is complete.
```

The MVP does **not** need:

- automatic LLM planning inside Go
- parallel agents
- Tree-sitter
- embeddings/vector database
- IDE extension
- model routing
- cloud execution

## 33. Recommended First Sprint

Build only:

```text
feature-dev init
feature-dev discover
feature-dev repositories
feature-dev status
```

Packages:

```text
workspace
repository
config
state
```

Tests:

```text
single Git repo
two sibling Git repos
workspace-root Git repo
nested repo
ignored directories
existing .feature state
invalid workspace
```

This establishes the foundation everything else depends on.

## 34. Second Sprint

Implement:

```text
task model
state transitions
DAG
feature-dev graph
feature-dev ready
feature-dev start
feature-dev complete
```

Prove deterministic workflow mechanics before deeper Cursor integration.

## 35. Third Sprint

Implement:

```text
Git snapshots
repository-aware verification
feature-dev verify
result artifacts
```

Then test the orchestrator manually with Cursor on a real feature.

This is the first major architecture validation point.

## 36. Fourth Sprint

Add:

```text
ORCHESTRATOR.md
execution.md
verification.md
recovery.md
```

Teach Cursor to drive `feature-dev`.

The interaction becomes:

```text
Developer
   ↓
Cursor
   ↓
feature-dev
   ↓
workspace repositories
```

## 37. Fifth Sprint

Add:

```text
repository cache
context references
fingerprints
reconciliation
```

Measure whether these reduce repeated repository exploration before adding more sophisticated indexing.

## 38. Features to Delay

Do not prioritize initially:

```text
vector database
semantic embeddings
custom LLM API
model routing
cloud execution
distributed scheduler
web dashboard
agent marketplace
automatic merge management
large plugin framework
complex token accounting
```

Validate the deterministic orchestration loop first.

## 39. First Real Validation Scenario

Use a multi-repository feature:

```text
workspace/
├── common/
├── devices/
└── analytics/
```

DAG:

```text
T001 [common]
Add shared contract
      |
      +----------------+
      |                |
      v                v
T002 [devices]     T003 [analytics]
Consume contract   Consume contract
      |                |
      +-------+--------+
              v
       T004 [workspace]
       Final verification
```

Validate:

- repository discovery
- DAG
- context isolation
- cross-repository ordering
- repository-aware verification
- resume
- upstream contract modification
- downstream verification invalidation

If this works reliably, the most important architectural assumptions are proven.

## 40. Future IDE Extension

If richer UX becomes useful:

```text
Cursor / VS Code Extension
        TypeScript
            |
            v
      feature-dev Go CLI
            |
            v
      workspace state
```

The extension can display:

- task graph
- current task
- repository status
- verification results
- feature progress

Keep the Go engine authoritative.

## 41. Final Development Order

```text
CLI
 ↓
Workspace Discovery
 ↓
Repository Registry
 ↓
State Engine
 ↓
Task Schema
 ↓
DAG
 ↓
Git
 ↓
Verification
 ↓
Cursor Instructions
 ↓
Real-World MVP Validation
 ↓
Context Cache
 ↓
Resume/Reconciliation
 ↓
Cross-Repo Contracts
 ↓
Selective Parallelism
 ↓
Optional IDE UI
```

## 42. Core Implementation Principles

### Cursor reasons; `feature-dev` controls

Keep semantic intelligence in Cursor/Copilot and deterministic mechanics in the CLI.

### Conversation history is disposable

Authoritative state is:

```text
.feature/
+ repository Git state
+ repository source
```

### Context is repository-scoped

Use:

```text
workspace knowledge
+ target repository knowledge
+ explicit dependency artifacts
+ task-specific source
```

rather than the entire Cursor workspace.

### Verification determines completion

A task becomes DONE only when implementation exists, scope is acceptable, required checks pass, and acceptance criteria are satisfied.

### Sequential first, parallel later

Do not build parallel execution before DAG, state, Git, verification, and reconciliation are reliable.

---

# 43. Recommended Engineering Target

The first engineering target should be:

> **A reliable Go CLI that discovers one or many repositories in a Cursor workspace, maintains a workspace-level task DAG and durable state, tells Cursor what task is ready, verifies implementation in the correct repository, and can recover the workflow in a fresh Cursor session.**

Once that works reliably on real features, add richer context optimization, cross-repository contracts, and selective parallelism.
