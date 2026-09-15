package orchestrator

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlanSubmit_CreatesImmutableRevisionDir(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	result := submitFixturePlan(t, workspaceRoot, "valid-minimal")
	if result.Revision != 1 {
		t.Fatalf("expected revision 1, got %d", result.Revision)
	}
	if !Exists(PlanRevisionPath(workspaceRoot, 1)) {
		t.Fatal("expected revision-001/plan.yaml")
	}
	first, _ := os.ReadFile(PlanRevisionPath(workspaceRoot, 1))
	doc := loadFixturePlan(t, "valid-minimal")
	doc.Tasks = append(doc.Tasks, PlanTask{ID: "T002", Title: "Added", Repository: "repo-a", Verification: []VerificationStep{{Command: "go test ./..."}}})
	doc.Requirements = append(doc.Requirements, PlanRequirement{ID: "R002", Description: "Added", Tasks: []string{"T002"}})
	doc.AcceptanceCriteria = append(doc.AcceptanceCriteria, PlanAcceptanceCriterion{ID: "AC002", ImplementationTasks: []string{"T002"}, PlannedVerification: []string{"test"}})
	writePlanDraft(t, workspaceRoot, doc)
	result2, err := SubmitPlan(workspaceRoot, SubmitPlanOptions{FromPath: PlanDraftPath(workspaceRoot), AutoApprove: false})
	if err != nil {
		t.Fatalf("SubmitPlan rev2: %v", err)
	}
	if result2.Revision != 2 {
		t.Fatalf("expected revision 2, got %d", result2.Revision)
	}
	second, _ := os.ReadFile(PlanRevisionPath(workspaceRoot, 2))
	if string(first) == string(second) {
		t.Fatal("expected different revision content")
	}
	orig, _ := os.ReadFile(PlanRevisionPath(workspaceRoot, 1))
	if string(orig) != string(first) {
		t.Fatal("revision-001 should remain immutable")
	}
}

func TestPlanSubmit_SyncsTasksJSONPreservesDone(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	submitFixturePlan(t, workspaceRoot, "valid-minimal")
	tasks, _ := LoadTasks(workspaceRoot)
	tasks[0].Status = StatusDone
	_ = SaveTasks(workspaceRoot, tasks)
	doc := loadFixturePlan(t, "valid-minimal")
	doc.Tasks = append(doc.Tasks, PlanTask{ID: "T002", Title: "Added", Repository: "repo-a", Verification: []VerificationStep{{Command: "go test ./..."}}})
	doc.Requirements = append(doc.Requirements, PlanRequirement{ID: "R002", Description: "Added", Tasks: []string{"T002"}})
	doc.AcceptanceCriteria = append(doc.AcceptanceCriteria, PlanAcceptanceCriterion{ID: "AC002", ImplementationTasks: []string{"T002"}, PlannedVerification: []string{"test"}})
	writePlanDraft(t, workspaceRoot, doc)
	_, _ = SubmitPlan(workspaceRoot, SubmitPlanOptions{FromPath: PlanDraftPath(workspaceRoot)})
	loaded, _ := LoadTasks(workspaceRoot)
	for _, task := range loaded {
		if task.ID == "T001" && task.Status != StatusDone {
			t.Fatalf("expected T001 DONE preserved, got %s", task.Status)
		}
	}
}

func TestPlanDiff_ShowsAddedRemovedModified(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	submitFixturePlan(t, workspaceRoot, "valid-minimal")
	doc := loadFixturePlan(t, "valid-minimal")
	doc.Tasks = append(doc.Tasks, PlanTask{ID: "T002", Title: "DeviceHealthController integration tests", Repository: "repo-a", Verification: []VerificationStep{{Command: "go test ./..."}}})
	doc.Requirements = append(doc.Requirements, PlanRequirement{ID: "R002", Description: "Tests", Tasks: []string{"T002"}})
	doc.AcceptanceCriteria = append(doc.AcceptanceCriteria, PlanAcceptanceCriterion{ID: "AC002", ImplementationTasks: []string{"T002"}, PlannedVerification: []string{"integration-test"}})
	writePlanDraft(t, workspaceRoot, doc)
	_, _ = SubmitPlan(workspaceRoot, SubmitPlanOptions{FromPath: PlanDraftPath(workspaceRoot)})
	diff, err := PlanDiffBetweenRevisions(workspaceRoot, 1, 2)
	if err != nil {
		t.Fatalf("PlanDiffBetweenRevisions: %v", err)
	}
	text := FormatPlanDiffOutput(diff)
	if !strings.Contains(text, "Added:") {
		t.Fatalf("expected Added section, got %s", text)
	}
}

func TestReplan_SetsReplanningState(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	submitFixturePlan(t, workspaceRoot, "valid-minimal")
	ws, err := Replan(workspaceRoot, "user requested changes")
	if err != nil {
		t.Fatalf("Replan: %v", err)
	}
	if ws.WorkflowStatus != WorkflowReplanning {
		t.Fatalf("expected REPLANNING, got %s", ws.WorkflowStatus)
	}
	if !Exists(PlanRevisionPath(workspaceRoot, 1)) {
		t.Fatal("prior revision should remain")
	}
}

func TestRepoSnapshot_WarnsOnDrift(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	submitFixturePlan(t, workspaceRoot, "valid-minimal")
	repoPath := filepath.Join(workspaceRoot, "repo-a")
	readme := filepath.Join(repoPath, "README.md")
	_ = os.WriteFile(readme, []byte("changed\n"), 0o644)
	add := exec.Command("git", "add", "README.md")
	add.Dir = repoPath
	_ = add.Run()
	commit := exec.Command("git", "commit", "-m", "drift")
	commit.Dir = repoPath
	_ = commit.Run()
	result, _, _, err := ApprovePlan(workspaceRoot, ApprovePlanOptions{})
	if err != nil {
		t.Fatalf("ApprovePlan: %v", err)
	}
	if len(result.Warnings) == 0 {
		t.Fatal("expected repository drift warning")
	}
}
