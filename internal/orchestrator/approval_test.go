package orchestrator

import (
	"errors"
	"testing"
)

func TestCanStartTask_LegacyBypass_NoRevision(t *testing.T) {
	ws := DefaultWorkflowState()
	task := Task{ID: "T001", Status: StatusPlanned}
	if err := CanStartTask(ws, task); err != nil {
		t.Fatalf("expected legacy bypass, got %v", err)
	}
}

func TestCanStartTask_DeniedWhenReviewPending(t *testing.T) {
	ws := DefaultWorkflowState()
	ws.CurrentPlanRevision = 1
	ws.WorkflowStatus = WorkflowReviewPending
	task := Task{ID: "T001", Status: StatusReady}
	err := CanStartTask(ws, task)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrPlanNotApproved) {
		t.Fatalf("expected ErrPlanNotApproved, got %v", err)
	}
}

func TestCanStartTask_DeniedWhenApprovedRevisionStale(t *testing.T) {
	rev2 := 2
	ws := DefaultWorkflowState()
	ws.CurrentPlanRevision = 3
	ws.ApprovedPlanRevision = &rev2
	ws.WorkflowStatus = WorkflowApproved
	task := Task{ID: "T001", Status: StatusReady}
	err := CanStartTask(ws, task)
	if !errors.Is(err, ErrPlanRevisionMismatch) {
		t.Fatalf("expected ErrPlanRevisionMismatch, got %v", err)
	}
}

func TestCanStartTask_AllowedAfterApprove(t *testing.T) {
	rev := 1
	ws := DefaultWorkflowState()
	ws.CurrentPlanRevision = 1
	ws.ApprovedPlanRevision = &rev
	ws.WorkflowStatus = WorkflowApproved
	task := Task{ID: "T001", Status: StatusReady}
	if err := CanStartTask(ws, task); err != nil {
		t.Fatalf("expected allowed, got %v", err)
	}
}

func TestCanStartTask_DeniedWhenTaskNotReady(t *testing.T) {
	rev := 1
	ws := DefaultWorkflowState()
	ws.CurrentPlanRevision = 1
	ws.ApprovedPlanRevision = &rev
	ws.WorkflowStatus = WorkflowApproved
	task := Task{ID: "T001", Status: StatusPlanned}
	if err := CanStartTask(ws, task); err == nil {
		t.Fatal("expected error for non-READY task")
	}
}

func TestApprovePlan_RecordsHistoryAndFingerprint(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	submitFixturePlan(t, workspaceRoot, "valid-minimal")

	result, ws, _, err := ApprovePlan(workspaceRoot, ApprovePlanOptions{})
	if err != nil {
		t.Fatalf("ApprovePlan: %v", err)
	}
	if result.Revision != 1 {
		t.Fatalf("expected revision 1, got %d", result.Revision)
	}
	if ws.WorkflowStatus != WorkflowApproved {
		t.Fatalf("expected APPROVED, got %s", ws.WorkflowStatus)
	}
	if ws.ApprovedPlanFingerprint == "" {
		t.Fatal("expected fingerprint recorded")
	}
	if len(ws.ApprovalHistory) == 0 || ws.ApprovalHistory[len(ws.ApprovalHistory)-1].Status != "APPROVED" {
		t.Fatalf("expected approval history, got %#v", ws.ApprovalHistory)
	}
}

func TestApprovePlan_RejectsStaleRevisionFlag(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	submitFixturePlan(t, workspaceRoot, "valid-minimal")
	_, _, _, err := ApprovePlan(workspaceRoot, ApprovePlanOptions{Revision: 99})
	if !errors.Is(err, ErrStaleRevisionApproval) {
		t.Fatalf("expected ErrStaleRevisionApproval, got %v", err)
	}
}

func TestApprovePlan_RejectsWhenNotReviewPending(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	submitFixturePlan(t, workspaceRoot, "valid-minimal")
	ws, _ := LoadWorkflowState(workspaceRoot)
	ws.WorkflowStatus = WorkflowApproved
	if err := SaveWorkflowState(workspaceRoot, ws); err != nil {
		t.Fatalf("SaveWorkflowState: %v", err)
	}
	_, _, _, err := ApprovePlan(workspaceRoot, ApprovePlanOptions{})
	if !errors.Is(err, ErrPlanNotReviewPending) {
		t.Fatalf("expected ErrPlanNotReviewPending, got %v", err)
	}
}

func TestRejectPlan_PersistsReasonAndLocksExecution(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	submitFixturePlan(t, workspaceRoot, "valid-minimal")
	ws, err := RejectPlan(workspaceRoot, "not needed")
	if err != nil {
		t.Fatalf("RejectPlan: %v", err)
	}
	if ws.WorkflowStatus != WorkflowRejected {
		t.Fatalf("expected REJECTED, got %s", ws.WorkflowStatus)
	}
	reloaded, _ := LoadWorkflowState(workspaceRoot)
	if len(reloaded.ApprovalHistory) == 0 || reloaded.ApprovalHistory[len(reloaded.ApprovalHistory)-1].Reason != "not needed" {
		t.Fatalf("expected rejection reason persisted, got %#v", reloaded.ApprovalHistory)
	}
	if err := CanExecute(workspaceRoot); !errors.Is(err, ErrPlanNotApproved) {
		t.Fatalf("expected execution locked, got %v", err)
	}
}

func TestNewRevisionInvalidatesApproval(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	submitFixturePlan(t, workspaceRoot, "valid-minimal")
	if _, _, _, err := ApprovePlan(workspaceRoot, ApprovePlanOptions{}); err != nil {
		t.Fatalf("ApprovePlan: %v", err)
	}
	doc := loadFixturePlan(t, "valid-minimal")
	doc.Tasks = append(doc.Tasks, PlanTask{ID: "T002", Title: "Second", Repository: "repo-a", Verification: []VerificationStep{{Command: "go test ./..."}}})
	doc.Requirements = append(doc.Requirements, PlanRequirement{ID: "R002", Description: "Second", Tasks: []string{"T002"}})
	doc.AcceptanceCriteria = append(doc.AcceptanceCriteria, PlanAcceptanceCriterion{ID: "AC002", ImplementationTasks: []string{"T002"}, PlannedVerification: []string{"test"}})
	writePlanDraft(t, workspaceRoot, doc)
	if _, err := SubmitPlan(workspaceRoot, SubmitPlanOptions{FromPath: PlanDraftPath(workspaceRoot)}); err != nil {
		t.Fatalf("SubmitPlan rev2: %v", err)
	}
	ws, _ := LoadWorkflowState(workspaceRoot)
	if ws.WorkflowStatus != WorkflowReviewPending {
		t.Fatalf("expected REVIEW_PENDING, got %s", ws.WorkflowStatus)
	}
	if ws.ApprovedPlanRevision != nil {
		t.Fatal("expected approval invalidated")
	}
}

func TestApprovalSurvivesReload(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	submitFixturePlan(t, workspaceRoot, "valid-minimal")
	if _, _, _, err := ApprovePlan(workspaceRoot, ApprovePlanOptions{}); err != nil {
		t.Fatalf("ApprovePlan: %v", err)
	}
	ws, err := LoadWorkflowState(workspaceRoot)
	if err != nil {
		t.Fatalf("LoadWorkflowState: %v", err)
	}
	if ws.WorkflowStatus != WorkflowApproved {
		t.Fatalf("expected APPROVED after reload, got %s", ws.WorkflowStatus)
	}
}

func TestFingerprintMismatchBlocksStart(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	submitFixturePlan(t, workspaceRoot, "valid-minimal")
	if _, _, _, err := ApprovePlan(workspaceRoot, ApprovePlanOptions{}); err != nil {
		t.Fatalf("ApprovePlan: %v", err)
	}
	doc, err := LoadCurrentPlanDocument(workspaceRoot)
	if err != nil {
		t.Fatalf("LoadCurrentPlanDocument: %v", err)
	}
	doc.Tasks[0].Title = "Tampered title"
	if err := SavePlanDocument(PlanRevisionPath(workspaceRoot, 1), doc); err != nil {
		t.Fatalf("SavePlanDocument: %v", err)
	}
	tasks, _ := LoadTasks(workspaceRoot)
	tasks[0].Status = StatusReady
	_ = SaveTasks(workspaceRoot, tasks)
	if err := CanStartTaskInWorkspace(workspaceRoot, tasks[0]); !errors.Is(err, ErrPlanFingerprintMismatch) {
		t.Fatalf("expected ErrPlanFingerprintMismatch, got %v", err)
	}
}
