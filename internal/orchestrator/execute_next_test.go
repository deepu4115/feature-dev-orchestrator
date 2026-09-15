package orchestrator

import (
	"os"
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

func TestExecuteNext_LegacyBypassStillStarts(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	tasks := []Task{{ID: "T001", Title: "A", Repository: "repo-a", Status: StatusPlanned}}
	if err := SaveTasks(workspaceRoot, tasks); err != nil {
		t.Fatalf("SaveTasks: %v", err)
	}
	result, err := ExecuteNext(workspaceRoot, ExecuteNextOptions{ContextLevel: "brief"})
	if err != nil {
		t.Fatalf("ExecuteNext: %v", err)
	}
	if result.Action != "start" {
		t.Fatalf("expected start, got %s", result.Action)
	}
}

func TestExecuteNext_DeniedBeforeApproval(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	submitFixturePlan(t, workspaceRoot, "valid-minimal")
	tasks := []Task{{ID: "T001", Title: "A", Repository: "repo-a", Status: StatusReviewPending}}
	_ = SaveTasks(workspaceRoot, tasks)
	_, err := ExecuteNext(workspaceRoot, ExecuteNextOptions{})
	if err == nil {
		t.Fatal("expected plan not approved error")
	}
	loaded, _ := LoadTasks(workspaceRoot)
	if loaded[0].Status == StatusRunning {
		t.Fatal("task should not be RUNNING")
	}
}

func TestExecuteNext_StartsAfterApproval(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	submitFixturePlan(t, workspaceRoot, "valid-minimal")
	if _, _, _, err := ApprovePlan(workspaceRoot, ApprovePlanOptions{}); err != nil {
		t.Fatalf("ApprovePlan: %v", err)
	}
	result, err := ExecuteNext(workspaceRoot, ExecuteNextOptions{ContextLevel: "brief"})
	if err != nil {
		t.Fatalf("ExecuteNext: %v", err)
	}
	if result.Action != "start" {
		t.Fatalf("expected start, got %s", result.Action)
	}
}

func TestExecuteLoop_StoppedReasonPlanNotApproved(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	submitFixturePlan(t, workspaceRoot, "valid-minimal")
	loop, err := ExecuteLoop(workspaceRoot, ExecuteLoopOptions{ExecuteNextOptions: ExecuteNextOptions{}})
	if err == nil {
		t.Fatal("expected error")
	}
	if loop.StoppedReason != "plan_not_approved" {
		t.Fatalf("expected plan_not_approved, got %s", loop.StoppedReason)
	}
}
