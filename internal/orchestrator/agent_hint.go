package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"
)

type AgentHint struct {
	Skill                string         `json:"skill"`
	TriggerPhrases       []string       `json:"trigger_phrases"`
	Reason               string         `json:"reason"`
	SuggestedPrompt      string         `json:"suggested_prompt"`
	SuggestedNextCommand string         `json:"suggested_next_command"`
	WorkspaceRoot        string         `json:"workspace_root"`
	WorkspaceInitialized bool           `json:"workspace_initialized"`
	RepositoriesCount    int            `json:"repositories_count"`
	TotalTasks           int            `json:"total_tasks"`
	ReadyTasks           []string       `json:"ready_tasks,omitempty"`
	RunningTasks         []string       `json:"running_tasks,omitempty"`
	ImplementedTasks     []string       `json:"implemented_tasks,omitempty"`
	TaskCountByStatus    map[string]int `json:"task_count_by_status,omitempty"`
	GraphValidationError string         `json:"graph_validation_error,omitempty"`
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

	repos, err := readRepositoryRegistry(workspaceRoot)
	if err != nil {
		return hint, err
	}
	hint.RepositoriesCount = len(repos)

	tasks, err := LoadTasks(workspaceRoot)
	if err != nil {
		return hint, err
	}
	hint.TotalTasks = len(tasks)
	hint.TaskCountByStatus = summarizeTaskStatuses(tasks)

	readyTasks, readyErr := ReadyTasks(tasks)
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

	switch {
	case hint.RepositoriesCount == 0:
		hint.Reason = "repositories_not_discovered"
		hint.SuggestedNextCommand = "feature-dev discover"
		hint.SuggestedPrompt = "Use feature-dev command, run discover, plan tasks and dependencies, then continue execute-loop orchestration."
	case hint.TotalTasks == 0:
		hint.Reason = "no_tasks_defined"
		hint.SuggestedNextCommand = "feature-dev task add T001 \"Define first feature slice\""
		hint.SuggestedPrompt = "Use feature-dev command, infer a task DAG with dependencies and repository ownership, write .feature/tasks/tasks.json, then run reconcile and execute-loop."
	case len(hint.ImplementedTasks) > 0 || len(hint.RunningTasks) > 0 || len(hint.ReadyTasks) > 0:
		hint.Reason = "ready_for_orchestration"
		hint.SuggestedNextCommand = "feature-dev reconcile && feature-dev execute-loop --json"
		hint.SuggestedPrompt = "Use feature-dev command and continue autonomous execute-loop cycles until all tasks are DONE."
	case remainingTaskCount(tasks) > 0:
		hint.Reason = "tasks_blocked_or_waiting"
		hint.SuggestedNextCommand = "feature-dev reconcile"
		hint.SuggestedPrompt = "Use feature-dev command, reconcile stale state, fix blocked dependencies, and continue orchestration."
	default:
		hint.Reason = "all_tasks_done"
		hint.SuggestedNextCommand = "feature-dev status --json"
		hint.SuggestedPrompt = "Use feature-dev command to summarize completion and final verification state."
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

func remainingTaskCount(tasks []Task) int {
	remaining := 0
	for _, task := range tasks {
		if task.Status != StatusDone {
			remaining++
		}
	}
	return remaining
}
