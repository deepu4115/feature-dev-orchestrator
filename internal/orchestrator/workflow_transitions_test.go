package orchestrator

import (
	"testing"
)

func TestSyncWorkflowFromTasks_Executing(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	submitFromTasksWithBundle(t, workspaceRoot)
	_, _, _, _ = ApprovePlan(workspaceRoot, ApprovePlanOptions{})
	tasks, _ := LoadTasks(workspaceRoot)
	tasks[0].Status = StatusRunning
	_ = SaveTasks(workspaceRoot, tasks)
	_ = SyncWorkflowFromTasks(workspaceRoot)
	ws, _ := LoadWorkflowState(workspaceRoot)
	if ws.WorkflowStatus != WorkflowExecuting {
		t.Fatalf("expected EXECUTING, got %s", ws.WorkflowStatus)
	}
}

func TestSyncWorkflowFromTasks_VerifyingWhenAllDone(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	submitFromTasksWithBundle(t, workspaceRoot)
	_, _, _, _ = ApprovePlan(workspaceRoot, ApprovePlanOptions{})
	tasks, _ := LoadTasks(workspaceRoot)
	tasks[0].Status = StatusDone
	_ = SaveTasks(workspaceRoot, tasks)
	_ = SyncWorkflowFromTasks(workspaceRoot)
	ws, _ := LoadWorkflowState(workspaceRoot)
	if ws.WorkflowStatus != WorkflowVerifying {
		t.Fatalf("expected VERIFYING, got %s", ws.WorkflowStatus)
	}
}

func TestSyncWorkflowFromTasks_ReworkReturnsExecuting(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	submitFromTasksWithBundle(t, workspaceRoot)
	_, _, _, _ = ApprovePlan(workspaceRoot, ApprovePlanOptions{})
	tasks, _ := LoadTasks(workspaceRoot)
	tasks[0].Status = StatusRework
	_ = SaveTasks(workspaceRoot, tasks)
	_ = SyncWorkflowFromTasks(workspaceRoot)
	ws, _ := LoadWorkflowState(workspaceRoot)
	if ws.WorkflowStatus != WorkflowExecuting {
		t.Fatalf("expected EXECUTING after REWORK, got %s", ws.WorkflowStatus)
	}
}
