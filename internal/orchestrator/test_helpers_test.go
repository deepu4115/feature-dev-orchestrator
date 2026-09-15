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
