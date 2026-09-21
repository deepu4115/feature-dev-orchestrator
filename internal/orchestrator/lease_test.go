package orchestrator

import (
	"sync"
	"testing"
	"time"
)

func TestSelectNextTaskPrefersActiveOverReady(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	tasks := []Task{
		{ID: "T007", Title: "A", Repository: "repo-a", Status: StatusRunning, UpdatedAt: time.Now().UTC()},
		{ID: "T011", Title: "B", Repository: "repo-a", Status: StatusReady, UpdatedAt: time.Now().UTC()},
	}
	if err := SaveTasks(workspaceRoot, tasks); err != nil {
		t.Fatal(err)
	}
	ws, _ := LoadWorkflowState(workspaceRoot)
	now := time.Now().UTC()
	_ = ClaimExecution(&ws, "T007", "test", DefaultLeaseTTL, now)
	_ = SaveWorkflowState(workspaceRoot, ws)

	result, err := ExecuteNext(workspaceRoot, ExecuteNextOptions{ContextLevel: "brief"})
	if err != nil {
		t.Fatalf("ExecuteNext: %v", err)
	}
	if result.TaskID != "T007" {
		t.Fatalf("expected resume T007, got %s action=%s", result.TaskID, result.Action)
	}
	if result.Action != "implement" {
		t.Fatalf("expected implement, got %s", result.Action)
	}
}

func TestExecuteNextRejectsSecondReadyWhileActive(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	tasks := []Task{
		{ID: "T007", Title: "A", Repository: "repo-a", Status: StatusRunning, UpdatedAt: time.Now().UTC()},
		{ID: "T011", Title: "B", Repository: "repo-a", Status: StatusReady, UpdatedAt: time.Now().UTC()},
	}
	_ = SaveTasks(workspaceRoot, tasks)
	ws, _ := LoadWorkflowState(workspaceRoot)
	now := time.Now().UTC()
	_ = ClaimExecution(&ws, "T007", "test", DefaultLeaseTTL, now)
	_ = SaveWorkflowState(workspaceRoot, ws)

	_, err := ExecuteNext(workspaceRoot, ExecuteNextOptions{TaskID: "T011"})
	if err == nil || !IsCodedError(err, ErrCodeWorkflowBusy) {
		t.Fatalf("expected workflow_busy, got %v", err)
	}
}

func TestExecuteNextRejectsDoneTask(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	tasks := []Task{{ID: "T001", Title: "A", Repository: "repo-a", Status: StatusDone}}
	_ = SaveTasks(workspaceRoot, tasks)
	_, err := ExecuteNext(workspaceRoot, ExecuteNextOptions{TaskID: "T001"})
	if err == nil || (!IsCodedError(err, ErrCodeAlreadyDone) && !IsCodedError(err, ErrCodeTaskNotExecutable)) {
		// already_done is expected
		if err == nil || CodedErrorCode(err) != ErrCodeAlreadyDone {
			t.Fatalf("expected already_done, got %v", err)
		}
	}
	loaded, _ := LoadTasks(workspaceRoot)
	if loaded[0].Status != StatusDone {
		t.Fatalf("DONE task mutated to %s", loaded[0].Status)
	}
}

func TestConcurrentExecuteNextSingleClaim(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	tasks := []Task{
		{ID: "T001", Title: "A", Repository: "repo-a", Status: StatusPlanned},
		{ID: "T002", Title: "B", Repository: "repo-a", Status: StatusPlanned},
	}
	_ = SaveTasks(workspaceRoot, tasks)

	var wg sync.WaitGroup
	results := make([]error, 2)
	actions := make([]string, 2)
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func(idx int) {
			defer wg.Done()
			res, err := ExecuteNext(workspaceRoot, ExecuteNextOptions{ContextLevel: "brief"})
			results[idx] = err
			actions[idx] = res.Action
		}(i)
	}
	wg.Wait()

	starts := 0
	for _, a := range actions {
		if a == "start" {
			starts++
		}
	}
	if starts != 1 {
		t.Fatalf("expected exactly one start, got actions=%v errs=%v", actions, results)
	}
	loaded, _ := LoadTasks(workspaceRoot)
	running := 0
	for _, task := range loaded {
		if task.Status == StatusRunning {
			running++
		}
	}
	if running != 1 {
		t.Fatalf("expected exactly one RUNNING, got %#v", loaded)
	}
}

func TestReconcileStaleRunningAuditsAndPauses(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	stale := time.Now().UTC().Add(-10 * time.Minute)
	tasks := []Task{{ID: "T007", Title: "A", Repository: "repo-a", Status: StatusRunning, UpdatedAt: stale}}
	_ = SaveTasks(workspaceRoot, tasks)
	before := map[string]Task{"T007": tasks[0]}
	report, err := ReconcileTasks(workspaceRoot, tasks)
	if err != nil {
		t.Fatal(err)
	}
	_ = SaveTasks(workspaceRoot, report.Tasks)
	_ = persistReconcileAudits(workspaceRoot, before, report.Tasks, "test")
	if report.Tasks[0].Status != StatusBlocked {
		t.Fatalf("expected BLOCKED, got %s", report.Tasks[0].Status)
	}
	if report.Tasks[0].BlockedReason == "" {
		t.Fatal("expected blocked_reason")
	}
	if report.Tasks[0].RecoveryCommand == "" {
		t.Fatal("expected recovery_command")
	}
	summaries, _ := LoadRecentTaskSummaries(workspaceRoot, "T007", 10)
	if len(summaries) == 0 {
		t.Fatal("expected audit event for reconcile block")
	}
	ws, _ := LoadWorkflowState(workspaceRoot)
	if ws.SchedulingPausedReason == "" {
		t.Fatal("expected scheduling pause")
	}
}

func TestTaskUnblockRecoverResume(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	now := time.Now().UTC()
	tasks := []Task{{
		ID: "T009", Title: "X", Repository: "repo-a", Status: StatusBlocked,
		BlockedReason: "stale", BlockKind: BlockKindLeaseExpired,
		RecoveryCommand: "feature-dev task unblock T009 --reason test",
		BlockedAt:       &now,
	}}
	_ = SaveTasks(workspaceRoot, tasks)
	_, err := ApplyTransition(workspaceRoot, tasks, TransitionSpec{
		TaskID: "T009", To: StatusReady, Reason: "manual unblock",
		Actor: "test", Command: "feature-dev task unblock", Event: "task_unblocked", ClearBlock: true, ClearLease: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	loaded, _ := LoadTasks(workspaceRoot)
	if loaded[0].Status != StatusReady {
		t.Fatalf("expected READY, got %s", loaded[0].Status)
	}
	if loaded[0].BlockedReason != "" {
		t.Fatal("expected block meta cleared")
	}
}
