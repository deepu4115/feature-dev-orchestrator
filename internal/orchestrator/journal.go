package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const commandJournalFile = "command-journal.jsonl"

type CommandJournalEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Command   string    `json:"command"`
	Args      []string  `json:"args,omitempty"`
	Revision  int       `json:"revision,omitempty"`
	Result    string    `json:"result,omitempty"`
	Error     string    `json:"error,omitempty"`
}

func commandJournalPath(workspaceRoot string) string {
	return filepath.Join(ResolveFeatureDir(workspaceRoot), "state", commandJournalFile)
}

func AppendCommandJournal(workspaceRoot, command string, args []string, revision int, result, errMsg string) error {
	entry := CommandJournalEntry{
		Timestamp: time.Now().UTC(),
		Command:   command,
		Args:      args,
		Revision:  revision,
		Result:    result,
		Error:     errMsg,
	}
	line, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal command journal: %w", err)
	}
	path := commandJournalPath(workspaceRoot)
	if err := EnsureDir(filepath.Dir(path)); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open command journal: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("write command journal: %w", err)
	}
	return nil
}
