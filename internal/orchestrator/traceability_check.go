package orchestrator

import (
	"fmt"
	"strings"
)

type TraceabilityReport struct {
	Valid    bool                  `json:"valid"`
	Errors   []PlanValidationIssue `json:"errors,omitempty"`
	Warnings []PlanValidationIssue `json:"warnings,omitempty"`
	Checks   map[string]string     `json:"checks"`
}

func RunTraceabilityCheck(workspaceRoot string) (TraceabilityReport, error) {
	report := TraceabilityReport{Valid: true, Checks: map[string]string{}}
	ws, err := LoadWorkflowState(workspaceRoot)
	if err != nil {
		return report, err
	}
	if ws.CurrentPlanRevision <= 0 {
		report.Valid = false
		report.Errors = append(report.Errors, PlanValidationIssue{
			Level: "error", Code: "no_plan_revision", Message: "no approved plan revision exists",
		})
		return report, nil
	}

	bundle, err := LoadRevisionPlanningBundle(workspaceRoot, ws.CurrentPlanRevision)
	if err != nil {
		return report, err
	}
	tasks, err := LoadTasks(workspaceRoot)
	if err != nil {
		return report, err
	}

	reqIDs := map[string]bool{}
	for _, r := range bundle.Requirements.Requirements {
		reqIDs[r.ID] = true
	}
	taskCovers := map[string]bool{}
	for _, task := range tasks {
		for _, reqID := range task.RequirementIDs {
			if !reqIDs[reqID] {
				report.Valid = false
				report.Errors = append(report.Errors, PlanValidationIssue{
					Level: "error", Code: "unknown_requirement_id",
					Message: fmt.Sprintf("task %s references unknown requirement %s", task.ID, reqID),
				})
			}
			if task.Status == StatusDone {
				taskCovers[reqID] = true
			}
		}
	}
	for _, req := range bundle.Requirements.Requirements {
		if !taskCovers[req.ID] {
			report.Valid = false
			report.Errors = append(report.Errors, PlanValidationIssue{
				Level: "error", Code: "requirement_not_done",
				Message: fmt.Sprintf("requirement %s is not satisfied by any DONE task", req.ID),
			})
		}
	}

	for _, task := range tasks {
		if task.Status != StatusDone {
			report.Valid = false
			report.Errors = append(report.Errors, PlanValidationIssue{
				Level: "error", Code: "task_not_done",
				Message: fmt.Sprintf("task %s is not DONE (status=%s)", task.ID, task.Status),
			})
		}
	}

	if !bundle.Loaded["requirements"] {
		report.Warnings = append(report.Warnings, PlanValidationIssue{
			Level: "warning", Code: "requirements_snapshot_missing",
			Message: "requirements snapshot missing for revision; traceability limited",
		})
	}

	if report.Valid {
		report.Checks["traceability"] = "PASS"
	} else {
		report.Checks["traceability"] = "FAIL"
	}
	return report, nil
}

func crossRepoCommands(workspaceRoot string, ws WorkflowState) []string {
	if ws.CurrentPlanRevision > 0 {
		bundle, err := LoadRevisionPlanningBundle(workspaceRoot, ws.CurrentPlanRevision)
		if err == nil && len(bundle.WorkspaceVerify.Commands) > 0 {
			return bundle.WorkspaceVerify.Commands
		}
	}
	doc, err := LoadCurrentPlanDocument(workspaceRoot)
	if err != nil {
		return nil
	}
	if cmds, ok := doc.Verification.ByRepo["workspace"]; ok {
		return cmds
	}
	return nil
}

type CrossRepoVerifyReport struct {
	Valid    bool                  `json:"valid"`
	Results  []VerificationResult  `json:"results,omitempty"`
	Errors   []PlanValidationIssue `json:"errors,omitempty"`
	Checks   map[string]string     `json:"checks"`
}

func RunCrossRepoVerification(workspaceRoot string, timeoutSeconds int) (CrossRepoVerifyReport, error) {
	report := CrossRepoVerifyReport{Valid: true, Checks: map[string]string{}}
	ws, err := LoadWorkflowState(workspaceRoot)
	if err != nil {
		return report, err
	}
	cmds := crossRepoCommands(workspaceRoot, ws)
	if len(cmds) == 0 {
		report.Checks["cross_repo_verify"] = "SKIP"
		return report, nil
	}
	for _, cmd := range cmds {
		cmd = strings.TrimSpace(cmd)
		if cmd == "" {
			continue
		}
		result, err := RunVerification(workspaceRoot, cmd, timeoutSeconds)
		report.Results = append(report.Results, result)
		if err != nil || result.ExitCode != 0 {
			report.Valid = false
			report.Errors = append(report.Errors, PlanValidationIssue{
				Level: "error", Code: "cross_repo_verify_failed",
				Message: fmt.Sprintf("cross-repo command failed: %s (exit=%d)", cmd, result.ExitCode),
			})
		}
	}
	if report.Valid {
		report.Checks["cross_repo_verify"] = "PASS"
	} else {
		report.Checks["cross_repo_verify"] = "FAIL"
	}
	return report, nil
}
