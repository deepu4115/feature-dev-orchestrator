package orchestrator

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func GitOutput(repoPath string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", repoPath}, args...)...)
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		if errOut.Len() > 0 {
			return "", fmt.Errorf("git %s failed: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(errOut.String()))
		}
		return "", fmt.Errorf("git %s failed: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(out.String()), nil
}

func DetectBuildSystem(repoPath string) string {
	for _, candidate := range []string{"pom.xml", "build.gradle", "build.gradle.kts", "package.json", "go.mod", "Cargo.toml"} {
		if _, err := os.Stat(filepath.Join(repoPath, candidate)); err == nil {
			switch candidate {
			case "pom.xml":
				return "maven"
			case "build.gradle", "build.gradle.kts":
				return "gradle"
			case "package.json":
				return "npm"
			case "go.mod":
				return "go"
			case "Cargo.toml":
				return "cargo"
			}
		}
	}
	return "unknown"
}

func CaptureRepositoryState(repoPath string) (RepositoryState, error) {
	repoState := RepositoryState{
		Path:    repoPath,
		GitRoot: repoPath,
	}

	branch, err := GitOutput(repoPath, "branch", "--show-current")
	if err != nil {
		return repoState, err
	}
	repoState.Branch = branch

	head, err := GitOutput(repoPath, "rev-parse", "HEAD")
	if err != nil {
		head = ""
	}
	repoState.Head = head

	status, err := GitOutput(repoPath, "status", "--short")
	if err != nil {
		return repoState, err
	}
	repoState.Status = status

	changedFiles, err := GitOutput(repoPath, "diff", "--name-only")
	if err != nil {
		return repoState, err
	}
	if changedFiles != "" {
		repoState.ChangedFiles = strings.Fields(changedFiles)
	}

	untracked, err := GitOutput(repoPath, "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return repoState, err
	}
	if untracked != "" {
		repoState.UntrackedFiles = strings.Fields(untracked)
	}

	diffSummary, err := GitOutput(repoPath, "diff", "--stat")
	if err != nil {
		return repoState, err
	}
	repoState.DiffSummary = diffSummary

	mergeState, err := GitOutput(repoPath, "rev-parse", "--git-dir")
	if err == nil {
		repoState.MergeState = mergeState
	}

	repoState.UpdatedAt = time.Now().UTC()
	return repoState, nil
}
