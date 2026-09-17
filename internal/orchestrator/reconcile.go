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
	Tasks        []Task `json:"tasks"`
	StaleCount   int    `json:"stale_count"`
	UpdatedCount int    `json:"updated_count"`
}

func ReconcileTasks(workspaceRoot string, tasks []Task) (ReconcileReport, error) {
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
	for i := range tasks {
		task := reconciled[i]
		wasChanged := false
		if task.Repository == "" {
			task.Status = StatusBlocked
			task.UpdatedAt = now
			wasChanged = true
		}

		if task.Repository != "" && task.Repository != "workspace" {
			repoPath := filepath.Join(workspaceRoot, task.Repository)
			if _, err := os.Stat(repoPath); err != nil {
				task.Status = StatusBlocked
				task.UpdatedAt = now
				wasChanged = true
			}
		}

		if task.Status == StatusRunning && now.Sub(task.UpdatedAt) > 5*time.Minute {
			task.Status = StatusBlocked
			task.UpdatedAt = now
			wasChanged = true
		}
		if task.Status == StatusVerifying && now.Sub(task.UpdatedAt) > 5*time.Minute {
			task.Status = StatusRework
			task.UpdatedAt = now
			wasChanged = true
		}

		reconciled[i] = task
		if wasChanged {
			report.Tasks = append(report.Tasks, task)
			report.UpdatedCount++
		}
	}
	report.StaleCount = len(report.Tasks)
	report.Tasks = reconciled

	if isExecutionAllowedWorkflowStatus(ws.WorkflowStatus) {
		unlocked, err := PromoteDependencyReadyTasks(report.Tasks)
		if err != nil {
			return report, err
		}
		report.UpdatedCount += len(unlocked)
	}

	return report, nil
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
			tasks, err := LoadTasks(workspaceRoot)
			if err != nil {
				return err
			}
			before := map[string]Task{}
			for _, task := range tasks {
				before[task.ID] = task
			}
			report, err := ReconcileTasks(workspaceRoot, tasks)
			if err != nil {
				return err
			}
			if dryRun {
				jsonFlag, _ := cmd.Flags().GetBool("json")
				if jsonFlag {
					enc := json.NewEncoder(os.Stdout)
					enc.SetIndent("", "  ")
					return enc.Encode(map[string]any{
						"dry_run":     true,
						"updated":     report.UpdatedCount,
						"reconciled":  report.StaleCount,
						"total_tasks": len(report.Tasks),
					})
				}
				fmt.Printf("dry run: would reconcile %d task(s)\n", report.UpdatedCount)
				return nil
			}
			if err := SaveTasks(workspaceRoot, report.Tasks); err != nil {
				return err
			}
			for _, task := range report.Tasks {
				prior, ok := before[task.ID]
				if !ok {
					continue
				}
				if prior.Status == task.Status && prior.WorkingDirectory == task.WorkingDirectory {
					continue
				}
				_ = AppendTaskSummary(workspaceRoot, task, "task_reconciled", fmt.Sprintf("status %s -> %s", prior.Status, task.Status))
			}

			jsonFlag, _ := cmd.Flags().GetBool("json")
			if jsonFlag {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]any{
					"updated":     report.UpdatedCount,
					"reconciled":  report.StaleCount,
					"total_tasks": len(report.Tasks),
				})
			}

			fmt.Printf("reconciled %d task(s)\n", report.UpdatedCount)
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "Emit reconcile summary as JSON")
	cmd.Flags().Bool("dry-run", false, "Show planned reconcile actions without mutating task state")
	return cmd
}
