package orchestrator

import (
	"fmt"
	"strings"
)

type InvariantReport struct {
	Valid    bool     `json:"valid"`
	Violations []string `json:"violations,omitempty"`
}

func AssertInvariants(tasks []Task, ws WorkflowState, summaries []TaskSummaryRecord) InvariantReport {
	report := InvariantReport{Valid: true, Violations: []string{}}

	active := ActiveTasks(tasks)
	if len(active) > 1 {
		ids := make([]string, 0, len(active))
		for _, t := range active {
			ids = append(ids, t.ID)
		}
		report.Valid = false
		report.Violations = append(report.Violations, fmt.Sprintf("multiple active tasks: %s", strings.Join(ids, ", ")))
	}

	if ws.ActiveTaskID != "" {
		found := false
		for _, t := range tasks {
			if t.ID == ws.ActiveTaskID {
				found = true
				if !IsActiveStatus(t.Status) && t.Status != StatusBlocked {
					report.Valid = false
					report.Violations = append(report.Violations, fmt.Sprintf("lease points to %s with non-active status %s", t.ID, t.Status))
				}
				break
			}
		}
		if !found {
			report.Valid = false
			report.Violations = append(report.Violations, fmt.Sprintf("lease points to missing task %s", ws.ActiveTaskID))
		}
		if len(active) == 1 && active[0].ID != ws.ActiveTaskID && active[0].Status != StatusBlocked {
			// allow mismatch only when scheduling paused for orphan recovery
			if ws.SchedulingPausedReason == "" {
				report.Valid = false
				report.Violations = append(report.Violations, fmt.Sprintf("lease task %s != active task %s", ws.ActiveTaskID, active[0].ID))
			}
		}
	}

	for _, t := range tasks {
		if t.Status == StatusBlocked {
			if strings.TrimSpace(t.BlockedReason) == "" {
				report.Valid = false
				report.Violations = append(report.Violations, fmt.Sprintf("task %s is BLOCKED without blocked_reason", t.ID))
			}
			if strings.TrimSpace(t.RecoveryCommand) == "" {
				report.Valid = false
				report.Violations = append(report.Violations, fmt.Sprintf("task %s is BLOCKED without recovery_command", t.ID))
			}
		}
	}

	// Soft check: non-draft statuses should have at least one summary if summaries provided.
	if len(summaries) > 0 {
		byTask := map[string]int{}
		for _, s := range summaries {
			byTask[s.TaskID]++
		}
		for _, t := range tasks {
			if t.Status == StatusDraft || t.Status == StatusReviewPending || t.Status == StatusPlanned || t.Status == StatusReady {
				continue
			}
			if byTask[t.ID] == 0 {
				report.Valid = false
				report.Violations = append(report.Violations, fmt.Sprintf("task %s status %s has no audit events", t.ID, t.Status))
			}
		}
	}

	return report
}

func CheckWorkspaceInvariants(workspaceRoot string) (InvariantReport, error) {
	tasks, err := LoadTasks(workspaceRoot)
	if err != nil {
		return InvariantReport{}, err
	}
	ws, err := LoadWorkflowState(workspaceRoot)
	if err != nil {
		return InvariantReport{}, err
	}
	summaries, err := LoadRecentTaskSummaries(workspaceRoot, "", 0)
	if err != nil {
		return InvariantReport{}, err
	}
	return AssertInvariants(tasks, ws, summaries), nil
}
