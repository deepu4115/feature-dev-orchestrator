package orchestrator

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitWorkspaceCreatesRequiredDirectories(t *testing.T) {
	workspaceRoot := t.TempDir()

	if err := InitWorkspace(workspaceRoot); err != nil {
		t.Fatalf("InitWorkspace returned error: %v", err)
	}

	for _, dir := range []string{".feature", ".feature/state", ".feature/context", ".feature/artifacts", ".feature/reviews", ".feature/logs", ".feature/plans"} {
		if _, err := os.Stat(filepath.Join(workspaceRoot, dir)); err != nil {
			t.Fatalf("expected directory %s to exist: %v", dir, err)
		}
	}

	if _, err := os.Stat(DefaultWorkspaceConfigPath(workspaceRoot)); err != nil {
		t.Fatalf("expected config file to exist: %v", err)
	}

	if _, err := os.Stat(DefaultRepositoriesPath(workspaceRoot)); err != nil {
		t.Fatalf("expected repositories registry to exist: %v", err)
	}

	if _, err := os.Stat(DefaultWorkflowPath(workspaceRoot)); err != nil {
		t.Fatalf("expected workflow file to exist: %v", err)
	}

	if _, err := os.Stat(DefaultWorkflowStatePath(workspaceRoot)); err != nil {
		t.Fatalf("expected workflow.json to exist: %v", err)
	}
}

func TestInitCreatesPlansAndWorkflowJSON(t *testing.T) {
	workspaceRoot := t.TempDir()
	if err := InitWorkspace(workspaceRoot); err != nil {
		t.Fatalf("InitWorkspace returned error: %v", err)
	}
	if _, err := os.Stat(PlansDir(workspaceRoot)); err != nil {
		t.Fatalf("expected plans dir: %v", err)
	}
	if _, err := os.Stat(DefaultWorkflowStatePath(workspaceRoot)); err != nil {
		t.Fatalf("expected workflow.json: %v", err)
	}
}

func TestDiscoverReposFindsGitRepositoriesAndSortsThem(t *testing.T) {
	workspaceRoot := t.TempDir()
	if err := InitWorkspace(workspaceRoot); err != nil {
		t.Fatalf("InitWorkspace returned error: %v", err)
	}

	for _, name := range []string{"common", "devices", "analytics"} {
		repoPath := filepath.Join(workspaceRoot, name)
		if err := os.MkdirAll(filepath.Join(repoPath, ".git"), 0o755); err != nil {
			t.Fatalf("create repo %s: %v", name, err)
		}
	}
	if err := os.MkdirAll(filepath.Join(workspaceRoot, "node_modules"), 0o755); err != nil {
		t.Fatalf("create ignored dir: %v", err)
	}

	repos, err := DiscoverRepos(workspaceRoot)
	if err != nil {
		t.Fatalf("DiscoverRepos returned error: %v", err)
	}

	if len(repos) != 3 {
		t.Fatalf("expected 3 repositories, got %d", len(repos))
	}

	if repos[0].ID != "analytics" || repos[1].ID != "common" || repos[2].ID != "devices" {
		t.Fatalf("unexpected repository ordering: %#v", repos)
	}
}

func TestDiscoverReposRespectsExcludePatterns(t *testing.T) {
	workspaceRoot := t.TempDir()
	if err := InitWorkspace(workspaceRoot); err != nil {
		t.Fatalf("InitWorkspace returned error: %v", err)
	}

	for _, name := range []string{"keepme"} {
		repoPath := filepath.Join(workspaceRoot, name)
		if err := os.MkdirAll(filepath.Join(repoPath, ".git"), 0o755); err != nil {
			t.Fatalf("create repo: %v", err)
		}
	}
	if err := os.MkdirAll(filepath.Join(workspaceRoot, "skipme", ".git"), 0o755); err != nil {
		t.Fatalf("create skipped repo: %v", err)
	}

	cfg := DefaultConfig()
	cfg.RepositoryDiscovery.Exclude = []string{"skipme"}
	if err := WriteFileAtomically(DefaultWorkspaceConfigPath(workspaceRoot), mustYAML(t, cfg)); err != nil {
		t.Fatalf("write config: %v", err)
	}

	repos, err := DiscoverRepos(workspaceRoot)
	if err != nil {
		t.Fatalf("DiscoverRepos returned error: %v", err)
	}
	if len(repos) != 1 || repos[0].ID != "keepme" {
		t.Fatalf("expected only keepme repository, got %#v", repos)
	}
}

func TestDiscoverReposFindsNestedRepositories(t *testing.T) {
	workspaceRoot := t.TempDir()
	if err := InitWorkspace(workspaceRoot); err != nil {
		t.Fatalf("InitWorkspace returned error: %v", err)
	}

	if err := os.MkdirAll(filepath.Join(workspaceRoot, "repo-a", ".git"), 0o755); err != nil {
		t.Fatalf("create repo-a git dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workspaceRoot, "services", "api", ".git"), 0o755); err != nil {
		t.Fatalf("create nested repo git dir: %v", err)
	}

	repos, err := DiscoverRepos(workspaceRoot)
	if err != nil {
		t.Fatalf("DiscoverRepos returned error: %v", err)
	}
	if len(repos) != 2 {
		t.Fatalf("expected 2 repositories, got %d", len(repos))
	}

	ids := []string{repos[0].ID, repos[1].ID}
	if !(ids[0] == "repo-a" && ids[1] == "services-api") {
		t.Fatalf("unexpected repository IDs: %#v", ids)
	}
}

func TestDiscoverReposSupportsExplicitMode(t *testing.T) {
	workspaceRoot := t.TempDir()
	if err := InitWorkspace(workspaceRoot); err != nil {
		t.Fatalf("InitWorkspace returned error: %v", err)
	}

	if err := os.MkdirAll(filepath.Join(workspaceRoot, "repo-a", ".git"), 0o755); err != nil {
		t.Fatalf("create repo-a git dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workspaceRoot, "repo-b", ".git"), 0o755); err != nil {
		t.Fatalf("create repo-b git dir: %v", err)
	}

	cfg := DefaultConfig()
	cfg.RepositoryDiscovery.Mode = "explicit"
	cfg.RepositoryDiscovery.Include = []string{"repo-b"}
	if err := WriteFileAtomically(DefaultWorkspaceConfigPath(workspaceRoot), mustYAML(t, cfg)); err != nil {
		t.Fatalf("write config: %v", err)
	}

	repos, err := DiscoverRepos(workspaceRoot)
	if err != nil {
		t.Fatalf("DiscoverRepos returned error: %v", err)
	}
	if len(repos) != 1 || repos[0].ID != "repo-b" {
		t.Fatalf("expected only repo-b, got %#v", repos)
	}
}

func mustYAML(t *testing.T, v any) []byte {
	t.Helper()
	data, err := marshalYAML(v)
	if err != nil {
		t.Fatalf("marshalYAML: %v", err)
	}
	return data
}
