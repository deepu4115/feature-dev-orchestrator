package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
)

type ReconcileReport struct {
	Tasks              []Task               `json:"tasks"`
	StaleCount         int                  `json:"stale_count"`
	UpdatedCount       int                  `json:"updated_count"`
	OrphanedActive     []string             `json:"orphaned_active,omitempty"`
	ProposedUnblocks   []string             `json:"proposed_unblocks,omitempty"`
	ProposedTransitions []ReconcileChange   `json:"proposed_transitions,omitempty"`
	SchedulingPaused   string               `json:"scheduling_paused_reason,omitempty"`
	InvariantReport    *InvariantReport     `json:"invariants,omitempty"`
}

type ReconcileChange struct {
	TaskID   string     `json:"task_id"`
	From     TaskStatus `json:"from"`
	To       TaskStatus `json:"to"`
	Reason   string     `json:"reason"`
	Recovery string     `json:"recovery_command,omitempty"`
}

type ReconcileOptions struct {
	DryRun           bool
	AutoUnblockStale bool
	RepairInvariants bool
}

func ReconcileTasks(workspaceRoot string, tasks []Task) (ReconcileReport, error) {
	return ReconcileTasksWithOptions(workspaceRoot, tasks, ReconcileOptions{})
}

func ReconcileTasksWithOptions(workspaceRoot string, tasks []Task, opts ReconcileOptions) (ReconcileReport, error) {
	ws, err := LoadWorkflowState(workspaceRoot)
	if err != nil {
		return ReconcileReport{}, err
	}
	if workspaceRoot == "" {
		return ReconcileReport{}, fmt.Errorf("workspace root cannot be empty")
	}

	now := time.Now().UTC()
	report := ReconcileReport{Tasks: make([]Task, 0)}
	reconciled := make([]Task, len(tasks))
	copy(reconciled, tasks)
	changes := make([]ReconcileChange, 0)
	orphans := make([]string, 0)

	for i := range reconciled {
		task := reconciled[i]
		prev := task.Status

		if task.Repository == "" {
			meta := &BlockMeta{
				Reason:          "task has empty repository",
				BlockKind:       BlockKindMissingRepo,
				RecoveryCommand: fmt.Sprintf("feature-dev task unblock %s --reason \"set repository and unblock\"", task.ID),
			}
			if err := ApplyTransitionInMemory(&task, StatusBlocked, meta, false, now); err != nil {
				return report, err
			}
			changes = append(changes, ReconcileChange{TaskID: task.ID, From: prev, To: StatusBlocked, Reason: meta.Reason, Recovery: meta.RecoveryCommand})
		} else if task.Repository != "workspace" {
			repoPath := filepath.Join(workspaceRoot, task.Repository)
			if _, err := os.Stat(repoPath); err != nil {
				meta := &BlockMeta{
					Reason:          fmt.Sprintf("repository path missing: %s", repoPath),
					BlockKind:       BlockKindMissingRepo,
					RecoveryCommand: fmt.Sprintf("feature-dev task unblock %s --reason \"restore repository and unblock\"", task.ID),
				}
				if err := ApplyTransitionInMemory(&task, StatusBlocked, meta, false, now); err != nil {
					return report, err
				}
				changes = append(changes, ReconcileChange{TaskID: task.ID, From: prev, To: StatusBlocked, Reason: meta.Reason, Recovery: meta.RecoveryCommand})
			}
		}

		if task.Status == StatusRunning && now.Sub(task.UpdatedAt) > 5*time.Minute {
			orphans = append(orphans, task.ID)
			meta := &BlockMeta{
				Reason:          "RUNNING lease expired / orphaned active task",
				BlockKind:       BlockKindLeaseExpired,
				RecoveryCommand: fmt.Sprintf("feature-dev task recover %s", task.ID),
			}
			if err := ApplyTransitionInMemory(&task, StatusBlocked, meta, false, now); err != nil {
				return report, err
			}
			changes = append(changes, ReconcileChange{TaskID: task.ID, From: prev, To: StatusBlocked, Reason: meta.Reason, Recovery: meta.RecoveryCommand})
			PauseScheduling(&ws, fmt.Sprintf("orphaned active task %s; run feature-dev task recover %s", task.ID, task.ID))
			if ws.ActiveTaskID == task.ID {
				ClearExecutionLease(&ws)
			}
		}
		if task.Status == StatusVerifying && now.Sub(task.UpdatedAt) > 5*time.Minute {
			if err := ApplyTransitionInMemory(&task, StatusRework, nil, true, now); err != nil {
				return report, err
			}
			changes = append(changes, ReconcileChange{
				TaskID: task.ID, From: prev, To: StatusRework,
				Reason:   "VERIFYING timed out",
				Recovery: fmt.Sprintf("feature-dev task resume %s", task.ID),
			})
			if ws.ActiveTaskID == task.ID {
				ClearExecutionLease(&ws)
			}
		}

		reconciled[i] = task
	}

	report.OrphanedActive = orphans
	report.ProposedTransitions = changes
	report.SchedulingPaused = ws.SchedulingPausedReason

	if isExecutionAllowedWorkflowStatus(ws.WorkflowStatus) && ws.SchedulingPausedReason == "" {
		unlocked, err := promoteDependencyReadyTasksInMemory(reconciled, now)
		if err != nil {
			return report, err
		}
		for _, id := range unlocked {
			changes = append(changes, ReconcileChange{
				TaskID: id, From: StatusReviewPending, To: StatusReady,
				Reason: "dependencies complete; promoted by reconcile",
			})
		}
		report.UpdatedCount += len(unlocked)
	}

	if opts.AutoUnblockStale {
		byID := map[string]Task{}
		for _, t := range reconciled {
			byID[t.ID] = t
		}
		for i := range reconciled {
			t := &reconciled[i]
			if t.Status != StatusBlocked {
				continue
			}
			if t.BlockKind != BlockKindLeaseExpired && t.BlockKind != BlockKindStaleRunning {
				continue
			}
			depsOK := true
			for _, dep := range t.Dependencies {
				d, ok := byID[dep]
				if !ok || d.Status != StatusDone {
					depsOK = false
					break
				}
			}
			if !depsOK {
				continue
			}
			prev := t.Status
			if err := ApplyTransitionInMemory(t, StatusReady, nil, true, now); err != nil {
				return report, err
			}
			report.ProposedUnblocks = append(report.ProposedUnblocks, t.ID)
			changes = append(changes, ReconcileChange{
				TaskID: t.ID, From: prev, To: StatusReady,
				Reason: "auto-unblock stale/lease_expired with complete dependencies",
			})
			ClearSchedulingPause(&ws)
		}
	} else {
		for _, t := range reconciled {
			if t.Status == StatusBlocked && (t.BlockKind == BlockKindLeaseExpired || t.BlockKind == BlockKindStaleRunning) {
				report.ProposedUnblocks = append(report.ProposedUnblocks, t.ID)
			}
		}
	}

	report.Tasks = reconciled
	report.UpdatedCount = len(changes)
	report.StaleCount = len(changes)
	report.ProposedTransitions = changes

	if !opts.DryRun {
		if err := SaveWorkflowState(workspaceRoot, ws); err != nil {
			return report, err
		}
	}

	return report, nil
}

func promoteDependencyReadyTasksInMemory(tasks []Task, now time.Time) ([]string, error) {
	byID := map[string]int{}
	for i, task := range tasks {
		byID[task.ID] = i
	}
	unlocked := []string{}
	for i := range tasks {
		task := &tasks[i]
		if task.Status != StatusReviewPending && task.Status != StatusPlanned && task.Status != StatusDraft {
			continue
		}
		blocked := false
		for _, depID := range task.Dependencies {
			idx, ok := byID[depID]
			if !ok || tasks[idx].Status != StatusDone {
				blocked = true
				break
			}
		}
		if blocked {
			continue
		}
		prev := task.Status
		if err := ApplyTransitionInMemory(task, StatusReady, nil, true, now); err != nil {
			return unlocked, err
		}
		_ = prev
		unlocked = append(unlocked, task.ID)
	}
	return unlocked, nil
}

func persistReconcileAudits(workspaceRoot string, before map[string]Task, after []Task, actor string) error {
	ws, err := LoadWorkflowState(workspaceRoot)
	if err != nil {
		return err
	}
	for _, task := range after {
		prior, ok := before[task.ID]
		if !ok {
			continue
		}
		if prior.Status == task.Status && prior.BlockedReason == task.BlockedReason {
			continue
		}
		entry := TaskSummaryRecord{
			Timestamp:        time.Now().UTC(),
			TaskID:           task.ID,
			Repository:       task.Repository,
			Status:           task.Status,
			PreviousStatus:   prior.Status,
			NextStatus:       task.Status,
			Event:            "task_reconciled",
			Message:          fmt.Sprintf("status %s -> %s", prior.Status, task.Status),
			Reason:           task.BlockedReason,
			Actor:            actor,
			Command:          "feature-dev reconcile",
			WorkflowRevision: ws.CurrentPlanRevision,
		}
		if entry.Reason == "" {
			entry.Reason = entry.Message
		}
		if err := appendTaskSummaryRecord(workspaceRoot, entry); err != nil {
			return err
		}
	}
	return nil
}

func BuildReconcileCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reconcile",
		Short: "Reconcile persisted task state for safe resume",
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceRoot, err := ResolveWorkspaceRoot()
			if err != nil {
				return err
			}
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			autoUnblock, _ := cmd.Flags().GetBool("auto-unblock-stale")
			repair, _ := cmd.Flags().GetBool("repair-invariants")
			jsonFlag, _ := cmd.Flags().GetBool("json")

			tasks, err := LoadTasks(workspaceRoot)
			if err != nil {
				return err
			}
			before := map[string]Task{}
			for _, task := range tasks {
				before[task.ID] = task
			}
			report, err := ReconcileTasksWithOptions(workspaceRoot, tasks, ReconcileOptions{
				DryRun:           dryRun,
				AutoUnblockStale: autoUnblock,
				RepairInvariants: repair,
			})
			if err != nil {
				return err
			}

			if dryRun {
				if jsonFlag {
					enc := json.NewEncoder(os.Stdout)
					enc.SetIndent("", "  ")
					return enc.Encode(map[string]any{
						"dry_run":               true,
						"updated":               report.UpdatedCount,
						"reconciled":            report.StaleCount,
						"total_tasks":           len(report.Tasks),
						"orphaned_active":       report.OrphanedActive,
						"proposed_unblocks":     report.ProposedUnblocks,
						"proposed_transitions":  report.ProposedTransitions,
						"scheduling_paused":     report.SchedulingPaused,
					})
				}
				fmt.Printf("dry run: would reconcile %d task(s)\n", report.UpdatedCount)
				for _, c := range report.ProposedTransitions {
					fmt.Printf("- %s: %s -> %s (%s)\n", c.TaskID, c.From, c.To, c.Reason)
					if c.Recovery != "" {
						fmt.Printf("  recovery: %s\n", c.Recovery)
					}
				}
				return nil
			}

			if err := SaveTasks(workspaceRoot, report.Tasks); err != nil {
				return err
			}
			if err := persistReconcileAudits(workspaceRoot, before, report.Tasks, "reconcile"); err != nil {
				return err
			}

			inv, err := CheckWorkspaceInvariants(workspaceRoot)
			if err != nil {
				return err
			}
			report.InvariantReport = &inv
			if repair && !inv.Valid {
				return NewCodedError(ErrCodeInvariantViolation, fmt.Sprintf("invariants failed: %v", inv.Violations))
			}

			_ = AppendCommandJournal(workspaceRoot, "reconcile", args, 0, fmt.Sprintf("updated=%d", report.UpdatedCount), "")

			if jsonFlag {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]any{
					"updated":              report.UpdatedCount,
					"reconciled":           report.StaleCount,
					"total_tasks":          len(report.Tasks),
					"orphaned_active":      report.OrphanedActive,
					"proposed_unblocks":    report.ProposedUnblocks,
					"scheduling_paused":    report.SchedulingPaused,
					"invariants":           inv,
				})
			}
			fmt.Printf("reconciled %d task(s)\n", report.UpdatedCount)
			if report.SchedulingPaused != "" {
				fmt.Printf("scheduling paused: %s\n", report.SchedulingPaused)
			}
			return nil
		},
	}
	cmd.Flags().Bool("dry-run", false, "Show proposed transitions without writing")
	cmd.Flags().Bool("auto-unblock-stale", false, "Auto-unblock lease_expired/stale_running tasks with complete deps")
	cmd.Flags().Bool("repair-invariants", false, "Fail if workspace invariants are violated after reconcile")
	cmd.Flags().Bool("json", false, "Emit reconcile report as JSON")
	return cmd
}
