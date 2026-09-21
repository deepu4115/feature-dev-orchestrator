package orchestrator

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func stateLockPath(workspaceRoot string) string {
	return filepath.Join(ResolveFeatureDir(workspaceRoot), "state", "workflow.lock")
}

// WithStateLock serializes workflow/task mutations for a workspace.
func WithStateLock(workspaceRoot string, fn func() error) error {
	path := stateLockPath(workspaceRoot)
	if err := EnsureDir(filepath.Dir(path)); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return fmt.Errorf("open state lock: %w", err)
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("acquire state lock: %w", err)
	}
	defer func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }()
	return fn()
}
