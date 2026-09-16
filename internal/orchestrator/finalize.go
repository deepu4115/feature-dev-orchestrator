package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
)

type FinalizeResult struct {
	Completed         bool                `json:"completed"`
	WorkflowStatus    WorkflowStatus      `json:"workflow_status"`
	Traceability      TraceabilityReport  `json:"traceability"`
	CrossRepoVerify   CrossRepoVerifyReport `json:"cross_repo_verify"`
	StoppedReason     string              `json:"stopped_reason,omitempty"`
}

func MaybeAutoCompleteFeature(workspaceRoot string, timeoutSeconds int) (FinalizeResult, error) {
	result := FinalizeResult{}
	ws, err := LoadWorkflowState(workspaceRoot)
	if err != nil {
		return result, err
	}
	tasks, err := LoadTasks(workspaceRoot)
	if err != nil {
		return result, err
	}
	if !allTasksInStatus(tasks, StatusDone) {
		result.StoppedReason = "not_all_tasks_done"
		result.WorkflowStatus = ws.WorkflowStatus
		return result, fmt.Errorf("not all tasks are DONE")
	}
	if ws.WorkflowStatus != WorkflowApproved && ws.WorkflowStatus != WorkflowExecuting &&
		ws.WorkflowStatus != WorkflowVerifying && ws.WorkflowStatus != WorkflowCompleted {
		result.StoppedReason = "invalid_workflow_state"
		result.WorkflowStatus = ws.WorkflowStatus
		return result, fmt.Errorf("workflow status %s cannot finalize", ws.WorkflowStatus)
	}

	trace, err := RunTraceabilityCheck(workspaceRoot)
	if err != nil {
		return result, err
	}
	result.Traceability = trace
	if !trace.Valid {
		ws.WorkflowStatus = WorkflowVerifying
		_ = SaveWorkflowState(workspaceRoot, ws)
		result.StoppedReason = "traceability_failed"
		result.WorkflowStatus = ws.WorkflowStatus
		return result, fmt.Errorf("traceability check failed")
	}

	cross, err := RunCrossRepoVerification(workspaceRoot, timeoutSeconds)
	if err != nil {
		return result, err
	}
	result.CrossRepoVerify = cross
	if !cross.Valid {
		ws.WorkflowStatus = WorkflowVerifying
		_ = SaveWorkflowState(workspaceRoot, ws)
		result.StoppedReason = "cross_repo_verify_failed"
		result.WorkflowStatus = ws.WorkflowStatus
		return result, fmt.Errorf("cross-repo verification failed")
	}

	ws.WorkflowStatus = WorkflowCompleted
	ws.ApprovalHistory = append(ws.ApprovalHistory, ApprovalHistoryEntry{
		Revision:  ws.CurrentPlanRevision,
		Status:    string(WorkflowCompleted),
		Reason:    "auto-finalize after traceability and cross-repo verification",
		Timestamp: time.Now().UTC(),
	})
	if err := SaveWorkflowState(workspaceRoot, ws); err != nil {
		return result, err
	}
	result.Completed = true
	result.WorkflowStatus = ws.WorkflowStatus
	return result, nil
}

func BuildFinalizeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "finalize",
		Short: "Run final traceability and cross-repo checks; auto-complete feature when passing",
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceRoot, err := ResolveWorkspaceRoot()
			if err != nil {
				return err
			}
			timeout, _ := cmd.Flags().GetInt("timeout")
			jsonFlag, _ := cmd.Flags().GetBool("json")
			result, err := MaybeAutoCompleteFeature(workspaceRoot, timeout)
			if jsonFlag {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				_ = enc.Encode(result)
				if err != nil {
					return err
				}
				return nil
			}
			if err != nil {
				fmt.Printf("finalize stopped: %s\n", result.StoppedReason)
				return err
			}
			fmt.Printf("Workflow: %s\n", result.WorkflowStatus)
			return nil
		},
	}
	cmd.Flags().Int("timeout", defaultVerifyTimeout, "Verification timeout in seconds")
	cmd.Flags().Bool("json", false, "Emit result as JSON")
	return cmd
}

func BuildTraceabilityCheckCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "traceability-check",
		Short: "Verify requirement traceability against DONE tasks",
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceRoot, err := ResolveWorkspaceRoot()
			if err != nil {
				return err
			}
			jsonFlag, _ := cmd.Flags().GetBool("json")
			report, err := RunTraceabilityCheck(workspaceRoot)
			if jsonFlag {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				_ = enc.Encode(report)
				if err != nil {
					return err
				}
				if !report.Valid {
					return fmt.Errorf("traceability check failed")
				}
				return nil
			}
			fmt.Printf("valid: %t\n", report.Valid)
			for _, e := range report.Errors {
				fmt.Printf("- %s\n", e.Message)
			}
			if !report.Valid {
				return fmt.Errorf("traceability check failed")
			}
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "Emit report as JSON")
	return cmd
}

func BuildVerifyCrossRepoCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "verify-cross-repo",
		Short: "Run workspace-level cross-repository verification commands",
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceRoot, err := ResolveWorkspaceRoot()
			if err != nil {
				return err
			}
			timeout, _ := cmd.Flags().GetInt("timeout")
			jsonFlag, _ := cmd.Flags().GetBool("json")
			report, err := RunCrossRepoVerification(workspaceRoot, timeout)
			if jsonFlag {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				_ = enc.Encode(report)
			}
			if !report.Valid && len(report.Errors) > 0 {
				if err == nil {
					err = fmt.Errorf("cross-repo verification failed")
				}
				return err
			}
			if !jsonFlag {
				fmt.Printf("valid: %t\n", report.Valid)
			}
			return err
		},
	}
	cmd.Flags().Int("timeout", defaultVerifyTimeout, "Verification timeout in seconds")
	cmd.Flags().Bool("json", false, "Emit report as JSON")
	return cmd
}

func buildPlanClarifyCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "clarify",
		Short: "Show clarification questions from the last failed plan validation",
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceRoot, err := ResolveWorkspaceRoot()
			if err != nil {
				return err
			}
			jsonFlag, _ := cmd.Flags().GetBool("json")
			req, err := LoadClarificationRequest(workspaceRoot)
			if err != nil {
				return err
			}
			if jsonFlag {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(req)
			}
			fmt.Printf("restart_from: %s\n", req.RestartFrom)
			for _, q := range req.Questions {
				fmt.Printf("- [%s] %s\n", q.ID, q.Question)
			}
			return nil
		},
	}
	cmd.Flags().Bool("json", true, "Emit clarification request as JSON")
	return cmd
}

func buildPlanClarifyRecordCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "clarify-record",
		Short: "Record user clarification responses",
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceRoot, err := ResolveWorkspaceRoot()
			if err != nil {
				return err
			}
			from, _ := cmd.Flags().GetString("from")
			if from == "" {
				return fmt.Errorf("--from is required")
			}
			data, err := os.ReadFile(from)
			if err != nil {
				return err
			}
			var responses ClarificationResponses
			if err := json.Unmarshal(data, &responses); err != nil {
				return err
			}
			return WriteClarificationResponses(workspaceRoot, responses)
		},
	}
	cmd.Flags().String("from", "", "Path to clarification responses JSON")
	return cmd
}
