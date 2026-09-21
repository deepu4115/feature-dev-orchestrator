package orchestrator

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func isExecutionAllowedWorkflowStatus(status WorkflowStatus) bool {
	return status == WorkflowApproved ||
		status == WorkflowExecuting ||
		status == WorkflowVerifying
}

func CanStartTask(ws WorkflowState, task Task) error {
	if !RequiresPlanApproval(ws) {
		return nil
	}
	if !isExecutionAllowedWorkflowStatus(ws.WorkflowStatus) {
		return fmt.Errorf("%w: status=%s revision=%d", ErrPlanNotApproved, ws.WorkflowStatus, ws.CurrentPlanRevision)
	}
	if ws.ApprovedPlanRevision == nil || *ws.ApprovedPlanRevision != ws.CurrentPlanRevision {
		return ErrPlanRevisionMismatch
	}
	if task.Status != StatusReady {
		return fmt.Errorf("task %s is not READY", task.ID)
	}
	return nil
}

func CanStartTaskInWorkspace(workspaceRoot string, task Task) error {
	ws, err := LoadWorkflowState(workspaceRoot)
	if err != nil {
		return err
	}
	if err := CanStartTask(ws, task); err != nil {
		return err
	}
	if !RequiresPlanApproval(ws) {
		return nil
	}
	if ws.ApprovedPlanFingerprint == "" {
		return nil
	}
	doc, err := LoadCurrentPlanDocument(workspaceRoot)
	if err != nil {
		return err
	}
	fp, err := ComputePlanFingerprint(doc)
	if err != nil {
		return err
	}
	if fp != ws.ApprovedPlanFingerprint {
		return ErrPlanFingerprintMismatch
	}
	return nil
}

func CanExecute(workspaceRoot string) error {
	ws, err := LoadWorkflowState(workspaceRoot)
	if err != nil {
		return err
	}
	if !RequiresPlanApproval(ws) {
		return nil
	}
	if !isExecutionAllowedWorkflowStatus(ws.WorkflowStatus) {
		return fmt.Errorf("%w: status=%s revision=%d", ErrPlanNotApproved, ws.WorkflowStatus, ws.CurrentPlanRevision)
	}
	if ws.ApprovedPlanRevision == nil || *ws.ApprovedPlanRevision != ws.CurrentPlanRevision {
		return ErrPlanRevisionMismatch
	}
	doc, err := LoadCurrentPlanDocument(workspaceRoot)
	if err != nil {
		return err
	}
	if ws.ApprovedPlanFingerprint != "" {
		fp, err := ComputePlanFingerprint(doc)
		if err != nil {
			return err
		}
		if fp != ws.ApprovedPlanFingerprint {
			return ErrPlanFingerprintMismatch
		}
	}
	return nil
}

func InvalidateApproval(ws *WorkflowState) {
	ws.ApprovedPlanRevision = nil
	ws.ApprovedPlanFingerprint = ""
	ws.WorkflowStatus = WorkflowReviewPending
}

type ApprovePlanOptions struct {
	Revision       int
	DeferUnmapped  string
}

type ApprovePlanResult struct {
	Revision      int      `json:"revision"`
	UnlockedTasks []string `json:"unlocked_tasks"`
	Warnings      []string `json:"warnings,omitempty"`
}

func ApprovePlan(workspaceRoot string, opts ApprovePlanOptions) (ApprovePlanResult, WorkflowState, []Task, error) {
	ws, err := LoadWorkflowState(workspaceRoot)
	if err != nil {
		return ApprovePlanResult{}, ws, nil, err
	}
	if ws.CurrentPlanRevision <= 0 {
		return ApprovePlanResult{}, ws, nil, fmt.Errorf("no plan revision to approve")
	}
	if ws.WorkflowStatus != WorkflowReviewPending {
		return ApprovePlanResult{}, ws, nil, fmt.Errorf("%w: current status is %s", ErrPlanNotReviewPending, ws.WorkflowStatus)
	}
	targetRevision := ws.CurrentPlanRevision
	if opts.Revision > 0 {
		if opts.Revision != ws.CurrentPlanRevision {
			return ApprovePlanResult{}, ws, nil, fmt.Errorf("%w: current revision is %d, requested %d", ErrStaleRevisionApproval, ws.CurrentPlanRevision, opts.Revision)
		}
		targetRevision = opts.Revision
	}

	doc, err := LoadPlanDocument(PlanRevisionPath(workspaceRoot, targetRevision))
	if err != nil {
		return ApprovePlanResult{}, ws, nil, err
	}
	fp, err := ComputePlanFingerprint(doc)
	if err != nil {
		return ApprovePlanResult{}, ws, nil, err
	}

	planPath := ResolvePlanPath(workspaceRoot, "PLAN.md")
	if Exists(planPath) {
		coverage, err := RunPlanCoverage(workspaceRoot, "PLAN.md")
		if err != nil {
			return ApprovePlanResult{}, ws, nil, err
		}
		if !coverage.Valid {
			if strings.TrimSpace(opts.DeferUnmapped) == "" {
				return ApprovePlanResult{}, ws, nil, fmt.Errorf("plan coverage incomplete: %s (use --defer-unmapped with reason to override)", coverage.SuggestedPrompt)
			}
			ws.CoverageDeferralReason = opts.DeferUnmapped
		}
	}

	result := ApprovePlanResult{Revision: targetRevision, Warnings: []string{}}
	if warns, err := CheckRepositorySnapshotDrift(workspaceRoot, doc); err == nil {
		result.Warnings = append(result.Warnings, warns...)
	}

	rev := targetRevision
	ws.ApprovedPlanRevision = &rev
	ws.ApprovedPlanFingerprint = fp
	ws.WorkflowStatus = WorkflowApproved
	ws.ApprovalHistory = append(ws.ApprovalHistory, ApprovalHistoryEntry{
		Revision:    targetRevision,
		Status:      "APPROVED",
		Fingerprint: fp,
		Timestamp:   time.Now().UTC(),
	})

	tasks, err := LoadTasks(workspaceRoot)
	if err != nil {
		return ApprovePlanResult{}, ws, nil, err
	}
	tasks, unlocked, err := PromoteDependencyReadyTasksAudited(workspaceRoot, tasks, "approve", "feature-dev approve")
	if err != nil {
		return ApprovePlanResult{}, ws, nil, err
	}
	result.UnlockedTasks = unlocked

	ClearValidationArtifactsOnSuccess(workspaceRoot)

	if err := SaveWorkflowState(workspaceRoot, ws); err != nil {
		return ApprovePlanResult{}, ws, nil, err
	}
	if err := SaveTasks(workspaceRoot, tasks); err != nil {
		return ApprovePlanResult{}, ws, nil, err
	}
	return result, ws, tasks, nil
}

func RejectPlan(workspaceRoot string, reason string) (WorkflowState, error) {
	ws, err := LoadWorkflowState(workspaceRoot)
	if err != nil {
		return ws, err
	}
	if ws.CurrentPlanRevision <= 0 {
		return ws, fmt.Errorf("no plan revision to reject")
	}
	ws.WorkflowStatus = WorkflowRejected
	ws.ApprovedPlanRevision = nil
	ws.ApprovedPlanFingerprint = ""
	ws.ApprovalHistory = append(ws.ApprovalHistory, ApprovalHistoryEntry{
		Revision:  ws.CurrentPlanRevision,
		Status:    "REJECTED",
		Reason:    reason,
		Timestamp: time.Now().UTC(),
	})
	if err := SaveWorkflowState(workspaceRoot, ws); err != nil {
		return ws, err
	}
	return ws, nil
}

func PromoteApprovedTasks(tasks []Task) ([]string, error) {
	return PromoteDependencyReadyTasks(tasks)
}

func PromoteDependencyReadyTasks(tasks []Task) ([]string, error) {
	now := time.Now().UTC()
	unlocked, err := promoteDependencyReadyTasksInMemory(tasks, now)
	if err != nil {
		return nil, err
	}
	sort.Strings(unlocked)
	return unlocked, nil
}

// PromoteDependencyReadyTasksAudited promotes and writes audit events for each unlock.
func PromoteDependencyReadyTasksAudited(workspaceRoot string, tasks []Task, actor, command string) ([]Task, []string, error) {
	before := map[string]TaskStatus{}
	for _, t := range tasks {
		before[t.ID] = t.Status
	}
	unlocked, err := PromoteDependencyReadyTasks(tasks)
	if err != nil {
		return tasks, nil, err
	}
	ws, err := LoadWorkflowState(workspaceRoot)
	if err != nil {
		return tasks, unlocked, err
	}
	for _, id := range unlocked {
		idx := findTaskIndex(tasks, id)
		if idx < 0 {
			continue
		}
		entry := TaskSummaryRecord{
			Timestamp:        time.Now().UTC(),
			TaskID:           id,
			Repository:       tasks[idx].Repository,
			Status:           StatusReady,
			PreviousStatus:   before[id],
			NextStatus:       StatusReady,
			Event:            "task_promoted_ready",
			Message:          fmt.Sprintf("status %s -> READY", before[id]),
			Reason:           "dependencies complete",
			Actor:            actor,
			Command:          command,
			WorkflowRevision: ws.CurrentPlanRevision,
		}
		if err := appendTaskSummaryRecord(workspaceRoot, entry); err != nil {
			return tasks, unlocked, err
		}
	}
	return tasks, unlocked, nil
}

func CheckRepositorySnapshotDrift(workspaceRoot string, doc PlanDocument) ([]string, error) {
	warnings := []string{}
	for repoID, snap := range doc.PlanSnapshot.Repositories {
		repoPath := filepathJoinRepo(workspaceRoot, repoID)
		if repoPath == "" {
			continue
		}
		state, err := CaptureRepositoryState(repoPath)
		if err != nil {
			continue
		}
		if snap.Head != "" && state.Head != "" && snap.Head != state.Head {
			warnings = append(warnings, fmt.Sprintf("repository %s HEAD changed since plan generation (%s -> %s)", repoID, snap.Head[:min(7, len(snap.Head))], state.Head[:min(7, len(state.Head))]))
		}
	}
	return warnings, nil
}

func filepathJoinRepo(workspaceRoot, repoID string) string {
	repos, err := readRepositoryRegistry(workspaceRoot)
	if err != nil {
		return ""
	}
	for _, r := range repos {
		if r.ID == repoID {
			if filepath.IsAbs(r.Path) {
				return r.Path
			}
			return filepath.Join(workspaceRoot, r.Path)
		}
	}
	return ""
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
