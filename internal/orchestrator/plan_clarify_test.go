package orchestrator

import (
	"errors"
	"os"
	"testing"
)

func TestBuildClarificationRequestMapsErrors(t *testing.T) {
	report := PlanValidationReport{
		Valid: false,
		Errors: []PlanValidationIssue{{
			Level: "error", Code: "orphan_requirement",
			Message: "requirement R004 is not covered by any task",
		}},
	}
	req := BuildClarificationRequest(report, 0)
	if len(req.Questions) == 0 {
		t.Fatal("expected clarification questions")
	}
	if req.RestartFrom != "requirement_mapping" {
		t.Fatalf("expected requirement_mapping restart, got %s", req.RestartFrom)
	}
}

func TestSubmitFromTasksWithoutBundle_EntersClarification(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	writeValidTasksJSON(t, workspaceRoot)
	_, err := SubmitPlan(workspaceRoot, SubmitPlanOptions{FromTasks: true})
	if err == nil || !errors.Is(err, ErrPlanValidationFailed) {
		t.Fatalf("expected validation failure, got %v", err)
	}
	ws, _ := LoadWorkflowState(workspaceRoot)
	if ws.WorkflowStatus != WorkflowClarificationNeeded {
		t.Fatalf("expected CLARIFICATION_NEEDED, got %s", ws.WorkflowStatus)
	}
}

func TestWriteClarificationRequestPersistsFile(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	report := PlanValidationReport{
		Valid: false,
		Errors: []PlanValidationIssue{{
			Level: "error", Code: "missing_assumptions_file",
			Message: "assumptions.json is required",
		}},
	}
	req := BuildClarificationRequest(report, 0)
	if err := WriteClarificationRequest(workspaceRoot, req); err != nil {
		t.Fatalf("WriteClarificationRequest: %v", err)
	}
	if _, err := os.Stat(PlanDraftClarificationRequestPath(workspaceRoot)); err != nil {
		t.Fatalf("expected clarification-request.json: %v", err)
	}
}
