# Feature Dev Orchestrator

Feature Dev Orchestrator is a workspace-native command-line tool designed to help development teams and AI coding agents manage feature work across one or many Git repositories in a structured, deterministic way.

It is especially useful when:

- your project lives across multiple repositories
- you want a clean workspace-level workflow for feature planning and execution
- you want to track which repo owns which task
- you want to reduce context confusion between repos
- you want state to survive across IDE sessions and agent restarts
- you want deterministic orchestration rather than ad hoc coding flow

## What this tool provides

This tool gives you a simple operational model:

- Workspace = the orchestration boundary
- Repository = the execution and verification boundary
- Cursor/Copilot = the reasoning and implementation layer
- feature-dev = the workflow control layer

In practical terms, the tool helps you:

- initialize a feature workspace
- discover Git repositories in the workspace
- keep repo information in a structured registry
- understand the current workspace state
- prepare for task planning and readiness checks
- keep future orchestration logic deterministic and reusable

## Why this tool exists

AI-assisted development can get messy when:

- the agent keeps re-discovering repo structure
- tasks are spread across repos without dependency tracking
- the wrong repo is used for verification or execution
- context is too large or too mixed across unrelated repos
- a session ends and the state is lost

Feature Dev Orchestrator fixes these problems by separating the responsibilities clearly:

- the AI agent reasons about code and implementation
- the tool handles workspace setup, repository discovery, state, and execution coordination

This makes the development workflow more reliable and easier to resume.

## How this is better than normal prompting in multi-repo workspaces

In a single repository, normal prompting can often be enough. In multi-repo workspaces, the same approach becomes fragile because context, execution location, and task ordering can drift between sessions.

Feature Dev Orchestrator improves this by adding deterministic coordination around the agent:

- clear boundaries: workspace for orchestration, repository for execution and verification
- durable state: task and repository state is persisted under `.feature` across restarts
- dependency-aware sequencing: tasks become ready only when dependencies are actually satisfied
- repository-aware verification: checks run in the correct repository with tracked outputs
- reconciliation support: stale or interrupted states are repaired before continuing
- lower cognitive load: less repeated prompting about repo ownership, status, and next steps
- consistent team workflow: shared commands create repeatable behavior across contributors

Practical takeaway: normal prompting is flexible but easy to derail in multi-repo feature work. This tool keeps prompting focused on reasoning and coding, while orchestration mechanics remain stable and resumable.

## How it works at a high level

The tool is intentionally simple and deterministic.

1. You run `feature-dev init` in a workspace.
2. It creates a `.feature` folder for orchestration metadata.
3. It discovers Git repositories under the workspace.
4. It stores repository information in a structured registry.
5. Future orchestration commands use that registry to determine the next task or repo context.
6. The result is a more controlled workflow for feature execution and resume.

The tool does not replace the coding agent. Instead, it helps the agent work inside a structured feature workflow.

## Project architecture

The following structure reflects the design:

```text
feature-dev-orchestrator/
├── cmd/
│   └── feature-dev/
│       └── main.go
├── internal/
│   └── orchestrator/
│       ├── root.go
│       ├── types.go
│       ├── workspace.go
│       ├── filesystem.go
│       ├── yaml.go
│       └── workspace_test.go
├── .feature/
├── README.md
├── go.mod
├── .gitignore
└── ...
```

## Key concepts

### 1. Workspace
A workspace is the folder opened in your editor or terminal for a feature. It is the coordination boundary for planning and execution.

### 2. Repository
A repository is a Git project inside the workspace. Each repo is treated as its own execution, context, Git, and verification boundary.

### 3. Feature state
The tool keeps orchestration metadata in `.feature`, which is reserved for workflow and state artifacts.

### 4. Deterministic behavior
The tool is designed to behave consistently and predictably, instead of depending on hidden agent state or ad hoc assumptions.

## Installation

### Prerequisites

You need:

- Go installed on your machine
- a terminal
- a workspace folder

### Build the CLI

From the project root:

```bash
go build ./cmd/feature-dev
```

This creates a binary named `feature-dev` in the current directory, or in the generated output path depending on your environment.

### Run it directly

```bash
./feature-dev --help
```

If you want to run it globally:

```bash
go install ./cmd/feature-dev
```

Go installs the binary into `$(go env GOPATH)/bin` unless `GOBIN` is set. Add
that directory to your shell startup files so the command works from every
workspace:

```bash
export PATH="$(go env GOPATH)/bin:$PATH"
```

For a permanent macOS setup, place the export in `.zshenv` and `.zshrc`. If
you use Bash, place it in `.bashrc` and source `.bashrc` from `.bash_profile`.
Then restart the terminal or reload the relevant startup file.

Verify the global installation:

```bash
command -v feature-dev
feature-dev version
feature-dev agent-hint --json
```

The optional helper alias below is convenient for agent routing:

```bash
alias feature-dev-agent='feature-dev agent-hint --json'
```

Then you can run it as:

```bash
feature-dev --help
```

## Beginner usage guide

The following steps show the simplest way to use the tool.

### Step 1: Open your workspace

Navigate to the folder you want to use as the orchestration workspace.

```bash
cd /path/to/your/workspace
```

### Step 2: Initialize the workspace

```bash
feature-dev init
```

What this does:

- creates the `.feature` directory
- creates default configuration
- creates repo registry storage
- creates state/context/artifact/log directories

You can verify it worked by listing the `.feature` directory:

```bash
ls -la .feature
```

### Step 3: Discover repos

```bash
feature-dev discover
```

This scans the workspace for Git repositories and records them in `.feature/repositories.json`.

You can view them with:

```bash
feature-dev repositories
```

### Step 4: Check workspace status

```bash
feature-dev status
```

This gives an overview of the workspace and helps confirm the setup is ready.

### Step 5: Check health

```bash
feature-dev doctor
```

This validates that the workspace is properly configured.

### Step 6: Create or import tasks

Make sure tasks exist in `.feature/tasks/tasks.json`.

If needed, add tasks manually:

```bash
feature-dev task add T001 "Implement feature slice"
feature-dev task add T002 "Add tests"
```

The `task add` command creates workspace tasks. For repository-scoped work,
edit `.feature/tasks/tasks.json` directly after adding the tasks so you can
provide the repository assignment, dependencies, description, and verification
command. The repository name must match an entry in
`.feature/repositories.json`.

### Step 7: Validate before execution

Run these checks before starting work:

```bash
feature-dev reconcile
feature-dev ready
feature-dev graph
```

What each command does:

- `reconcile`: repairs stale or interrupted task state
- `ready`: shows tasks eligible for execution
- `graph`: validates dependency relationships

### Step 8: Execute work (recommended autonomous mode)

For a single safe autonomous step:

```bash
feature-dev execute-next --json
```

For bounded multi-step autonomous execution:

```bash
feature-dev execute-loop --json
```

Use `execute-loop` as default and rerun it after each coding pass.

### Step 9: Handle stop reasons

When `execute-loop` stops, use this mapping:

- `awaiting_code_changes`: implement code changes, then rerun `feature-dev execute-loop --json`
- `verify_failure_budget_reached`: inspect verify output/artifacts, fix issues, then rerun loop
- `no_executable_task`: add/fix tasks or dependencies, run `reconcile`, rerun loop
- `max_steps_reached`: rerun loop or increase `--max-steps`

### Step 10: Optional manual mode

Use manual mode when you need explicit control over each transition:

```bash
feature-dev start T001
feature-dev context T001 --level brief --json
# implement code changes
feature-dev verify T001
feature-dev complete T001
```

If work fails:

```bash
feature-dev fail T001
feature-dev reconcile
```

## Example flow

Here is a typical autonomous flow:

```bash
cd my-feature-workspace
feature-dev init
feature-dev discover
feature-dev repositories
feature-dev status
feature-dev doctor
feature-dev reconcile
feature-dev ready
feature-dev graph
feature-dev execute-loop --json
```

This is the core orchestration loop used for dependency-aware execution and resume across sessions.

Rerun `feature-dev execute-loop --json` after each code-change pass until all planned tasks are complete.

## Using with GitHub Copilot or Cursor

The tool does not run inside Copilot or Cursor as an extension. It is a
workspace-level CLI that the coding agent runs from the integrated terminal.
The agent remains responsible for reasoning and editing code; `feature-dev`
keeps task selection, repository ownership, state transitions, and verification
deterministic.

Open the same workspace folder in VS Code with Copilot or in Cursor, then give
the agent a request like this:

```text
Use the feature-dev CLI as the orchestration layer for this feature.
Run `feature-dev doctor`, then `feature-dev reconcile` and
`feature-dev execute-loop --json`.

When the result says `awaiting_code_changes`, inspect the returned task and
repository, make the required changes only in that repository, run the
repository's tests, and run `feature-dev execute-loop --json` again.
When verification fails, inspect the verification output and artifacts, fix the
underlying issue, and rerun the loop. Continue until all tasks are DONE or
report the exact stop reason and task that needs input.
```

To implement a written plan, name it directly in the prompt:

```text
Use feature-dev command to Implement PLAN.md.
Read PLAN.md first, infer the repository-aware task graph and dependencies,
write .feature/tasks/tasks.json, validate it, and continue execute-loop cycles
until all tasks are DONE. Only stop for a genuine blocker.
```

A normal agent pass looks like this:

1. Run `feature-dev execute-loop --json` from the workspace root.
2. Read the JSON `status`, `task`, and `repository` fields.
3. If the status is `awaiting_code_changes`, open the assigned repository and
	implement that task. Do not assume the workspace root is the repository.
4. Run the repository's tests or the verification command configured for the
	task.
5. Run `feature-dev execute-loop --json` again so the orchestrator records the
	result and selects the next dependency-ready task.

Useful manual commands while working with an agent are:

```bash
feature-dev status --json
feature-dev repositories --json
feature-dev ready --json
feature-dev agent-hint --json
feature-dev context T001 --level brief --json
feature-dev execute-next --json
```

Use `execute-next` when you want to supervise one transition at a time. Use
`execute-loop` for the normal bounded workflow. The agent should treat a stop
reason such as `awaiting_code_changes`, `verify_failure_budget_reached`,
`no_executable_task`, or `max_steps_reached` as an instruction about what to do
next, rather than repeatedly invoking the command without inspecting the
result.

### Copilot in VS Code

1. Open the orchestration workspace in VS Code.
2. Open Copilot Chat or Agent mode with the integrated terminal available.
3. Paste the operating procedure above, or ask Copilot to use
	`feature-dev execute-loop --json` for the current feature.
4. Review the proposed edits and terminal commands. Copilot should edit the
	repository named by the CLI result, then return to the workspace root for
	the next orchestration command.

### Cursor

1. Open the orchestration workspace as the Cursor project.
2. Ensure the integrated terminal starts at the workspace root and the
	`feature-dev` binary is on `PATH`.
3. Give Cursor the same operating procedure and ask it to preserve the
	`.feature` state directory.
4. Let Cursor work on one returned task at a time, checking the JSON result
	after each coding and verification pass.

Do not ask the agent to invent task state by changing statuses manually unless
you are using the explicit manual commands. The CLI is the source of truth for
transitions and resume behavior.

## Commands overview

```bash
feature-dev version
feature-dev doctor
feature-dev init
feature-dev discover
feature-dev repositories
feature-dev status
feature-dev agent-hint
feature-dev graph
feature-dev ready
feature-dev start T001
feature-dev context T001
feature-dev verify T001
feature-dev complete T001
feature-dev fail T001
feature-dev reconcile
feature-dev verify-workspace
feature-dev execute-next --json
feature-dev execute-loop --json
```

### `feature-dev version`
Shows the current CLI version.

### `feature-dev doctor`
Checks whether the workspace is initialized and configured correctly.

### `feature-dev init`
Creates the workspace orchestration scaffolding.

### `feature-dev discover`
Scans for Git repositories and saves them to the registry.

### `feature-dev repositories`
Shows the discovered repository registry.

### `feature-dev status`
Shows basic workspace state information.

### `feature-dev agent-hint`
Emits a structured routing hint for coding agents, including suggested next command, prompt text, trigger phrases, and orchestration readiness signals.

Use JSON mode for agent parsing:

```bash
feature-dev agent-hint --json
```

### `feature-dev ready`
Checks if any tasks are ready for execution.

### `feature-dev graph`
Validates and prints task dependency edges.

### `feature-dev start <task-id>`
Transitions a ready task to `RUNNING`.

### `feature-dev context <task-id>`
Shows repository-aware Git context for the task.

### `feature-dev verify <task-id>`
Runs repository-scoped verification, writes a verification artifact to `.feature/artifacts`, and updates task state on failure.

### `feature-dev complete <task-id>`
Marks verified work as `DONE`.

### `feature-dev fail <task-id>`
Marks running or verifying work as `FAILED`.

### `feature-dev reconcile`
Repairs stale task state to support safe resume.

### `feature-dev verify-workspace`
Runs verification for all repository-scoped tasks in `IMPLEMENTED` or `VERIFYING` state.

### `feature-dev execute-next`
Runs one autonomous orchestration step with minimal manual intervention.

What it does in one command:

- reconciles persisted state
- chooses the next deterministic task (or uses `--task`)
- starts a ready task or verifies an implemented task
- marks verified tasks as `DONE` on pass
- returns structured output for the coding agent

Useful flags:

- `--json` for machine-readable output
- `--dry-run` to preview without changing state
- `--task T001` to force a specific task
- `--level brief|full` to control context size
- `--max-files`, `--max-chars`, `--max-file-chars` for context budgeting

Token-efficient defaults:

- `--level brief` minimizes context for fast orchestration decisions
- `--level full` adds budgeted snippets only when needed

### `feature-dev execute-loop`
Runs multiple autonomous steps in a bounded loop and stops safely when coding input is needed or failure budgets are reached.

What it adds over `execute-next`:

- bounded multi-step execution in a single command
- safe stop when task reaches `RUNNING` and needs implementation changes
- configurable verification failure budget to prevent runaway retries

Useful flags:

- `--max-steps` maximum number of autonomous steps per run (default 3)
- `--max-verify-failures` stop after repeated verify failures (default 2)
- also supports all context and output flags from `execute-next`

Practical use:

- use `--json` for agent-driven parsing
- combine `--max-steps` and `--max-verify-failures` to keep latency and retries bounded

When to execute which:

- `execute-next`: use for one deterministic step and tighter supervision
- `execute-loop`: use as the default low-intervention mode

## Current status of the project

This repository includes the core orchestration MVP and early hardening. It includes:

- workspace bootstrap
- repository discovery
- repository registry persistence
- task state machine and dependency readiness
- repository-aware context and verification
- context budgeting (`brief`/`full` + hard character/file caps)
- rolling task summaries in `.feature/state/task-summaries.jsonl`
- lifecycle commands (`start`, `complete`, `fail`)
- reconciliation for stale/incomplete sessions
- autonomous bounded execution commands (`execute-next`, `execute-loop`)
- tests for workspace, task transitions, DAG behavior, and reconciliation

The next implementation stages are planned to include:

- richer persisted verification history and summaries
- deeper reconcile handling for moved repositories and partial writes
- additional schema validation and policy checks

## Important behavior and boundaries

This tool is designed around a strict boundary:

- the workspace orchestrates the feature
- each repo is an execution boundary
- the AI agent reasons, while the tool coordinates

This prevents the orchestration logic from becoming too clever or too dependent on a specific IDE runtime.

## Typical uses

This tool is useful for:

- multi-repo feature work
- repo-aware engineering workflows
- AI-assisted feature development with clearer state
- planning and verifying work before deep code edits
- building a deterministic orchestration foundation for larger feature systems

## Common beginner mistakes

### Mistake 1: running commands before initialization
Always run:

```bash
feature-dev init
```

before using workspace-specific orchestration commands.

### Mistake 2: running verify before implementation state
`feature-dev verify <task-id>` expects the task to already be in `IMPLEMENTED` or `VERIFYING`.

### Mistake 3: assuming the workspace itself is a Git repo
The tool is designed for workspaces containing one or more Git repositories, not only a single top-level repo.

## Troubleshooting

### The command says the workspace is not initialized
Run:

```bash
feature-dev init
```

### No repositories are listed
Make sure your repositories are actual folders containing a `.git` directory.

### The tool is not recognized
Build it or install it globally:

```bash
go build ./cmd/feature-dev
```

or

```bash
go install ./cmd/feature-dev
```

## Future roadmap

The planned roadmap now focuses on hardening and scale:

1. deeper reconciliation for repository moves/disappearances
2. richer verification summaries and workspace-level reporting
3. schema-driven validation for task/workflow artifacts
4. improved examples for multi-repository onboarding

## Summary

Feature Dev Orchestrator is a simple but important foundation for workflow-driven software development. It helps you create a structured environment for AI-assisted feature work, especially when multiple repositories and long-running tasks are involved.

The current project is intentionally modular and can expand into a fuller orchestration engine without becoming IDE-specific or overly complex.

## Quick start

```bash
go build ./cmd/feature-dev
./feature-dev init
./feature-dev discover
./feature-dev repositories
./feature-dev status
./feature-dev doctor
./feature-dev execute-loop --json
```

That is the beginner-friendly path to getting started with the tool.
