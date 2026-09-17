package orchestrator

import (
	"testing"
)

func TestValidateDraftBundleEarly_RejectsMissingFiles(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	bundle, _ := LoadPlanningDraftBundle(workspaceRoot)
	report := ValidateDraftBundleEarly(workspaceRoot, bundle)
	if report.Valid || !hasCode(report.Errors, "missing_requirements_file") {
		t.Fatalf("expected missing requirements file, got %#v", report.Errors)
	}
}

func TestSubmitFromTasksWithBundle_ReviewPending(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	result := submitFromTasksWithBundle(t, workspaceRoot)
	if !result.Valid || result.Status != WorkflowReviewPending {
		t.Fatalf("expected valid REVIEW_PENDING, got %#v", result)
	}
	if _, err := LoadClarificationRequest(workspaceRoot); err == nil {
		t.Fatal("did not expect clarification request on success")
	}
}
