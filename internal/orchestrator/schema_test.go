package orchestrator

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSchemaShowTasks(t *testing.T) {
	payload, err := SchemaShow("tasks")
	if err != nil {
		t.Fatalf("SchemaShow: %v", err)
	}
	if payload["root_key"] != "(top-level array)" {
		t.Fatalf("unexpected root_key: %v", payload["root_key"])
	}
}

func TestInitPlanningDraftFromTemplates(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	written, err := InitPlanningDraftFromTemplates(workspaceRoot, false)
	if err != nil {
		t.Fatalf("InitPlanningDraftFromTemplates: %v", err)
	}
	if len(written) != 6 {
		t.Fatalf("expected 6 draft files, got %d: %v", len(written), written)
	}
	for _, name := range []string{"requirements.json", "assumptions.json", "risks.json", "impact.json", "repo-analysis.json"} {
		if !Exists(filepath.Join(PlanDraftDir(workspaceRoot), name)) {
			t.Fatalf("missing %s", name)
		}
	}
}

func TestInitTasksFromTemplate(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	path, err := InitTasksFromTemplate(workspaceRoot, false)
	if err != nil {
		t.Fatalf("InitTasksFromTemplate: %v", err)
	}
	result, err := LoadTasksWithReport(workspaceRoot)
	if err != nil {
		t.Fatalf("LoadTasksWithReport: %v", err)
	}
	if !result.Report.Valid {
		t.Fatalf("expected valid tasks template, got %#v", result.Report.Errors)
	}
	if len(result.Tasks) == 0 {
		t.Fatal("expected example tasks")
	}
	if path != TaskStoragePath(workspaceRoot) {
		t.Fatalf("unexpected path %s", path)
	}
}

func TestLoadTasksWithReport_InvalidVerificationType(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	path := TaskStoragePath(workspaceRoot)
	if err := os.WriteFile(path, []byte(`[{"id":"T001","title":"x","repository":"repo-a","verification":"go test ./..."}]`), 0644); err != nil {
		t.Fatal(err)
	}
	result, err := LoadTasksWithReport(workspaceRoot)
	if err != nil {
		t.Fatalf("LoadTasksWithReport: %v", err)
	}
	if result.Report.Valid {
		t.Fatal("expected invalid report")
	}
	if !hasCode(result.Report.Errors, "invalid_verification_type") {
		t.Fatalf("expected invalid_verification_type, got %#v", result.Report.Errors)
	}
}

func TestLoadTasksWithReport_InvalidTasksWrapper(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	path := TaskStoragePath(workspaceRoot)
	if err := os.WriteFile(path, []byte(`{"tasks":[{"id":"T001","title":"x","repository":"repo-a","verification":[{"command":"go test ./..."}]}]}`), 0644); err != nil {
		t.Fatal(err)
	}
	result, err := LoadTasksWithReport(workspaceRoot)
	if err != nil {
		t.Fatalf("LoadTasksWithReport: %v", err)
	}
	if result.Report.Valid {
		t.Fatal("expected invalid report")
	}
	if !hasCode(result.Report.Errors, "invalid_tasks_wrapper") {
		t.Fatalf("expected invalid_tasks_wrapper, got %#v", result.Report.Errors)
	}
}

func TestPreviewTaskPlan_StructuralTasksError(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	path := TaskStoragePath(workspaceRoot)
	if err := os.WriteFile(path, []byte(`[{"id":"T001","title":"x","repository":"repo-a","verification":"bad"}]`), 0644); err != nil {
		t.Fatal(err)
	}
	_, _, report, err := PreviewTaskPlan(workspaceRoot)
	if err != nil {
		t.Fatalf("PreviewTaskPlan: %v", err)
	}
	if report.Valid || !hasCode(report.Errors, "invalid_verification_type") {
		t.Fatalf("expected structural task error in preview, got %#v", report.Errors)
	}
}

func TestSubmitPlan_StructuralErrorSetsPlanningStatus(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	if _, err := InitPlanningDraftFromTemplates(workspaceRoot, false); err != nil {
		t.Fatal(err)
	}
	path := TaskStoragePath(workspaceRoot)
	if err := os.WriteFile(path, []byte(`[{"id":"T001","title":"x","repository":"repo-a","verification":"bad"}]`), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := SubmitPlan(workspaceRoot, SubmitPlanOptions{FromTasks: true})
	if err == nil {
		t.Fatal("expected submit failure")
	}
	ws, _ := LoadWorkflowState(workspaceRoot)
	if ws.WorkflowStatus != WorkflowPlanning {
		t.Fatalf("expected PLANNING for structural error, got %s", ws.WorkflowStatus)
	}
	req, err := LoadClarificationRequest(workspaceRoot)
	if err != nil {
		t.Fatalf("LoadClarificationRequest: %v", err)
	}
	if len(req.Questions) != 0 {
		t.Fatalf("expected no domain questions for structural errors, got %#v", req.Questions)
	}
}

func TestBuildClarificationRequest_SkipsStructuralErrors(t *testing.T) {
	report := PlanValidationReport{
		Valid: false,
		Errors: []PlanValidationIssue{
			{Level: "error", Code: "invalid_verification_type", Message: "bad verification"},
			{Level: "error", Code: "missing_assumptions_file", Message: "missing assumptions"},
		},
	}
	req := BuildClarificationRequest(report, 0)
	if len(req.Questions) != 1 {
		t.Fatalf("expected one domain question, got %#v", req.Questions)
	}
	if req.Questions[0].ErrorCode != "missing_assumptions_file" {
		t.Fatalf("unexpected question: %#v", req.Questions[0])
	}
}

func TestAgentHintSchemaFixRequired(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	path := TaskStoragePath(workspaceRoot)
	if err := os.WriteFile(path, []byte(`[{"id":"T001","title":"x","repository":"repo-a","verification":"bad"}]`), 0644); err != nil {
		t.Fatal(err)
	}
	hint, err := BuildAgentHint(workspaceRoot)
	if err != nil {
		t.Fatalf("BuildAgentHint: %v", err)
	}
	if hint.Reason != "schema_fix_required" {
		t.Fatalf("expected schema_fix_required, got %s", hint.Reason)
	}
}
