package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	defaultTasksFile = "tasks.json"
)

func TasksDir(workspaceRoot string) string {
	return filepath.Join(ResolveFeatureDir(workspaceRoot), "tasks")
}

func TaskStoragePath(workspaceRoot string) string {
	return filepath.Join(TasksDir(workspaceRoot), defaultTasksFile)
}

func (t *Task) TransitionTo(next TaskStatus) error {
	if !t.Status.CanTransitionTo(next) {
		return fmt.Errorf("illegal transition: %s -> %s", t.Status, next)
	}
	t.Status = next
	t.UpdatedAt = time.Now().UTC()
	return nil
}

func ValidateGraph(tasks []Task) ([]Task, error) {
	index := map[string]Task{}
	for _, task := range tasks {
		if task.ID == "" {
			return nil, fmt.Errorf("task ID cannot be empty")
		}
		if _, exists := index[task.ID]; exists {
			return nil, fmt.Errorf("duplicate task ID: %s", task.ID)
		}
		index[task.ID] = task
	}

	visited := map[string]bool{}
	stack := map[string]bool{}
	var visit func(string) error
	visit = func(id string) error {
		if stack[id] {
			return fmt.Errorf("cycle detected involving task %s", id)
		}
		if visited[id] {
			return nil
		}
		visited[id] = true
		stack[id] = true
		for _, dep := range index[id].Dependencies {
			if _, ok := index[dep]; !ok {
				continue
			}
			if err := visit(dep); err != nil {
				return err
			}
		}
		stack[id] = false
		return nil
	}

	for id := range index {
		if err := visit(id); err != nil {
			return nil, err
		}
	}

	return tasks, nil
}

func ReadyTasks(workspaceRoot string, tasks []Task) ([]Task, error) {
	if _, err := ValidateGraph(tasks); err != nil {
		return nil, err
	}

	ws, err := LoadWorkflowState(workspaceRoot)
	if err != nil {
		return nil, err
	}
	gated := RequiresPlanApproval(ws)

	byID := map[string]Task{}
	for _, task := range tasks {
		byID[task.ID] = task
	}

	ready := make([]Task, 0)
	for _, task := range tasks {
		if gated {
			if task.Status != StatusReady {
				continue
			}
		} else if task.Status != StatusPlanned && task.Status != StatusBlocked && task.Status != StatusRework && task.Status != StatusFailed {
			continue
		}
		blocked := false
		for _, depID := range task.Dependencies {
			d, ok := byID[depID]
			if !ok {
				blocked = true
				break
			}
			if d.Status != StatusDone {
				blocked = true
				break
			}
		}
		if !blocked {
			ready = append(ready, task)
		}
	}

	sort.Slice(ready, func(i, j int) bool {
		return ready[i].ID < ready[j].ID
	})
	return ready, nil
}

func SaveTasks(workspaceRoot string, tasks []Task) error {
	path := TaskStoragePath(workspaceRoot)
	if err := EnsureDir(filepath.Dir(path)); err != nil {
		return fmt.Errorf("create tasks dir: %w", err)
	}
	data, err := json.MarshalIndent(tasks, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal tasks: %w", err)
	}
	return WriteFileAtomically(path, append(data, '\n'))
}

type TasksLoadResult struct {
	Tasks  []Task
	Report PlanValidationReport
}

func LoadTasksWithReport(workspaceRoot string) (TasksLoadResult, error) {
	path := TaskStoragePath(workspaceRoot)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return TasksLoadResult{Tasks: []Task{}, Report: structuralReportFromIssues(nil)}, nil
		}
		return TasksLoadResult{}, err
	}
	var tasks []Task
	if err := json.Unmarshal(data, &tasks); err != nil {
		issues := translateTasksJSONError(path, data, err)
		return TasksLoadResult{
			Tasks:  []Task{},
			Report: structuralReportFromIssues(issues),
		}, nil
	}
	return TasksLoadResult{Tasks: tasks, Report: structuralReportFromIssues(nil)}, nil
}

func LoadTasks(workspaceRoot string) ([]Task, error) {
	result, err := LoadTasksWithReport(workspaceRoot)
	if err != nil {
		return nil, err
	}
	if !result.Report.Valid {
		return nil, fmt.Errorf("%s", result.Report.Errors[0].Message)
	}
	return result.Tasks, nil
}

func TaskSummary(tasks []Task) map[string]any {
	summary := map[string]any{
		"total":    len(tasks),
		"byStatus": map[string]int{},
	}
	for _, task := range tasks {
		status := string(task.Status)
		m := summary["byStatus"].(map[string]int)
		m[status]++
	}
	return summary
}

func (t Task) String() string {
	parts := []string{t.ID, string(t.Status), t.Repository}
	if t.Title != "" {
		parts = append(parts, t.Title)
	}
	return strings.Join(parts, " | ")
}
