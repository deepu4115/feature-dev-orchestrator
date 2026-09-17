package orchestrator

func SyncWorkflowFromTasks(workspaceRoot string) error {
	ws, err := LoadWorkflowState(workspaceRoot)
	if err != nil {
		return err
	}
	if ws.WorkflowStatus == WorkflowClarificationNeeded ||
		ws.WorkflowStatus == WorkflowReviewPending ||
		ws.WorkflowStatus == WorkflowReplanning ||
		ws.WorkflowStatus == WorkflowRejected {
		return SaveWorkflowState(workspaceRoot, ws)
	}
	if ws.WorkflowStatus == WorkflowCompleted || ws.WorkflowStatus == WorkflowCancelled {
		return SaveWorkflowState(workspaceRoot, ws)
	}

	tasks, err := LoadTasks(workspaceRoot)
	if err != nil {
		return err
	}

	if ws.WorkflowStatus == WorkflowApproved || ws.WorkflowStatus == WorkflowExecuting || ws.WorkflowStatus == WorkflowVerifying {
		if allTasksInStatus(tasks, StatusDone) {
			if ws.WorkflowStatus != WorkflowCompleted {
				ws.WorkflowStatus = WorkflowVerifying
			}
		} else if anyTaskInStatuses(tasks, StatusVerifying) {
			ws.WorkflowStatus = WorkflowVerifying
		} else if anyTaskInStatuses(tasks, StatusRunning, StatusImplemented, StatusRework) {
			ws.WorkflowStatus = WorkflowExecuting
		} else if ws.WorkflowStatus == WorkflowApproved {
			// stay approved until execution starts
		}
	}

	return SaveWorkflowState(workspaceRoot, ws)
}

func allTasksInStatus(tasks []Task, status TaskStatus) bool {
	if len(tasks) == 0 {
		return false
	}
	for _, t := range tasks {
		if t.Status != status {
			return false
		}
	}
	return true
}

func anyTaskInStatuses(tasks []Task, statuses ...TaskStatus) bool {
	allowed := map[TaskStatus]bool{}
	for _, s := range statuses {
		allowed[s] = true
	}
	for _, t := range tasks {
		if allowed[t.Status] {
			return true
		}
	}
	return false
}

func countTasksDone(tasks []Task) (done, total int) {
	total = len(tasks)
	for _, t := range tasks {
		if t.Status == StatusDone {
			done++
		}
	}
	return done, total
}
