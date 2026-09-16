package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

type ClarificationQuestion struct {
	ID         string            `json:"id"`
	ErrorCode  string            `json:"error_code"`
	Context    map[string]string `json:"context,omitempty"`
	Question   string            `json:"question"`
	AnswerType string            `json:"answer_type"`
	Choices    []string          `json:"choices,omitempty"`
}

type ClarificationRequest struct {
	AttemptAt         time.Time             `json:"attempt_at"`
	RevisionAttempted int                   `json:"revision_attempted"`
	ValidationFailed  bool                  `json:"validation_failed"`
	Errors            []PlanValidationIssue `json:"errors"`
	Warnings          []PlanValidationIssue `json:"warnings,omitempty"`
	Questions         []ClarificationQuestion `json:"questions"`
	RestartFrom       string                `json:"restart_from"`
}

type ClarificationResponses struct {
	RecordedAt time.Time `json:"recorded_at"`
	Answers    []struct {
		QuestionID string `json:"question_id"`
		Answer     string `json:"answer"`
	} `json:"answers"`
}

func BuildClarificationRequest(report PlanValidationReport, revisionAttempted int) ClarificationRequest {
	req := ClarificationRequest{
		AttemptAt:         time.Now().UTC(),
		RevisionAttempted: revisionAttempted,
		ValidationFailed:  true,
		Errors:            report.Errors,
		Warnings:          report.Warnings,
		RestartFrom:       inferRestartPhase(report),
	}
	seen := map[string]bool{}
	qNum := 1
	for _, err := range report.Errors {
		if seen[err.Code] {
			continue
		}
		q := questionForError(err, qNum)
		if q.ID != "" {
			req.Questions = append(req.Questions, q)
			seen[err.Code] = true
			qNum++
		}
	}
	return req
}

func inferRestartPhase(report PlanValidationReport) string {
	for _, e := range report.Errors {
		switch e.Code {
		case "missing_requirements_file", "orphan_requirement", "unknown_requirement_id", "requirements_not_explicitly_traced":
			return "requirement_mapping"
		case "missing_assumptions_file":
			return "assumptions"
		case "missing_risks_file", "critical_risk_without_mitigation":
			return "risks"
		case "missing_impact_file", "invalid_impact_repository", "missing_expected_change_areas":
			return "impact"
		case "missing_repo_analysis_file", "repo_analysis_incomplete":
			return "repo_analysis"
		case "invalid_repository", "missing_repository":
			return "tasks"
		case "missing_verification", "dag_invalid":
			return "verification"
		}
	}
	return "tasks"
}

func questionForError(err PlanValidationIssue, num int) ClarificationQuestion {
	id := fmt.Sprintf("Q%03d", num)
	switch err.Code {
	case "missing_requirements_file":
		return ClarificationQuestion{
			ID: id, ErrorCode: err.Code,
			Question: "List the requirements extracted from PLAN.md so we can create requirements.json.",
			AnswerType: "text",
		}
	case "orphan_requirement":
		reqID := extractAfter(err.Message, "requirement ")
		return ClarificationQuestion{
			ID: id, ErrorCode: err.Code,
			Context:  map[string]string{"requirement_id": strings.TrimSpace(strings.Split(reqID, " ")[0])},
			Question: fmt.Sprintf("Requirement %s has no implementing task. Should it be implemented, deferred, merged, or removed?", strings.TrimSpace(strings.Split(reqID, " ")[0])),
			AnswerType: "choice",
			Choices:    []string{"implement", "defer", "merge", "remove"},
		}
	case "unknown_requirement_id":
		return ClarificationQuestion{
			ID: id, ErrorCode: err.Code,
			Question: err.Message + " Which requirement should this task map to?",
			AnswerType: "text",
		}
	case "invalid_repository", "missing_repository":
		return ClarificationQuestion{
			ID: id, ErrorCode: err.Code,
			Question: err.Message + " Which repository from repositories.json should own this work?",
			AnswerType: "repo_choice",
		}
	case "low_confidence_cross_repo_downstream":
		return ClarificationQuestion{
			ID: id, ErrorCode: err.Code,
			Question: err.Message + " Confirm repo ownership or split the dependency chain.",
			AnswerType: "text",
		}
	case "missing_assumptions_file":
		return ClarificationQuestion{
			ID: id, ErrorCode: err.Code,
			Question: "What assumptions must be documented before implementation (compatibility, migrations, API contracts)?",
			AnswerType: "text",
		}
	case "missing_risks_file":
		return ClarificationQuestion{
			ID: id, ErrorCode: err.Code,
			Question: "What risks should be recorded for this plan (cross-repo, API, data model, security)?",
			AnswerType: "text",
		}
	case "critical_risk_without_mitigation":
		return ClarificationQuestion{
			ID: id, ErrorCode: err.Code,
			Question: err.Message + " What mitigation should be applied?",
			AnswerType: "text",
		}
	case "missing_impact_file", "missing_expected_change_areas":
		return ClarificationQuestion{
			ID: id, ErrorCode: err.Code,
			Question: "Which repositories and change areas will be affected? Provide impact.json entries.",
			AnswerType: "text",
		}
	case "invalid_impact_repository":
		return ClarificationQuestion{
			ID: id, ErrorCode: err.Code,
			Question: err.Message + " Which registered repository should be used?",
			AnswerType: "repo_choice",
		}
	case "missing_repo_analysis_file", "repo_analysis_incomplete":
		return ClarificationQuestion{
			ID: id, ErrorCode: err.Code,
			Question: "Which repositories were analyzed and what evidence paths support the plan?",
			AnswerType: "text",
		}
	case "requirements_not_explicitly_traced":
		return ClarificationQuestion{
			ID: id, ErrorCode: err.Code,
			Question: "Map each requirement to tasks using requirement_ids on tasks.json or requirements.json.",
			AnswerType: "text",
		}
	default:
		return ClarificationQuestion{
			ID: id, ErrorCode: err.Code,
			Question: err.Message,
			AnswerType: "text",
		}
	}
}

func extractAfter(s, prefix string) string {
	if i := strings.Index(s, prefix); i >= 0 {
		return s[i+len(prefix):]
	}
	return s
}

func WriteClarificationRequest(workspaceRoot string, req ClarificationRequest) error {
	if err := EnsureDir(PlanDraftDir(workspaceRoot)); err != nil {
		return err
	}
	data, err := json.MarshalIndent(req, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomically(PlanDraftClarificationRequestPath(workspaceRoot), append(data, '\n'))
}

func LoadClarificationRequest(workspaceRoot string) (ClarificationRequest, error) {
	path := PlanDraftClarificationRequestPath(workspaceRoot)
	if !Exists(path) {
		return ClarificationRequest{}, fmt.Errorf("no clarification request found; run plan submit first")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ClarificationRequest{}, fmt.Errorf("read clarification request: %w", err)
	}
	var req ClarificationRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return ClarificationRequest{}, err
	}
	return req, nil
}

func WriteClarificationResponses(workspaceRoot string, responses ClarificationResponses) error {
	if err := EnsureDir(PlanDraftDir(workspaceRoot)); err != nil {
		return err
	}
	responses.RecordedAt = time.Now().UTC()
	data, err := json.MarshalIndent(responses, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomically(PlanDraftClarificationResponsesPath(workspaceRoot), append(data, '\n'))
}

func WriteLastValidationReport(workspaceRoot string, report PlanValidationReport) error {
	if err := EnsureDir(PlanDraftDir(workspaceRoot)); err != nil {
		return err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomically(PlanDraftLastValidationPath(workspaceRoot), append(data, '\n'))
}

func handleValidationFailure(workspaceRoot string, ws *WorkflowState, report PlanValidationReport, revisionAttempted int) error {
	req := BuildClarificationRequest(report, revisionAttempted)
	if err := WriteClarificationRequest(workspaceRoot, req); err != nil {
		return err
	}
	if err := WriteLastValidationReport(workspaceRoot, report); err != nil {
		return err
	}
	ws.WorkflowStatus = WorkflowClarificationNeeded
	return SaveWorkflowState(workspaceRoot, *ws)
}
