package orchestrator

import (
	"fmt"
	"path/filepath"
	"sort"
	"time"
)

func CanStartTask(ws WorkflowState, task Task) error {
	if !RequiresPlanApproval(ws) {
		return nil
	}
	if ws.WorkflowStatus != WorkflowApproved {
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
	if ws.WorkflowStatus != WorkflowApproved {
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
	Revision int
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
	unlocked, err := PromoteApprovedTasks(tasks)
	if err != nil {
		return ApprovePlanResult{}, ws, nil, err
	}
	result.UnlockedTasks = unlocked

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
		task.Status = StatusReady
		task.UpdatedAt = time.Now().UTC()
		unlocked = append(unlocked, task.ID)
	}
	sort.Strings(unlocked)
	return unlocked, nil
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
