package orchestrator

import (
	"fmt"
	"time"
)

type TaskExplainResult struct {
	Task                 string            `json:"task"`
	Status               TaskStatus        `json:"status"`
	Ready                bool              `json:"ready"`
	BlockingReasons      []string          `json:"blocking_reasons"`
	SuggestedNextCommand string            `json:"suggested_next_command"`
	DependencyStatus     map[string]string `json:"dependency_status,omitempty"`
	WorkflowGate         string            `json:"workflow_gate,omitempty"`
	BlockedReason        string            `json:"blocked_reason,omitempty"`
	BlockedBy            []string          `json:"blocked_by,omitempty"`
	BlockedAt            *time.Time        `json:"blocked_at,omitempty"`
	RecoveryCommand      string            `json:"recovery_command,omitempty"`
	BlockKind            string            `json:"block_kind,omitempty"`
	WorkflowBusy         bool              `json:"workflow_busy,omitempty"`
	ActiveTaskID         string            `json:"active_task_id,omitempty"`
}

func ExplainTask(workspaceRoot string, taskID string) (TaskExplainResult, error) {
	tasks, err := LoadTasks(workspaceRoot)
	if err != nil {
		return TaskExplainResult{}, err
	}
	idx := findTaskIndex(tasks, taskID)
	if idx == -1 {
		return TaskExplainResult{}, fmt.Errorf("task %s not found", taskID)
	}
	task := tasks[idx]
	ws, err := LoadWorkflowState(workspaceRoot)
	if err != nil {
		return TaskExplainResult{}, err
	}

	result := TaskExplainResult{
		Task:             taskID,
		Status:           task.Status,
		DependencyStatus: map[string]string{},
		BlockingReasons:  []string{},
		BlockedReason:    task.BlockedReason,
		BlockedBy:        task.BlockedBy,
		BlockedAt:        task.BlockedAt,
		RecoveryCommand:  task.RecoveryCommand,
		BlockKind:        task.BlockKind,
		ActiveTaskID:     ws.ActiveTaskID,
	}

	byID := map[string]Task{}
	for _, t := range tasks {
		byID[t.ID] = t
	}
	for _, depID := range task.Dependencies {
		dep, ok := byID[depID]
		if !ok {
			result.DependencyStatus[depID] = "MISSING"
			result.BlockingReasons = append(result.BlockingReasons, fmt.Sprintf("dependency %s not found", depID))
			continue
		}
		result.DependencyStatus[depID] = string(dep.Status)
		if dep.Status != StatusDone {
			result.BlockingReasons = append(result.BlockingReasons, fmt.Sprintf("dependency %s is %s, expected DONE", depID, dep.Status))
		}
	}

	if RequiresPlanApproval(ws) {
		if err := CanExecute(workspaceRoot); err != nil {
			result.WorkflowGate = err.Error()
			result.BlockingReasons = append(result.BlockingReasons, err.Error())
		}
	}

	if ws.SchedulingPausedReason != "" {
		result.BlockingReasons = append(result.BlockingReasons, ws.SchedulingPausedReason)
	}
	if HasActiveTask(tasks) {
		active := ActiveTasks(tasks)
		if len(active) > 0 && active[0].ID != taskID {
			result.WorkflowBusy = true
			result.BlockingReasons = append(result.BlockingReasons, fmt.Sprintf("workflow busy: task %s is active", active[0].ID))
		}
	}

	switch task.Status {
	case StatusBlocked:
		reason := task.BlockedReason
		if reason == "" {
			reason = "task status is BLOCKED"
		}
		result.BlockingReasons = append(result.BlockingReasons, reason)
		if task.RecoveryCommand != "" {
			result.SuggestedNextCommand = task.RecoveryCommand
		} else {
			result.SuggestedNextCommand = fmt.Sprintf("feature-dev task unblock %s --reason \"manual unblock\"", taskID)
		}
	case StatusRework:
		result.BlockingReasons = append(result.BlockingReasons, "task requires rework after verification failure")
		result.SuggestedNextCommand = fmt.Sprintf("feature-dev task resume %s", taskID)
	case StatusFailed:
		result.BlockingReasons = append(result.BlockingReasons, "task status is FAILED")
		result.SuggestedNextCommand = fmt.Sprintf("feature-dev task resume %s", taskID)
	case StatusReviewPending, StatusPlanned, StatusDraft:
		result.BlockingReasons = append(result.BlockingReasons, fmt.Sprintf("task status is %s; run feature-dev reconcile to promote when dependencies are DONE", task.Status))
		result.SuggestedNextCommand = "feature-dev reconcile"
	case StatusReady, StatusRunning, StatusImplemented, StatusVerifying, StatusDone:
		// handled below
	default:
		result.BlockingReasons = append(result.BlockingReasons, fmt.Sprintf("task status is %s", task.Status))
	}

	readyTasks, readyErr := ReadyTasks(workspaceRoot, tasks)
	if readyErr != nil {
		result.BlockingReasons = append(result.BlockingReasons, readyErr.Error())
	} else {
		for _, ready := range readyTasks {
			if ready.ID == taskID {
				result.Ready = true
				break
			}
		}
	}

	if result.SuggestedNextCommand == "" {
		if result.Ready {
			switch task.Status {
			case StatusReady:
				if result.WorkflowBusy {
					result.SuggestedNextCommand = fmt.Sprintf("feature-dev execute-next (finish active task %s first)", ws.ActiveTaskID)
				} else {
					result.SuggestedNextCommand = fmt.Sprintf("feature-dev task start %s", taskID)
				}
			case StatusRunning:
				result.SuggestedNextCommand = fmt.Sprintf("feature-dev task implement %s", taskID)
			case StatusImplemented, StatusVerifying:
				result.SuggestedNextCommand = fmt.Sprintf("feature-dev verify %s", taskID)
			case StatusDone:
				result.SuggestedNextCommand = "feature-dev execute-loop --json"
			}
		} else if len(result.BlockingReasons) > 0 {
			result.SuggestedNextCommand = "feature-dev reconcile"
		}
	}

	return result, nil
}

func PromoteTasksAfterCompletion(workspaceRoot string, tasks []Task) ([]Task, []string, error) {
	ws, err := LoadWorkflowState(workspaceRoot)
	if err != nil {
		return tasks, nil, err
	}
	if !isExecutionAllowedWorkflowStatus(ws.WorkflowStatus) {
		return tasks, nil, nil
	}
	return PromoteDependencyReadyTasksAudited(workspaceRoot, tasks, "promote-after-completion", "feature-dev execute-next")
}
