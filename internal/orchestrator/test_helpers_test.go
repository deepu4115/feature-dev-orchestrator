package orchestrator

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func setupTestWorkspace(t *testing.T) string {
	t.Helper()
	workspaceRoot := t.TempDir()
	if err := InitWorkspace(workspaceRoot); err != nil {
		t.Fatalf("InitWorkspace: %v", err)
	}
	repoPath := filepath.Join(workspaceRoot, "repo-a")
	if err := initGitRepo(repoPath); err != nil {
		t.Fatalf("initGitRepo: %v", err)
	}
	repos := []Repository{{ID: "repo-a", Path: "repo-a", GitRoot: "repo-a", Mode: "read_write"}}
	if err := SaveRepositories(workspaceRoot, repos); err != nil {
		t.Fatalf("SaveRepositories: %v", err)
	}
	return workspaceRoot
}

func loadFixturePlan(t *testing.T, name string) PlanDocument {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", "plans", name, "plan.yaml")
	doc, err := LoadPlanDocument(path)
	if err != nil {
		t.Fatalf("LoadPlanDocument(%s): %v", name, err)
	}
	return doc
}

func submitFixturePlan(t *testing.T, workspaceRoot, fixtureName string) SubmitPlanResult {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", "plans", fixtureName, "plan.yaml")
	result, err := SubmitPlan(workspaceRoot, SubmitPlanOptions{FromPath: path})
	if err != nil {
		t.Fatalf("SubmitPlan: %v", err)
	}
	return result
}

func writePlanDraft(t *testing.T, workspaceRoot string, doc PlanDocument) {
	t.Helper()
	draftDir := filepath.Join(PlansDir(workspaceRoot), "draft")
	if err := EnsureDir(draftDir); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if err := SavePlanDocument(PlanDraftPath(workspaceRoot), doc); err != nil {
		t.Fatalf("SavePlanDocument: %v", err)
	}
}

func validPlanningBundle() PlanningDraftBundle {
	return PlanningDraftBundle{
		Requirements: DraftRequirementsFile{Requirements: []DraftRequirement{{ID: "R001", Description: "Add health endpoint"}}},
		Assumptions:  DraftAssumptionsFile{Assumptions: []DraftAssumption{{ID: "A001", Statement: "repo-a owns API", Confidence: "HIGH", Impact: "MEDIUM"}}},
		Risks:        DraftRisksFile{Risks: []DraftRisk{{ID: "RK001", Level: "MEDIUM", Description: "API change", Mitigation: []string{"add tests"}}}},
		Impact:       DraftImpactFile{Impact: []DraftImpactEntry{{Repository: "repo-a", Areas: []string{"api/handlers"}}}},
		RepoAnalysis: DraftRepoAnalysisFile{Repositories: []DraftRepoAnalysisEntry{{Repository: "repo-a", Evidence: []string{"repo-a/main.go"}}}},
	}
}

func writeValidPlanningBundle(t *testing.T, workspaceRoot string) {
	t.Helper()
	if err := WritePlanningBundleFixture(workspaceRoot, validPlanningBundle()); err != nil {
		t.Fatalf("WritePlanningBundleFixture: %v", err)
	}
}

func writeValidTasksJSON(t *testing.T, workspaceRoot string) {
	t.Helper()
	tasks := []Task{{
		ID: "T001", Title: "Add health endpoint", Repository: "repo-a", RequirementIDs: []string{"R001"},
		OwnershipConfidence: "HIGH", RepositoryRationale: "repo-a owns API", Status: StatusReviewPending,
		Verification: []VerificationStep{{Command: "go test ./..."}},
	}}
	if err := SaveTasks(workspaceRoot, tasks); err != nil {
		t.Fatalf("SaveTasks: %v", err)
	}
}

func copyPlanningFixture(t *testing.T, workspaceRoot, fixtureName string) {
	t.Helper()
	srcDir := filepath.Join("..", "..", "testdata", "plans", fixtureName)
	dstDir := PlanDraftDir(workspaceRoot)
	if err := EnsureDir(dstDir); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	for _, name := range []string{
		"requirements.json", "assumptions.json", "risks.json", "impact.json", "repo-analysis.json", "workspace-verify.json",
	} {
		src := filepath.Join(srcDir, name)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		data, err := os.ReadFile(src)
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", src, err)
		}
		if err := WriteFileAtomically(filepath.Join(dstDir, name), data); err != nil {
			t.Fatalf("WriteFileAtomically: %v", err)
		}
	}
}

func submitFromTasksWithBundle(t *testing.T, workspaceRoot string) SubmitPlanResult {
	t.Helper()
	writeValidPlanningBundle(t, workspaceRoot)
	writeValidTasksJSON(t, workspaceRoot)
	result, err := SubmitPlan(workspaceRoot, SubmitPlanOptions{FromTasks: true})
	if err != nil {
		t.Fatalf("SubmitPlan from tasks: %v", err)
	}
	return result
}

func initGitRepo(repoPath string) error {
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		return err
	}
	cmd := exec.Command("git", "init")
	cmd.Dir = repoPath
	if _, err := cmd.CombinedOutput(); err != nil {
		return err
	}
	cfg := exec.Command("git", "config", "user.email", "test@example.com")
	cfg.Dir = repoPath
	_ = cfg.Run()
	cfg = exec.Command("git", "config", "user.name", "Test")
	cfg.Dir = repoPath
	_ = cfg.Run()
	readme := filepath.Join(repoPath, "README.md")
	if err := os.WriteFile(readme, []byte("test\n"), 0o644); err != nil {
		return err
	}
	add := exec.Command("git", "add", "README.md")
	add.Dir = repoPath
	if err := add.Run(); err != nil {
		return err
	}
	commit := exec.Command("git", "commit", "-m", "init")
	commit.Dir = repoPath
	return commit.Run()
}
