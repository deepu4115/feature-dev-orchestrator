package orchestrator

import (
	"encoding/json"
	"fmt"
	"strings"
)

var structuralErrorCodes = map[string]bool{
	"invalid_task_json":         true,
	"invalid_verification_type": true,
	"invalid_tasks_wrapper":     true,
	"invalid_draft_wrapper":     true,
	"draft_bundle_load_error":   true,
	"invalid_repository":        true,
	"unknown_requirement_id":    true,
	"verification_command_unavailable": true,
}

func IsStructuralErrorCode(code string) bool {
	return structuralErrorCodes[code]
}

func HasOnlyStructuralErrors(report PlanValidationReport) bool {
	if len(report.Errors) == 0 {
		return false
	}
	for _, err := range report.Errors {
		if !IsStructuralErrorCode(err.Code) {
			return false
		}
	}
	return true
}

func HasStructuralErrors(report PlanValidationReport) bool {
	for _, err := range report.Errors {
		if IsStructuralErrorCode(err.Code) {
			return true
		}
	}
	return false
}

func translateTasksJSONError(path string, data []byte, err error) []PlanValidationIssue {
	msg := err.Error()
	issues := []PlanValidationIssue{}

	if strings.Contains(msg, "cannot unmarshal object into Go value of type []") {
		var wrapper struct {
			Tasks []Task `json:"tasks"`
		}
		if json.Unmarshal(data, &wrapper) == nil && len(wrapper.Tasks) > 0 {
			issues = append(issues, PlanValidationIssue{
				Level: "error", Code: "invalid_tasks_wrapper",
				Message: fmt.Sprintf("%s: tasks.json must be a top-level JSON array, not {\"tasks\": [...]}. Run: feature-dev task init --force", path),
			})
			return issues
		}
	}

	if strings.Contains(msg, "verification") && strings.Contains(msg, "VerificationStep") {
		issues = append(issues, PlanValidationIssue{
			Level: "error", Code: "invalid_verification_type",
			Message: fmt.Sprintf("%s: verification must be an array of objects with a command field, e.g. [{\"command\": \"go test ./...\"}]. Run: feature-dev schema show tasks --json", path),
		})
		return issues
	}

	issues = append(issues, PlanValidationIssue{
		Level: "error", Code: "invalid_task_json",
		Message: fmt.Sprintf("%s: invalid tasks.json — %s. Expected a top-level JSON array. Run: feature-dev schema show tasks --json", path, msg),
	})
	return issues
}

func translateDraftJSONError(path string, draftName string, err error) PlanValidationIssue {
	msg := err.Error()
	rootKey := draftRootKey(draftName)
	if rootKey != "" && (strings.Contains(msg, "cannot unmarshal array") || strings.Contains(msg, "cannot unmarshal")) {
		return PlanValidationIssue{
			Level: "error", Code: "invalid_draft_wrapper",
			Message: fmt.Sprintf("%s: %s must be a JSON object with a top-level \"%s\" array. Run: feature-dev schema show %s --json", path, draftName, rootKey, draftSchemaName(draftName)),
		}
	}
	return PlanValidationIssue{
		Level: "error", Code: "draft_bundle_load_error",
		Message: fmt.Sprintf("%s: %s — %s. Run: feature-dev schema show %s --json", path, draftName, msg, draftSchemaName(draftName)),
	}
}

func draftRootKey(draftName string) string {
	switch draftName {
	case "requirements":
		return "requirements"
	case "assumptions":
		return "assumptions"
	case "risks":
		return "risks"
	case "impact":
		return "impact"
	case "repo-analysis":
		return "repositories"
	default:
		return ""
	}
}

func draftSchemaName(draftName string) string {
	if draftName == "repo-analysis" {
		return "repo-analysis"
	}
	return draftName
}

func translateDraftLoadError(err error) PlanValidationIssue {
	msg := err.Error()
	if strings.Contains(msg, "unmarshal ") {
		draftName := draftNameFromLoadError(msg)
		if draftName != "" {
			return translateDraftJSONError(extractPathFromLoadError(msg), draftName, err)
		}
	}
	return PlanValidationIssue{
		Level: "error", Code: "draft_bundle_load_error",
		Message: fmt.Sprintf("%s. Run: feature-dev plan draft init", msg),
	}
}

func draftNameFromLoadError(msg string) string {
	for _, name := range []string{"requirements.json", "assumptions.json", "risks.json", "impact.json", "repo-analysis.json", "workspace-verify.json"} {
		if strings.Contains(msg, name) {
			return strings.TrimSuffix(name, ".json")
		}
	}
	if strings.Contains(msg, "missing draft file:") {
		for _, name := range []string{"requirements", "assumptions", "risks", "impact", "repo-analysis"} {
			if strings.Contains(msg, name) {
				return name
			}
		}
	}
	return ""
}

func extractPathFromLoadError(msg string) string {
	if idx := strings.Index(msg, "unmarshal "); idx >= 0 {
		rest := msg[idx+len("unmarshal "):]
		if colon := strings.Index(rest, ":"); colon > 0 {
			return strings.TrimSpace(rest[:colon])
		}
	}
	return msg
}

func structuralReportFromIssues(issues []PlanValidationIssue) PlanValidationReport {
	report := PlanValidationReport{
		Valid:  len(issues) == 0,
		Checks: map[string]string{},
	}
	if len(issues) > 0 {
		report.Errors = issues
		report.Checks["structure"] = "FAIL"
	} else {
		report.Checks["structure"] = "PASS"
	}
	return report
}
