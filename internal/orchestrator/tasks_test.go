package orchestrator

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTaskTransitionValidation(t *testing.T) {
	task := Task{ID: "T1", Title: "Example", Repository: "workspace", Status: StatusPlanned}

	if err := task.TransitionTo(StatusReady); err != nil {
		t.Fatalf("expected planned -> ready to succeed: %v", err)
	}
	if task.Status != StatusReady {
		t.Fatalf("expected status READY, got %s", task.Status)
	}

	if err := task.TransitionTo(StatusPlanned); err == nil {
		t.Fatal("expected READY -> PLANNED to fail")
	}
}

func TestReadyTasksUsesDependencyCompletion(t *testing.T) {
	tasks := []Task{
		{ID: "T1", Title: "Base", Repository: "workspace", Status: StatusDone},
		{ID: "T2", Title: "Depends on T1", Repository: "workspace", Status: StatusPlanned, Dependencies: []string{"T1"}},
		{ID: "T3", Title: "Unblocked", Repository: "workspace", Status: StatusPlanned},
		{ID: "T4", Title: "Still blocked", Repository: "workspace", Status: StatusPlanned, Dependencies: []string{"missing"}},
	}

	ready, err := ReadyTasks(tasks)
	if err != nil {
		t.Fatalf("ReadyTasks returned error: %v", err)
	}
	if len(ready) != 2 {
		t.Fatalf("expected 2 ready tasks, got %d (%v)", len(ready), ready)
	}
	if ready[0].ID != "T2" || ready[1].ID != "T3" {
		t.Fatalf("unexpected ready tasks: %#v", ready)
	}
}

func TestValidateGraphDetectsCycles(t *testing.T) {
	tasks := []Task{
		{ID: "T1", Title: "A", Repository: "workspace", Status: StatusPlanned, Dependencies: []string{"T2"}},
		{ID: "T2", Title: "B", Repository: "workspace", Status: StatusPlanned, Dependencies: []string{"T1"}},
	}

	_, err := ValidateGraph(tasks)
	if err == nil {
		t.Fatal("expected cycle detection to fail")
	}
}

func TestTaskPersistenceRoundTrip(t *testing.T) {
	workspaceRoot := t.TempDir()
	if err := InitWorkspace(workspaceRoot); err != nil {
		t.Fatalf("InitWorkspace returned error: %v", err)
	}

	tasks := []Task{
		{ID: "T1", Title: "Base task", Repository: "workspace", Status: StatusPlanned, UpdatedAt: time.Now()},
	}

	if err := SaveTasks(workspaceRoot, tasks); err != nil {
		t.Fatalf("SaveTasks returned error: %v", err)
	}
	loaded, err := LoadTasks(workspaceRoot)
	if err != nil {
		t.Fatalf("LoadTasks returned error: %v", err)
	}
	if len(loaded) != 1 || loaded[0].ID != "T1" {
		t.Fatalf("unexpected loaded tasks: %#v", loaded)
	}
	if _, err := os.Stat(filepath.Join(workspaceRoot, ".feature", "tasks", "tasks.json")); err != nil {
		t.Fatalf("expected tasks file to exist: %v", err)
	}
}

func TestReconcileTasksMarksStaleState(t *testing.T) {
	workspaceRoot := t.TempDir()
	if err := InitWorkspace(workspaceRoot); err != nil {
		t.Fatalf("InitWorkspace returned error: %v", err)
	}

	tasks := []Task{
		{ID: "T1", Title: "Needs repo", Repository: "missing-repo", Status: StatusRunning, UpdatedAt: time.Now().Add(-10 * time.Minute)},
		{ID: "T2", Title: "Ready task", Repository: "workspace", Status: StatusDone, UpdatedAt: time.Now()},
	}

	result, err := ReconcileTasks(workspaceRoot, tasks)
	if err != nil {
		t.Fatalf("ReconcileTasks returned error: %v", err)
	}
	if result.StaleCount != 1 {
		t.Fatalf("expected stale count 1, got %d", result.StaleCount)
	}
	if result.Tasks[0].ID != "T1" {
		t.Fatalf("expected stale task T1, got %#v", result.Tasks[0])
	}
}
