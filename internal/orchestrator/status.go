package orchestrator

type FinalizationStatus string

const (
	FinalizationNotRun FinalizationStatus = "NOT_RUN"
	FinalizationPassed FinalizationStatus = "PASSED"
	FinalizationFailed FinalizationStatus = "FAILED"
)

type WorkspaceStatus struct {
	Workspace             string             `json:"workspace"`
	FeatureDir            string             `json:"feature_dir"`
	WorkflowStatus        WorkflowStatus     `json:"workflow_status"`
	CurrentPlanRevision   int                `json:"current_plan_revision"`
	ApprovedPlanRevision  *int               `json:"approved_plan_revision,omitempty"`
	ApprovalRequired      bool               `json:"approval_required"`
	TasksDone             int                `json:"tasks_done"`
	TasksTotal            int                `json:"tasks_total"`
	Completed             bool               `json:"completed"`
	CompletionReady       bool               `json:"completion_ready"`
	Finalization          FinalizationStatus `json:"finalization"`
	FinalGates            []string           `json:"final_gates"`
}

func ApprovalRequired(ws WorkflowState) bool {
	if !RequiresPlanApproval(ws) {
		return false
	}
	switch ws.WorkflowStatus {
	case WorkflowApproved, WorkflowExecuting, WorkflowVerifying, WorkflowCompleted:
		return false
	default:
		return true
	}
}

func DeriveFinalizationStatus(ws WorkflowState) FinalizationStatus {
	if ws.WorkflowStatus == WorkflowCompleted {
		return FinalizationPassed
	}
	for i := len(ws.ApprovalHistory) - 1; i >= 0; i-- {
		entry := ws.ApprovalHistory[i]
		if entry.Status == string(WorkflowCompleted) {
			return FinalizationPassed
		}
		if entry.Status == "FINALIZE_FAILED" {
			return FinalizationFailed
		}
	}
	return FinalizationNotRun
}

func BuildWorkspaceStatus(workspaceRoot string) (WorkspaceStatus, error) {
	ws, err := LoadWorkflowState(workspaceRoot)
	if err != nil {
		return WorkspaceStatus{}, err
	}
	tasks, _ := LoadTasks(workspaceRoot)
	done, total := countTasksDone(tasks)
	finalization := DeriveFinalizationStatus(ws)
	completed := ws.WorkflowStatus == WorkflowCompleted
	completionReady := completed || (done == total && total > 0 && (ws.WorkflowStatus == WorkflowVerifying || allTasksInStatus(tasks, StatusDone)))

	return WorkspaceStatus{
		Workspace:            workspaceRoot,
		FeatureDir:           ResolveFeatureDir(workspaceRoot),
		WorkflowStatus:       ws.WorkflowStatus,
		CurrentPlanRevision:  ws.CurrentPlanRevision,
		ApprovedPlanRevision: ws.ApprovedPlanRevision,
		ApprovalRequired:     ApprovalRequired(ws),
		TasksDone:            done,
		TasksTotal:           total,
		Completed:            completed,
		CompletionReady:      completionReady,
		Finalization:         finalization,
		FinalGates:           []string{"traceability-check", "verify-cross-repo", "finalize"},
	}, nil
}
