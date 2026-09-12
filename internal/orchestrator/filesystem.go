package orchestrator

import (
	"os"
	"path/filepath"
)

func EnsureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}

func WriteFileAtomically(path string, data []byte) error {
	if err := EnsureDir(filepath.Dir(path)); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func ResolveFeatureDir(workspaceRoot string) string {
	return filepath.Join(workspaceRoot, DefaultFeatureDir)
}
