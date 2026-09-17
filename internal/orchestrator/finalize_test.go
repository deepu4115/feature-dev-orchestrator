package orchestrator

import (
	"testing"
)

func TestMaybeAutoCompleteFeature(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	submitFromTasksWithBundle(t, workspaceRoot)
	_, _, _, _ = ApprovePlan(workspaceRoot, ApprovePlanOptions{})
	tasks, _ := LoadTasks(workspaceRoot)
	tasks[0].Status = StatusDone
	_ = SaveTasks(workspaceRoot, tasks)
	_ = SyncWorkflowFromTasks(workspaceRoot)
	result, err := MaybeAutoCompleteFeature(workspaceRoot, 60)
	if err != nil {
		t.Fatalf("MaybeAutoCompleteFeature: %v", err)
	}
	if !result.Completed || result.WorkflowStatus != WorkflowCompleted {
		t.Fatalf("expected COMPLETED, got %#v", result)
	}
}

func TestE2E_FullLifecycleThroughCompleted(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	submitFromTasksWithBundle(t, workspaceRoot)
	if _, _, _, err := ApprovePlan(workspaceRoot, ApprovePlanOptions{}); err != nil {
		t.Fatalf("ApprovePlan: %v", err)
	}
	tasks, _ := LoadTasks(workspaceRoot)
	tasks[0].Status = StatusDone
	if err := SaveTasks(workspaceRoot, tasks); err != nil {
		t.Fatalf("SaveTasks: %v", err)
	}
	result, err := MaybeAutoCompleteFeature(workspaceRoot, 60)
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}
	if !result.Completed {
		t.Fatalf("expected completed feature, got %#v", result)
	}
	ws, _ := LoadWorkflowState(workspaceRoot)
	if ws.WorkflowStatus != WorkflowCompleted {
		t.Fatalf("expected COMPLETED workflow, got %s", ws.WorkflowStatus)
	}
}
