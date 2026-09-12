package orchestrator

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

func ResolveWorkspaceRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return wd, nil
}

func DefaultWorkspaceConfigPath(workspaceRoot string) string {
	return filepath.Join(ResolveFeatureDir(workspaceRoot), DefaultConfigFile)
}

func DefaultRepositoriesPath(workspaceRoot string) string {
	return filepath.Join(ResolveFeatureDir(workspaceRoot), DefaultRepoFile)
}

func DefaultWorkflowPath(workspaceRoot string) string {
	return filepath.Join(ResolveFeatureDir(workspaceRoot), "workflow.yaml")
}

func LoadWorkspaceConfig(workspaceRoot string) (WorkspaceConfig, error) {
	configPath := DefaultWorkspaceConfigPath(workspaceRoot)
	cfg := DefaultConfig()

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return WorkspaceConfig{}, err
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return WorkspaceConfig{}, err
	}

	if cfg.SchemaVersion == "" {
		cfg.SchemaVersion = "1.0"
	}
	if cfg.RepositoryDiscovery.Mode == "" {
		cfg.RepositoryDiscovery.Mode = "auto"
	}
	if len(cfg.RepositoryDiscovery.Include) == 0 {
		cfg.RepositoryDiscovery.Include = []string{"./*"}
	}
	if len(cfg.RepositoryDiscovery.Exclude) == 0 {
		cfg.RepositoryDiscovery.Exclude = []string{".git", "node_modules", "dist", "build", "target"}
	}
	return cfg, nil
}

func InitWorkspace(workspaceRoot string) error {
	featureDir := ResolveFeatureDir(workspaceRoot)
	if err := EnsureDir(featureDir); err != nil {
		return fmt.Errorf("create feature dir: %w", err)
	}
	for _, dir := range []string{"state", "context", "artifacts", "reviews", "logs", "tasks"} {
		if err := EnsureDir(filepath.Join(featureDir, dir)); err != nil {
			return fmt.Errorf("create %s dir: %w", dir, err)
		}
	}

	cfgPath := DefaultWorkspaceConfigPath(workspaceRoot)
	if !Exists(cfgPath) {
		cfgData, err := marshalYAML(DefaultConfig())
		if err != nil {
			return fmt.Errorf("marshal config: %w", err)
		}
		if err := WriteFileAtomically(cfgPath, cfgData); err != nil {
			return fmt.Errorf("write config: %w", err)
		}
	}

	repoPath := DefaultRepositoriesPath(workspaceRoot)
	if !Exists(repoPath) {
		repos := []Repository{}
		data, err := json.MarshalIndent(repos, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal repos: %w", err)
		}
		if err := WriteFileAtomically(repoPath, append(data, '\n')); err != nil {
			return fmt.Errorf("write repos: %w", err)
		}
	}

	workflowPath := DefaultWorkflowPath(workspaceRoot)
	if !Exists(workflowPath) {
		workflow := map[string]any{
			"schema_version": "1.0",
			"updated_at":     "",
			"last_action":    "",
		}
		data, err := marshalYAML(workflow)
		if err != nil {
			return fmt.Errorf("marshal workflow: %w", err)
		}
		if err := WriteFileAtomically(workflowPath, data); err != nil {
			return fmt.Errorf("write workflow: %w", err)
		}
	}

	return nil
}

func DiscoverRepos(workspaceRoot string) ([]Repository, error) {
	cfg, err := LoadWorkspaceConfig(workspaceRoot)
	if err != nil {
		return nil, err
	}

	repoSet := map[string]Repository{}
	if err := filepath.WalkDir(workspaceRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !d.IsDir() {
			return nil
		}

		name := d.Name()
		if name == ".git" {
			repoPath := filepath.Dir(path)
			rel, err := filepath.Rel(workspaceRoot, repoPath)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			if !shouldIncludeRepository(rel, cfg.RepositoryDiscovery) {
				return filepath.SkipDir
			}

			id := sanitizeID(strings.ReplaceAll(rel, "/", "-"))
			repoSet[repoPath] = Repository{
				ID:      id,
				Path:    repoPath,
				GitRoot: repoPath,
				Mode:    cfg.RepositoryDiscovery.Mode,
			}
			return filepath.SkipDir
		}
		if name == DefaultFeatureDir || matchesDiscoveryExclude(name, cfg.RepositoryDiscovery.Exclude) {
			return filepath.SkipDir
		}
		if name != ".git" {
			return nil
		}

		return nil
	}); err != nil {
		return nil, err
	}

	repos := make([]Repository, 0, len(repoSet))
	for _, repo := range repoSet {
		repos = append(repos, repo)
	}

	sort.Slice(repos, func(i, j int) bool {
		return repos[i].ID < repos[j].ID
	})
	return repos, nil
}

func matchesDiscoveryInclude(name string, includePatterns []string) bool {
	if len(includePatterns) == 0 {
		return true
	}
	for _, pattern := range includePatterns {
		if pattern == "./*" || pattern == "*" {
			return true
		}
		pattern = strings.TrimPrefix(pattern, "./")
		if pattern == name || pattern == "./"+name {
			return true
		}
		if ok, _ := filepath.Match(pattern, name); ok {
			return true
		}
	}
	return false
}

func shouldIncludeRepository(relativePath string, cfg RepositoryDiscoveryConfig) bool {
	if cfg.Mode == "auto" || cfg.Mode == "hybrid" {
		return true
	}
	base := filepath.Base(relativePath)
	return matchesDiscoveryInclude(relativePath, cfg.Include) || matchesDiscoveryInclude(base, cfg.Include)
}

func matchesDiscoveryExclude(name string, excludePatterns []string) bool {
	for _, pattern := range excludePatterns {
		if pattern == name || pattern == "./"+name {
			return true
		}
	}
	return false
}

func isGitRepo(path string) bool {
	_, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil
}

func sanitizeID(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, " ", "-")
	name = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, name)
	return strings.Trim(name, "-")
}

func SaveRepositories(workspaceRoot string, repos []Repository) error {
	path := DefaultRepositoriesPath(workspaceRoot)
	data, err := json.MarshalIndent(repos, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomically(path, append(data, '\n'))
}
