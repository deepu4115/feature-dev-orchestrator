package orchestrator

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const summaryLogFile = "task-summaries.jsonl"

func summaryLogPath(workspaceRoot string) string {
	return filepath.Join(ResolveFeatureDir(workspaceRoot), "state", summaryLogFile)
}

func AppendTaskSummary(workspaceRoot string, task Task, event, message string) error {
	entry := TaskSummaryRecord{
		Timestamp:  time.Now().UTC(),
		TaskID:     task.ID,
		Repository: task.Repository,
		Status:     task.Status,
		Event:      strings.TrimSpace(event),
		Message:    strings.TrimSpace(message),
	}
	if entry.Event == "" {
		entry.Event = "update"
	}
	if entry.Message == "" {
		entry.Message = "task updated"
	}

	line, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal summary entry: %w", err)
	}
	path := summaryLogPath(workspaceRoot)
	if err := EnsureDir(filepath.Dir(path)); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open summary log: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("write summary log: %w", err)
	}
	return nil
}

func LoadRecentTaskSummaries(workspaceRoot, taskID string, limit int) ([]TaskSummaryRecord, error) {
	path := summaryLogPath(workspaceRoot)
	data, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []TaskSummaryRecord{}, nil
		}
		return nil, err
	}
	defer data.Close()

	entries := make([]TaskSummaryRecord, 0)
	scanner := bufio.NewScanner(data)
	for scanner.Scan() {
		var entry TaskSummaryRecord
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		if taskID != "" && entry.TaskID != taskID {
			continue
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if limit <= 0 || len(entries) <= limit {
		return entries, nil
	}
	return entries[len(entries)-limit:], nil
}
