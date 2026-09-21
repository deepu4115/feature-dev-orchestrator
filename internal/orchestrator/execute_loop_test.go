package orchestrator

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExecuteLoopStopsWhenImplementationNeeded(t *testing.T) {
	workspaceRoot := t.TempDir()
	if err := InitWorkspace(workspaceRoot); err != nil {
		t.Fatalf("InitWorkspace returned error: %v", err)
	}

	repoPath := filepath.Join(workspaceRoot, "repo-a")
	if err := initGitRepo(repoPath); err != nil {
		t.Fatalf("init repo: %v", err)
	}

	tasks := []Task{{ID: "T001", Repository: "repo-a", Status: StatusPlanned}}
	if err := SaveTasks(workspaceRoot, tasks); err != nil {
		t.Fatalf("SaveTasks returned error: %v", err)
	}

	result, err := ExecuteLoop(workspaceRoot, ExecuteLoopOptions{MaxSteps: 4})
	if err != nil {
		t.Fatalf("ExecuteLoop returned error: %v", err)
	}
	if result.StoppedReason != "awaiting_code_changes" {
		t.Fatalf("expected awaiting_code_changes, got %s", result.StoppedReason)
	}
	if result.CompletedSteps != 1 {
		t.Fatalf("expected completed steps 1, got %d", result.CompletedSteps)
	}
}

func TestExecuteLoopRespectsVerifyFailureBudget(t *testing.T) {
	workspaceRoot := t.TempDir()
	if err := InitWorkspace(workspaceRoot); err != nil {
		t.Fatalf("InitWorkspace returned error: %v", err)
	}

	repoPath := filepath.Join(workspaceRoot, "repo-a")
	if err := initGitRepo(repoPath); err != nil {
		t.Fatalf("init repo: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, "go.mod"), []byte("module repo-a\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	tasks := []Task{{
		ID:         "T001",
		Repository: "repo-a",
		Status:     StatusImplemented,
		Verification: []VerificationStep{{
			Command: "false",
		}},
	}}
	if err := SaveTasks(workspaceRoot, tasks); err != nil {
		t.Fatalf("SaveTasks returned error: %v", err)
	}

	result, err := ExecuteLoop(workspaceRoot, ExecuteLoopOptions{MaxSteps: 3, MaxVerifyFailures: 1})
	if err != nil {
		t.Fatalf("ExecuteLoop returned error: %v", err)
	}
	if result.StoppedReason != "awaiting_rework" {
		t.Fatalf("expected awaiting_rework, got %s", result.StoppedReason)
	}
	if result.VerifyFailures != 1 {
		t.Fatalf("expected verify failures 1, got %d", result.VerifyFailures)
	}
}

func TestExecuteLoopDryRunLeavesTaskStateUntouched(t *testing.T) {
	workspaceRoot := t.TempDir()
	if err := InitWorkspace(workspaceRoot); err != nil {
		t.Fatalf("InitWorkspace returned error: %v", err)
	}

	repoPath := filepath.Join(workspaceRoot, "repo-a")
	if err := initGitRepo(repoPath); err != nil {
		t.Fatalf("init repo: %v", err)
	}

	tasks := []Task{{ID: "T001", Repository: "repo-a", Status: StatusPlanned}}
	if err := SaveTasks(workspaceRoot, tasks); err != nil {
		t.Fatalf("SaveTasks returned error: %v", err)
	}

	result, err := ExecuteLoop(workspaceRoot, ExecuteLoopOptions{ExecuteNextOptions: ExecuteNextOptions{DryRun: true}, MaxSteps: 2})
	if err != nil {
		t.Fatalf("ExecuteLoop returned error: %v", err)
	}
	if result.CompletedSteps != 1 {
		t.Fatalf("expected completed steps 1, got %d", result.CompletedSteps)
	}

	loaded, err := LoadTasks(workspaceRoot)
	if err != nil {
		t.Fatalf("LoadTasks returned error: %v", err)
	}
	if len(loaded) != 1 || loaded[0].Status != StatusPlanned {
		t.Fatalf("expected task to remain PLANNED, got %#v", loaded)
	}
}
