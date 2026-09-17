package orchestrator

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	VerificationCWDRepositoryRoot = "repository_root"
	VerificationCWDWorkspaceRoot  = "workspace_root"
	VerificationCWDCustom         = "custom"
)

func DefaultVerificationWorkingDirectory(task Task) string {
	if task.VerificationWorkingDirectory != "" {
		return task.VerificationWorkingDirectory
	}
	if task.Repository == "workspace" || task.Repository == "workspace-root" {
		return VerificationCWDWorkspaceRoot
	}
	return VerificationCWDRepositoryRoot
}

func ResolveVerificationDirectory(workspaceRoot string, task Task) (string, error) {
	cwd := DefaultVerificationWorkingDirectory(task)
	switch cwd {
	case VerificationCWDWorkspaceRoot:
		return workspaceRoot, nil
	case VerificationCWDCustom:
		if task.WorkingDirectory == "" {
			return "", fmt.Errorf("task %s requires working_directory when verification_working_directory is custom", task.ID)
		}
		if filepath.IsAbs(task.WorkingDirectory) {
			return task.WorkingDirectory, nil
		}
		return filepath.Join(workspaceRoot, task.WorkingDirectory), nil
	default:
		if task.Repository == "" || task.Repository == "workspace" {
			return workspaceRoot, nil
		}
		repoPath := filepathJoinRepo(workspaceRoot, task.Repository)
		if repoPath == "" {
			return filepath.Join(workspaceRoot, task.Repository), nil
		}
		return repoPath, nil
	}
}

var redundantCdPattern = regexp.MustCompile(`(?i)^cd\s+[^\s&]+\s*&&\s*`)

func ValidateVerificationCommands(task Task) ([]PlanValidationIssue, []string) {
	issues := []PlanValidationIssue{}
	warnings := []string{}
	cwd := DefaultVerificationWorkingDirectory(task)
	for i, step := range task.Verification {
		cmd := strings.TrimSpace(step.Command)
		if cmd == "" {
			continue
		}
		if cwd == VerificationCWDRepositoryRoot && redundantCdPattern.MatchString(cmd) {
			normalized := redundantCdPattern.ReplaceAllString(cmd, "")
			normalized = strings.TrimSpace(normalized)
			issues = append(issues, PlanValidationIssue{
				Level:   "warning",
				Code:    "redundant_repository_cd_prefix",
				Message: fmt.Sprintf("task %s verification[%d] uses redundant cd prefix; commands run from repository root (use: %s)", task.ID, i, normalized),
			})
			warnings = append(warnings, normalized)
		}
	}
	return issues, warnings
}

func NormalizeVerificationCommand(task Task, command string) string {
	if DefaultVerificationWorkingDirectory(task) != VerificationCWDRepositoryRoot {
		return command
	}
	return strings.TrimSpace(redundantCdPattern.ReplaceAllString(command, ""))
}
