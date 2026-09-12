package orchestrator

import (
	"path/filepath"
	"testing"
)

func TestBuildAgentHintWorkspaceNotInitialized(t *testing.T) {
	workspaceRoot := t.TempDir()

	hint, err := BuildAgentHint(workspaceRoot)
	if err != nil {
		t.Fatalf("BuildAgentHint returned error: %v", err)
	}

	if hint.Reason != "workspace_not_initialized" {
		t.Fatalf("expected workspace_not_initialized, got %s", hint.Reason)
	}
	if hint.SuggestedNextCommand != "feature-dev init" {
		t.Fatalf("expected init suggestion, got %s", hint.SuggestedNextCommand)
	}
}

func TestBuildAgentHintRepositoriesNotDiscovered(t *testing.T) {
	workspaceRoot := t.TempDir()
	if err := InitWorkspace(workspaceRoot); err != nil {
		t.Fatalf("InitWorkspace returned error: %v", err)
	}

	hint, err := BuildAgentHint(workspaceRoot)
	if err != nil {
		t.Fatalf("BuildAgentHint returned error: %v", err)
	}

	if hint.Reason != "repositories_not_discovered" {
		t.Fatalf("expected repositories_not_discovered, got %s", hint.Reason)
	}
	if hint.SuggestedNextCommand != "feature-dev discover" {
		t.Fatalf("expected discover suggestion, got %s", hint.SuggestedNextCommand)
	}
}

func TestBuildAgentHintNoTasksDefined(t *testing.T) {
	workspaceRoot := t.TempDir()
	if err := InitWorkspace(workspaceRoot); err != nil {
		t.Fatalf("InitWorkspace returned error: %v", err)
	}

	repoPath := filepath.Join(workspaceRoot, "repo-a")
	if err := initGitRepo(repoPath); err != nil {
		t.Fatalf("init repo: %v", err)
	}
	if err := SaveRepositories(workspaceRoot, []Repository{{ID: "repo-a", Path: "repo-a", GitRoot: "repo-a", Mode: "read_write"}}); err != nil {
		t.Fatalf("SaveRepositories returned error: %v", err)
	}

	hint, err := BuildAgentHint(workspaceRoot)
	if err != nil {
		t.Fatalf("BuildAgentHint returned error: %v", err)
	}

	if hint.Reason != "no_tasks_defined" {
		t.Fatalf("expected no_tasks_defined, got %s", hint.Reason)
	}
}

func TestBuildAgentHintReadyForOrchestration(t *testing.T) {
	workspaceRoot := t.TempDir()
	if err := InitWorkspace(workspaceRoot); err != nil {
		t.Fatalf("InitWorkspace returned error: %v", err)
	}

	repoPath := filepath.Join(workspaceRoot, "repo-a")
	if err := initGitRepo(repoPath); err != nil {
		t.Fatalf("init repo: %v", err)
	}
	if err := SaveRepositories(workspaceRoot, []Repository{{ID: "repo-a", Path: "repo-a", GitRoot: "repo-a", Mode: "read_write"}}); err != nil {
		t.Fatalf("SaveRepositories returned error: %v", err)
	}

	tasks := []Task{{ID: "T001", Title: "A", Repository: "repo-a", Status: StatusPlanned}}
	if err := SaveTasks(workspaceRoot, tasks); err != nil {
		t.Fatalf("SaveTasks returned error: %v", err)
	}

	hint, err := BuildAgentHint(workspaceRoot)
	if err != nil {
		t.Fatalf("BuildAgentHint returned error: %v", err)
	}

	if hint.Reason != "ready_for_orchestration" {
		t.Fatalf("expected ready_for_orchestration, got %s", hint.Reason)
	}
	if hint.SuggestedNextCommand != "feature-dev reconcile && feature-dev execute-loop --json" {
		t.Fatalf("unexpected next command: %s", hint.SuggestedNextCommand)
	}
	if len(hint.ReadyTasks) != 1 || hint.ReadyTasks[0] != "T001" {
		t.Fatalf("unexpected ready tasks: %#v", hint.ReadyTasks)
	}
}
