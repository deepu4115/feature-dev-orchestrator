# Feature Dev Orchestrator — Implementation Plan

## 1. Objective

Build a reusable workspace-native orchestration engine named feature-dev, installed once and usable from any Cursor/Copilot workspace.

It must support both single-repository and multi-repository workspaces with:

- workspace-level feature planning and state
- automatic Git repository discovery
- repository-scoped execution and context
- cross-repository task dependencies
- repository-aware Git and verification
- resume across IDE-agent sessions
- deterministic orchestration mechanics
- Cursor/Copilot as the semantic reasoning and coding layer

> Workspace = orchestration boundary. Repository = execution/context/Git/verification boundary.

## 2. Recommended Stack

Use Go for the core CLI and deterministic engine.

Use:

- Cobra — CLI command structure
- Markdown — PLAN.md, tasks, results, reviews, and agent instructions
- YAML — human-editable configuration
- JSON — machine-managed workflow state and repository registry
- JSON Schema — artifact/schema validation
- Native Git CLI — initially shell out using git -C <repo> ...
- Native build tools — Maven, Gradle, npm, Go, Perl, etc.
- ripgrep/filesystem scanning — initial repository search
- Tree-sitter later — only if richer symbol analysis proves necessary

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

> Cursor reasons. feature-dev controls.

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

Support --json on commands used heavily by the agent.

Avoid a giant autonomous feature-dev run command initially.

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

init should:

1. resolve the workspace root
2. create .feature/
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

feature-dev ready --json should return a deterministic structured result.

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

Prefer wrappers such as ./mvnw and ./gradlew.

Every verification execution must carry:

```text
repository ID
working directory
command
timeout
```

## 16. Phase 8 — Reconciliation and Resume

Add a reconciliation loop to restore consistency when:

- repositories move
- repos disappear
- tasks are stale
- verification was interrupted
- state is partially written

The orchestrator should be able to resume from saved state in a later IDE session without re-deriving the feature from scratch.

## 17. Phase 9 — Context Engine

The orchestrator should retain a narrow context model:

- task goal
- repo metadata
- file list relevant to task
- acceptance criteria
- verification commands
- previous result summaries

Use repository-scoped context injection rather than full workspace context dumping.

## 18. Phase 10 — Hardening and Documentation

Add:

- README and usage docs
- instructions for Cursor/Copilot prompts and checks
- examples for multi-repo workspaces
- troubleshooting and recovery flows
- schema and example state files

The result should be understandable to both agents and developers.

## 19. V1 Scope Summary

### In scope

- Cursor/Copilot-native workflow
- single-repository workspaces
- multi-repository workspaces
- automatic repository discovery
- explicit repository registry
- workspace-level PLAN.md
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

### Out of scope for V1

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

## 20. Product Positioning

Do not position this as another AI coding agent.

Position it as:

> A workspace-native orchestration system that makes Cursor/Copilot develop features across one or many repositories with less context, less rework, and stronger verification.

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

> Use intelligence only where intelligence is needed.

## 21. Implementation Ordering

Recommended rollout order:

1. CLI foundation + workspace init
2. repository discovery + registry
3. task contract + state machine
4. DAG scheduling
5. Git layer
6. verification engine
7. reconciliation and resume
8. context optimization
9. documentation and polish

This sequence keeps the deterministic core stable before adding orchestration complexity.

## 22. Example Workflow

```text
feature-dev init
feature-dev discover
feature-dev repositories
feature-dev status
feature-dev ready
feature-dev context T003
# Cursor implements the task
feature-dev verify T003
feature-dev complete T003
feature-dev reconcile
```

The agent can resume this flow after a fresh session and continue from saved state.

## 23. Final Guidance

The most important design rule is:

> Workspace = orchestration boundary.
> Repository = execution and verification boundary.

This is the foundation that keeps the system reliable, resumable, and safe for multi-repository feature work.
