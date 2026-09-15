package orchestrator

import (
	"errors"
	"testing"
)

func TestE2E_Scenario1_SimpleApproval(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	submitFixturePlan(t, workspaceRoot, "valid-minimal")
	if _, _, _, err := ReviewPlan(workspaceRoot); err != nil {
		t.Fatalf("ReviewPlan: %v", err)
	}
	_, _, tasks, err := ApprovePlan(workspaceRoot, ApprovePlanOptions{})
	if err != nil {
		t.Fatalf("ApprovePlan: %v", err)
	}
	if len(tasks) == 0 {
		t.Fatal("expected tasks")
	}
	if err := CanStartTaskInWorkspace(workspaceRoot, Task{ID: "T001", Status: StatusReady}); err != nil {
		t.Fatalf("expected start allowed: %v", err)
	}
}

func TestE2E_Scenario2_StartBeforeApproval(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	submitFixturePlan(t, workspaceRoot, "valid-minimal")
	if err := CanExecute(workspaceRoot); !errors.Is(err, ErrPlanNotApproved) {
		t.Fatalf("expected DENIED, got %v", err)
	}
}

func TestE2E_Scenario3_OnlyCurrentRevisionExecutable(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	submitFixturePlan(t, workspaceRoot, "valid-minimal")
	if _, _, _, err := ApprovePlan(workspaceRoot, ApprovePlanOptions{Revision: 1}); err != nil {
		t.Fatalf("ApprovePlan rev1: %v", err)
	}
	doc := loadFixturePlan(t, "valid-minimal")
	doc.Tasks = append(doc.Tasks, PlanTask{ID: "T002", Title: "Second", Repository: "repo-a", Verification: []VerificationStep{{Command: "go test ./..."}}})
	doc.Requirements = append(doc.Requirements, PlanRequirement{ID: "R002", Description: "Second", Tasks: []string{"T002"}})
	doc.AcceptanceCriteria = append(doc.AcceptanceCriteria, PlanAcceptanceCriterion{ID: "AC002", ImplementationTasks: []string{"T002"}, PlannedVerification: []string{"test"}})
	writePlanDraft(t, workspaceRoot, doc)
	if _, err := SubmitPlan(workspaceRoot, SubmitPlanOptions{FromPath: PlanDraftPath(workspaceRoot)}); err != nil {
		t.Fatalf("SubmitPlan rev2: %v", err)
	}
	_, _, _, err := ApprovePlan(workspaceRoot, ApprovePlanOptions{Revision: 1})
	if !errors.Is(err, ErrStaleRevisionApproval) {
		t.Fatalf("expected stale revision error, got %v", err)
	}
	if _, _, _, err := ApprovePlan(workspaceRoot, ApprovePlanOptions{Revision: 2}); err != nil {
		t.Fatalf("ApprovePlan rev2: %v", err)
	}
	ws, _ := LoadWorkflowState(workspaceRoot)
	if ws.ApprovedPlanRevision == nil || *ws.ApprovedPlanRevision != 2 {
		t.Fatalf("expected approved revision 2, got %#v", ws.ApprovedPlanRevision)
	}
}

func TestE2E_Scenario4_ApprovedPlanModifiedInvalidatesApproval(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	submitFixturePlan(t, workspaceRoot, "valid-minimal")
	if _, _, _, err := ApprovePlan(workspaceRoot, ApprovePlanOptions{}); err != nil {
		t.Fatalf("ApprovePlan: %v", err)
	}
	doc := loadFixturePlan(t, "valid-minimal")
	doc.Tasks = append(doc.Tasks, PlanTask{ID: "T002", Title: "Third", Repository: "repo-a", Verification: []VerificationStep{{Command: "go test ./..."}}})
	doc.Requirements = append(doc.Requirements, PlanRequirement{ID: "R002", Description: "Third", Tasks: []string{"T002"}})
	doc.AcceptanceCriteria = append(doc.AcceptanceCriteria, PlanAcceptanceCriterion{ID: "AC002", ImplementationTasks: []string{"T002"}, PlannedVerification: []string{"test"}})
	writePlanDraft(t, workspaceRoot, doc)
	if _, err := SubmitPlan(workspaceRoot, SubmitPlanOptions{FromPath: PlanDraftPath(workspaceRoot)}); err != nil {
		t.Fatalf("SubmitPlan rev3: %v", err)
	}
	if err := CanExecute(workspaceRoot); !errors.Is(err, ErrPlanNotApproved) {
		t.Fatalf("expected locked execution, got %v", err)
	}
}

func TestE2E_Scenario5_ResumeSession(t *testing.T) {
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
		t.Fatalf("expected approval restored, got %s", ws.WorkflowStatus)
	}
	if err := CanExecute(workspaceRoot); err != nil {
		t.Fatalf("expected no re-approve needed: %v", err)
	}
}

func TestE2E_Scenario6_MidImplementationReplanPreservesDone(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	submitFixturePlan(t, workspaceRoot, "valid-minimal")
	_, _, tasks, err := ApprovePlan(workspaceRoot, ApprovePlanOptions{})
	if err != nil {
		t.Fatalf("ApprovePlan: %v", err)
	}
	tasks[0].Status = StatusDone
	_ = SaveTasks(workspaceRoot, tasks)
	if _, err := Replan(workspaceRoot, "T002 blocked"); err != nil {
		t.Fatalf("Replan: %v", err)
	}
	doc := loadFixturePlan(t, "valid-minimal")
	doc.Tasks = append(doc.Tasks,
		PlanTask{ID: "T002", Title: "Blocked follow-up", Repository: "repo-a", Dependencies: []string{"T001"}, Verification: []VerificationStep{{Command: "go test ./..."}}},
		PlanTask{ID: "T003", Title: "Replacement path", Repository: "repo-a", Dependencies: []string{"T001"}, Verification: []VerificationStep{{Command: "go test ./..."}}},
	)
	doc.Requirements = append(doc.Requirements,
		PlanRequirement{ID: "R002", Description: "Follow up", Tasks: []string{"T002"}},
		PlanRequirement{ID: "R003", Description: "Replacement", Tasks: []string{"T003"}},
	)
	doc.AcceptanceCriteria = append(doc.AcceptanceCriteria,
		PlanAcceptanceCriterion{ID: "AC002", ImplementationTasks: []string{"T002"}, PlannedVerification: []string{"test"}},
		PlanAcceptanceCriterion{ID: "AC003", ImplementationTasks: []string{"T003"}, PlannedVerification: []string{"test"}},
	)
	writePlanDraft(t, workspaceRoot, doc)
	if _, err := SubmitPlan(workspaceRoot, SubmitPlanOptions{FromPath: PlanDraftPath(workspaceRoot), AutoApprove: true}); err != nil {
		t.Fatalf("SubmitPlan: %v", err)
	}
	loaded, _ := LoadTasks(workspaceRoot)
	for _, task := range loaded {
		if task.ID == "T001" && task.Status != StatusDone {
			t.Fatalf("expected T001 preserved DONE, got %s", task.Status)
		}
	}
}
