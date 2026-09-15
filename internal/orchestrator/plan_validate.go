package orchestrator

import (
	"fmt"
	"strings"
)

type PlanValidationIssue struct {
	Level   string `json:"level"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type PlanValidationReport struct {
	Valid    bool                  `json:"valid"`
	Errors   []PlanValidationIssue `json:"errors"`
	Warnings []PlanValidationIssue `json:"warnings"`
	Checks   map[string]string     `json:"checks"`
}

func ValidatePlanDocument(workspaceRoot string, doc PlanDocument) (PlanValidationReport, error) {
	report := PlanValidationReport{
		Valid:  true,
		Checks: map[string]string{},
	}

	runtimeTasks := PlanTasksToRuntimeTasks(doc.Tasks)
	taskReport, err := ValidateTaskPlan(workspaceRoot, runtimeTasks, false)
	if err != nil {
		return report, err
	}
	report = MergeValidationReports(report, taskReport)

	taskCoversReq := map[string]bool{}
	for _, req := range doc.Requirements {
		if len(req.Tasks) == 0 {
			report.Valid = false
			report.Errors = append(report.Errors, PlanValidationIssue{
				Level: "error", Code: "orphan_requirement",
				Message: fmt.Sprintf("requirement %s is not covered by any task", req.ID),
			})
			continue
		}
		for range req.Tasks {
			taskCoversReq[req.ID] = true
		}
	}
	for _, req := range doc.Requirements {
		if !taskCoversReq[req.ID] {
			report.Valid = false
			report.Errors = append(report.Errors, PlanValidationIssue{
				Level: "error", Code: "orphan_requirement",
				Message: fmt.Sprintf("requirement %s is not covered by any task", req.ID),
			})
		}
	}
	if !hasCode(report.Errors, "orphan_requirement") {
		report.Checks["requirements_coverage"] = "PASS"
	} else {
		report.Checks["requirements_coverage"] = "FAIL"
	}

	for _, ac := range doc.AcceptanceCriteria {
		if len(ac.ImplementationTasks) == 0 || len(ac.PlannedVerification) == 0 {
			report.Valid = false
			report.Errors = append(report.Errors, PlanValidationIssue{
				Level: "error", Code: "acceptance_criteria_incomplete",
				Message: fmt.Sprintf("acceptance criterion %s missing implementation_tasks or planned_verification", ac.ID),
			})
		}
	}
	if !hasCode(report.Errors, "acceptance_criteria_incomplete") {
		report.Checks["verification_mapping"] = "PASS"
	} else {
		report.Checks["verification_mapping"] = "FAIL"
	}

	hasVerification := len(doc.Verification.ByRepo) > 0
	if !hasVerification {
		for _, task := range doc.Tasks {
			if len(task.Verification) > 0 {
				hasVerification = true
				break
			}
		}
	}
	if !hasVerification {
		report.Valid = false
		report.Errors = append(report.Errors, PlanValidationIssue{
			Level: "error", Code: "empty_verification_strategy",
			Message: "verification strategy is empty",
		})
		report.Checks["verification_strategy"] = "FAIL"
	} else {
		report.Checks["verification_strategy"] = "PASS"
	}

	for _, a := range doc.Assumptions {
		conf := strings.ToUpper(a.Confidence)
		impact := strings.ToUpper(a.Impact)
		if conf == "LOW" && impact == "HIGH" {
			report.Warnings = append(report.Warnings, PlanValidationIssue{
				Level: "warning", Code: "low_confidence_high_impact_assumption",
				Message: fmt.Sprintf("assumption %s is LOW confidence with HIGH impact", a.ID),
			})
		}
	}

	for _, r := range doc.Risks {
		if strings.ToUpper(r.Level) == "CRITICAL" && len(r.Mitigation) == 0 {
			report.Valid = false
			report.Errors = append(report.Errors, PlanValidationIssue{
				Level: "error", Code: "critical_risk_without_mitigation",
				Message: fmt.Sprintf("risk %s is CRITICAL without mitigation", r.ID),
			})
		} else if len(r.Mitigation) == 0 {
			report.Warnings = append(report.Warnings, PlanValidationIssue{
				Level: "warning", Code: "risk_without_mitigation",
				Message: fmt.Sprintf("risk %s has no mitigation", r.ID),
			})
		}
	}

	if report.Valid {
		report.Checks["overall"] = "PASS"
	} else {
		report.Checks["overall"] = "FAIL"
	}
	return report, nil
}

func hasCode(issues []PlanValidationIssue, code string) bool {
	for _, i := range issues {
		if i.Code == code {
			return true
		}
	}
	return false
}
