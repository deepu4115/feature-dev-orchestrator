package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func BuildTaskCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "task",
		Short: "Manage workspace tasks",
	}
	cmd.AddCommand(BuildTaskInitCommand())
	cmd.AddCommand(BuildAddTaskCommand())
	cmd.AddCommand(BuildListTasksCommand())
	cmd.AddCommand(BuildReadyTaskListCommand())
	cmd.AddCommand(BuildTaskPreviewCommand())
	cmd.AddCommand(BuildStartTaskCommand())
	cmd.AddCommand(BuildImplementTaskCommand())
	cmd.AddCommand(BuildCompleteTaskCommand())
	cmd.AddCommand(BuildFailTaskCommand())
	cmd.AddCommand(BuildTaskExplainCommand())
	cmd.AddCommand(BuildTaskValidateCommand())
	cmd.AddCommand(BuildTaskUnblockCommand())
	cmd.AddCommand(BuildTaskResumeCommand())
	cmd.AddCommand(BuildTaskRecoverCommand())
	return cmd
}

func BuildAddTaskCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <task-id> <title>",
		Short: "Add a task to the workspace task list",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceRoot, err := ResolveWorkspaceRoot()
			if err != nil {
				return err
			}
			tasks, err := LoadTasks(workspaceRoot)
			if err != nil {
				return err
			}
			task := Task{
				ID:         args[0],
				Title:      strings.Join(args[1:], " "),
				Repository: "workspace",
				Status:     StatusPlanned,
			}
			for _, existing := range tasks {
				if existing.ID == task.ID {
					return fmt.Errorf("task %s already exists", task.ID)
				}
			}
			tasks = append(tasks, task)
			if err := SaveTasks(workspaceRoot, tasks); err != nil {
				return err
			}
			if err := AppendTaskSummary(workspaceRoot, task, "task_added", "task registered in workspace"); err != nil {
				return err
			}
			fmt.Printf("added task %s\n", task.ID)
			return nil
		},
	}
	return cmd
}

func BuildListTasksCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all tasks",
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceRoot, err := ResolveWorkspaceRoot()
			if err != nil {
				return err
			}
			tasks, err := LoadTasks(workspaceRoot)
			if err != nil {
				return err
			}
			jsonFlag, _ := cmd.Flags().GetBool("json")
			if jsonFlag {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(tasks)
			}
			for _, task := range tasks {
				fmt.Println(task.String())
			}
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "Emit the task list as JSON")
	return cmd
}

func BuildReadyTaskListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ready",
		Short: "List tasks that are currently ready to work on",
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceRoot, err := ResolveWorkspaceRoot()
			if err != nil {
				return err
			}
			tasks, err := LoadTasks(workspaceRoot)
			if err != nil {
				return err
			}
			ready, err := ReadyTasks(workspaceRoot, tasks)
			if err != nil {
				return err
			}
			jsonFlag, _ := cmd.Flags().GetBool("json")
			if jsonFlag {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(ready)
			}
			for _, task := range ready {
				fmt.Println(task.String())
			}
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "Emit ready tasks as JSON")
	return cmd
}

func BuildStartTaskCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start <task-id>",
		Short: "Transition a ready task into RUNNING",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceRoot, err := ResolveWorkspaceRoot()
			if err != nil {
				return err
			}
			tasks, err := LoadTasks(workspaceRoot)
			if err != nil {
				return err
			}
			idx := findTaskIndex(tasks, args[0])
			if idx == -1 {
				return fmt.Errorf("task %s not found", args[0])
			}

			if err := CanExecute(workspaceRoot); err != nil {
				return err
			}

			ws, err := LoadWorkflowState(workspaceRoot)
			if err != nil {
				return err
			}

			ready, err := ReadyTasks(workspaceRoot, tasks)
			if err != nil {
				return err
			}
			isReady := false
			for _, task := range ready {
				if task.ID == args[0] {
					isReady = true
					break
				}
			}
			if !isReady && tasks[idx].Status != StatusReady {
				return fmt.Errorf("task %s is not ready", args[0])
			}

			if RequiresPlanApproval(ws) {
				if tasks[idx].Status != StatusReady {
					return fmt.Errorf("task %s is not ready", args[0])
				}
				if err := CanStartTaskInWorkspace(workspaceRoot, tasks[idx]); err != nil {
					return err
				}
			}

			if HasActiveTask(tasks) {
				for _, a := range ActiveTasks(tasks) {
					if a.ID != args[0] {
						return NewCodedError(ErrCodeWorkflowBusy, fmt.Sprintf("task %s is already active", a.ID))
					}
				}
			}

			if tasks[idx].Status != StatusReady {
				tr, err := ApplyTransition(workspaceRoot, tasks, TransitionSpec{
					TaskID:  args[0],
					To:      StatusReady,
					Reason:  "promoted before start",
					Actor:   "task start",
					Command: "feature-dev task start",
					Event:   "task_promoted_ready",
				})
				if err != nil {
					return err
				}
				tasks = tr.Tasks
			}
			tr, err := ApplyTransition(workspaceRoot, tasks, TransitionSpec{
				TaskID:     args[0],
				To:         StatusRunning,
				Reason:     "task start claimed execution",
				Actor:      "task start",
				Command:    "feature-dev task start",
				Event:      "task_started",
				Message:    "task moved to RUNNING",
				ClaimLease: true,
				ClearBlock: true,
			})
			if err != nil {
				return err
			}
			_ = SyncWorkflowFromTasks(workspaceRoot)
			_ = tr
			fmt.Printf("task %s is now RUNNING\n", args[0])
			return nil
		},
	}
	return cmd
}

func ImplementTask(workspaceRoot string, taskID string) error {
	if err := CanExecute(workspaceRoot); err != nil {
		return err
	}

	tasks, err := LoadTasks(workspaceRoot)
	if err != nil {
		return err
	}
	idx := findTaskIndex(tasks, taskID)
	if idx == -1 {
		return fmt.Errorf("task %s not found", taskID)
	}
	if tasks[idx].Status != StatusRunning {
		return fmt.Errorf("task %s must be RUNNING to mark implemented (current: %s)", taskID, tasks[idx].Status)
	}
	_, err = ApplyTransition(workspaceRoot, tasks, TransitionSpec{
		TaskID:         taskID,
		To:             StatusImplemented,
		Reason:         "implementation marked complete",
		Actor:          "task implement",
		Command:        "feature-dev task implement",
		Event:          "task_implemented",
		Message:        "task moved to IMPLEMENTED",
		HeartbeatLease: true,
	})
	if err != nil {
		return err
	}
	_ = SyncWorkflowFromTasks(workspaceRoot)
	return nil
}

func BuildImplementTaskCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "implement <task-id>",
		Short: "Mark a running task as IMPLEMENTED after code changes",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceRoot, err := ResolveWorkspaceRoot()
			if err != nil {
				return err
			}
			if err := ImplementTask(workspaceRoot, args[0]); err != nil {
				return err
			}
			fmt.Printf("task %s is now IMPLEMENTED\n", args[0])
			return nil
		},
	}
	return cmd
}

func BuildCompleteTaskCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "complete <task-id>",
		Short: "Mark a verified task as DONE",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceRoot, err := ResolveWorkspaceRoot()
			if err != nil {
				return err
			}
			tasks, err := LoadTasks(workspaceRoot)
			if err != nil {
				return err
			}
			idx := findTaskIndex(tasks, args[0])
			if idx == -1 {
				return fmt.Errorf("task %s not found", args[0])
			}

			switch tasks[idx].Status {
			case StatusVerifying:
				if _, err := ApplyTransition(workspaceRoot, tasks, TransitionSpec{
					TaskID: args[0], To: StatusDone, Reason: "task complete",
					Actor: "task complete", Command: "feature-dev task complete",
					Event: "task_completed", Message: "task moved to DONE", ClearLease: true, ClearBlock: true,
				}); err != nil {
					return err
				}
			case StatusImplemented:
				tr, err := ApplyTransition(workspaceRoot, tasks, TransitionSpec{
					TaskID: args[0], To: StatusVerifying, Reason: "complete via verifying",
					Actor: "task complete", Command: "feature-dev task complete",
					Event: "verification_started", HeartbeatLease: true,
				})
				if err != nil {
					return err
				}
				tasks = tr.Tasks
				if _, err := ApplyTransition(workspaceRoot, tasks, TransitionSpec{
					TaskID: args[0], To: StatusDone, Reason: "task complete",
					Actor: "task complete", Command: "feature-dev task complete",
					Event: "task_completed", Message: "task moved to DONE", ClearLease: true, ClearBlock: true,
				}); err != nil {
					return err
				}
			default:
				return fmt.Errorf("task %s must be IMPLEMENTED or VERIFYING to complete", args[0])
			}

			tasks, err = LoadTasks(workspaceRoot)
			if err != nil {
				return err
			}
			tasks, unlocked, _ := PromoteTasksAfterCompletion(workspaceRoot, tasks)
			if len(unlocked) > 0 {
				if err := SaveTasks(workspaceRoot, tasks); err != nil {
					return err
				}
			}
			_ = SyncWorkflowFromTasks(workspaceRoot)
			fmt.Printf("task %s is now DONE\n", args[0])
			return nil
		},
	}
	return cmd
}

func BuildTaskExplainCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "explain <task-id>",
		Short: "Explain why a task is or is not ready",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceRoot, err := ResolveWorkspaceRoot()
			if err != nil {
				return err
			}
			result, err := ExplainTask(workspaceRoot, args[0])
			if err != nil {
				return err
			}
			jsonFlag, _ := cmd.Flags().GetBool("json")
			if jsonFlag {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(result)
			}
			fmt.Printf("task: %s\n", result.Task)
			fmt.Printf("status: %s\n", result.Status)
			fmt.Printf("ready: %t\n", result.Ready)
			for _, reason := range result.BlockingReasons {
				fmt.Printf("blocking: %s\n", reason)
			}
			fmt.Printf("suggested: %s\n", result.SuggestedNextCommand)
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "Emit explanation as JSON")
	return cmd
}

func BuildTaskValidateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate tasks.json syntax and schema without writing",
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceRoot, err := ResolveWorkspaceRoot()
			if err != nil {
				return err
			}
			result, err := LoadTasksWithReport(workspaceRoot)
			if err != nil {
				return err
			}
			jsonFlag, _ := cmd.Flags().GetBool("json")
			if jsonFlag {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(result.Report)
			}
			fmt.Printf("valid: %t\n", result.Report.Valid)
			for _, e := range result.Report.Errors {
				fmt.Printf("- %s\n", e.Message)
			}
			if !result.Report.Valid {
				return fmt.Errorf("tasks.json validation failed")
			}
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "Emit validation report as JSON")
	return cmd
}

func BuildFailTaskCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fail <task-id>",
		Short: "Mark a running or verifying task as FAILED",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceRoot, err := ResolveWorkspaceRoot()
			if err != nil {
				return err
			}
			tasks, err := LoadTasks(workspaceRoot)
			if err != nil {
				return err
			}
			idx := findTaskIndex(tasks, args[0])
			if idx == -1 {
				return fmt.Errorf("task %s not found", args[0])
			}

			if tasks[idx].Status != StatusRunning && tasks[idx].Status != StatusVerifying {
				return fmt.Errorf("task %s must be RUNNING or VERIFYING to fail", args[0])
			}
			if _, err := ApplyTransition(workspaceRoot, tasks, TransitionSpec{
				TaskID: args[0], To: StatusFailed, Reason: "task failed",
				Actor: "task fail", Command: "feature-dev task fail",
				Event: "task_failed", Message: "task moved to FAILED", ClearLease: true,
			}); err != nil {
				return err
			}
			fmt.Printf("task %s is now FAILED\n", args[0])
			return nil
		},
	}
	return cmd
}

func BuildTaskUnblockCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unblock <task-id>",
		Short: "Move a BLOCKED task to READY with an audited reason",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceRoot, err := ResolveWorkspaceRoot()
			if err != nil {
				return err
			}
			reason, _ := cmd.Flags().GetString("reason")
			force, _ := cmd.Flags().GetBool("force")
			if strings.TrimSpace(reason) == "" {
				return fmt.Errorf("--reason is required")
			}
			tasks, err := LoadTasks(workspaceRoot)
			if err != nil {
				return err
			}
			idx := findTaskIndex(tasks, args[0])
			if idx == -1 {
				return fmt.Errorf("task %s not found", args[0])
			}
			if tasks[idx].Status != StatusBlocked {
				return fmt.Errorf("task %s is not BLOCKED (status=%s)", args[0], tasks[idx].Status)
			}
			if !force {
				byID := map[string]Task{}
				for _, t := range tasks {
					byID[t.ID] = t
				}
				for _, dep := range tasks[idx].Dependencies {
					d, ok := byID[dep]
					if !ok || d.Status != StatusDone {
						return fmt.Errorf("dependencies incomplete; use --force to override")
					}
				}
			}
			if _, err := ApplyTransition(workspaceRoot, tasks, TransitionSpec{
				TaskID: args[0], To: StatusReady, Reason: reason,
				Actor: "task unblock", Command: "feature-dev task unblock",
				Event: "task_unblocked", Message: "task moved to READY", ClearBlock: true, ClearLease: true,
			}); err != nil {
				return err
			}
			fmt.Printf("task %s unblocked to READY\n", args[0])
			return nil
		},
	}
	cmd.Flags().String("reason", "", "Reason for unblocking (required)")
	cmd.Flags().Bool("force", false, "Force unblock even if dependencies are incomplete")
	return cmd
}

func BuildTaskResumeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resume <task-id>",
		Short: "Move a REWORK or FAILED task back to READY",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceRoot, err := ResolveWorkspaceRoot()
			if err != nil {
				return err
			}
			tasks, err := LoadTasks(workspaceRoot)
			if err != nil {
				return err
			}
			idx := findTaskIndex(tasks, args[0])
			if idx == -1 {
				return fmt.Errorf("task %s not found", args[0])
			}
			if tasks[idx].Status != StatusRework && tasks[idx].Status != StatusFailed {
				return fmt.Errorf("task %s must be REWORK or FAILED to resume (status=%s)", args[0], tasks[idx].Status)
			}
			if _, err := ApplyTransition(workspaceRoot, tasks, TransitionSpec{
				TaskID: args[0], To: StatusReady, Reason: "resumed after rework/failure",
				Actor: "task resume", Command: "feature-dev task resume",
				Event: "task_resumed", Message: "task moved to READY", ClearBlock: true, ClearLease: true,
			}); err != nil {
				return err
			}
			fmt.Printf("task %s resumed to READY\n", args[0])
			return nil
		},
	}
	return cmd
}

func BuildTaskRecoverCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "recover <task-id>",
		Short: "Recover an orphaned RUNNING/BLOCKED lease-expired task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceRoot, err := ResolveWorkspaceRoot()
			if err != nil {
				return err
			}
			toReady, _ := cmd.Flags().GetBool("to-ready")
			reason, _ := cmd.Flags().GetString("reason")
			if strings.TrimSpace(reason) == "" {
				reason = "recovered orphaned/active task"
			}
			tasks, err := LoadTasks(workspaceRoot)
			if err != nil {
				return err
			}
			idx := findTaskIndex(tasks, args[0])
			if idx == -1 {
				return fmt.Errorf("task %s not found", args[0])
			}
			target := StatusRework
			if toReady || tasks[idx].Status == StatusBlocked {
				target = StatusReady
			}
			switch tasks[idx].Status {
			case StatusRunning:
				// RUNNING -> BLOCKED first if going to READY isn't direct; RUNNING can go BLOCKED then READY
				if target == StatusReady {
					tr, err := ApplyTransition(workspaceRoot, tasks, TransitionSpec{
						TaskID: args[0], To: StatusBlocked, Reason: reason,
						Actor: "task recover", Command: "feature-dev task recover",
						Event: "task_recover_block", SetBlock: &BlockMeta{
							Reason: reason, BlockKind: BlockKindLeaseExpired,
							RecoveryCommand: fmt.Sprintf("feature-dev task unblock %s --reason %q", args[0], reason),
						}, ClearLease: true,
					})
					if err != nil {
						return err
					}
					tasks = tr.Tasks
					if _, err := ApplyTransition(workspaceRoot, tasks, TransitionSpec{
						TaskID: args[0], To: StatusReady, Reason: reason,
						Actor: "task recover", Command: "feature-dev task recover",
						Event: "task_recovered", Message: "task recovered to READY", ClearBlock: true, ClearLease: true,
					}); err != nil {
						return err
					}
				} else {
					tr, err := ApplyTransition(workspaceRoot, tasks, TransitionSpec{
						TaskID: args[0], To: StatusBlocked, Reason: reason,
						Actor: "task recover", Command: "feature-dev task recover",
						Event: "task_recover_block", SetBlock: &BlockMeta{
							Reason: reason, BlockKind: BlockKindLeaseExpired,
							RecoveryCommand: fmt.Sprintf("feature-dev task resume %s", args[0]),
						}, ClearLease: true,
					})
					if err != nil {
						return err
					}
					_ = tr
					// Blocked -> cannot go to REWORK directly. Use READY then leave for resume, or fail->rework path.
					// Prefer READY as recovery landing for orphans.
					tasks, _ = LoadTasks(workspaceRoot)
					if _, err := ApplyTransition(workspaceRoot, tasks, TransitionSpec{
						TaskID: args[0], To: StatusReady, Reason: reason,
						Actor: "task recover", Command: "feature-dev task recover",
						Event: "task_recovered", ClearBlock: true, ClearLease: true,
					}); err != nil {
						return err
					}
				}
			case StatusBlocked:
				if _, err := ApplyTransition(workspaceRoot, tasks, TransitionSpec{
					TaskID: args[0], To: StatusReady, Reason: reason,
					Actor: "task recover", Command: "feature-dev task recover",
					Event: "task_recovered", Message: "task recovered to READY", ClearBlock: true, ClearLease: true,
				}); err != nil {
					return err
				}
			default:
				return fmt.Errorf("task %s must be RUNNING or BLOCKED to recover (status=%s)", args[0], tasks[idx].Status)
			}
			fmt.Printf("task %s recovered to READY\n", args[0])
			return nil
		},
	}
	cmd.Flags().Bool("to-ready", true, "Recover to READY (default)")
	cmd.Flags().String("reason", "", "Recovery reason")
	return cmd
}

func BuildGraphCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "graph",
		Short: "Print task dependency graph",
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceRoot, err := ResolveWorkspaceRoot()
			if err != nil {
				return err
			}
			tasks, err := LoadTasks(workspaceRoot)
			if err != nil {
				return err
			}
			if _, err := ValidateGraph(tasks); err != nil {
				return err
			}

			jsonFlag, _ := cmd.Flags().GetBool("json")
			if jsonFlag {
				edges := make([]map[string]string, 0)
				for _, task := range tasks {
					for _, dep := range task.Dependencies {
						edges = append(edges, map[string]string{"from": dep, "to": task.ID})
					}
				}
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]any{"tasks": tasks, "edges": edges})
			}

			if len(tasks) == 0 {
				fmt.Println("no tasks found")
				return nil
			}
			for _, task := range tasks {
				if len(task.Dependencies) == 0 {
					fmt.Printf("%s\n", task.ID)
					continue
				}
				for _, dep := range task.Dependencies {
					fmt.Printf("%s -> %s\n", dep, task.ID)
				}
			}
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "Emit task graph as JSON")
	return cmd
}

func findTaskIndex(tasks []Task, taskID string) int {
	for i, task := range tasks {
		if task.ID == taskID {
			return i
		}
	}
	return -1
}

func BuildTaskPreviewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "preview",
		Short: "Preview the current tasks.json plan for review",
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceRoot, err := ResolveWorkspaceRoot()
			if err != nil {
				return err
			}
			jsonFlag, _ := cmd.Flags().GetBool("json")
			payload, doc, report, err := PreviewTaskPlan(workspaceRoot)
			if err != nil {
				return err
			}
			if jsonFlag {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(payload)
			}
			ws, _ := LoadWorkflowState(workspaceRoot)
			fmt.Print(RenderTasksReviewText(workspaceRoot, ws, payload.TaskItems, doc, report))
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "Emit preview as JSON")
	return cmd
}
