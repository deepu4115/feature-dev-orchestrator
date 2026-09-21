package orchestrator

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateRepoAwareTasksUsesMavenWithoutWrapper(t *testing.T) {
	workspaceRoot := t.TempDir()
	repoPath := filepath.Join(workspaceRoot, "user-service")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, "pom.xml"), []byte("<project></project>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	repos := []Repository{{ID: "user-service", Path: "user-service"}}
	tasks := GenerateRepoAwareTasks(workspaceRoot, repos)
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].Repository != "user-service" {
		t.Fatalf("unexpected repo %s", tasks[0].Repository)
	}
	if len(tasks[0].Verification) == 0 || tasks[0].Verification[0].Command != "mvn test" {
		t.Fatalf("expected mvn test, got %#v", tasks[0].Verification)
	}
}

func TestValidateVerificationCommandUnavailable(t *testing.T) {
	dir := t.TempDir()
	err := ValidateVerificationCommandAvailable(dir, "./mvnw test")
	if err == nil || !IsCodedError(err, ErrCodeVerificationCmdMissing) {
		t.Fatalf("expected verification_command_unavailable, got %v", err)
	}
}

func TestSchemaShowWorkspaceVerify(t *testing.T) {
	info, err := SchemaShow("workspace-verify")
	if err != nil {
		t.Fatal(err)
	}
	if info["artifact"] != "workspace-verify" {
		t.Fatalf("unexpected %#v", info)
	}
}

func TestInitTasksFromTemplateRepoAware(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	// ensure repo has a detectable build system
	_ = os.WriteFile(filepath.Join(workspaceRoot, "repo-a", "go.mod"), []byte("module repo-a\n\ngo 1.22\n"), 0o644)
	path, err := InitTasksFromTemplate(workspaceRoot, true)
	if err != nil {
		t.Fatal(err)
	}
	if path == "" {
		t.Fatal("empty path")
	}
	tasks, err := LoadTasks(workspaceRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) == 0 {
		t.Fatal("expected repo-aware tasks")
	}
	for _, task := range tasks {
		if task.Repository == "repo-b" {
			t.Fatal("placeholder repo-b should not appear")
		}
	}
}
