package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"
)

type ExecuteNextOptions struct {
	TaskID       string
	Timeout      int
	DryRun       bool
	ContextLevel string
	ContextBudget
}

type ExecuteNextResult struct {
	Action         string              `json:"action"`
	TaskID         string              `json:"task_id,omitempty"`
	StatusBefore   TaskStatus          `json:"status_before,omitempty"`
	StatusAfter    TaskStatus          `json:"status_after,omitempty"`
	Message        string              `json:"message"`
	Reconciled     int                 `json:"reconciled"`
	Context        *ContextPayload     `json:"context,omitempty"`
	Verification   *VerificationResult `json:"verification,omitempty"`
	ReadyQueueSize int                 `json:"ready_queue_size"`
}

type ExecuteLoopOptions struct {
	ExecuteNextOptions
	MaxSteps          int
	MaxVerifyFailures int
}

type ExecuteLoopResult struct {
	Steps           []ExecuteNextResult `json:"steps"`
	CompletedSteps  int                 `json:"completed_steps"`
	VerifyFailures  int                 `json:"verify_failures"`
	StoppedReason   string              `json:"stopped_reason"`
	FinalReadyQueue int                 `json:"final_ready_queue_size"`
}

func ExecuteNext(workspaceRoot string, opts ExecuteNextOptions) (ExecuteNextResult, error) {
	tasks, err := LoadTasks(workspaceRoot)
	if err != nil {
		return ExecuteNextResult{}, err
	}

	reconcileReport, err := ReconcileTasks(workspaceRoot, tasks)
	if err != nil {
		return ExecuteNextResult{}, err
	}
	tasks = reconcileReport.Tasks

	if !opts.DryRun {
		if err := SaveTasks(workspaceRoot, tasks); err != nil {
			return ExecuteNextResult{}, err
		}
	}

	if err := CanExecute(workspaceRoot); err != nil {
		ready, _ := ReadyTasks(workspaceRoot, tasks)
		return ExecuteNextResult{
			Action:         "plan_not_approved",
			Message:        err.Error(),
			Reconciled:     reconcileReport.UpdatedCount,
			ReadyQueueSize: len(ready),
		}, err
	}

	selectedIdx, readyCount, err := selectNextTask(workspaceRoot, tasks, opts.TaskID)
	if err != nil {
		return ExecuteNextResult{}, err
	}
	if selectedIdx == -1 {
		return ExecuteNextResult{
			Action:         "noop",
			Message:        "no executable task found",
			Reconciled:     reconcileReport.UpdatedCount,
			ReadyQueueSize: readyCount,
		}, nil
	}

	task := tasks[selectedIdx]
	result := ExecuteNextResult{
		TaskID:         task.ID,
		StatusBefore:   task.Status,
		Reconciled:     reconcileReport.UpdatedCount,
		ReadyQueueSize: readyCount,
	}

	switch task.Status {
	case StatusImplemented, StatusVerifying:
		result.Action = "verify"
		if opts.DryRun {
			result.StatusAfter = task.Status
			result.Message = "dry run: would verify and complete on pass"
			return result, nil
		}

		nextTasks, verification, verifyErr := verifyTask(workspaceRoot, tasks, task.ID, opts.Timeout)
		if nextTasks != nil {
			tasks = nextTasks
		}
		result.Verification = &verification
		idx := findTaskIndex(tasks, task.ID)
		if idx >= 0 {
			result.StatusAfter = tasks[idx].Status
		}
		if verifyErr != nil {
			result.Action = "verify_failed"
			result.Message = verifyErr.Error()
			if err := SaveTasks(workspaceRoot, tasks); err != nil {
				return ExecuteNextResult{}, err
			}
			return result, nil
		}

		idx = findTaskIndex(tasks, task.ID)
		if idx == -1 {
			return ExecuteNextResult{}, fmt.Errorf("task %s not found after verification", task.ID)
		}
		if tasks[idx].Status != StatusVerifying {
			return ExecuteNextResult{}, fmt.Errorf("task %s expected VERIFYING state after verification", task.ID)
		}
		if err := tasks[idx].TransitionTo(StatusDone); err != nil {
			return ExecuteNextResult{}, err
		}
		if err := AppendTaskSummary(workspaceRoot, tasks[idx], "execute_next_completed", "execute-next moved task to DONE"); err != nil {
			return ExecuteNextResult{}, err
		}
		if err := SaveTasks(workspaceRoot, tasks); err != nil {
			return ExecuteNextResult{}, err
		}
		tasks, unlocked, _ := PromoteTasksAfterCompletion(workspaceRoot, tasks)
		if len(unlocked) > 0 {
			if err := SaveTasks(workspaceRoot, tasks); err != nil {
				return ExecuteNextResult{}, err
			}
		}
		_ = SyncWorkflowFromTasks(workspaceRoot)
		result.StatusAfter = tasks[idx].Status
		result.Message = "verification passed and task marked DONE"
		return result, nil

	case StatusRunning:
		result.Action = "implement"
		ctx, err := BuildTaskContext(workspaceRoot, task, opts.ContextLevel, opts.ContextBudget)
		if err != nil {
			return ExecuteNextResult{}, err
		}
		result.Context = &ctx
		result.StatusAfter = task.Status
		result.Message = "task is RUNNING; continue implementation using context"
		return result, nil

	default:
		result.Action = "start"
		if opts.DryRun {
			result.StatusAfter = StatusRunning
			result.Message = "dry run: would transition task to RUNNING"
			return result, nil
		}
		if task.Status != StatusReady {
			if err := tasks[selectedIdx].TransitionTo(StatusReady); err != nil {
				return ExecuteNextResult{}, err
			}
		}
		if err := tasks[selectedIdx].TransitionTo(StatusRunning); err != nil {
			return ExecuteNextResult{}, err
		}
		if err := AppendTaskSummary(workspaceRoot, tasks[selectedIdx], "execute_next_started", "execute-next moved task to RUNNING"); err != nil {
			return ExecuteNextResult{}, err
		}
		if err := SaveTasks(workspaceRoot, tasks); err != nil {
			return ExecuteNextResult{}, err
		}
		_ = SyncWorkflowFromTasks(workspaceRoot)

		ctx, err := BuildTaskContext(workspaceRoot, tasks[selectedIdx], opts.ContextLevel, opts.ContextBudget)
		if err != nil {
			return ExecuteNextResult{}, err
		}
		result.Context = &ctx
		result.StatusAfter = tasks[selectedIdx].Status
		result.Message = "task moved to RUNNING"
		return result, nil
	}
}

func selectNextTask(workspaceRoot string, tasks []Task, requestedTaskID string) (int, int, error) {
	if requestedTaskID != "" {
		for i, task := range tasks {
			if task.ID == requestedTaskID {
				return i, 0, nil
			}
		}
		return -1, 0, fmt.Errorf("task %s not found", requestedTaskID)
	}

	ready, err := ReadyTasks(workspaceRoot, tasks)
	if err != nil {
		return -1, 0, err
	}
	readyIDs := make([]string, 0, len(ready))
	for _, task := range ready {
		readyIDs = append(readyIDs, task.ID)
	}
	sort.Strings(readyIDs)

	for _, task := range tasks {
		if task.Status == StatusImplemented || task.Status == StatusVerifying {
			return findTaskIndex(tasks, task.ID), len(readyIDs), nil
		}
	}

	if len(readyIDs) > 0 {
		return findTaskIndex(tasks, readyIDs[0]), len(readyIDs), nil
	}

	runningIDs := make([]string, 0)
	for _, task := range tasks {
		if task.Status == StatusRunning {
			runningIDs = append(runningIDs, task.ID)
		}
	}
	sort.Strings(runningIDs)
	if len(runningIDs) > 0 {
		return findTaskIndex(tasks, runningIDs[0]), 0, nil
	}

	return -1, 0, nil
}

func BuildExecuteNextCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "execute-next",
		Short: "Run one autonomous orchestration step",
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceRoot, err := ResolveWorkspaceRoot()
			if err != nil {
				return err
			}

			taskID, _ := cmd.Flags().GetString("task")
			timeoutSeconds, _ := cmd.Flags().GetInt("timeout")
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			level, _ := cmd.Flags().GetString("level")
			maxFiles, _ := cmd.Flags().GetInt("max-files")
			maxChars, _ := cmd.Flags().GetInt("max-chars")
			maxFileChars, _ := cmd.Flags().GetInt("max-file-chars")
			jsonFlag, _ := cmd.Flags().GetBool("json")

			result, err := ExecuteNext(workspaceRoot, ExecuteNextOptions{
				TaskID:       taskID,
				Timeout:      timeoutSeconds,
				DryRun:       dryRun,
				ContextLevel: level,
				ContextBudget: ContextBudget{
					MaxFiles:      maxFiles,
					MaxTotalChars: maxChars,
					MaxFileChars:  maxFileChars,
				},
			})
			if err != nil {
				return err
			}

			if jsonFlag {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(result)
			}

			fmt.Printf("action: %s\n", result.Action)
			if result.TaskID != "" {
				fmt.Printf("task: %s\n", result.TaskID)
			}
			if result.StatusBefore != "" || result.StatusAfter != "" {
				fmt.Printf("status: %s -> %s\n", result.StatusBefore, result.StatusAfter)
			}
			fmt.Printf("reconciled tasks: %d\n", result.Reconciled)
			fmt.Printf("ready queue size: %d\n", result.ReadyQueueSize)
			fmt.Printf("message: %s\n", result.Message)
			if result.Context != nil {
				fmt.Println("context:")
				fmt.Println(RenderContextText(*result.Context))
			}
			if result.Verification != nil {
				fmt.Printf("verification exit code: %d\n", result.Verification.ExitCode)
			}
			return nil
		},
	}

	cmd.Flags().String("task", "", "Optional task ID to execute")
	cmd.Flags().Int("timeout", defaultVerifyTimeout, "Verification timeout in seconds")
	cmd.Flags().Bool("dry-run", false, "Show planned action without mutating task state")
	cmd.Flags().Bool("json", false, "Emit result as JSON")
	cmd.Flags().String("level", "brief", "Context detail level: brief or full")
	cmd.Flags().Int("max-files", DefaultContextBudget().MaxFiles, "Maximum number of files to include for full context")
	cmd.Flags().Int("max-chars", DefaultContextBudget().MaxTotalChars, "Maximum total snippet characters for full context")
	cmd.Flags().Int("max-file-chars", DefaultContextBudget().MaxFileChars, "Maximum characters per snippet file")
	return cmd
}

func ExecuteLoop(workspaceRoot string, opts ExecuteLoopOptions) (ExecuteLoopResult, error) {
	if opts.MaxSteps <= 0 {
		opts.MaxSteps = 3
	}
	if opts.MaxVerifyFailures <= 0 {
		opts.MaxVerifyFailures = 2
	}

	loop := ExecuteLoopResult{Steps: make([]ExecuteNextResult, 0, opts.MaxSteps)}
	for i := 0; i < opts.MaxSteps; i++ {
		step, err := ExecuteNext(workspaceRoot, opts.ExecuteNextOptions)
		if err != nil {
			if len(loop.Steps) == 0 && step.Action == "plan_not_approved" {
				loop.Steps = append(loop.Steps, step)
				loop.StoppedReason = "plan_not_approved"
				return loop, err
			}
			return loop, err
		}
		loop.Steps = append(loop.Steps, step)
		loop.CompletedSteps++
		loop.FinalReadyQueue = step.ReadyQueueSize

		switch step.Action {
		case "noop":
			tasks, _ := LoadTasks(workspaceRoot)
			if allTasksInStatus(tasks, StatusDone) {
				if fin, finErr := MaybeAutoCompleteFeature(workspaceRoot, opts.Timeout); finErr == nil && fin.Completed {
					loop.StoppedReason = "feature_completed"
					return loop, nil
				}
				loop.StoppedReason = "awaiting_final_verification"
				return loop, nil
			}
			loop.StoppedReason = "no_executable_task"
			return loop, nil
		case "plan_not_approved":
			loop.StoppedReason = "plan_not_approved"
			return loop, fmt.Errorf("%s", step.Message)
		case "implement", "start":
			loop.StoppedReason = "awaiting_code_changes"
			return loop, nil
		case "verify_failed":
			loop.VerifyFailures++
			if loop.VerifyFailures >= opts.MaxVerifyFailures {
				loop.StoppedReason = "verify_failure_budget_reached"
				return loop, nil
			}
		case "verify":
			// keep processing next deterministic step until max-steps or a stop condition.
		default:
			loop.StoppedReason = "unknown_action"
			return loop, nil
		}

		// When a specific task is requested, keep loop focused on the same task.
	}

	loop.StoppedReason = "max_steps_reached"
	return loop, nil
}

func BuildExecuteLoopCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "execute-loop",
		Short: "Run bounded autonomous orchestration steps",
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceRoot, err := ResolveWorkspaceRoot()
			if err != nil {
				return err
			}

			taskID, _ := cmd.Flags().GetString("task")
			timeoutSeconds, _ := cmd.Flags().GetInt("timeout")
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			level, _ := cmd.Flags().GetString("level")
			maxFiles, _ := cmd.Flags().GetInt("max-files")
			maxChars, _ := cmd.Flags().GetInt("max-chars")
			maxFileChars, _ := cmd.Flags().GetInt("max-file-chars")
			maxSteps, _ := cmd.Flags().GetInt("max-steps")
			maxVerifyFailures, _ := cmd.Flags().GetInt("max-verify-failures")
			jsonFlag, _ := cmd.Flags().GetBool("json")

			result, err := ExecuteLoop(workspaceRoot, ExecuteLoopOptions{
				ExecuteNextOptions: ExecuteNextOptions{
					TaskID:       taskID,
					Timeout:      timeoutSeconds,
					DryRun:       dryRun,
					ContextLevel: level,
					ContextBudget: ContextBudget{
						MaxFiles:      maxFiles,
						MaxTotalChars: maxChars,
						MaxFileChars:  maxFileChars,
					},
				},
				MaxSteps:          maxSteps,
				MaxVerifyFailures: maxVerifyFailures,
			})
			if err != nil {
				return err
			}

			if jsonFlag {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(result)
			}

			fmt.Printf("completed steps: %d\n", result.CompletedSteps)
			fmt.Printf("verify failures: %d\n", result.VerifyFailures)
			fmt.Printf("final ready queue size: %d\n", result.FinalReadyQueue)
			fmt.Printf("stopped reason: %s\n", result.StoppedReason)
			for i, step := range result.Steps {
				fmt.Printf("step %d: action=%s task=%s status=%s->%s\n", i+1, step.Action, step.TaskID, step.StatusBefore, step.StatusAfter)
			}
			return nil
		},
	}

	cmd.Flags().String("task", "", "Optional task ID to execute")
	cmd.Flags().Int("timeout", defaultVerifyTimeout, "Verification timeout in seconds")
	cmd.Flags().Bool("dry-run", false, "Show planned actions without mutating task state")
	cmd.Flags().Bool("json", false, "Emit result as JSON")
	cmd.Flags().String("level", "brief", "Context detail level: brief or full")
	cmd.Flags().Int("max-files", DefaultContextBudget().MaxFiles, "Maximum number of files to include for full context")
	cmd.Flags().Int("max-chars", DefaultContextBudget().MaxTotalChars, "Maximum total snippet characters for full context")
	cmd.Flags().Int("max-file-chars", DefaultContextBudget().MaxFileChars, "Maximum characters per snippet file")
	cmd.Flags().Int("max-steps", 3, "Maximum autonomous steps to run in one loop")
	cmd.Flags().Int("max-verify-failures", 2, "Maximum verification failures allowed before stopping")
	return cmd
}
