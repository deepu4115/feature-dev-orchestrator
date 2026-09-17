package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func DefaultWorkflowStatePath(workspaceRoot string) string {
	return filepath.Join(ResolveFeatureDir(workspaceRoot), "state", "workflow.json")
}

func DefaultWorkflowState() WorkflowState {
	return WorkflowState{
		SchemaVersion:       "1.0",
		CurrentPlanRevision: 0,
		WorkflowStatus:      WorkflowNew,
		UpdatedAt:           time.Now().UTC(),
	}
}

func LoadWorkflowState(workspaceRoot string) (WorkflowState, error) {
	path := DefaultWorkflowStatePath(workspaceRoot)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultWorkflowState(), nil
		}
		return WorkflowState{}, fmt.Errorf("read workflow state: %w", err)
	}
	var ws WorkflowState
	if err := json.Unmarshal(data, &ws); err != nil {
		return WorkflowState{}, fmt.Errorf("unmarshal workflow state: %w", err)
	}
	if ws.SchemaVersion == "" {
		ws.SchemaVersion = "1.0"
	}
	return ws, nil
}

func SaveWorkflowState(workspaceRoot string, ws WorkflowState) error {
	ws.UpdatedAt = time.Now().UTC()
	if ws.SchemaVersion == "" {
		ws.SchemaVersion = "1.0"
	}
	data, err := json.MarshalIndent(ws, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal workflow state: %w", err)
	}
	return WriteFileAtomically(DefaultWorkflowStatePath(workspaceRoot), append(data, '\n'))
}

func RequiresPlanApproval(ws WorkflowState) bool {
	return ws.CurrentPlanRevision > 0
}

