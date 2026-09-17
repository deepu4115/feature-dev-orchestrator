package orchestrator

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateTaskPlan_RejectsUnknownRepository(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	tasks := []Task{{
		ID: "T001", Title: "Bad repo", Repository: "unknown", Status: StatusReviewPending,
		Verification: []VerificationStep{{Command: "go test ./..."}},
	}}
	report, err := ValidateTaskPlan(workspaceRoot, tasks, true)
	if err != nil {
		t.Fatalf("ValidateTaskPlan: %v", err)
	}
	if report.Valid || !hasCode(report.Errors, "invalid_repository") {
		t.Fatalf("expected invalid_repository, got %#v", report.Errors)
	}
}

func TestValidateTaskPlan_RejectsMissingVerification(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	tasks := []Task{{ID: "T001", Title: "No verify", Repository: "repo-a", Status: StatusReviewPending}}
	report, _ := ValidateTaskPlan(workspaceRoot, tasks, true)
	if report.Valid || !hasCode(report.Errors, "missing_verification") {
		t.Fatalf("expected missing_verification, got %#v", report.Errors)
	}
}

func TestValidateTaskPlan_WarnsOnLowOwnershipConfidence(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	tasks := []Task{{
		ID: "T001", Title: "Low conf", Repository: "repo-a", OwnershipConfidence: "LOW", Status: StatusReviewPending,
		Verification: []VerificationStep{{Command: "go test ./..."}},
	}}
	report, _ := ValidateTaskPlan(workspaceRoot, tasks, true)
	if !hasCode(report.Warnings, "low_ownership_confidence") {
		t.Fatalf("expected low ownership warning, got %#v", report.Warnings)
	}
}

func TestValidateTaskPlan_ErrorsOnLowConfidenceCrossRepoDownstream(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	repos := []Repository{
		{ID: "repo-a", Path: "repo-a", GitRoot: "repo-a", Mode: "read_write"},
		{ID: "repo-b", Path: "repo-b", GitRoot: "repo-b", Mode: "read_write"},
	}
	if err := SaveRepositories(workspaceRoot, repos); err != nil {
		t.Fatalf("SaveRepositories: %v", err)
	}
	if err := initGitRepo(filepath.Join(workspaceRoot, "repo-b")); err != nil {
		t.Fatalf("initGitRepo repo-b: %v", err)
	}
	tasks := []Task{
		{ID: "T001", Title: "Shared", Repository: "repo-a", OwnershipConfidence: "LOW", Status: StatusReviewPending, Verification: []VerificationStep{{Command: "go test ./..."}}},
		{ID: "T002", Title: "Downstream", Repository: "repo-b", Dependencies: []string{"T001"}, Status: StatusReviewPending, Verification: []VerificationStep{{Command: "npm test"}}},
	}
	report, _ := ValidateTaskPlan(workspaceRoot, tasks, true)
	if report.Valid || !hasCode(report.Errors, "low_confidence_cross_repo_downstream") {
		t.Fatalf("expected low confidence cross-repo error, got %#v", report.Errors)
	}
}

func TestSubmitCopiesTasksSnapshot(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	submitFixturePlan(t, workspaceRoot, "valid-minimal")
	snapshotPath := PlanRevisionTasksPath(workspaceRoot, 1)
	if _, err := os.Stat(snapshotPath); err != nil {
		t.Fatalf("expected revision tasks snapshot: %v", err)
	}
}

func TestReviewJSONIncludesTasksFileAndGraph(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	submitFixturePlan(t, workspaceRoot, "valid-minimal")
	payload, _, _, err := ReviewPlan(workspaceRoot)
	if err != nil {
		t.Fatalf("ReviewPlan: %v", err)
	}
	if payload.TasksFile == "" {
		t.Fatal("expected tasks_file")
	}
	if payload.Graph == nil {
		t.Fatal("expected graph payload")
	}
}

func TestE2E_TasksJSONReviewBeforeApprove(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	submitFromTasksWithBundle(t, workspaceRoot)
	payload, _, _, err := ReviewPlan(workspaceRoot)
	if err != nil {
		t.Fatalf("ReviewPlan: %v", err)
	}
	if len(payload.TaskItems) != 1 {
		t.Fatalf("expected 1 task in review, got %d", len(payload.TaskItems))
	}
	if err := CanExecute(workspaceRoot); !errors.Is(err, ErrPlanNotApproved) {
		t.Fatalf("expected locked execution before approve")
	}
	if _, _, _, err := ApprovePlan(workspaceRoot, ApprovePlanOptions{}); err != nil {
		t.Fatalf("ApprovePlan: %v", err)
	}
	if err := CanExecute(workspaceRoot); err != nil {
		t.Fatalf("expected execution allowed after approve: %v", err)
	}
}

func TestTaskPreviewCommandPayload(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	tasks := []Task{{
		ID: "T001", Title: "Preview me", Repository: "repo-a", Status: StatusReviewPending,
		Verification: []VerificationStep{{Command: "go test ./..."}},
	}}
	if err := SaveTasks(workspaceRoot, tasks); err != nil {
		t.Fatalf("SaveTasks: %v", err)
	}
	payload, _, _, err := PreviewTaskPlan(workspaceRoot)
	if err != nil {
		t.Fatalf("PreviewTaskPlan: %v", err)
	}
	if len(payload.TaskItems) != 1 || payload.TaskItems[0].ID != "T001" {
		t.Fatalf("unexpected preview tasks: %#v", payload.TaskItems)
	}
}
