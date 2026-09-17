package orchestrator

import "fmt"

type TaskExplainResult struct {
	Task                string            `json:"task"`
	Status              TaskStatus        `json:"status"`
	Ready               bool              `json:"ready"`
	BlockingReasons     []string          `json:"blocking_reasons"`
	SuggestedNextCommand string           `json:"suggested_next_command"`
	DependencyStatus    map[string]string `json:"dependency_status,omitempty"`
	WorkflowGate        string            `json:"workflow_gate,omitempty"`
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

	if task.Status != StatusReady && task.Status != StatusRunning && task.Status != StatusImplemented && task.Status != StatusVerifying {
		if task.Status == StatusReviewPending || task.Status == StatusPlanned || task.Status == StatusDraft {
			result.BlockingReasons = append(result.BlockingReasons, fmt.Sprintf("task status is %s; run feature-dev reconcile to promote when dependencies are DONE", task.Status))
		} else {
			result.BlockingReasons = append(result.BlockingReasons, fmt.Sprintf("task status is %s", task.Status))
		}
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

	if result.Ready {
		switch task.Status {
		case StatusReady:
			result.SuggestedNextCommand = fmt.Sprintf("feature-dev task start %s", taskID)
		case StatusRunning:
			result.SuggestedNextCommand = fmt.Sprintf("feature-dev implement %s", taskID)
		case StatusImplemented, StatusVerifying:
			result.SuggestedNextCommand = fmt.Sprintf("feature-dev verify %s", taskID)
		case StatusDone:
			result.SuggestedNextCommand = "feature-dev execute-loop --json"
		}
	} else if len(result.BlockingReasons) > 0 {
		result.SuggestedNextCommand = "feature-dev reconcile"
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
	unlocked, err := PromoteDependencyReadyTasks(tasks)
	return tasks, unlocked, err
}
