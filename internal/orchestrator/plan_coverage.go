package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

type SourcePlanItem struct {
	Title                  string `json:"title"`
	SourceSection          string `json:"source_section"`
	SourceLine             int    `json:"source_line,omitempty"`
	SuggestedRequirementID string `json:"suggested_requirement_id,omitempty"`
	Kind                   string `json:"kind,omitempty"`
}

type DecompositionHint struct {
	SourceItem    SourcePlanItem `json:"source_item"`
	SuggestedSubs []string       `json:"suggested_sub_items"`
	Message       string         `json:"message"`
}

type CoverageMatrixRow struct {
	PlanItem      string   `json:"plan_item"`
	SourceSection string   `json:"source_section"`
	RequirementID string   `json:"requirement_id,omitempty"`
	TaskIDs       []string `json:"task_ids,omitempty"`
	Status        string   `json:"status"`
}

type PlanCoverageReport struct {
	Valid                     bool                `json:"valid"`
	Source                    string              `json:"source"`
	Checks                    map[string]string   `json:"checks"`
	CoveredRequirementIDs     []string            `json:"covered_requirement_ids,omitempty"`
	UnmappedSourceItems       []SourcePlanItem    `json:"unmapped_source_items,omitempty"`
	RequirementsWithoutTasks  []string            `json:"requirements_without_tasks,omitempty"`
	UnmappedAcceptanceItems   []SourcePlanItem    `json:"unmapped_acceptance_items,omitempty"`
	DecompositionHints        []DecompositionHint `json:"decomposition_hints,omitempty"`
	CoverageMatrix            []CoverageMatrixRow `json:"coverage_matrix,omitempty"`
	AgentActions              []string            `json:"agent_actions,omitempty"`
	SuggestedPrompt           string              `json:"suggested_prompt,omitempty"`
}

var planSectionHeadings = []string{
	"requirements",
	"acceptance criteria",
	"additional business requirements",
	"risks",
	"demo flow",
	"testing requirements",
}

var reqMarkerPattern = regexp.MustCompile(`<!--\s*req:\s*([A-Za-z0-9_-]+)\s*-->`)
var compoundPattern = regexp.MustCompile(`(?i)\b(including|such as|add|edit|select|switch|delete|create|update)\b`)

func PlanDraftCoveragePath(workspaceRoot string) string {
	return draftFilePath(workspaceRoot, "plan-coverage.json")
}

func LoadPlanCoverageReport(workspaceRoot string) (PlanCoverageReport, error) {
	path := PlanDraftCoveragePath(workspaceRoot)
	data, err := os.ReadFile(path)
	if err != nil {
		return PlanCoverageReport{}, err
	}
	var report PlanCoverageReport
	if err := json.Unmarshal(data, &report); err != nil {
		return PlanCoverageReport{}, err
	}
	return report, nil
}

func WritePlanCoverageReport(workspaceRoot string, report PlanCoverageReport) error {
	if err := EnsureDir(PlanDraftDir(workspaceRoot)); err != nil {
		return err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomically(PlanDraftCoveragePath(workspaceRoot), append(data, '\n'))
}

func ResolvePlanPath(workspaceRoot, from string) string {
	if from == "" {
		from = "PLAN.md"
	}
	if filepath.IsAbs(from) {
		return from
	}
	return filepath.Join(workspaceRoot, from)
}

func ParsePlanSourceItems(planPath string) ([]SourcePlanItem, error) {
	data, err := os.ReadFile(planPath)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	items := []SourcePlanItem{}
	currentSection := ""
	reqCounter := 0

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			currentSection = strings.TrimSpace(strings.TrimPrefix(trimmed, "## "))
			continue
		}
		if !isTrackedSection(currentSection) {
			continue
		}
		if marker := reqMarkerPattern.FindStringSubmatch(trimmed); len(marker) == 2 {
			title := strings.TrimSpace(reqMarkerPattern.ReplaceAllString(trimmed, ""))
			if title == "" {
				title = marker[1]
			}
			items = append(items, SourcePlanItem{
				Title:                  title,
				SourceSection:          currentSection,
				SourceLine:             i + 1,
				SuggestedRequirementID: strings.ToUpper(marker[1]),
				Kind:                   sectionKind(currentSection),
			})
			continue
		}
		itemText := extractListItem(trimmed)
		if itemText == "" {
			continue
		}
		reqCounter++
		items = append(items, SourcePlanItem{
			Title:                  itemText,
			SourceSection:          currentSection,
			SourceLine:             i + 1,
			SuggestedRequirementID: fmt.Sprintf("R%03d", reqCounter),
			Kind:                   sectionKind(currentSection),
		})
	}
	return items, nil
}

func isTrackedSection(section string) bool {
	lower := strings.ToLower(strings.TrimSpace(section))
	for _, h := range planSectionHeadings {
		if lower == h {
			return true
		}
	}
	return false
}

func sectionKind(section string) string {
	if strings.EqualFold(section, "Acceptance Criteria") {
		return "acceptance"
	}
	return "requirement"
}

func extractListItem(line string) string {
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}
	if strings.HasPrefix(line, "#") {
		return ""
	}
	prefixes := []string{"- [ ] ", "- [x] ", "- [X] ", "* ", "- "}
	for _, p := range prefixes {
		if strings.HasPrefix(line, p) {
			return strings.TrimSpace(strings.TrimPrefix(line, p))
		}
	}
	if len(line) > 2 && unicode.IsDigit(rune(line[0])) && (line[1] == '.' || (len(line) > 2 && line[2] == '.')) {
		parts := strings.SplitN(line, ".", 2)
		if len(parts) == 2 {
			return strings.TrimSpace(parts[1])
		}
	}
	return ""
}

func normalizeCoverageText(s string) string {
	s = strings.ToLower(s)
	s = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return ' '
	}, s)
	return strings.Join(strings.Fields(s), " ")
}

func requirementMatchesItem(req DraftRequirement, item SourcePlanItem) bool {
	if req.ID != "" && strings.EqualFold(req.ID, item.SuggestedRequirementID) {
		return true
	}
	if req.SourceRef != "" && normalizeCoverageText(req.SourceRef) == normalizeCoverageText(item.Title) {
		return true
	}
	if req.SourceSection != "" && !strings.EqualFold(req.SourceSection, item.SourceSection) {
		return false
	}
	reqText := normalizeCoverageText(req.Description)
	itemText := normalizeCoverageText(item.Title)
	if reqText == "" || itemText == "" {
		return false
	}
	return strings.Contains(reqText, itemText) || strings.Contains(itemText, reqText)
}

func tasksForRequirement(tasks []Task, reqID string) []string {
	ids := []string{}
	for _, task := range tasks {
		for _, rid := range task.RequirementIDs {
			if rid == reqID {
				ids = append(ids, task.ID)
			}
		}
	}
	return ids
}

func detectDecompositionHints(items []SourcePlanItem) []DecompositionHint {
	hints := []DecompositionHint{}
	for _, item := range items {
		if !compoundPattern.MatchString(item.Title) {
			continue
		}
		subs := suggestSubItems(item.Title)
		if len(subs) < 2 {
			continue
		}
		hints = append(hints, DecompositionHint{
			SourceItem:    item,
			SuggestedSubs: subs,
			Message:       fmt.Sprintf("Requirement '%s' appears compound; split into sub-requirements or multiple tasks", item.Title),
		})
	}
	return hints
}

func suggestSubItems(title string) []string {
	lower := strings.ToLower(title)
	if idx := strings.Index(lower, "including "); idx >= 0 {
		rest := title[idx+len("including "):]
		parts := splitCompoundList(rest)
		if len(parts) >= 2 {
			return parts
		}
	}
	verbs := []string{"add", "edit", "select", "switch", "delete", "create", "update"}
	found := []string{}
	for _, v := range verbs {
		if strings.Contains(lower, v) {
			found = append(found, v)
		}
	}
	return found
}

func splitCompoundList(s string) []string {
	s = strings.TrimSuffix(strings.TrimSpace(s), ".")
	re := regexp.MustCompile(`\s*,\s*|\s+and\s+`)
	parts := re.Split(s, -1)
	out := []string{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func RunPlanCoverage(workspaceRoot, planPath string) (PlanCoverageReport, error) {
	resolved := ResolvePlanPath(workspaceRoot, planPath)
	report := PlanCoverageReport{
		Valid:  true,
		Source: resolved,
		Checks: map[string]string{},
	}

	items, err := ParsePlanSourceItems(resolved)
	if err != nil {
		return report, err
	}

	bundle, _ := LoadPlanningDraftBundle(workspaceRoot)
	tasks, _ := LoadTasks(workspaceRoot)

	reqByID := map[string]DraftRequirement{}
	for _, req := range bundle.Requirements.Requirements {
		reqByID[req.ID] = req
	}

	mappedReqIDs := map[string]bool{}
	matrix := []CoverageMatrixRow{}

	for _, item := range items {
		var matchedID string
		for _, req := range bundle.Requirements.Requirements {
			if requirementMatchesItem(req, item) {
				matchedID = req.ID
				mappedReqIDs[req.ID] = true
				break
			}
		}
		row := CoverageMatrixRow{
			PlanItem:      item.Title,
			SourceSection: item.SourceSection,
			Status:        "UNMAPPED",
		}
		if matchedID != "" {
			row.RequirementID = matchedID
			row.TaskIDs = tasksForRequirement(tasks, matchedID)
			if len(row.TaskIDs) > 0 {
				row.Status = "COVERED"
			} else {
				row.Status = "MISSING_TASK"
				report.RequirementsWithoutTasks = appendUnique(report.RequirementsWithoutTasks, matchedID)
			}
		} else {
			report.UnmappedSourceItems = append(report.UnmappedSourceItems, item)
			row.Status = "MISSING_REQUIREMENT"
		}
		matrix = append(matrix, row)
	}

	for _, req := range bundle.Requirements.Requirements {
		if len(tasksForRequirement(tasks, req.ID)) == 0 {
			report.RequirementsWithoutTasks = appendUnique(report.RequirementsWithoutTasks, req.ID)
		}
	}

	for _, item := range items {
		if item.Kind != "acceptance" {
			continue
		}
		covered := false
		for _, req := range bundle.Requirements.Requirements {
			if requirementMatchesItem(req, item) {
				if len(tasksForRequirement(tasks, req.ID)) > 0 {
					covered = true
					break
				}
			}
		}
		if !covered {
			report.UnmappedAcceptanceItems = append(report.UnmappedAcceptanceItems, item)
		}
	}

	report.CoverageMatrix = matrix
	report.DecompositionHints = detectDecompositionHints(items)

	for id := range mappedReqIDs {
		report.CoveredRequirementIDs = appendUnique(report.CoveredRequirementIDs, id)
	}

	report.Checks["completeness"] = "PASS"
	if len(report.UnmappedSourceItems) > 0 {
		report.Checks["completeness"] = "FAIL"
		report.Valid = false
	}

	report.Checks["decomposition"] = "PASS"
	if len(report.RequirementsWithoutTasks) > 0 {
		report.Checks["decomposition"] = "FAIL"
		report.Valid = false
	}

	report.Checks["traceability"] = "PASS"
	for _, task := range tasks {
		for _, reqID := range task.RequirementIDs {
			if _, ok := reqByID[reqID]; !ok {
				report.Checks["traceability"] = "FAIL"
				report.Valid = false
			}
		}
	}

	report.Checks["acceptance_coverage"] = "PASS"
	if len(report.UnmappedAcceptanceItems) > 0 {
		report.Checks["acceptance_coverage"] = "FAIL"
		report.Valid = false
	} else if len(report.DecompositionHints) > 0 {
		report.Checks["acceptance_coverage"] = "WARN"
	}

	report.AgentActions = buildCoverageAgentActions(report, planPath)
	report.SuggestedPrompt = buildCoverageSuggestedPrompt(report)

	_ = WritePlanCoverageReport(workspaceRoot, report)
	return report, nil
}

func buildCoverageAgentActions(report PlanCoverageReport, planPath string) []string {
	actions := []string{}
	for _, item := range report.UnmappedSourceItems {
		actions = append(actions, fmt.Sprintf("Add requirement %s for '%s' (%s) to requirements.json", item.SuggestedRequirementID, item.Title, item.SourceSection))
	}
	for _, reqID := range report.RequirementsWithoutTasks {
		actions = append(actions, fmt.Sprintf("Map requirement %s to at least one task via requirement_ids in tasks.json", reqID))
	}
	for _, item := range report.UnmappedAcceptanceItems {
		actions = append(actions, fmt.Sprintf("Link acceptance criterion '%s' to a task with requirement_ids and verification", item.Title))
	}
	for _, hint := range report.DecompositionHints {
		actions = append(actions, hint.Message)
	}
	if len(actions) == 0 {
		return actions
	}
	actions = append(actions, fmt.Sprintf("Re-run: feature-dev plan coverage --from %s --json", planPath))
	return actions
}

func buildCoverageSuggestedPrompt(report PlanCoverageReport) string {
	if report.Valid {
		return "PLAN.md coverage is complete. Run feature-dev task preview --json then plan submit --from-tasks."
	}
	parts := []string{}
	if len(report.UnmappedSourceItems) > 0 {
		parts = append(parts, fmt.Sprintf("%d unmapped PLAN item(s)", len(report.UnmappedSourceItems)))
	}
	if len(report.RequirementsWithoutTasks) > 0 {
		parts = append(parts, fmt.Sprintf("%d requirement(s) without tasks", len(report.RequirementsWithoutTasks)))
	}
	if len(report.UnmappedAcceptanceItems) > 0 {
		parts = append(parts, fmt.Sprintf("%d acceptance criterion(s) without tasks", len(report.UnmappedAcceptanceItems)))
	}
	return "PLAN.md coverage incomplete: " + strings.Join(parts, ", ") + ". Update requirements.json and tasks.json using agent_actions from plan coverage output."
}

func appendUnique(items []string, value string) []string {
	for _, v := range items {
		if v == value {
			return items
		}
	}
	return append(items, value)
}

func MergeCoverageIntoValidation(report PlanValidationReport, coverage PlanCoverageReport) PlanValidationReport {
	out := report
	for k, v := range coverage.Checks {
		out.Checks[k] = v
	}
	if !coverage.Valid {
		out.Valid = false
		for _, item := range coverage.UnmappedSourceItems {
			out.Errors = append(out.Errors, PlanValidationIssue{
				Level:   "error",
				Code:    "plan_completeness_gap",
				Message: fmt.Sprintf("PLAN item '%s' in section %s is not mapped to requirements.json", item.Title, item.SourceSection),
			})
		}
		for _, reqID := range coverage.RequirementsWithoutTasks {
			out.Errors = append(out.Errors, PlanValidationIssue{
				Level:   "error",
				Code:    "orphan_requirement",
				Message: fmt.Sprintf("requirement %s is not covered by any task", reqID),
			})
		}
		for _, item := range coverage.UnmappedAcceptanceItems {
			out.Errors = append(out.Errors, PlanValidationIssue{
				Level:   "error",
				Code:    "acceptance_coverage_gap",
				Message: fmt.Sprintf("acceptance criterion '%s' is not linked to any task", item.Title),
			})
		}
	}
	for _, hint := range coverage.DecompositionHints {
		out.Warnings = append(out.Warnings, PlanValidationIssue{
			Level:   "warning",
			Code:    "decomposition_hint",
			Message: hint.Message,
		})
	}
	return out
}
