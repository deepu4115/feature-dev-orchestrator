package orchestrator

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func DefaultContextBudget() ContextBudget {
	return ContextBudget{
		MaxFiles:      4,
		MaxTotalChars: 12000,
		MaxFileChars:  3000,
	}
}

func BuildTaskContext(workspaceRoot string, task Task, level string, budget ContextBudget) (ContextPayload, error) {
	level = strings.ToLower(strings.TrimSpace(level))
	if level == "" {
		level = "brief"
	}
	if level != "brief" && level != "full" {
		return ContextPayload{}, fmt.Errorf("invalid context level %q: use brief or full", level)
	}

	if budget.MaxFiles <= 0 {
		budget.MaxFiles = DefaultContextBudget().MaxFiles
	}
	if budget.MaxTotalChars <= 0 {
		budget.MaxTotalChars = DefaultContextBudget().MaxTotalChars
	}
	if budget.MaxFileChars <= 0 {
		budget.MaxFileChars = DefaultContextBudget().MaxFileChars
	}

	repoPath := filepath.Join(workspaceRoot, task.Repository)
	state, err := CaptureRepositoryState(repoPath)
	if err != nil {
		return ContextPayload{}, err
	}

	payload := ContextPayload{
		TaskID:             task.ID,
		Repository:         task.Repository,
		Goal:               task.Goal,
		AcceptanceCriteria: task.AcceptanceCriteria,
		Verification:       task.Verification,
		Branch:             state.Branch,
		Head:               state.Head,
		ChangedFiles:       state.ChangedFiles,
		Budget:             budget,
		Level:              level,
	}

	summaries, err := LoadRecentTaskSummaries(workspaceRoot, task.ID, 3)
	if err != nil {
		return ContextPayload{}, err
	}
	payload.Summaries = summaries

	if level == "brief" {
		payload.EstimatedTokens = estimatePayloadTokens(payload)
		return payload, nil
	}

	candidateFiles := collectCandidateFiles(task, state)
	snippets, err := collectSnippets(repoPath, candidateFiles, budget)
	if err != nil {
		return ContextPayload{}, err
	}
	payload.Snippets = snippets
	payload.EstimatedTokens = estimatePayloadTokens(payload)
	return payload, nil
}

func collectCandidateFiles(task Task, state RepositoryState) []string {
	score := map[string]int{}
	for _, path := range task.ExpectedFiles {
		score[path] += 6
	}
	for _, path := range task.ContextRefs {
		score[path] += 5
	}
	for _, path := range state.ChangedFiles {
		score[path] += 4
	}
	for _, path := range state.UntrackedFiles {
		score[path] += 3
	}

	items := make([]string, 0, len(score))
	for path := range score {
		if strings.TrimSpace(path) == "" {
			continue
		}
		items = append(items, filepath.ToSlash(path))
	}
	sort.Slice(items, func(i, j int) bool {
		if score[items[i]] == score[items[j]] {
			return items[i] < items[j]
		}
		return score[items[i]] > score[items[j]]
	})
	return items
}

func collectSnippets(repoPath string, candidates []string, budget ContextBudget) ([]ContextSnippet, error) {
	snippets := make([]ContextSnippet, 0)
	remainingChars := budget.MaxTotalChars
	for _, rel := range candidates {
		if len(snippets) >= budget.MaxFiles || remainingChars <= 0 {
			break
		}
		abs := filepath.Join(repoPath, rel)
		info, err := os.Stat(abs)
		if err != nil || info.IsDir() {
			continue
		}
		data, err := os.ReadFile(abs)
		if err != nil {
			continue
		}
		content := string(data)
		if len(content) > budget.MaxFileChars {
			content = content[:budget.MaxFileChars]
		}
		if len(content) > remainingChars {
			content = content[:remainingChars]
		}
		if strings.TrimSpace(content) == "" {
			continue
		}
		snippets = append(snippets, ContextSnippet{
			Path:  rel,
			Chars: len(content),
			Text:  content,
		})
		remainingChars -= len(content)
	}
	return snippets, nil
}

func estimatePayloadTokens(payload ContextPayload) int {
	chars := len(payload.TaskID) + len(payload.Repository) + len(payload.Goal) + len(payload.Branch) + len(payload.Head)
	for _, c := range payload.AcceptanceCriteria {
		chars += len(c)
	}
	for _, v := range payload.Verification {
		chars += len(v.Name) + len(v.Command)
	}
	for _, p := range payload.ChangedFiles {
		chars += len(p)
	}
	for _, s := range payload.Summaries {
		chars += len(s.Event) + len(s.Message)
	}
	for _, snip := range payload.Snippets {
		chars += len(snip.Path) + len(snip.Text)
	}
	if chars <= 0 {
		return 0
	}
	return chars / 4
}

func RenderContextText(payload ContextPayload) string {
	lines := []string{
		fmt.Sprintf("task: %s", payload.TaskID),
		fmt.Sprintf("repository: %s", payload.Repository),
		fmt.Sprintf("branch: %s", payload.Branch),
		fmt.Sprintf("head: %s", payload.Head),
		fmt.Sprintf("level: %s", payload.Level),
		fmt.Sprintf("estimated tokens: %d", payload.EstimatedTokens),
	}
	if payload.Goal != "" {
		lines = append(lines, fmt.Sprintf("goal: %s", payload.Goal))
	}
	if len(payload.ChangedFiles) > 0 {
		lines = append(lines, fmt.Sprintf("changed files: %s", strings.Join(payload.ChangedFiles, ", ")))
	}
	if len(payload.Summaries) > 0 {
		lines = append(lines, "recent summaries:")
		for _, summary := range payload.Summaries {
			lines = append(lines, fmt.Sprintf("- %s | %s | %s", summary.Timestamp.Format(time.RFC3339), summary.Event, summary.Message))
		}
	}
	if len(payload.Snippets) > 0 {
		lines = append(lines, "snippets:")
		for _, snippet := range payload.Snippets {
			lines = append(lines, fmt.Sprintf("--- %s (%d chars) ---", snippet.Path, snippet.Chars))
			lines = append(lines, snippet.Text)
		}
	}
	return strings.Join(lines, "\n")
}
