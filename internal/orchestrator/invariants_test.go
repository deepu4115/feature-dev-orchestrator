package orchestrator

import (
	"testing"
	"time"
)

func TestAssertInvariantsMultipleActive(t *testing.T) {
	tasks := []Task{
		{ID: "T1", Status: StatusRunning},
		{ID: "T2", Status: StatusImplemented},
	}
	report := AssertInvariants(tasks, WorkflowState{}, nil)
	if report.Valid {
		t.Fatal("expected invalid for multiple active")
	}
}

func TestAssertInvariantsBlockedRequiresReason(t *testing.T) {
	tasks := []Task{{ID: "T1", Status: StatusBlocked}}
	report := AssertInvariants(tasks, WorkflowState{}, nil)
	if report.Valid {
		t.Fatal("expected invalid for BLOCKED without reason")
	}
}

func TestAssertInvariantsPass(t *testing.T) {
	now := time.Now().UTC()
	tasks := []Task{{
		ID: "T1", Status: StatusBlocked, BlockedReason: "missing repo",
		RecoveryCommand: "feature-dev task unblock T1 --reason fix", BlockKind: BlockKindMissingRepo, BlockedAt: &now,
	}}
	ws := WorkflowState{}
	summaries := []TaskSummaryRecord{{TaskID: "T1", Status: StatusBlocked, Event: "task_reconciled"}}
	report := AssertInvariants(tasks, ws, summaries)
	if !report.Valid {
		t.Fatalf("expected valid, got %v", report.Violations)
	}
}

func TestApplyTransitionWritesAudit(t *testing.T) {
	workspaceRoot := setupTestWorkspace(t)
	tasks := []Task{{ID: "T001", Title: "A", Repository: "repo-a", Status: StatusReady}}
	_ = SaveTasks(workspaceRoot, tasks)
	_, err := ApplyTransition(workspaceRoot, tasks, TransitionSpec{
		TaskID: "T001", To: StatusRunning, Reason: "start", Actor: "test",
		Command: "test", Event: "task_started", ClaimLease: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	summaries, err := LoadRecentTaskSummaries(workspaceRoot, "T001", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) == 0 {
		t.Fatal("expected audit event")
	}
	if summaries[0].PreviousStatus != StatusReady || summaries[0].NextStatus != StatusRunning {
		t.Fatalf("unexpected transition fields: %#v", summaries[0])
	}
	ws, _ := LoadWorkflowState(workspaceRoot)
	if ws.ActiveTaskID != "T001" {
		t.Fatalf("expected lease on T001, got %s", ws.ActiveTaskID)
	}
}

func TestWriteFileAtomicallyRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/out.json"
	if err := WriteFileAtomically(path, []byte("{\"ok\":true}\n")); err != nil {
		t.Fatal(err)
	}
	if !Exists(path) {
		t.Fatal("file missing")
	}
}
