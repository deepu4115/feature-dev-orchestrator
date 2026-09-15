package orchestrator

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

type PlanReviewJSON struct {
	Revision            int                   `json:"revision"`
	Status              string                `json:"status"`
	ApprovalRequired    bool                  `json:"approval_required"`
	TasksFile           string                `json:"tasks_file"`
	TasksSnapshot       string                `json:"tasks_snapshot,omitempty"`
	TaskItems           []Task                `json:"tasks"`
	Graph               map[string]any        `json:"graph"`
	Tasks               int                   `json:"task_count"`
	Repositories        int                   `json:"repositories"`
	Assumptions         int                   `json:"assumptions"`
	HighRiskAssumptions int                   `json:"high_risk_assumptions"`
	Risks               map[string]int        `json:"risks"`
	Validation          map[string]string     `json:"validation"`
	Errors              []PlanValidationIssue `json:"errors,omitempty"`
	Warnings            []PlanValidationIssue `json:"warnings,omitempty"`
}

func BuildPlanReviewJSON(workspaceRoot string, doc PlanDocument, ws WorkflowState, report PlanValidationReport, runtimeTasks []Task) PlanReviewJSON {
	riskCounts := map[string]int{}
	for _, r := range doc.Risks {
		riskCounts[strings.ToLower(r.Level)]++
	}
	highRiskAssumptions := 0
	for _, a := range doc.Assumptions {
		if strings.ToUpper(a.Confidence) == "LOW" && strings.ToUpper(a.Impact) == "HIGH" {
			highRiskAssumptions++
		}
	}
	snapshot := ""
	if ws.CurrentPlanRevision > 0 {
		snapshot = PlanRevisionTasksPath(workspaceRoot, ws.CurrentPlanRevision)
	}
	return PlanReviewJSON{
		Revision:            ws.CurrentPlanRevision,
		Status:              string(ws.WorkflowStatus),
		ApprovalRequired:    RequiresPlanApproval(ws) && ws.WorkflowStatus != WorkflowApproved,
		TasksFile:           TaskStorageRelativePath(),
		TasksSnapshot:       snapshot,
		TaskItems:           runtimeTasks,
		Graph:               BuildTaskGraphPayload(runtimeTasks),
		Tasks:               len(runtimeTasks),
		Repositories:        len(doc.Repositories),
		Assumptions:         len(doc.Assumptions),
		HighRiskAssumptions: highRiskAssumptions,
		Risks:               riskCounts,
		Validation:          report.Checks,
		Errors:              report.Errors,
		Warnings:            report.Warnings,
	}
}

func RenderTasksReviewText(workspaceRoot string, ws WorkflowState, tasks []Task, report PlanValidationReport) string {
	var b strings.Builder
	b.WriteString("Task Plan Review")
	if ws.CurrentPlanRevision > 0 {
		b.WriteString(fmt.Sprintf(" (revision %d)", ws.CurrentPlanRevision))
	}
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("Status: %s\n\n", ws.WorkflowStatus))
	b.WriteString(fmt.Sprintf("Review file: %s\n", TaskStorageRelativePath()))
	if ws.CurrentPlanRevision > 0 {
		b.WriteString(fmt.Sprintf("Snapshot:    %s\n", PlanRevisionTasksPath(workspaceRoot, ws.CurrentPlanRevision)))
	}
	b.WriteString("\nTasks:\n")
	for _, task := range tasks {
		line := fmt.Sprintf("  %s [%s] %s", task.ID, task.Repository, task.Title)
		if len(task.Dependencies) > 0 {
			line += fmt.Sprintf(" (depends: %s)", strings.Join(task.Dependencies, ", "))
		}
		b.WriteString(line + "\n")
		if task.RepositoryRationale != "" {
			b.WriteString(fmt.Sprintf("    rationale: %s\n", task.RepositoryRationale))
		}
		if task.OwnershipConfidence != "" {
			b.WriteString(fmt.Sprintf("    ownership_confidence: %s\n", task.OwnershipConfidence))
		}
	}
	b.WriteString("\nDependency graph:\n")
	b.WriteString(RenderTaskGraphASCII(tasks))
	if len(report.Warnings) > 0 {
		b.WriteString("\nWarnings:\n")
		for _, w := range report.Warnings {
			b.WriteString(fmt.Sprintf("  - %s\n", w.Message))
		}
	}
	if len(report.Errors) > 0 {
		b.WriteString("\nValidation errors:\n")
		for _, e := range report.Errors {
			b.WriteString(fmt.Sprintf("  - %s\n", e.Message))
		}
	}
	if ws.WorkflowStatus == WorkflowReviewPending {
		b.WriteString("\nUSER APPROVAL REQUIRED\n")
	} else if ws.WorkflowStatus == WorkflowApproved {
		b.WriteString("\nApproved.\n")
	}
	return b.String()
}

func GenerateReviewArtifacts(workspaceRoot string, revision int, doc PlanDocument, prev *PlanDocument, ws WorkflowState, report PlanValidationReport) error {
	dir := PlanRevisionDir(workspaceRoot, revision)
	reviewMD := RenderReviewMarkdown(doc, prev, ws, report)
	summaryMD := RenderSummaryMarkdown(doc, ws)
	dagJSON, err := json.MarshalIndent(buildDAGPayload(doc), "", "  ")
	if err != nil {
		return err
	}
	if err := WriteFileAtomically(filepath.Join(dir, "review.md"), []byte(reviewMD)); err != nil {
		return err
	}
	if err := WriteFileAtomically(filepath.Join(dir, "summary.md"), []byte(summaryMD)); err != nil {
		return err
	}
	return WriteFileAtomically(filepath.Join(dir, "dag.json"), append(dagJSON, '\n'))
}

func buildDAGPayload(doc PlanDocument) map[string]any {
	edges := []map[string]string{}
	for _, task := range doc.Tasks {
		for _, dep := range task.Dependencies {
			edges = append(edges, map[string]string{"from": dep, "to": task.ID})
		}
	}
	return map[string]any{"tasks": doc.Tasks, "edges": edges}
}

func RenderReviewMarkdown(doc PlanDocument, prev *PlanDocument, ws WorkflowState, report PlanValidationReport) string {
	var b strings.Builder
	b.WriteString("# Feature Plan Review\n\n")
	b.WriteString("## Objective\n\n")
	b.WriteString(fmt.Sprintf("%s\n\n", doc.Feature.Title))
	b.WriteString("## Relevant Repositories\n\n")
	for _, r := range doc.Repositories {
		if r.Relevant {
			b.WriteString(fmt.Sprintf("- %s\n", r.ID))
		}
	}
	b.WriteString("\n## Requirements\n\n")
	for _, req := range doc.Requirements {
		b.WriteString(fmt.Sprintf("- **%s**: %s\n", req.ID, req.Description))
	}
	b.WriteString("\n## Proposed Tasks\n\n")
	for _, task := range doc.Tasks {
		b.WriteString(fmt.Sprintf("- **%s** [%s]: %s", task.ID, task.Repository, task.Title))
		if len(task.Dependencies) > 0 {
			b.WriteString(fmt.Sprintf(" (depends on: %s)", strings.Join(task.Dependencies, ", ")))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n## Dependency Graph\n\n```text\n")
	b.WriteString(RenderDAGASCII(doc))
	b.WriteString("```\n\n")
	b.WriteString("## Cross-Repository Dependencies\n\n")
	b.WriteString(renderCrossRepoDeps(doc))
	b.WriteString("\n## Assumptions\n\n")
	for _, a := range doc.Assumptions {
		b.WriteString(fmt.Sprintf("- **%s** (%s", a.ID, a.Confidence))
		if a.Impact != "" {
			b.WriteString(fmt.Sprintf(", impact %s", a.Impact))
		}
		b.WriteString(fmt.Sprintf("): %s\n", a.Statement))
	}
	b.WriteString("\n## Risks\n\n")
	for _, r := range doc.Risks {
		b.WriteString(fmt.Sprintf("- **%s** [%s/%s]: %s\n", r.ID, r.Level, r.Type, r.Description))
	}
	b.WriteString("\n## Expected Change Areas\n\n")
	for repo, areas := range doc.ExpectedChangeAreas {
		b.WriteString(fmt.Sprintf("- **%s**: %s\n", repo, strings.Join(areas, ", ")))
	}
	b.WriteString("\n## Verification Plan\n\n")
	if doc.Verification.Strategy != "" {
		b.WriteString(fmt.Sprintf("Strategy: %s\n\n", doc.Verification.Strategy))
	}
	for repo, cmds := range doc.Verification.ByRepo {
		b.WriteString(fmt.Sprintf("- **%s**: %s\n", repo, strings.Join(cmds, ", ")))
	}
	b.WriteString("\n## Open Questions\n\n")
	if len(doc.OpenQuestions) == 0 {
		b.WriteString("- None\n")
	} else {
		for _, q := range doc.OpenQuestions {
			b.WriteString(fmt.Sprintf("- %s\n", q))
		}
	}
	b.WriteString("\n## Plan Changes Since Previous Revision\n\n")
	b.WriteString(renderPlanChangesSince(prev, doc))
	b.WriteString("\n## Approval Status\n\n")
	b.WriteString(fmt.Sprintf("Revision: %d\n\nStatus: %s\n\n", ws.CurrentPlanRevision, ws.WorkflowStatus))
	if ws.WorkflowStatus == WorkflowReviewPending {
		b.WriteString("**USER APPROVAL REQUIRED**\n")
	} else if ws.WorkflowStatus == WorkflowApproved {
		b.WriteString("Approved.\n")
	}
	if len(report.Warnings) > 0 {
		b.WriteString("\n### Validation Warnings\n\n")
		for _, w := range report.Warnings {
			b.WriteString(fmt.Sprintf("- %s\n", w.Message))
		}
	}
	return b.String()
}

func RenderSummaryMarkdown(doc PlanDocument, ws WorkflowState) string {
	var b strings.Builder
	b.WriteString("Feature Plan\n")
	b.WriteString(strings.Repeat("─", 40) + "\n\n")
	b.WriteString(fmt.Sprintf("Revision: %d\nStatus: %s\n\n", ws.CurrentPlanRevision, ws.WorkflowStatus))
	b.WriteString("Repositories:\n")
	for _, r := range doc.Repositories {
		if r.Relevant {
			b.WriteString(fmt.Sprintf("  %s\n", r.ID))
		}
	}
	b.WriteString(fmt.Sprintf("\nTasks: %d\n\n", len(doc.Tasks)))
	for _, task := range doc.Tasks {
		b.WriteString(fmt.Sprintf("%s [%s]\n%s\n", task.ID, task.Repository, task.Title))
		if len(task.Dependencies) > 0 {
			b.WriteString(fmt.Sprintf("Depends on: %s\n", strings.Join(task.Dependencies, ", ")))
		}
		b.WriteString("\n")
	}
	if ws.WorkflowStatus == WorkflowReviewPending {
		b.WriteString("Status:\nUSER APPROVAL REQUIRED\n")
	}
	return b.String()
}

func RenderDAGASCII(doc PlanDocument) string {
	if len(doc.Tasks) == 0 {
		return "(empty)\n"
	}
	lines := []string{}
	for _, task := range doc.Tasks {
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

func renderCrossRepoDeps(doc PlanDocument) string {
	byID := map[string]PlanTask{}
	for _, t := range doc.Tasks {
		byID[t.ID] = t
	}
	lines := []string{}
	for _, task := range doc.Tasks {
		for _, dep := range task.Dependencies {
			d, ok := byID[dep]
			if ok && d.Repository != task.Repository {
				lines = append(lines, fmt.Sprintf("- %s (%s) -> %s (%s)", dep, d.Repository, task.ID, task.Repository))
			}
		}
	}
	if len(lines) == 0 {
		return "- None\n"
	}
	return strings.Join(lines, "\n") + "\n"
}

func renderPlanChangesSince(prev *PlanDocument, current PlanDocument) string {
	if prev == nil {
		return "- Initial plan revision\n"
	}
	diff := ComparePlanDocuments(*prev, current)
	return RenderPlanDiffText(diff)
}
