package orchestrator

import (
	"testing"
)

func TestRunTraceabilityCheck_PassWhenAllDone(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	submitFromTasksWithBundle(t, workspaceRoot)
	_, _, _, _ = ApprovePlan(workspaceRoot, ApprovePlanOptions{})
	tasks, _ := LoadTasks(workspaceRoot)
	tasks[0].Status = StatusDone
	_ = SaveTasks(workspaceRoot, tasks)
	report, err := RunTraceabilityCheck(workspaceRoot)
	if err != nil {
		t.Fatalf("RunTraceabilityCheck: %v", err)
	}
	if !report.Valid || report.Checks["traceability"] != "PASS" {
		t.Fatalf("expected PASS traceability, got %#v", report)
	}
}

func TestRunTraceabilityCheck_FailsWhenTaskNotDone(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	submitFromTasksWithBundle(t, workspaceRoot)
	_, _, _, _ = ApprovePlan(workspaceRoot, ApprovePlanOptions{})
	report, err := RunTraceabilityCheck(workspaceRoot)
	if err != nil {
		t.Fatalf("RunTraceabilityCheck: %v", err)
	}
	if report.Valid {
		t.Fatal("expected traceability failure when task not DONE")
	}
	if !hasCode(report.Errors, "task_not_done") {
		t.Fatalf("expected task_not_done, got %#v", report.Errors)
	}
}

func TestRunCrossRepoVerification_SkipsWhenNoCommands(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	submitFromTasksWithBundle(t, workspaceRoot)
	report, err := RunCrossRepoVerification(workspaceRoot, 30)
	if err != nil {
		t.Fatalf("RunCrossRepoVerification: %v", err)
	}
	if !report.Valid || report.Checks["cross_repo_verify"] != "SKIP" {
		t.Fatalf("expected SKIP, got %#v", report)
	}
}
