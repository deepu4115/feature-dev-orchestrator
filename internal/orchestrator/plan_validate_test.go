package orchestrator

import (
	"os"
	"strings"
	"testing"
)

func TestValidatePlan_RejectsMissingRepository(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	doc := loadFixturePlan(t, "invalid-missing-repo")
	report, err := ValidatePlanDocument(workspaceRoot, doc)
	if err != nil {
		t.Fatalf("ValidatePlanDocument: %v", err)
	}
	if report.Valid {
		t.Fatal("expected invalid plan")
	}
	if !hasCode(report.Errors, "invalid_repository") {
		t.Fatalf("expected invalid_repository error, got %#v", report.Errors)
	}
}

func TestValidatePlan_RejectsMissingDependency(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	doc := loadFixturePlan(t, "valid-minimal")
	doc.Tasks[0].Dependencies = []string{"T999"}
	report, _ := ValidatePlanDocument(workspaceRoot, doc)
	if report.Valid || !hasCode(report.Errors, "missing_dependency") {
		t.Fatalf("expected missing_dependency, got %#v", report.Errors)
	}
}

func TestValidatePlan_RejectsCycle(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	doc := loadFixturePlan(t, "invalid-cycle")
	report, _ := ValidatePlanDocument(workspaceRoot, doc)
	if report.Valid || report.Checks["dag"] != "FAIL" {
		t.Fatalf("expected cycle failure, got %#v", report)
	}
}

func TestValidatePlan_RejectsOrphanRequirement(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	doc := loadFixturePlan(t, "invalid-orphan-requirement")
	report, _ := ValidatePlanDocument(workspaceRoot, doc)
	if report.Valid || !hasCode(report.Errors, "orphan_requirement") {
		t.Fatalf("expected orphan_requirement, got %#v", report.Errors)
	}
}

func TestValidatePlan_RejectsACWithoutVerification(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	doc := loadFixturePlan(t, "valid-minimal")
	doc.AcceptanceCriteria[0].PlannedVerification = nil
	report, _ := ValidatePlanDocument(workspaceRoot, doc)
	if report.Valid {
		t.Fatal("expected invalid when AC missing planned_verification")
	}
}

func TestValidatePlan_RejectsEmptyVerificationStrategy(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	doc := loadFixturePlan(t, "valid-minimal")
	doc.Verification = PlanVerificationStrategy{}
	for i := range doc.Tasks {
		doc.Tasks[i].Verification = nil
	}
	report, _ := ValidatePlanDocument(workspaceRoot, doc)
	if report.Valid || !hasCode(report.Errors, "empty_verification_strategy") {
		t.Fatalf("expected empty_verification_strategy, got %#v", report.Errors)
	}
}

func TestValidatePlan_WarnsLowConfidenceHighImpactAssumption(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	doc := loadFixturePlan(t, "valid-minimal")
	doc.Assumptions = []PlanAssumption{{ID: "A002", Statement: "No migration", Confidence: "LOW", Impact: "HIGH"}}
	report, _ := ValidatePlanDocument(workspaceRoot, doc)
	if !hasCode(report.Warnings, "low_confidence_high_impact_assumption") {
		t.Fatalf("expected warning, got %#v", report.Warnings)
	}
}

func TestValidatePlan_RejectsCriticalRiskWithoutMitigation(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	doc := loadFixturePlan(t, "valid-minimal")
	doc.Risks = []PlanRisk{{ID: "RK002", Level: "CRITICAL", Description: "bad"}}
	report, _ := ValidatePlanDocument(workspaceRoot, doc)
	if report.Valid || !hasCode(report.Errors, "critical_risk_without_mitigation") {
		t.Fatalf("expected critical risk error, got %#v", report.Errors)
	}
}

func TestValidatePlan_PassMovesToReviewPending(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	result := submitFixturePlan(t, workspaceRoot, "valid-minimal")
	if !result.Valid || result.Status != WorkflowReviewPending {
		t.Fatalf("expected valid REVIEW_PENDING submit, got %#v", result)
	}
}

func TestValidatePlan_InvalidStaysPlanGenerated(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	path := "../../testdata/plans/invalid-orphan-requirement/plan.yaml"
	result, err := SubmitPlan(workspaceRoot, SubmitPlanOptions{FromPath: path})
	if err != nil {
		t.Fatalf("SubmitPlan: %v", err)
	}
	if result.Valid {
		t.Fatal("expected invalid submit")
	}
	ws, _ := LoadWorkflowState(workspaceRoot)
	if ws.WorkflowStatus != WorkflowPlanGenerated {
		t.Fatalf("expected PLAN_GENERATED, got %s", ws.WorkflowStatus)
	}
}

func TestReviewJSONIncludesValidationBlock(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	submitFixturePlan(t, workspaceRoot, "valid-minimal")
	payload, _, _, err := ReviewPlan(workspaceRoot)
	if err != nil {
		t.Fatalf("ReviewPlan: %v", err)
	}
	if payload.Validation["dag"] != "PASS" {
		t.Fatalf("expected validation block, got %#v", payload.Validation)
	}
	if payload.TasksFile != TaskStorageRelativePath() {
		t.Fatalf("expected tasks_file in review JSON, got %s", payload.TasksFile)
	}
	if len(payload.TaskItems) == 0 {
		t.Fatal("expected task items in review JSON")
	}
}

func TestReviewGeneratesReviewMDSections(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	submitFixturePlan(t, workspaceRoot, "valid-minimal")
	reviewPath := PlanRevisionDir(workspaceRoot, 1) + "/review.md"
	data, err := os.ReadFile(reviewPath)
	if err != nil {
		t.Fatalf("read review.md: %v", err)
	}
	content := string(data)
	for _, heading := range []string{"## Objective", "## Relevant Repositories", "## Requirements", "## Proposed Tasks", "## Dependency Graph", "## Assumptions", "## Risks", "## Verification Plan", "## Approval Status"} {
		if !strings.Contains(content, heading) {
			t.Fatalf("missing heading %s in review.md", heading)
		}
	}
}