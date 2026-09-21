package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const defaultVerifyTimeout = 600

func RunVerification(repoPath, command string, timeoutSeconds int) (VerificationResult, error) {
	if timeoutSeconds <= 0 {
		timeoutSeconds = defaultVerifyTimeout
	}

	result := VerificationResult{
		RepositoryID:   filepath.Base(repoPath),
		WorkingDir:     repoPath,
		BuildSystem:    DetectBuildSystem(repoPath),
		Command:        command,
		TimeoutSeconds: timeoutSeconds,
		ExecutedAt:     time.Now().UTC(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = repoPath
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			result.ExitCode = 124
			result.Output = strings.TrimSpace(stdout.String() + "\n" + stderr.String() + "\nverification timed out")
			return result, fmt.Errorf("verification command timed out after %d seconds", timeoutSeconds)
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = 1
		}
		result.Output = strings.TrimSpace(stdout.String() + "\n" + stderr.String())
		return result, fmt.Errorf("verification command failed: %w", err)
	}

	result.ExitCode = 0
	result.Output = strings.TrimSpace(stdout.String() + "\n" + stderr.String())
	return result, nil
}

func defaultVerificationCommand(repoPath string) string {
	buildSystem := DetectBuildSystem(repoPath)
	switch buildSystem {
	case "maven":
		if Exists(filepath.Join(repoPath, "mvnw")) {
			return "./mvnw test"
		}
		return "mvn test"
	case "gradle":
		if Exists(filepath.Join(repoPath, "gradlew")) {
			return "./gradlew test"
		}
		return "gradle test"
	case "npm":
		return "npm test -- --runInBand"
	case "go":
		return "go test ./..."
	case "cargo":
		return "cargo test"
	default:
		return "echo verification not configured"
	}
}

func saveVerificationResult(workspaceRoot string, result VerificationResult) error {
	artifactDir := filepath.Join(ResolveFeatureDir(workspaceRoot), "artifacts")
	if err := EnsureDir(artifactDir); err != nil {
		return fmt.Errorf("create artifact dir: %w", err)
	}

	stamp := result.ExecutedAt.Format("20060102T150405Z")
	name := fmt.Sprintf("verify-%s-%s.json", strings.ToLower(result.TaskID), stamp)
	path := filepath.Join(artifactDir, name)
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal verification result: %w", err)
	}
	if err := WriteFileAtomically(path, append(data, '\n')); err != nil {
		return fmt.Errorf("write verification artifact: %w", err)
	}
	return nil
}

func verifyTask(workspaceRoot string, tasks []Task, taskID string, timeoutSeconds int) ([]Task, VerificationResult, error) {
	idx := -1
	for i, task := range tasks {
		if task.ID == taskID {
			idx = i
			break
		}
	}
	if idx == -1 {
		return nil, VerificationResult{}, fmt.Errorf("task %s not found", taskID)
	}

	target := tasks[idx]
	if target.Repository == "" || target.Repository == "workspace" {
		return nil, VerificationResult{}, fmt.Errorf("verification requires a repository-scoped task")
	}
	if target.Status != StatusImplemented && target.Status != StatusVerifying {
		return nil, VerificationResult{}, fmt.Errorf("task %s must be IMPLEMENTED or VERIFYING before verification", target.ID)
	}

	repoPath, err := ResolveVerificationDirectory(workspaceRoot, target)
	if err != nil {
		return nil, VerificationResult{}, err
	}
	if _, err := os.Stat(repoPath); err != nil {
		return nil, VerificationResult{}, fmt.Errorf("verification directory %s is not available: %w", repoPath, err)
	}

	if target.Status == StatusImplemented {
		tr, err := ApplyTransition(workspaceRoot, tasks, TransitionSpec{
			TaskID:         taskID,
			To:             StatusVerifying,
			Reason:         "starting verification",
			Actor:          "verify",
			Command:        "feature-dev verify",
			Event:          "verification_started",
			Message:        "task moved to VERIFYING",
			ClaimLease:     true,
			HeartbeatLease: false,
		})
		if err != nil {
			return nil, VerificationResult{}, err
		}
		tasks = tr.Tasks
		idx = tr.Index
	} else {
		ws, _ := LoadWorkflowState(workspaceRoot)
		now := time.Now().UTC()
		_ = HeartbeatLease(&ws, taskID, DefaultLeaseTTL, now)
		_ = SaveWorkflowState(workspaceRoot, ws)
	}

	command := defaultVerificationCommand(repoPath)
	for _, step := range tasks[idx].Verification {
		candidate := strings.TrimSpace(step.Command)
		if candidate != "" {
			command = NormalizeVerificationCommand(tasks[idx], candidate)
			break
		}
	}
	result, err := RunVerification(repoPath, command, timeoutSeconds)
	result.TaskID = target.ID
	result.RepositoryID = target.Repository
	if saveErr := saveVerificationResult(workspaceRoot, result); saveErr != nil {
		return nil, result, saveErr
	}

	if err != nil {
		tr, trErr := ApplyTransition(workspaceRoot, tasks, TransitionSpec{
			TaskID:          taskID,
			To:              StatusRework,
			Reason:          fmt.Sprintf("verification failed exit %d", result.ExitCode),
			Actor:           "verify",
			Command:         "feature-dev verify",
			Event:           "verification_failed",
			Message:         fmt.Sprintf("verify command failed with exit code %d", result.ExitCode),
			VerifierCommand: command,
			ClearLease:      true,
			ClearBlock:      true,
		})
		if trErr != nil {
			_ = ApplyTransitionInMemory(&tasks[idx], StatusRework, nil, true, time.Now().UTC())
			_ = AppendTaskSummary(workspaceRoot, tasks[idx], "verification_failed", fmt.Sprintf("verify command failed with exit code %d", result.ExitCode))
			return tasks, result, err
		}
		return tr.Tasks, result, err
	}

	entry := TaskSummaryRecord{
		Timestamp:       time.Now().UTC(),
		TaskID:          tasks[idx].ID,
		Repository:      tasks[idx].Repository,
		Status:          tasks[idx].Status,
		Event:           "verification_passed",
		Message:         "verification command completed successfully",
		Reason:          "verification passed",
		Actor:           "verify",
		Command:         "feature-dev verify",
		VerifierCommand: command,
	}
	if appendErr := appendTaskSummaryRecord(workspaceRoot, entry); appendErr != nil {
		return tasks, result, appendErr
	}

	return tasks, result, nil
}

func BuildVerifyCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "verify <task-id>",
		Short: "Verify a task using repo-aware checks",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			timeoutSeconds, _ := cmd.Flags().GetInt("timeout")
			workspaceRoot, err := ResolveWorkspaceRoot()
			if err != nil {
				return err
			}
			tasks, err := LoadTasks(workspaceRoot)
			if err != nil {
				return err
			}
			tasks, result, err := verifyTask(workspaceRoot, tasks, args[0], timeoutSeconds)
			if tasks != nil {
				if saveErr := SaveTasks(workspaceRoot, tasks); saveErr != nil {
					return saveErr
				}
			}
			if err != nil {
				return err
			}
			fmt.Printf("repository: %s\n", result.RepositoryID)
			fmt.Printf("build system: %s\n", result.BuildSystem)
			fmt.Printf("exit code: %d\n", result.ExitCode)
			fmt.Printf("timeout: %ds\n", result.TimeoutSeconds)
			fmt.Printf("output:\n%s\n", result.Output)
			return nil
		},
	}
	cmd.Flags().Int("timeout", defaultVerifyTimeout, "Verification timeout in seconds")
	return cmd
}

func BuildVerifyWorkspaceCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "verify-workspace",
		Short: "Run verification for all IMPLEMENTED or VERIFYING repository-scoped tasks",
		RunE: func(cmd *cobra.Command, args []string) error {
			timeoutSeconds, _ := cmd.Flags().GetInt("timeout")
			workspaceRoot, err := ResolveWorkspaceRoot()
			if err != nil {
				return err
			}
			tasks, err := LoadTasks(workspaceRoot)
			if err != nil {
				return err
			}

			verified := 0
			for _, task := range tasks {
				if task.Repository == "" || task.Repository == "workspace" {
					continue
				}
				if task.Status != StatusImplemented && task.Status != StatusVerifying {
					continue
				}

				nextTasks, result, verifyErr := verifyTask(workspaceRoot, tasks, task.ID, timeoutSeconds)
				if nextTasks != nil {
					tasks = nextTasks
				}
				if verifyErr != nil {
					if saveErr := SaveTasks(workspaceRoot, tasks); saveErr != nil {
						return saveErr
					}
					return fmt.Errorf("verify %s in %s failed: %w", task.ID, result.RepositoryID, verifyErr)
				}
				verified++
			}

			if err := SaveTasks(workspaceRoot, tasks); err != nil {
				return err
			}
			fmt.Printf("verified %d tasks\n", verified)
			return nil
		},
	}
	cmd.Flags().Int("timeout", defaultVerifyTimeout, "Verification timeout in seconds")
	return cmd
}

// ValidateVerificationCommandAvailable checks that the command's first token exists in the repo or PATH.
func ValidateVerificationCommandAvailable(repoPath, command string) error {
	command = strings.TrimSpace(command)
	if command == "" {
		return fmt.Errorf("empty verification command")
	}
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return fmt.Errorf("empty verification command")
	}
	bin := fields[0]
	switch {
	case strings.HasPrefix(bin, "./") || strings.HasPrefix(bin, "../"):
		full := filepath.Join(repoPath, bin)
		if !Exists(full) {
			return NewCodedError(ErrCodeVerificationCmdMissing, fmt.Sprintf("verification binary %s not found in %s", bin, repoPath))
		}
	case bin == "mvnw" || bin == "gradlew":
		if !Exists(filepath.Join(repoPath, bin)) {
			return NewCodedError(ErrCodeVerificationCmdMissing, fmt.Sprintf("verification wrapper %s not found in %s", bin, repoPath))
		}
	default:
		if _, err := exec.LookPath(bin); err != nil {
			// local wrappers without ./
			if Exists(filepath.Join(repoPath, bin)) {
				return nil
			}
			return NewCodedError(ErrCodeVerificationCmdMissing, fmt.Sprintf("verification command %q not found on PATH or in repo", bin))
		}
	}
	return nil
}
