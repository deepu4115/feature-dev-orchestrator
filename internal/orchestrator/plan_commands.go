package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
)

type SubmitPlanOptions struct {
	FromPath    string
	FromTasks   bool
	AutoApprove bool
	FeatureID   string
	FeatureTitle string
}

type SubmitPlanResult struct {
	Revision   int                   `json:"revision"`
	Status     WorkflowStatus        `json:"status"`
	Valid      bool                  `json:"valid"`
	Report     PlanValidationReport  `json:"report"`
	PlanPath   string                `json:"plan_path"`
	ReviewPath string                `json:"review_path"`
}

func CapturePlanSnapshot(workspaceRoot string, doc *PlanDocument) error {
	repos, err := readRepositoryRegistry(workspaceRoot)
	if err != nil {
		return err
	}
	doc.PlanSnapshot.Repositories = map[string]PlanSnapshotRepo{}
	for _, r := range repos {
		repoPath := r.Path
		if !filepath.IsAbs(repoPath) {
			repoPath = filepath.Join(workspaceRoot, repoPath)
		}
		if !isGitRepo(repoPath) {
			continue
		}
		state, err := CaptureRepositoryState(repoPath)
		if err != nil {
			continue
		}
		doc.PlanSnapshot.Repositories[r.ID] = PlanSnapshotRepo{
			Branch: state.Branch,
			Head:   state.Head,
		}
	}
	return nil
}

func SubmitPlan(workspaceRoot string, opts SubmitPlanOptions) (SubmitPlanResult, error) {
	ws, err := LoadWorkflowState(workspaceRoot)
	if err != nil {
		return SubmitPlanResult{}, err
	}

	var doc PlanDocument
	var sourceTasks []Task
	switch {
	case opts.FromPath != "":
		doc, err = LoadPlanDocument(opts.FromPath)
		if err != nil {
			return SubmitPlanResult{}, err
		}
	case opts.FromTasks:
		sourceTasks, err = LoadTasks(workspaceRoot)
		if err != nil {
			return SubmitPlanResult{}, err
		}
		taskReport, err := ValidateTaskPlan(workspaceRoot, sourceTasks, true)
		if err != nil {
			return SubmitPlanResult{}, err
		}
		if !taskReport.Valid {
			result := SubmitPlanResult{Valid: false, Report: taskReport}
			return result, fmt.Errorf("%w: fix tasks.json and resubmit", ErrPlanValidationFailed)
		}
		doc = BuildPlanFromTasks(workspaceRoot, sourceTasks, PlanFeature{
			ID:    opts.FeatureID,
			Title: opts.FeatureTitle,
		})
	default:
		draft := PlanDraftPath(workspaceRoot)
		if !Exists(draft) {
			return SubmitPlanResult{}, fmt.Errorf("no plan input: use --from, --from-tasks, or create %s", draft)
		}
		doc, err = LoadPlanDocument(draft)
		if err != nil {
			return SubmitPlanResult{}, err
		}
	}

	nextRevision := ws.CurrentPlanRevision + 1
	doc.Plan.Revision = nextRevision
	doc.Plan.GeneratedAt = time.Now().UTC()

	if err := CapturePlanSnapshot(workspaceRoot, &doc); err != nil {
		return SubmitPlanResult{}, err
	}

	report, err := ValidatePlanDocument(workspaceRoot, doc)
	if err != nil {
		return SubmitPlanResult{}, err
	}

	fp, err := ComputePlanFingerprint(doc)
	if err != nil {
		return SubmitPlanResult{}, err
	}
	doc.Plan.Fingerprint = fp

	result := SubmitPlanResult{
		Revision: nextRevision,
		Valid:    report.Valid,
		Report:   report,
	}

	revDir := PlanRevisionDir(workspaceRoot, nextRevision)
	if err := EnsureDir(revDir); err != nil {
		return SubmitPlanResult{}, err
	}
	planPath := PlanRevisionPath(workspaceRoot, nextRevision)

	var prev *PlanDocument
	if ws.CurrentPlanRevision > 0 {
		prevDoc, err := LoadPlanDocument(PlanRevisionPath(workspaceRoot, ws.CurrentPlanRevision))
		if err == nil {
			prev = &prevDoc
		}
	}

	if report.Valid {
		doc.Plan.Status = string(WorkflowReviewPending)
		ws.CurrentPlanRevision = nextRevision
		InvalidateApproval(&ws)
		ws.WorkflowStatus = WorkflowReviewPending
	} else {
		doc.Plan.Status = string(WorkflowPlanGenerated)
		ws.CurrentPlanRevision = nextRevision
		ws.WorkflowStatus = WorkflowPlanGenerated
	}

	if err := SavePlanDocument(planPath, doc); err != nil {
		return SubmitPlanResult{}, err
	}
	result.PlanPath = planPath

	if report.Valid {
		if err := GenerateReviewArtifacts(workspaceRoot, nextRevision, doc, prev, ws, report); err != nil {
			return SubmitPlanResult{}, err
		}
		result.ReviewPath = filepath.Join(revDir, "review.md")
	}

	tasks, err := SyncPlanTasksToRuntime(workspaceRoot, doc.Tasks)
	if err != nil {
		return SubmitPlanResult{}, err
	}
	if err := SaveTasks(workspaceRoot, tasks); err != nil {
		return SubmitPlanResult{}, err
	}
	if err := SnapshotTasksRevision(workspaceRoot, nextRevision, tasks); err != nil {
		return SubmitPlanResult{}, err
	}

	result.Status = ws.WorkflowStatus
	if err := SaveWorkflowState(workspaceRoot, ws); err != nil {
		return SubmitPlanResult{}, err
	}

	if opts.AutoApprove && report.Valid {
		if _, _, _, err := ApprovePlan(workspaceRoot, ApprovePlanOptions{Revision: nextRevision}); err != nil {
			return SubmitPlanResult{}, err
		}
		ws, _ = LoadWorkflowState(workspaceRoot)
		result.Status = ws.WorkflowStatus
	}

	return result, nil
}

func ReviewPlan(workspaceRoot string) (PlanReviewJSON, PlanDocument, PlanValidationReport, error) {
	ws, err := LoadWorkflowState(workspaceRoot)
	if err != nil {
		return PlanReviewJSON{}, PlanDocument{}, PlanValidationReport{}, err
	}
	if ws.CurrentPlanRevision <= 0 {
		return PlanReviewJSON{}, PlanDocument{}, PlanValidationReport{}, fmt.Errorf("no plan revision exists")
	}
	doc, err := LoadCurrentPlanDocument(workspaceRoot)
	if err != nil {
		return PlanReviewJSON{}, PlanDocument{}, PlanValidationReport{}, err
	}
	runtimeTasks, err := LoadTasks(workspaceRoot)
	if err != nil {
		return PlanReviewJSON{}, PlanDocument{}, PlanValidationReport{}, err
	}
	report, err := ValidatePlanDocument(workspaceRoot, doc)
	if err != nil {
		return PlanReviewJSON{}, PlanDocument{}, PlanValidationReport{}, err
	}
	return BuildPlanReviewJSON(workspaceRoot, doc, ws, report, runtimeTasks), doc, report, nil
}

func PreviewTaskPlan(workspaceRoot string) (PlanReviewJSON, PlanValidationReport, error) {
	ws, err := LoadWorkflowState(workspaceRoot)
	if err != nil {
		return PlanReviewJSON{}, PlanValidationReport{}, err
	}
	tasks, err := LoadTasks(workspaceRoot)
	if err != nil {
		return PlanReviewJSON{}, PlanValidationReport{}, err
	}
	report, err := ValidateTaskPlan(workspaceRoot, tasks, true)
	if err != nil {
		return PlanReviewJSON{}, PlanValidationReport{}, err
	}
	doc := PlanDocument{}
	if ws.CurrentPlanRevision > 0 {
		doc, _ = LoadCurrentPlanDocument(workspaceRoot)
	}
	payload := BuildPlanReviewJSON(workspaceRoot, doc, ws, report, tasks)
	return payload, report, nil
}

func Replan(workspaceRoot string, reason string) (WorkflowState, error) {
	ws, err := LoadWorkflowState(workspaceRoot)
	if err != nil {
		return ws, err
	}
	if ws.CurrentPlanRevision <= 0 {
		return ws, fmt.Errorf("no plan revision to replan from")
	}
	ws.WorkflowStatus = WorkflowReplanning
	ws.ApprovedPlanRevision = nil
	ws.ApprovedPlanFingerprint = ""
	if reason != "" {
		ws.ApprovalHistory = append(ws.ApprovalHistory, ApprovalHistoryEntry{
			Revision:  ws.CurrentPlanRevision,
			Status:    "REPLANNING",
			Reason:    reason,
			Timestamp: time.Now().UTC(),
		})
	}
	if err := SaveWorkflowState(workspaceRoot, ws); err != nil {
		return ws, err
	}
	return ws, nil
}

func BuildPlanCommand() *cobra.Command {
	plan := &cobra.Command{
		Use:   "plan",
		Short: "Manage feature implementation plans",
	}
	plan.AddCommand(buildPlanSubmitCommand())
	return plan
}

func buildPlanSubmitCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "submit",
		Short: "Submit a plan revision for review",
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceRoot, err := ResolveWorkspaceRoot()
			if err != nil {
				return err
			}
			from, _ := cmd.Flags().GetString("from")
			fromTasks, _ := cmd.Flags().GetBool("from-tasks")
			autoApprove, _ := cmd.Flags().GetBool("auto-approve")
			featureID, _ := cmd.Flags().GetString("feature-id")
			featureTitle, _ := cmd.Flags().GetString("feature-title")
			jsonFlag, _ := cmd.Flags().GetBool("json")

			result, err := SubmitPlan(workspaceRoot, SubmitPlanOptions{
				FromPath:     from,
				FromTasks:    fromTasks,
				AutoApprove:  autoApprove,
				FeatureID:    featureID,
				FeatureTitle: featureTitle,
			})
			if err != nil {
				return err
			}
			if jsonFlag {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(result)
			}
			fmt.Printf("plan revision %d submitted\n", result.Revision)
			fmt.Printf("status: %s\n", result.Status)
			fmt.Printf("valid: %t\n", result.Valid)
			if result.ReviewPath != "" {
				fmt.Printf("review: %s\n", result.ReviewPath)
			}
			if !result.Valid {
				fmt.Println("validation failed; plan not moved to REVIEW_PENDING")
				for _, e := range result.Report.Errors {
					fmt.Printf("- %s\n", e.Message)
				}
			}
			return nil
		},
	}
	cmd.Flags().String("from", "", "Path to plan YAML input")
	cmd.Flags().Bool("from-tasks", false, "Build plan from existing tasks.json")
	cmd.Flags().Bool("auto-approve", false, "Auto-approve after successful validation (non-production/testing only)")
	cmd.Flags().String("feature-id", "feature", "Feature ID when using --from-tasks")
	cmd.Flags().String("feature-title", "Feature implementation plan", "Feature title when using --from-tasks")
	cmd.Flags().Bool("json", false, "Emit result as JSON")
	return cmd
}

func BuildReviewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "review",
		Short: "Review the current plan revision",
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceRoot, err := ResolveWorkspaceRoot()
			if err != nil {
				return err
			}
			jsonFlag, _ := cmd.Flags().GetBool("json")
			payload, doc, report, err := ReviewPlan(workspaceRoot)
			if err != nil {
				return err
			}
			if jsonFlag {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(payload)
			}
			ws, _ := LoadWorkflowState(workspaceRoot)
			tasks, _ := LoadTasks(workspaceRoot)
			if len(tasks) == 0 {
				tasks = payload.TaskItems
			}
			fmt.Print(RenderTasksReviewText(workspaceRoot, ws, tasks, report))
			_ = doc
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "Emit review as JSON")
	return cmd
}

func BuildApproveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "approve",
		Short: "Approve the current plan revision for implementation",
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceRoot, err := ResolveWorkspaceRoot()
			if err != nil {
				return err
			}
			revision, _ := cmd.Flags().GetInt("revision")
			jsonFlag, _ := cmd.Flags().GetBool("json")
			result, _, _, err := ApprovePlan(workspaceRoot, ApprovePlanOptions{Revision: revision})
			if err != nil {
				return err
			}
			if jsonFlag {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(result)
			}
			fmt.Printf("Plan revision %d approved.\n", result.Revision)
			if len(result.UnlockedTasks) > 0 {
				fmt.Println("Tasks unlocked:")
				for _, id := range result.UnlockedTasks {
					fmt.Printf("  %s\n", id)
				}
			}
			fmt.Println("\nWorkflow: APPROVED")
			fmt.Println("\nNext: feature-dev ready")
			return nil
		},
	}
	cmd.Flags().Int("revision", 0, "Explicit revision to approve (defaults to current)")
	cmd.Flags().Bool("json", false, "Emit result as JSON")
	return cmd
}

func BuildRejectCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reject",
		Short: "Reject the current plan revision",
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceRoot, err := ResolveWorkspaceRoot()
			if err != nil {
				return err
			}
			reason, _ := cmd.Flags().GetString("reason")
			if reason == "" {
				return fmt.Errorf("--reason is required")
			}
			ws, err := RejectPlan(workspaceRoot, reason)
			if err != nil {
				return err
			}
			fmt.Printf("Workflow: %s\n", ws.WorkflowStatus)
			fmt.Println("Implementation remains locked.")
			return nil
		},
	}
	cmd.Flags().String("reason", "", "Reason for rejection")
	return cmd
}

func BuildReplanCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "replan",
		Short: "Enter replanning mode for the current feature plan",
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceRoot, err := ResolveWorkspaceRoot()
			if err != nil {
				return err
			}
			reason, _ := cmd.Flags().GetString("reason")
			ws, err := Replan(workspaceRoot, reason)
			if err != nil {
				return err
			}
			fmt.Printf("Workflow: %s\n", ws.WorkflowStatus)
			fmt.Println("Edit the draft plan and run feature-dev plan submit.")
			return nil
		},
	}
	cmd.Flags().String("reason", "", "Reason for replanning")
	return cmd
}

func BuildPlanDiffCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plan-diff",
		Short: "Show differences between plan revisions",
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceRoot, err := ResolveWorkspaceRoot()
			if err != nil {
				return err
			}
			fromRev, _ := cmd.Flags().GetInt("from")
			toRev, _ := cmd.Flags().GetInt("to")
			diff, err := PlanDiffBetweenRevisions(workspaceRoot, fromRev, toRev)
			if err != nil {
				return err
			}
			fmt.Print(FormatPlanDiffOutput(diff))
			return nil
		},
	}
	cmd.Flags().Int("from", 0, "From revision (default previous)")
	cmd.Flags().Int("to", 0, "To revision (default current)")
	return cmd
}
