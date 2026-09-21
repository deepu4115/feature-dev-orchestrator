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
	ErrorCode      string              `json:"error_code,omitempty"`
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

	before := map[string]Task{}
	for _, t := range tasks {
		before[t.ID] = t
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
		if err := persistReconcileAudits(workspaceRoot, before, tasks, "execute-next"); err != nil {
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

	ws, err := LoadWorkflowState(workspaceRoot)
	if err != nil {
		return ExecuteNextResult{}, err
	}
	if ws.SchedulingPausedReason != "" && opts.TaskID == "" {
		ready, _ := ReadyTasks(workspaceRoot, tasks)
		return ExecuteNextResult{
			Action:         "scheduling_paused",
			Message:        ws.SchedulingPausedReason,
			ErrorCode:      ErrCodeSchedulingPaused,
			Reconciled:     reconcileReport.UpdatedCount,
			ReadyQueueSize: len(ready),
		}, NewCodedError(ErrCodeSchedulingPaused, ws.SchedulingPausedReason)
	}

	selectedIdx, readyCount, err := selectNextTask(workspaceRoot, tasks, opts.TaskID)
	if err != nil {
		code := CodedErrorCode(err)
		return ExecuteNextResult{
			Action:         "rejected",
			Message:        err.Error(),
			ErrorCode:      code,
			Reconciled:     reconcileReport.UpdatedCount,
			ReadyQueueSize: readyCount,
		}, err
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
			// Release lease on REWORK so resume can reclaim; do not schedule peers.
			ws, _ := LoadWorkflowState(workspaceRoot)
			if ws.ActiveTaskID == task.ID {
				ClearExecutionLease(&ws)
				_ = SaveWorkflowState(workspaceRoot, ws)
			}
			_ = SyncWorkflowFromTasks(workspaceRoot)
			return result, nil
		}

		idx = findTaskIndex(tasks, task.ID)
		if idx == -1 {
			return ExecuteNextResult{}, fmt.Errorf("task %s not found after verification", task.ID)
		}
		if tasks[idx].Status != StatusVerifying {
			return ExecuteNextResult{}, fmt.Errorf("task %s expected VERIFYING state after verification", task.ID)
		}
		tr, err := ApplyTransition(workspaceRoot, tasks, TransitionSpec{
			TaskID:     task.ID,
			To:         StatusDone,
			Reason:     "verification passed",
			Actor:      "execute-next",
			Command:    "feature-dev execute-next",
			Event:      "execute_next_completed",
			Message:    "execute-next moved task to DONE",
			ClearLease: true,
			ClearBlock: true,
		})
		if err != nil {
			return ExecuteNextResult{}, err
		}
		tasks = tr.Tasks
		tasks, unlocked, _ := PromoteTasksAfterCompletion(workspaceRoot, tasks)
		if len(unlocked) > 0 {
			if err := SaveTasks(workspaceRoot, tasks); err != nil {
				return ExecuteNextResult{}, err
			}
		}
		_ = SyncWorkflowFromTasks(workspaceRoot)
		result.StatusAfter = StatusDone
		result.Message = "verification passed and task marked DONE"
		return result, nil

	case StatusRunning:
		result.Action = "implement"
		if !opts.DryRun {
			ws, _ := LoadWorkflowState(workspaceRoot)
			_ = HeartbeatLease(&ws, task.ID, DefaultLeaseTTL, ws.UpdatedAt)
			if ws.ActiveTaskID == "" {
				_ = ClaimExecution(&ws, task.ID, "execute-next", DefaultLeaseTTL, ws.UpdatedAt)
			}
			_ = SaveWorkflowState(workspaceRoot, ws)
		}
		ctx, err := BuildTaskContext(workspaceRoot, task, opts.ContextLevel, opts.ContextBudget)
		if err != nil {
			return ExecuteNextResult{}, err
		}
		result.Context = &ctx
		result.StatusAfter = task.Status
		result.Message = "task is RUNNING; continue implementation using context"
		return result, nil

	case StatusDone:
		result.Action = "already_done"
		result.StatusAfter = StatusDone
		result.Message = "task is already DONE"
		result.ErrorCode = ErrCodeAlreadyDone
		return result, nil

	default:
		result.Action = "start"
		if opts.DryRun {
			result.StatusAfter = StatusRunning
			result.Message = "dry run: would transition task to RUNNING"
			return result, nil
		}

		// Ensure READY is persisted+audited before RUNNING when promoting.
		if task.Status != StatusReady {
			tr, err := ApplyTransition(workspaceRoot, tasks, TransitionSpec{
				TaskID:  task.ID,
				To:      StatusReady,
				Reason:  "promoted before start",
				Actor:   "execute-next",
				Command: "feature-dev execute-next",
				Event:   "task_promoted_ready",
				Message: fmt.Sprintf("status %s -> READY before start", task.Status),
			})
			if err != nil {
				return ExecuteNextResult{}, err
			}
			tasks = tr.Tasks
		}

		tr, err := ApplyTransition(workspaceRoot, tasks, TransitionSpec{
			TaskID:     task.ID,
			To:         StatusRunning,
			Reason:     "execute-next claimed task",
			Actor:      "execute-next",
			Command:    "feature-dev execute-next",
			Event:      "execute_next_started",
			Message:    "execute-next moved task to RUNNING",
			ClaimLease: true,
			ClearBlock: true,
		})
		if err != nil {
			code := CodedErrorCode(err)
			result.Action = "rejected"
			result.Message = err.Error()
			result.ErrorCode = code
			return result, err
		}
		tasks = tr.Tasks
		_ = SyncWorkflowFromTasks(workspaceRoot)

		ctx, err := BuildTaskContext(workspaceRoot, tasks[tr.Index], opts.ContextLevel, opts.ContextBudget)
		if err != nil {
			return ExecuteNextResult{}, err
		}
		result.Context = &ctx
		result.StatusAfter = StatusRunning
		result.Message = "task moved to RUNNING"
		_ = AppendCommandJournal(workspaceRoot, "execute-next", []string{task.ID}, 0, "started", "")
		return result, nil
	}
}

func selectNextTask(workspaceRoot string, tasks []Task, requestedTaskID string) (int, int, error) {
	ready, err := ReadyTasks(workspaceRoot, tasks)
	if err != nil {
		return -1, 0, err
	}
	readyCount := len(ready)

	if requestedTaskID != "" {
		idx := findTaskIndex(tasks, requestedTaskID)
		if idx == -1 {
			return -1, readyCount, fmt.Errorf("task %s not found", requestedTaskID)
		}
		task := tasks[idx]
		switch task.Status {
		case StatusDone:
			return -1, readyCount, NewCodedError(ErrCodeAlreadyDone, fmt.Sprintf("task %s is already DONE", requestedTaskID))
		case StatusFailed:
			return -1, readyCount, NewCodedError(ErrCodeTaskNotExecutable, fmt.Sprintf("task %s is FAILED; run feature-dev task resume %s", requestedTaskID, requestedTaskID))
		case StatusBlocked:
			return -1, readyCount, NewCodedError(ErrCodeTaskNotExecutable, fmt.Sprintf("task %s is BLOCKED; run feature-dev task unblock %s", requestedTaskID, requestedTaskID))
		case StatusRework:
			return -1, readyCount, NewCodedError(ErrCodeTaskNotExecutable, fmt.Sprintf("task %s requires rework; run feature-dev task resume %s", requestedTaskID, requestedTaskID))
		case StatusRunning, StatusImplemented, StatusVerifying, StatusReady:
			// executable
		case StatusReviewPending, StatusPlanned, StatusDraft:
			return -1, readyCount, NewCodedError(ErrCodeTaskNotExecutable, fmt.Sprintf("task %s is %s; run feature-dev reconcile first", requestedTaskID, task.Status))
		default:
			return -1, readyCount, NewCodedError(ErrCodeTaskNotExecutable, fmt.Sprintf("task %s status %s is not executable", requestedTaskID, task.Status))
		}

		// If another active task exists and requested is a different READY start, refuse.
		active := ActiveTasks(tasks)
		if task.Status == StatusReady {
			for _, a := range active {
				if a.ID != task.ID {
					return -1, readyCount, NewCodedError(ErrCodeWorkflowBusy, fmt.Sprintf("task %s is already active", a.ID))
				}
			}
			ws, err := LoadWorkflowState(workspaceRoot)
			if err != nil {
				return -1, readyCount, err
			}
			if LeaseHeld(ws, ws.UpdatedAt) && ws.ActiveTaskID != "" && ws.ActiveTaskID != task.ID {
				return -1, readyCount, NewCodedError(ErrCodeWorkflowBusy, fmt.Sprintf("active task %s holds execution lease", ws.ActiveTaskID))
			}
		}
		return idx, readyCount, nil
	}

	// Prefer existing active tasks (resume) over starting new READY work.
	activeIDs := make([]string, 0)
	for _, task := range tasks {
		if IsActiveStatus(task.Status) {
			activeIDs = append(activeIDs, task.ID)
		}
	}
	sort.Strings(activeIDs)
	if len(activeIDs) > 0 {
		return findTaskIndex(tasks, activeIDs[0]), readyCount, nil
	}

	schedulable, err := SchedulableTasks(workspaceRoot, tasks)
	if err != nil {
		return -1, readyCount, err
	}
	if len(schedulable) > 0 {
		return findTaskIndex(tasks, schedulable[0].ID), readyCount, nil
	}

	return -1, readyCount, nil
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
			if err != nil && result.Action == "" {
				return err
			}
			if err != nil && result.Action != "plan_not_approved" && result.Action != "rejected" && result.Action != "scheduling_paused" && result.Action != "already_done" {
				if jsonFlag {
					enc := json.NewEncoder(os.Stdout)
					enc.SetIndent("", "  ")
					_ = enc.Encode(result)
				}
				return err
			}

			if jsonFlag {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				if encErr := enc.Encode(result); encErr != nil {
					return encErr
				}
				if result.Action == "plan_not_approved" || result.Action == "rejected" || result.Action == "scheduling_paused" {
					return err
				}
				return nil
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
			if result.Action == "plan_not_approved" || result.Action == "rejected" || result.Action == "scheduling_paused" {
				return err
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
			if step.Action == "plan_not_approved" || step.Action == "rejected" || step.Action == "scheduling_paused" {
				loop.Steps = append(loop.Steps, step)
				loop.StoppedReason = step.Action
				return loop, err
			}
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
		case "already_done":
			loop.StoppedReason = "already_done"
			return loop, nil
		case "verify_failed":
			loop.VerifyFailures++
			loop.StoppedReason = "awaiting_rework"
			return loop, nil
		case "verify":
			// keep processing next deterministic step until max-steps or a stop condition.
		default:
			loop.StoppedReason = "unknown_action"
			return loop, nil
		}
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
