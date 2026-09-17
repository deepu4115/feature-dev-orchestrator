package orchestrator

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStatus_CompletedFieldsAligned(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	submitFromTasksWithBundle(t, workspaceRoot)
	_, _, _, _ = ApprovePlan(workspaceRoot, ApprovePlanOptions{})
	tasks, _ := LoadTasks(workspaceRoot)
	tasks[0].Status = StatusDone
	_ = SaveTasks(workspaceRoot, tasks)
	result, err := MaybeAutoCompleteFeature(workspaceRoot, 60)
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}
	status, err := BuildWorkspaceStatus(workspaceRoot)
	if err != nil {
		t.Fatalf("BuildWorkspaceStatus: %v", err)
	}
	if !result.Completed || !status.Completed {
		t.Fatalf("expected completed=true, got result=%#v status=%#v", result, status)
	}
	if !status.CompletionReady || status.ApprovalRequired {
		t.Fatalf("expected completion_ready=true approval_required=false, got %#v", status)
	}
	if status.Finalization != FinalizationPassed {
		t.Fatalf("expected finalization PASSED, got %s", status.Finalization)
	}
}

func TestAgentHint_CompletedIgnoresStaleValidation(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	submitFromTasksWithBundle(t, workspaceRoot)
	_, _, _, _ = ApprovePlan(workspaceRoot, ApprovePlanOptions{})
	tasks, _ := LoadTasks(workspaceRoot)
	tasks[0].Status = StatusDone
	_ = SaveTasks(workspaceRoot, tasks)
	_, _ = MaybeAutoCompleteFeature(workspaceRoot, 60)

	invalid := PlanValidationReport{
		Valid: false,
		Errors: []PlanValidationIssue{{
			Level: "error", Code: "invalid_verification_type", Message: "stale",
		}},
	}
	_ = WriteLastValidationReport(workspaceRoot, invalid)

	hint, err := BuildAgentHint(workspaceRoot)
	if err != nil {
		t.Fatalf("BuildAgentHint: %v", err)
	}
	if hint.Reason != "feature_completed" {
		t.Fatalf("expected feature_completed, got %s", hint.Reason)
	}
}

func TestValidateTaskPlan_WarnsRedundantCdPrefix(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	tasks := []Task{{
		ID: "T001", Title: "Svc", Repository: "repo-a", Status: StatusReviewPending,
		Verification: []VerificationStep{{Command: "cd repo-a && mvn test"}},
	}}
	report, err := ValidateTaskPlan(workspaceRoot, tasks, false)
	if err != nil {
		t.Fatalf("ValidateTaskPlan: %v", err)
	}
	if !hasCode(report.Warnings, "redundant_repository_cd_prefix") {
		t.Fatalf("expected redundant_repository_cd_prefix warning, got %#v", report.Warnings)
	}
}

func TestPromoteDependencyReadyTasks_AfterT001Done(t *testing.T) {
	tasks := []Task{
		{ID: "T001", Status: StatusDone},
		{ID: "T002", Status: StatusReviewPending, Dependencies: []string{"T001"}},
	}
	unlocked, err := PromoteDependencyReadyTasks(tasks)
	if err != nil {
		t.Fatalf("PromoteDependencyReadyTasks: %v", err)
	}
	if len(unlocked) != 1 || unlocked[0] != "T002" {
		t.Fatalf("expected T002 unlocked, got %#v", unlocked)
	}
	if tasks[1].Status != StatusReady {
		t.Fatalf("expected T002 READY, got %s", tasks[1].Status)
	}
}

func TestRunPlanCoverage_MissingRequirement(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	planPath := filepath.Join(workspaceRoot, "PLAN.md")
	plan := `## Requirements
- Multiple address management including add, edit, primary selection
`
	if err := os.WriteFile(planPath, []byte(plan), 0644); err != nil {
		t.Fatalf("write plan: %v", err)
	}
	_, _ = InitPlanningDraftFromTemplates(workspaceRoot, true)
	_, _ = InitTasksFromTemplate(workspaceRoot, true)
	_ = os.WriteFile(PlanDraftRequirementsPath(workspaceRoot), []byte(`{"requirements":[]}`+"\n"), 0644)
	tasks, _ := LoadTasks(workspaceRoot)
	for i := range tasks {
		tasks[i].RequirementIDs = nil
	}
	_ = SaveTasks(workspaceRoot, tasks)

	report, err := RunPlanCoverage(workspaceRoot, "PLAN.md")
	if err != nil {
		t.Fatalf("RunPlanCoverage: %v", err)
	}
	if report.Valid || report.Checks["completeness"] != "FAIL" {
		t.Fatalf("expected completeness FAIL, got %#v", report)
	}
	if len(report.UnmappedSourceItems) == 0 {
		t.Fatal("expected unmapped source items")
	}
}

func TestRunPlanCoverage_DecompositionHints(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	planPath := filepath.Join(workspaceRoot, "PLAN.md")
	plan := `## Additional Business Requirements
- Multiple address management including add, edit, primary selection, and switching
`
	if err := os.WriteFile(planPath, []byte(plan), 0644); err != nil {
		t.Fatalf("write plan: %v", err)
	}
	report, err := RunPlanCoverage(workspaceRoot, "PLAN.md")
	if err != nil {
		t.Fatalf("RunPlanCoverage: %v", err)
	}
	if len(report.DecompositionHints) == 0 {
		t.Fatalf("expected decomposition hints, got %#v", report)
	}
}

func TestExplainTask_BlockedByDependency(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	submitFromTasksWithBundle(t, workspaceRoot)
	_, _, _, _ = ApprovePlan(workspaceRoot, ApprovePlanOptions{})
	result, err := ExplainTask(workspaceRoot, "T001")
	if err != nil {
		t.Fatalf("ExplainTask: %v", err)
	}
	if !result.Ready {
		t.Fatalf("expected T001 ready after approve, got %#v", result)
	}
}

func TestNormalizeVerificationCommand_StripsCdPrefix(t *testing.T) {
	task := Task{Repository: "user-service", VerificationWorkingDirectory: VerificationCWDRepositoryRoot}
	normalized := NormalizeVerificationCommand(task, "cd user-service && mvn test")
	if normalized != "mvn test" {
		t.Fatalf("expected mvn test, got %s", normalized)
	}
}
