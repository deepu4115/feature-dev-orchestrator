package orchestrator

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestExecuteNextStartsReadyTask(t *testing.T) {
	workspaceRoot := t.TempDir()
	if err := InitWorkspace(workspaceRoot); err != nil {
		t.Fatalf("InitWorkspace returned error: %v", err)
	}

	repoPath := filepath.Join(workspaceRoot, "repo-a")
	if err := initGitRepo(repoPath); err != nil {
		t.Fatalf("init repo: %v", err)
	}

	tasks := []Task{
		{ID: "T001", Title: "A", Repository: "repo-a", Status: StatusPlanned},
	}
	if err := SaveTasks(workspaceRoot, tasks); err != nil {
		t.Fatalf("SaveTasks returned error: %v", err)
	}

	result, err := ExecuteNext(workspaceRoot, ExecuteNextOptions{ContextLevel: "brief"})
	if err != nil {
		t.Fatalf("ExecuteNext returned error: %v", err)
	}
	if result.Action != "start" {
		t.Fatalf("expected action start, got %s", result.Action)
	}
	if result.StatusAfter != StatusRunning {
		t.Fatalf("expected status RUNNING, got %s", result.StatusAfter)
	}
	if result.Context == nil {
		t.Fatal("expected context payload")
	}
}

func TestExecuteNextDryRunDoesNotMutateState(t *testing.T) {
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

	result, err := ExecuteNext(workspaceRoot, ExecuteNextOptions{DryRun: true})
	if err != nil {
		t.Fatalf("ExecuteNext returned error: %v", err)
	}
	if result.Action != "start" {
		t.Fatalf("expected action start, got %s", result.Action)
	}

	loaded, err := LoadTasks(workspaceRoot)
	if err != nil {
		t.Fatalf("LoadTasks returned error: %v", err)
	}
	if loaded[0].Status != StatusPlanned {
		t.Fatalf("expected status PLANNED to remain unchanged, got %s", loaded[0].Status)
	}
}

func TestExecuteNextVerifiesAndCompletesTask(t *testing.T) {
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
	if err := os.WriteFile(filepath.Join(repoPath, "repo_test.go"), []byte("package main\n\nimport \"testing\"\n\nfunc TestPass(t *testing.T) {}\n"), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	tasks := []Task{{
		ID:         "T100",
		Repository: "repo-a",
		Status:     StatusImplemented,
		Verification: []VerificationStep{{
			Command: "go test ./...",
		}},
	}}
	if err := SaveTasks(workspaceRoot, tasks); err != nil {
		t.Fatalf("SaveTasks returned error: %v", err)
	}

	result, err := ExecuteNext(workspaceRoot, ExecuteNextOptions{ContextLevel: "brief", Timeout: 30})
	if err != nil {
		t.Fatalf("ExecuteNext returned error: %v", err)
	}
	if result.Action != "verify" {
		t.Fatalf("expected action verify, got %s", result.Action)
	}
	if result.StatusAfter != StatusDone {
		t.Fatalf("expected status DONE, got %s", result.StatusAfter)
	}
	if result.Verification == nil || result.Verification.ExitCode != 0 {
		t.Fatalf("expected successful verification, got %#v", result.Verification)
	}
}

func TestExecuteNextVerificationFailureReturnsStructuredOutcome(t *testing.T) {
	workspaceRoot := t.TempDir()
	if err := InitWorkspace(workspaceRoot); err != nil {
		t.Fatalf("InitWorkspace returned error: %v", err)
	}

	repoPath := filepath.Join(workspaceRoot, "repo-a")
	if err := initGitRepo(repoPath); err != nil {
		t.Fatalf("init repo: %v", err)
	}

	tasks := []Task{{
		ID:         "T200",
		Repository: "repo-a",
		Status:     StatusImplemented,
		Verification: []VerificationStep{{
			Command: "false",
		}},
	}}
	if err := SaveTasks(workspaceRoot, tasks); err != nil {
		t.Fatalf("SaveTasks returned error: %v", err)
	}

	result, err := ExecuteNext(workspaceRoot, ExecuteNextOptions{Timeout: 5})
	if err != nil {
		t.Fatalf("expected structured failure outcome without command error, got: %v", err)
	}
	if result.Action != "verify_failed" {
		t.Fatalf("expected action verify_failed, got %s", result.Action)
	}
	if result.StatusAfter != StatusRework {
		t.Fatalf("expected status REWORK, got %s", result.StatusAfter)
	}
}

func initGitRepo(repoPath string) error {
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		return err
	}
	cmd := exec.Command("git", "init")
	cmd.Dir = repoPath
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git init failed: %w: %s", err, string(out))
	}
	return nil
}
