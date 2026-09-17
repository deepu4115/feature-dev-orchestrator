package orchestrator

import (
	"fmt"
)

type DoctorCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
	Fix     string `json:"fix,omitempty"`
}

type DoctorReport struct {
	Valid  bool          `json:"valid"`
	Checks []DoctorCheck `json:"checks"`
}

func RunOrderedPlanValidation(workspaceRoot string, planPath string) (PlanValidationReport, error) {
	taskLoad, err := LoadTasksWithReport(workspaceRoot)
	if err != nil {
		return PlanValidationReport{}, err
	}
	if !taskLoad.Report.Valid {
		return taskLoad.Report, nil
	}

	report, err := ValidateTaskPlan(workspaceRoot, taskLoad.Tasks, true)
	if err != nil {
		return report, err
	}

	bundle, err := LoadPlanningDraftBundle(workspaceRoot)
	if err != nil {
		report.Valid = false
		report.Errors = append(report.Errors, PlanValidationIssue{
			Level: "error", Code: "draft_bundle_load_error", Message: err.Error(),
		})
		return report, nil
	}

	bundleEarly := ValidateDraftBundleEarly(workspaceRoot, bundle)
	report = MergeValidationReports(report, bundleEarly)
	if !report.Valid {
		return report, nil
	}

	doc := MergeDraftBundleIntoPlan(bundle, taskLoad.Tasks, PlanFeature{})
	merged := ValidateDraftBundleMerged(workspaceRoot, bundle, taskLoad.Tasks, doc)
	report = MergeValidationReports(report, merged)

	if planPath == "" {
		planPath = "PLAN.md"
	}
	resolvedPlan := ResolvePlanPath(workspaceRoot, planPath)
	if Exists(resolvedPlan) {
		coverage, err := RunPlanCoverage(workspaceRoot, planPath)
		if err == nil {
			report = MergeCoverageIntoValidation(report, coverage)
		}
	}

	return report, nil
}

func RunDoctor(workspaceRoot string) (DoctorReport, error) {
	report := DoctorReport{Valid: true, Checks: []DoctorCheck{}}

	if !Exists(ResolveFeatureDir(workspaceRoot)) {
		report.Valid = false
		report.Checks = append(report.Checks, DoctorCheck{
			Name: "workspace_init", Status: "FAIL",
			Message: "workspace not initialized",
			Fix:     "feature-dev init",
		})
		return report, nil
	}
	report.Checks = append(report.Checks, DoctorCheck{Name: "workspace_init", Status: "PASS"})

	cfg, err := LoadWorkspaceConfig(workspaceRoot)
	if err != nil || cfg.SchemaVersion == "" {
		report.Valid = false
		report.Checks = append(report.Checks, DoctorCheck{
			Name: "workspace_config", Status: "FAIL",
			Message: "invalid workspace config",
			Fix:     "feature-dev init",
		})
	} else {
		report.Checks = append(report.Checks, DoctorCheck{Name: "workspace_config", Status: "PASS"})
	}

	taskLoad, err := LoadTasksWithReport(workspaceRoot)
	if err != nil {
		report.Valid = false
		report.Checks = append(report.Checks, DoctorCheck{Name: "tasks_json", Status: "FAIL", Message: err.Error()})
	} else if !taskLoad.Report.Valid {
		report.Valid = false
		report.Checks = append(report.Checks, DoctorCheck{
			Name: "tasks_json", Status: "FAIL",
			Message: firstErrorMessage(taskLoad.Report),
			Fix:     "feature-dev schema show tasks --json",
		})
	} else {
		report.Checks = append(report.Checks, DoctorCheck{Name: "tasks_json", Status: "PASS"})
	}

	bundle, err := LoadPlanningDraftBundle(workspaceRoot)
	if err != nil {
		report.Valid = false
		report.Checks = append(report.Checks, DoctorCheck{
			Name: "draft_bundle", Status: "FAIL", Message: err.Error(),
			Fix: "feature-dev plan draft init",
		})
	} else {
		early := ValidateDraftBundleEarly(workspaceRoot, bundle)
		if !early.Valid {
			report.Valid = false
			report.Checks = append(report.Checks, DoctorCheck{
				Name: "draft_bundle", Status: "FAIL",
				Message: firstErrorMessage(early),
				Fix:     "feature-dev plan draft init --force",
			})
		} else {
			report.Checks = append(report.Checks, DoctorCheck{Name: "draft_bundle", Status: "PASS"})
		}
	}

	ws, _ := LoadWorkflowState(workspaceRoot)
	if ws.WorkflowStatus == WorkflowApproved && ws.ApprovedPlanRevision != nil && *ws.ApprovedPlanRevision != ws.CurrentPlanRevision {
		report.Valid = false
		report.Checks = append(report.Checks, DoctorCheck{
			Name: "workflow_consistency", Status: "FAIL",
			Message: "approved revision does not match current revision",
			Fix:     "feature-dev replan",
		})
	} else {
		report.Checks = append(report.Checks, DoctorCheck{Name: "workflow_consistency", Status: "PASS"})
	}

	if Exists(PlanDraftLastValidationPath(workspaceRoot)) && ws.WorkflowStatus != WorkflowPlanning && ws.WorkflowStatus != WorkflowClarificationNeeded {
		report.Checks = append(report.Checks, DoctorCheck{
			Name: "stale_validation_artifact", Status: "WARN",
			Message: "last-validation.json exists but workflow has moved past planning",
			Fix:     "feature-dev task preview --json",
		})
	} else {
		report.Checks = append(report.Checks, DoctorCheck{Name: "stale_validation_artifact", Status: "PASS"})
	}

	planPath := ResolvePlanPath(workspaceRoot, "PLAN.md")
	if Exists(planPath) {
		coverage, err := RunPlanCoverage(workspaceRoot, "PLAN.md")
		if err == nil && !coverage.Valid {
			report.Valid = false
			report.Checks = append(report.Checks, DoctorCheck{
				Name: "plan_coverage", Status: "FAIL",
				Message: coverage.SuggestedPrompt,
				Fix:     fmt.Sprintf("feature-dev plan coverage --from %s --json", "PLAN.md"),
			})
		} else if err == nil {
			report.Checks = append(report.Checks, DoctorCheck{Name: "plan_coverage", Status: "PASS"})
		}
	}

	return report, nil
}

func firstErrorMessage(report PlanValidationReport) string {
	if len(report.Errors) > 0 {
		return report.Errors[0].Message
	}
	return "validation failed"
}

func ClearValidationArtifactsOnSuccess(workspaceRoot string) {
	_ = ClearLastValidationReport(workspaceRoot)
}
