package orchestrator

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

func PlanRevisionTasksPath(workspaceRoot string, revision int) string {
	return filepath.Join(PlanRevisionDir(workspaceRoot, revision), "tasks.json")
}

func TaskStorageRelativePath() string {
	return filepath.Join(DefaultFeatureDir, "tasks", "tasks.json")
}

func MergeValidationReports(base, extra PlanValidationReport) PlanValidationReport {
	out := base
	out.Errors = append(out.Errors, extra.Errors...)
	out.Warnings = append(out.Warnings, extra.Warnings...)
	for k, v := range extra.Checks {
		out.Checks[k] = v
	}
	if !extra.Valid {
		out.Valid = false
	}
	if out.Valid {
		out.Checks["overall"] = "PASS"
	} else {
		out.Checks["overall"] = "FAIL"
	}
	return out
}

func ValidateTaskPlan(workspaceRoot string, tasks []Task, fromTasks bool) (PlanValidationReport, error) {
	report := PlanValidationReport{
		Valid:  true,
		Checks: map[string]string{},
	}

	if len(tasks) == 0 {
		report.Valid = false
		report.Errors = append(report.Errors, PlanValidationIssue{
			Level: "error", Code: "empty_task_plan",
			Message: "task plan is empty",
		})
		report.Checks["task_plan"] = "FAIL"
		return report, nil
	}

	repos, err := readRepositoryRegistry(workspaceRoot)
	if err != nil {
		return report, err
	}
	repoIDs := map[string]bool{"workspace": true}
	for _, r := range repos {
		repoIDs[r.ID] = true
	}

	seenIDs := map[string]bool{}
	byID := map[string]Task{}
	for _, task := range tasks {
		if task.ID == "" {
			report.Valid = false
			report.Errors = append(report.Errors, PlanValidationIssue{
				Level: "error", Code: "empty_task_id", Message: "task ID cannot be empty",
			})
			continue
		}
		if seenIDs[task.ID] {
			report.Valid = false
			report.Errors = append(report.Errors, PlanValidationIssue{
				Level: "error", Code: "duplicate_task_id",
				Message: fmt.Sprintf("duplicate task ID: %s", task.ID),
			})
		}
		seenIDs[task.ID] = true
		byID[task.ID] = task

		if strings.TrimSpace(task.Title) == "" {
			report.Valid = false
			report.Errors = append(report.Errors, PlanValidationIssue{
				Level: "error", Code: "missing_title",
				Message: fmt.Sprintf("task %s has no title", task.ID),
			})
		}
		if strings.TrimSpace(task.Repository) == "" {
			report.Valid = false
			report.Errors = append(report.Errors, PlanValidationIssue{
				Level: "error", Code: "missing_repository",
				Message: fmt.Sprintf("task %s has no repository assignment", task.ID),
			})
		} else if !repoIDs[task.Repository] {
			report.Valid = false
			report.Errors = append(report.Errors, PlanValidationIssue{
				Level: "error", Code: "invalid_repository",
				Message: fmt.Sprintf("task %s references unknown repository %s", task.ID, task.Repository),
			})
		}

		hasVerify := false
		for _, step := range task.Verification {
			if strings.TrimSpace(step.Command) != "" {
				hasVerify = true
				break
			}
		}
		if !hasVerify {
			report.Valid = false
			report.Errors = append(report.Errors, PlanValidationIssue{
				Level: "error", Code: "missing_verification",
				Message: fmt.Sprintf("task %s has no verification command", task.ID),
			})
		}

		if strings.ToUpper(strings.TrimSpace(task.OwnershipConfidence)) == "LOW" {
			report.Warnings = append(report.Warnings, PlanValidationIssue{
				Level: "warning", Code: "low_ownership_confidence",
				Message: fmt.Sprintf("task %s repository ownership confidence is LOW", task.ID),
			})
		}
	}

	if _, err := ValidateGraph(tasks); err != nil {
		report.Valid = false
		report.Errors = append(report.Errors, PlanValidationIssue{
			Level: "error", Code: "dag_invalid", Message: err.Error(),
		})
		report.Checks["dag"] = "FAIL"
	} else {
		report.Checks["dag"] = "PASS"
	}

	for _, task := range tasks {
		for _, dep := range task.Dependencies {
			d, ok := byID[dep]
			if !ok {
				report.Valid = false
				report.Errors = append(report.Errors, PlanValidationIssue{
					Level: "error", Code: "missing_dependency",
					Message: fmt.Sprintf("task %s depends on missing task %s", task.ID, dep),
				})
				continue
			}
			if d.Repository != task.Repository {
				report.Warnings = append(report.Warnings, PlanValidationIssue{
					Level: "warning", Code: "cross_repository_dependency",
					Message: fmt.Sprintf("cross-repository dependency: %s (%s) -> %s (%s)", dep, d.Repository, task.ID, task.Repository),
				})
			}
			if strings.ToUpper(strings.TrimSpace(d.OwnershipConfidence)) == "LOW" && d.Repository != task.Repository {
				report.Valid = false
				report.Errors = append(report.Errors, PlanValidationIssue{
					Level: "error", Code: "low_confidence_cross_repo_downstream",
					Message: fmt.Sprintf("task %s depends on LOW-confidence cross-repo task %s", task.ID, dep),
				})
			}
		}
	}

	if !hasCode(report.Errors, "missing_repository") && !hasCode(report.Errors, "invalid_repository") {
		report.Checks["task_repository_validity"] = "PASS"
	} else {
		report.Checks["task_repository_validity"] = "FAIL"
	}

	if fromTasks {
		hasRequirementTrace := false
		for _, task := range tasks {
			if len(task.RequirementIDs) > 0 {
				hasRequirementTrace = true
				break
			}
		}
		if !hasRequirementTrace && !Exists(filepath.Join(PlansDir(workspaceRoot), "draft", "requirements.json")) {
			report.Warnings = append(report.Warnings, PlanValidationIssue{
				Level: "warning", Code: "requirements_not_explicitly_traced",
				Message: "requirements not explicitly traced; add requirement_ids on tasks or .feature/plans/draft/requirements.json",
			})
		}
	}

	if allTasksBlocked(tasks) {
		report.Warnings = append(report.Warnings, PlanValidationIssue{
			Level: "warning", Code: "all_tasks_blocked",
			Message: "all tasks appear blocked with no executable path",
		})
	}

	if report.Valid {
		report.Checks["task_plan"] = "PASS"
	} else {
		report.Checks["task_plan"] = "FAIL"
	}
	return report, nil
}

func allTasksBlocked(tasks []Task) bool {
	if len(tasks) == 0 {
		return false
	}
	byID := map[string]Task{}
	for _, t := range tasks {
		byID[t.ID] = t
	}
	for _, task := range tasks {
		if task.Status == StatusDone {
			continue
		}
		blocked := false
		for _, dep := range task.Dependencies {
			d, ok := byID[dep]
			if !ok || d.Status != StatusDone {
				blocked = true
				break
			}
		}
		if !blocked {
			return false
		}
	}
	return true
}

func BuildTaskGraphPayload(tasks []Task) map[string]any {
	edges := []map[string]string{}
	for _, task := range tasks {
		for _, dep := range task.Dependencies {
			edges = append(edges, map[string]string{"from": dep, "to": task.ID})
		}
	}
	return map[string]any{"tasks": tasks, "edges": edges}
}

func RenderTaskGraphASCII(tasks []Task) string {
	if len(tasks) == 0 {
		return "(empty)\n"
	}
	lines := []string{}
	for _, task := range tasks {
		if len(task.Dependencies) == 0 {
			lines = append(lines, fmt.Sprintf("%s [%s] %s", task.ID, task.Repository, task.Title))
			continue
		}
		for _, dep := range task.Dependencies {
			lines = append(lines, fmt.Sprintf("%s -> %s [%s]", dep, task.ID, task.Repository))
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

func SnapshotTasksRevision(workspaceRoot string, revision int, tasks []Task) error {
	data, err := json.MarshalIndent(tasks, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomically(PlanRevisionTasksPath(workspaceRoot, revision), append(data, '\n'))
}
