package orchestrator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCollectSnippetsRespectsBudgets(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, "a.txt"), []byte("aaaaaaaaaa"), 0o644); err != nil {
		t.Fatalf("write a.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "b.txt"), []byte("bbbbbbbbbb"), 0o644); err != nil {
		t.Fatalf("write b.txt: %v", err)
	}

	snippets, err := collectSnippets(repoRoot, []string{"a.txt", "b.txt"}, ContextBudget{
		MaxFiles:      1,
		MaxTotalChars: 6,
		MaxFileChars:  8,
	})
	if err != nil {
		t.Fatalf("collectSnippets returned error: %v", err)
	}
	if len(snippets) != 1 {
		t.Fatalf("expected 1 snippet, got %d", len(snippets))
	}
	if snippets[0].Chars != 6 {
		t.Fatalf("expected snippet chars 6, got %d", snippets[0].Chars)
	}
}

func TestSummaryAppendAndLoad(t *testing.T) {
	workspaceRoot := t.TempDir()
	if err := InitWorkspace(workspaceRoot); err != nil {
		t.Fatalf("InitWorkspace returned error: %v", err)
	}

	task := Task{ID: "T100", Repository: "repo-a", Status: StatusRunning}
	if err := AppendTaskSummary(workspaceRoot, task, "task_started", "started work"); err != nil {
		t.Fatalf("AppendTaskSummary returned error: %v", err)
	}
	task.Status = StatusRework
	if err := AppendTaskSummary(workspaceRoot, task, "verification_failed", "tests failed"); err != nil {
		t.Fatalf("AppendTaskSummary returned error: %v", err)
	}

	entries, err := LoadRecentTaskSummaries(workspaceRoot, "T100", 1)
	if err != nil {
		t.Fatalf("LoadRecentTaskSummaries returned error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(entries))
	}
	if entries[0].Event != "verification_failed" {
		t.Fatalf("expected latest event verification_failed, got %s", entries[0].Event)
	}
}

func TestRenderContextTextIncludesSummaryAndSnippets(t *testing.T) {
	payload := ContextPayload{
		TaskID:          "T1",
		Repository:      "repo-a",
		Level:           "full",
		EstimatedTokens: 42,
		Summaries: []TaskSummaryRecord{{
			Event:   "task_started",
			Message: "started",
		}},
		Snippets: []ContextSnippet{{
			Path:  "a.txt",
			Chars: 3,
			Text:  "abc",
		}},
	}
	text := RenderContextText(payload)
	if text == "" {
		t.Fatal("expected rendered context text")
	}
	if !containsAll(text, []string{"task: T1", "recent summaries:", "snippets:"}) {
		t.Fatalf("rendered text missing expected segments: %s", text)
	}
}

func containsAll(text string, values []string) bool {
	for _, value := range values {
		if !strings.Contains(text, value) {
			return false
		}
	}
	return true
}
