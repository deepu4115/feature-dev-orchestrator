package orchestrator

import (
	"errors"
	"testing"
)

func TestAgentHint_ReviewPending(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	submitFixturePlan(t, workspaceRoot, "valid-minimal")
	hint, err := BuildAgentHint(workspaceRoot)
	if err != nil {
		t.Fatalf("BuildAgentHint: %v", err)
	}
	if hint.Reason != "tasks_awaiting_approval" {
		t.Fatalf("expected tasks_awaiting_approval, got %s", hint.Reason)
	}
}

func TestAgentHint_Approved(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	submitFixturePlan(t, workspaceRoot, "valid-minimal")
	if _, _, _, err := ApprovePlan(workspaceRoot, ApprovePlanOptions{}); err != nil {
		t.Fatalf("ApprovePlan: %v", err)
	}
	hint, err := BuildAgentHint(workspaceRoot)
	if err != nil {
		t.Fatalf("BuildAgentHint: %v", err)
	}
	if hint.Reason != "plan_approved_ready" {
		t.Fatalf("expected plan_approved_ready, got %s", hint.Reason)
	}
}

func TestAgentHint_Rejected(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	submitFixturePlan(t, workspaceRoot, "valid-minimal")
	if _, err := RejectPlan(workspaceRoot, "no"); err != nil {
		t.Fatalf("RejectPlan: %v", err)
	}
	hint, err := BuildAgentHint(workspaceRoot)
	if err != nil {
		t.Fatalf("BuildAgentHint: %v", err)
	}
	if hint.Reason != "plan_rejected" {
		t.Fatalf("expected plan_rejected, got %s", hint.Reason)
	}
}

func TestAgentHint_TasksNeedSubmit(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	tasks := []Task{{ID: "T001", Title: "A", Repository: "repo-a", Status: StatusPlanned}}
	_ = SaveTasks(workspaceRoot, tasks)
	hint, err := BuildAgentHint(workspaceRoot)
	if err != nil {
		t.Fatalf("BuildAgentHint: %v", err)
	}
	if hint.Reason != "planning_bundle_incomplete" {
		t.Fatalf("expected planning_bundle_incomplete, got %s", hint.Reason)
	}
	if hint.SuggestedNextCommand != "feature-dev task preview --json" {
		t.Fatalf("unexpected next command: %s", hint.SuggestedNextCommand)
	}
}

func TestStartTaskDeniedBeforeApproval(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	submitFixturePlan(t, workspaceRoot, "valid-minimal")
	if err := CanExecute(workspaceRoot); !errors.Is(err, ErrPlanNotApproved) {
		t.Fatalf("expected ErrPlanNotApproved, got %v", err)
	}
}
