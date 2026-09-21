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
		SchemaVersion:       WorkflowSchemaVersion,
		CurrentPlanRevision: 0,
		WorkflowStatus:      WorkflowNew,
		UpdatedAt:           time.Now().UTC(),
	}
}

const WorkflowSchemaVersion = "1.1"

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
	migrateWorkflowState(&ws)
	return ws, nil
}

func migrateWorkflowState(ws *WorkflowState) {
	if ws.SchemaVersion == "" || ws.SchemaVersion == "1.0" {
		ws.SchemaVersion = WorkflowSchemaVersion
	}
}

func SaveWorkflowState(workspaceRoot string, ws WorkflowState) error {
	ws.UpdatedAt = time.Now().UTC()
	if ws.SchemaVersion == "" || ws.SchemaVersion == "1.0" {
		ws.SchemaVersion = WorkflowSchemaVersion
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

