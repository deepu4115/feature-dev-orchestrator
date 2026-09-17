package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"
)

type AgentHint struct {
	Skill                 string         `json:"skill"`
	TriggerPhrases        []string       `json:"trigger_phrases"`
	Reason                string         `json:"reason"`
	SuggestedPrompt       string         `json:"suggested_prompt"`
	SuggestedNextCommand  string         `json:"suggested_next_command"`
	WorkspaceRoot         string         `json:"workspace_root"`
	WorkspaceInitialized  bool           `json:"workspace_initialized"`
	RepositoriesCount     int            `json:"repositories_count"`
	TotalTasks            int            `json:"total_tasks"`
	ReadyTasks            []string       `json:"ready_tasks,omitempty"`
	RunningTasks          []string       `json:"running_tasks,omitempty"`
	ImplementedTasks      []string       `json:"implemented_tasks,omitempty"`
	TaskCountByStatus     map[string]int `json:"task_count_by_status,omitempty"`
	GraphValidationError  string         `json:"graph_validation_error,omitempty"`
	WorkflowStatus        string         `json:"workflow_status,omitempty"`
	CurrentPlanRevision   int            `json:"current_plan_revision,omitempty"`
	ApprovedPlanRevision  *int           `json:"approved_plan_revision,omitempty"`
	ApprovalRequired      bool           `json:"approval_required,omitempty"`
}

func BuildAgentHint(workspaceRoot string) (AgentHint, error) {
	hint := AgentHint{
		Skill:                "feature-dev-orchestrator",
		TriggerPhrases:       []string{"use feature-dev command", "use feature-dev workflow", "execute-loop orchestration", "reconcile and continue"},
		WorkspaceRoot:        workspaceRoot,
		SuggestedPrompt:      "Use feature-dev command for this feature and continue autonomously.",
		Reason:               "workspace_not_initialized",
		SuggestedNextCommand: "feature-dev init",
	}

	if !Exists(ResolveFeatureDir(workspaceRoot)) {
		return hint, nil
	}
	hint.WorkspaceInitialized = true

	ws, err := LoadWorkflowState(workspaceRoot)
	if err != nil {
		return hint, err
	}
	hint.WorkflowStatus = string(ws.WorkflowStatus)
	hint.CurrentPlanRevision = ws.CurrentPlanRevision
	hint.ApprovedPlanRevision = ws.ApprovedPlanRevision
	hint.ApprovalRequired = ApprovalRequired(ws)

	repos, err := readRepositoryRegistry(workspaceRoot)
	if err != nil {
		return hint, err
	}
	hint.RepositoriesCount = len(repos)

	taskLoad, err := LoadTasksWithReport(workspaceRoot)
	if err != nil {
		return hint, err
	}
	if !taskLoad.Report.Valid {
		hint.Reason = "schema_fix_required"
		hint.SuggestedNextCommand = "feature-dev schema show tasks --json && feature-dev task init --force"
		hint.SuggestedPrompt = "Fix tasks.json structure using schema show output, then run task preview --json before plan submit."
		return hint, nil
	}
	tasks := taskLoad.Tasks
	hint.TotalTasks = len(tasks)
	hint.TaskCountByStatus = summarizeTaskStatuses(tasks)

	readyTasks, readyErr := ReadyTasks(workspaceRoot, tasks)
	if readyErr != nil {
		hint.Reason = "task_graph_invalid"
		hint.GraphValidationError = readyErr.Error()
		hint.SuggestedNextCommand = "feature-dev graph"
		hint.SuggestedPrompt = "Use feature-dev command, inspect dependency errors, repair the task graph, and continue orchestration."
		return hint, nil
	}
	hint.ReadyTasks = taskIDs(readyTasks)
	hint.RunningTasks = taskIDsByStatus(tasks, StatusRunning)
	hint.ImplementedTasks = append(taskIDsByStatus(tasks, StatusImplemented), taskIDsByStatus(tasks, StatusVerifying)...)
	sort.Strings(hint.ImplementedTasks)

	if structuralReport, ok := loadLastStructuralValidation(workspaceRoot); ok {
		hint.Reason = "schema_fix_required"
		hint.SuggestedNextCommand = structuralFixCommand(structuralReport)
		hint.SuggestedPrompt = "Fix JSON structure errors in planning artifacts using schema show and init commands, then run feature-dev task preview --json."
		return hint, nil
	}

	if RequiresPlanApproval(ws) {
		switch ws.WorkflowStatus {
		case WorkflowPlanning:
			if report, ok := loadLastValidationReport(workspaceRoot); ok && HasStructuralErrors(report) {
				hint.Reason = "schema_fix_required"
				hint.SuggestedNextCommand = structuralFixCommand(report)
				hint.SuggestedPrompt = "Fix JSON structure errors before domain clarification. Use schema show and plan draft init / task init, then task preview --json."
				return hint, nil
			}
			hint.Reason = "planning_in_progress"
			hint.SuggestedNextCommand = "feature-dev plan draft init && feature-dev task init && feature-dev task preview --json"
			hint.SuggestedPrompt = "Complete the planning bundle and tasks.json, validate with task preview, then plan submit --from-tasks."
			return hint, nil
		case WorkflowClarificationNeeded:
			hint.Reason = "plan_needs_clarification"
			hint.SuggestedNextCommand = "feature-dev plan clarify --json"
			hint.SuggestedPrompt = "Validation failed. Present clarification questions, wait for my answers, update the planning bundle, and resubmit with plan submit --from-tasks."
			return hint, nil
		case WorkflowReviewPending:
			hint.Reason = "tasks_awaiting_approval"
			hint.SuggestedNextCommand = "feature-dev review --json"
			hint.SuggestedPrompt = "Present .feature/tasks/tasks.json, repo assignments, and DAG to the user. Wait for explicit approval, then run feature-dev approve."
			return hint, nil
		case WorkflowPlanGenerated:
			hint.Reason = "task_plan_invalid"
			hint.SuggestedNextCommand = "feature-dev plan clarify --json"
			hint.SuggestedPrompt = "Fix plan validation issues using clarification-request.json, then resubmit with plan submit --from-tasks."
			return hint, nil
		case WorkflowReplanning:
			hint.Reason = "plan_replanning"
			hint.SuggestedNextCommand = "feature-dev plan submit --from-tasks"
			hint.SuggestedPrompt = "Revise .feature/tasks/tasks.json based on user feedback, submit a new revision, and request approval."
			return hint, nil
		case WorkflowRejected:
			hint.Reason = "plan_rejected"
			hint.SuggestedNextCommand = "feature-dev replan"
			hint.SuggestedPrompt = "The plan was rejected. Enter replanning, revise the plan, submit a new revision, and request approval."
			return hint, nil
		case WorkflowApproved:
			hint.Reason = "plan_approved_ready"
			hint.SuggestedNextCommand = "feature-dev reconcile && feature-dev execute-loop --json"
			hint.SuggestedPrompt = "The plan is approved. Continue autonomous execute-loop cycles until all tasks are DONE."
			return hint, nil
		case WorkflowExecuting:
			if len(hint.RunningTasks) > 0 {
				hint.Reason = "task_awaiting_implementation"
				hint.SuggestedNextCommand = fmt.Sprintf("feature-dev implement %s && feature-dev execute-loop --json", hint.RunningTasks[0])
				hint.SuggestedPrompt = "Finish code changes for the RUNNING task, mark it IMPLEMENTED with feature-dev implement, then continue execute-loop."
				return hint, nil
			}
			hint.Reason = "ready_for_orchestration"
			hint.SuggestedNextCommand = "feature-dev execute-loop --json"
			hint.SuggestedPrompt = "Implementation in progress. Continue execute-loop until all tasks are DONE."
			return hint, nil
		case WorkflowVerifying:
			if allTasksInStatus(tasks, StatusDone) {
				hint.Reason = "awaiting_final_verification"
				hint.SuggestedNextCommand = "feature-dev finalize --json"
				hint.SuggestedPrompt = "All tasks are DONE. Run finalize to complete traceability and cross-repo verification."
				return hint, nil
			}
			hint.Reason = "ready_for_orchestration"
			hint.SuggestedNextCommand = "feature-dev execute-loop --json"
			return hint, nil
		case WorkflowCompleted:
			hint.Reason = "feature_completed"
			hint.SuggestedNextCommand = "feature-dev status --json"
			hint.SuggestedPrompt = "Feature workflow is COMPLETED. Summarize outcomes and verification results."
			return hint, nil
		default:
			if ws.WorkflowStatus != WorkflowExecuting && ws.WorkflowStatus != WorkflowCompleted {
				hint.Reason = "plan_awaiting_approval"
				hint.SuggestedNextCommand = "feature-dev review --json"
				hint.SuggestedPrompt = "Complete plan submission and obtain user approval before implementation."
				return hint, nil
			}
		}
	}

	switch {
	case hint.RepositoriesCount == 0:
		hint.Reason = "repositories_not_discovered"
		hint.SuggestedNextCommand = "feature-dev discover"
		hint.SuggestedPrompt = "Use feature-dev command, run discover, analyze repositories, write .feature/tasks/tasks.json, then plan submit --from-tasks."
	case hint.TotalTasks == 0:
		hint.Reason = "no_tasks_defined"
		hint.SuggestedNextCommand = "feature-dev plan draft init && feature-dev task init"
		hint.SuggestedPrompt = "Scaffold planning draft bundle and tasks.json from CLI templates, fill in domain content from PLAN.md, then run task preview --json and plan submit --from-tasks."
	case hint.TotalTasks > 0 && ws.CurrentPlanRevision == 0:
		if !Exists(PlanDraftRequirementsPath(workspaceRoot)) {
			hint.Reason = "planning_bundle_incomplete"
			hint.SuggestedNextCommand = "feature-dev plan draft init && feature-dev task preview --json"
			hint.SuggestedPrompt = "Scaffold the planning bundle with plan draft init, complete domain content, validate with task preview, then plan submit --from-tasks."
		} else {
			hint.Reason = "tasks_need_submit"
			hint.SuggestedNextCommand = "feature-dev plan submit --from-tasks"
			hint.SuggestedPrompt = "Submit the tasks.json plan for validation and user review before implementation."
		}
	case len(hint.ImplementedTasks) > 0 || len(hint.RunningTasks) > 0 || len(hint.ReadyTasks) > 0:
		hint.Reason = "ready_for_orchestration"
		hint.SuggestedNextCommand = "feature-dev reconcile && feature-dev execute-loop --json"
		hint.SuggestedPrompt = "Use feature-dev command and continue autonomous execute-loop cycles until all tasks are DONE."
	case remainingTaskCount(tasks) > 0:
		hint.Reason = "tasks_blocked_or_waiting"
		hint.SuggestedNextCommand = "feature-dev reconcile"
		hint.SuggestedPrompt = "Use feature-dev command, reconcile stale state, fix blocked dependencies, and continue orchestration."
	default:
		if ws.WorkflowStatus == WorkflowCompleted {
			hint.Reason = "feature_completed"
			hint.SuggestedNextCommand = "feature-dev status --json"
			hint.SuggestedPrompt = "Feature workflow is COMPLETED."
		} else if allTasksInStatus(tasks, StatusDone) {
			hint.Reason = "awaiting_final_verification"
			hint.SuggestedNextCommand = "feature-dev finalize --json"
			hint.SuggestedPrompt = "All tasks are DONE. Run finalize to complete the feature."
		} else {
			hint.Reason = "all_tasks_done"
			hint.SuggestedNextCommand = "feature-dev status --json"
			hint.SuggestedPrompt = "Use feature-dev command to summarize completion and final verification state."
		}
	}

	return hint, nil
}

func BuildAgentHintCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent-hint",
		Short: "Emit agent routing hints for feature-dev orchestration",
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceRoot, err := ResolveWorkspaceRoot()
			if err != nil {
				return err
			}

			hint, err := BuildAgentHint(workspaceRoot)
			if err != nil {
				return err
			}

			jsonFlag, _ := cmd.Flags().GetBool("json")
			if jsonFlag {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(hint)
			}

			fmt.Printf("skill: %s\n", hint.Skill)
			fmt.Printf("reason: %s\n", hint.Reason)
			fmt.Printf("workspace initialized: %t\n", hint.WorkspaceInitialized)
			fmt.Printf("repositories: %d\n", hint.RepositoriesCount)
			fmt.Printf("tasks: %d\n", hint.TotalTasks)
			if hint.WorkflowStatus != "" {
				fmt.Printf("workflow status: %s\n", hint.WorkflowStatus)
			}
			if hint.CurrentPlanRevision > 0 {
				fmt.Printf("plan revision: %d\n", hint.CurrentPlanRevision)
			}
			if len(hint.ReadyTasks) > 0 {
				fmt.Printf("ready: %v\n", hint.ReadyTasks)
			}
			if len(hint.RunningTasks) > 0 {
				fmt.Printf("running: %v\n", hint.RunningTasks)
			}
			if len(hint.ImplementedTasks) > 0 {
				fmt.Printf("implemented/verifying: %v\n", hint.ImplementedTasks)
			}
			if hint.GraphValidationError != "" {
				fmt.Printf("graph error: %s\n", hint.GraphValidationError)
			}
			fmt.Printf("suggested next command: %s\n", hint.SuggestedNextCommand)
			fmt.Printf("suggested prompt: %s\n", hint.SuggestedPrompt)
			return nil
		},
	}
	cmd.Flags().Bool("json", true, "Emit hint as JSON")
	return cmd
}

func readRepositoryRegistry(workspaceRoot string) ([]Repository, error) {
	path := DefaultRepositoriesPath(workspaceRoot)
	if !Exists(path) {
		return []Repository{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read repositories file: %w", err)
	}
	var repos []Repository
	if err := json.Unmarshal(data, &repos); err != nil {
		return nil, fmt.Errorf("unmarshal repositories: %w", err)
	}
	return repos, nil
}

func summarizeTaskStatuses(tasks []Task) map[string]int {
	counts := map[string]int{}
	for _, task := range tasks {
		counts[string(task.Status)]++
	}
	return counts
}

func taskIDs(tasks []Task) []string {
	ids := make([]string, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}
	sort.Strings(ids)
	return ids
}

func taskIDsByStatus(tasks []Task, status TaskStatus) []string {
	ids := make([]string, 0)
	for _, task := range tasks {
		if task.Status == status {
			ids = append(ids, task.ID)
		}
	}
	sort.Strings(ids)
	return ids
}

func loadLastValidationReport(workspaceRoot string) (PlanValidationReport, bool) {
	path := PlanDraftLastValidationPath(workspaceRoot)
	if !Exists(path) {
		return PlanValidationReport{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return PlanValidationReport{}, false
	}
	var report PlanValidationReport
	if err := json.Unmarshal(data, &report); err != nil {
		return PlanValidationReport{}, false
	}
	return report, true
}

func loadLastStructuralValidation(workspaceRoot string) (PlanValidationReport, bool) {
	report, ok := loadLastValidationReport(workspaceRoot)
	if !ok || !HasStructuralErrors(report) {
		return PlanValidationReport{}, false
	}
	return report, true
}

func structuralFixCommand(report PlanValidationReport) string {
	for _, err := range report.Errors {
		if !IsStructuralErrorCode(err.Code) {
			continue
		}
		switch err.Code {
		case "invalid_task_json", "invalid_verification_type", "invalid_tasks_wrapper":
			return "feature-dev schema show tasks --json && feature-dev task init --force"
		case "invalid_draft_wrapper", "draft_bundle_load_error":
			return "feature-dev schema show requirements --json && feature-dev plan draft init --force"
		}
	}
	return "feature-dev schema list --json && feature-dev plan draft init --force && feature-dev task init --force"
}

func remainingTaskCount(tasks []Task) int {
	remaining := 0
	for _, task := range tasks {
		if task.Status != StatusDone {
			remaining++
		}
	}
	return remaining
}
