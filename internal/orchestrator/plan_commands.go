package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
		taskLoad, err := LoadTasksWithReport(workspaceRoot)
		if err != nil {
			return SubmitPlanResult{}, err
		}
		sourceTasks = taskLoad.Tasks
		bundle, err := LoadPlanningDraftBundle(workspaceRoot)
		if err != nil {
			return SubmitPlanResult{}, err
		}
		var taskReport PlanValidationReport
		if !taskLoad.Report.Valid {
			taskReport = taskLoad.Report
		} else {
			taskReport, err = ValidateTaskPlan(workspaceRoot, sourceTasks, true)
			if err != nil {
				return SubmitPlanResult{}, err
			}
		}
		bundleEarly := ValidateDraftBundleEarly(workspaceRoot, bundle)
		earlyReport := MergeValidationReports(taskReport, bundleEarly)
		if !earlyReport.Valid {
			_ = handleValidationFailure(workspaceRoot, &ws, earlyReport, ws.CurrentPlanRevision)
			result := SubmitPlanResult{Valid: false, Report: earlyReport, Status: ws.WorkflowStatus}
			return result, fmt.Errorf("%w: fix planning artifacts and resubmit", ErrPlanValidationFailed)
		}
		doc = MergeDraftBundleIntoPlan(bundle, sourceTasks, PlanFeature{
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
	if opts.FromTasks {
		bundle, _ := LoadPlanningDraftBundle(workspaceRoot)
		mergedReport := ValidateDraftBundleMerged(workspaceRoot, bundle, sourceTasks, doc)
		report = MergeValidationReports(report, mergedReport)
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
		ws.WorkflowStatus = WorkflowClarificationNeeded
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
		ClearValidationArtifactsOnSuccess(workspaceRoot)
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
	if opts.FromTasks {
		if err := SnapshotPlanningDraftBundle(workspaceRoot, nextRevision); err != nil {
			return SubmitPlanResult{}, err
		}
	}

	result.Status = ws.WorkflowStatus
	if !report.Valid {
		_ = handleValidationFailure(workspaceRoot, &ws, report, nextRevision)
		result.Status = ws.WorkflowStatus
	}
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

func PreviewTaskPlan(workspaceRoot string) (PlanReviewJSON, PlanDocument, PlanValidationReport, error) {
	ws, err := LoadWorkflowState(workspaceRoot)
	if err != nil {
		return PlanReviewJSON{}, PlanDocument{}, PlanValidationReport{}, err
	}
	taskLoad, err := LoadTasksWithReport(workspaceRoot)
	if err != nil {
		return PlanReviewJSON{}, PlanDocument{}, PlanValidationReport{}, err
	}
	tasks := taskLoad.Tasks
	report, err := RunOrderedPlanValidation(workspaceRoot, "PLAN.md")
	if err != nil {
		return PlanReviewJSON{}, PlanDocument{}, PlanValidationReport{}, err
	}
	if !taskLoad.Report.Valid {
		report = MergeValidationReports(taskLoad.Report, report)
	}
	doc := PlanDocument{}
	bundle, _ := LoadPlanningDraftBundle(workspaceRoot)
	if Exists(PlanDraftRequirementsPath(workspaceRoot)) {
		doc = MergeDraftBundleIntoPlan(bundle, tasks, PlanFeature{ID: "feature", Title: "Feature implementation plan"})
	} else if ws.CurrentPlanRevision > 0 {
		doc, _ = LoadCurrentPlanDocument(workspaceRoot)
	}
	payload := BuildPlanReviewJSON(workspaceRoot, doc, ws, report, tasks)
	return payload, doc, report, nil
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
	plan.AddCommand(BuildPlanDraftInitCommand())
	plan.AddCommand(buildPlanValidateCommand())
	plan.AddCommand(buildPlanClarifyCommand())
	plan.AddCommand(buildPlanClarifyRecordCommand())
	plan.AddCommand(buildPlanCoverageCommand())
	plan.AddCommand(buildPlanScaffoldCommand())
	return plan
}

func buildPlanCoverageCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "coverage",
		Short: "Compare PLAN.md source items against requirements and tasks",
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceRoot, err := ResolveWorkspaceRoot()
			if err != nil {
				return err
			}
			from, _ := cmd.Flags().GetString("from")
			jsonFlag, _ := cmd.Flags().GetBool("json")
			report, err := RunPlanCoverage(workspaceRoot, from)
			if err != nil {
				return err
			}
			if jsonFlag {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(report)
			}
			fmt.Printf("valid: %t\n", report.Valid)
			fmt.Printf("source: %s\n", report.Source)
			for k, v := range report.Checks {
				fmt.Printf("check %s: %s\n", k, v)
			}
			for _, action := range report.AgentActions {
				fmt.Printf("- %s\n", action)
			}
			if !report.Valid {
				return fmt.Errorf("plan coverage incomplete")
			}
			return nil
		},
	}
	cmd.Flags().String("from", "PLAN.md", "Path to PLAN.md")
	cmd.Flags().Bool("json", false, "Emit coverage report as JSON")
	return cmd
}

func buildPlanScaffoldCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scaffold",
		Short: "Scaffold planning bundle and tasks, then run PLAN coverage",
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceRoot, err := ResolveWorkspaceRoot()
			if err != nil {
				return err
			}
			if !Exists(ResolveFeatureDir(workspaceRoot)) {
				return fmt.Errorf("workspace not initialized; run feature-dev init first")
			}
			from, _ := cmd.Flags().GetString("from")
			jsonFlag, _ := cmd.Flags().GetBool("json")
			force, _ := cmd.Flags().GetBool("force")

			draftWritten, err := InitPlanningDraftFromTemplates(workspaceRoot, force)
			if err != nil {
				return err
			}
			_ = SeedRequirementsFromRepos(workspaceRoot, force)
			taskPath, err := InitTasksFromTemplate(workspaceRoot, force)
			if err != nil && !strings.Contains(err.Error(), "already exists") {
				return err
			}
			coverage, err := RunPlanCoverage(workspaceRoot, from)
			if err != nil && !os.IsNotExist(err) {
				return err
			}

			// Fail closed on placeholder artifacts when repos are registered.
			tasks, loadErr := LoadTasks(workspaceRoot)
			if loadErr == nil && len(tasks) > 0 {
				for _, t := range tasks {
					if t.Repository == "repo-a" || t.Repository == "repo-b" {
						return fmt.Errorf("scaffold produced placeholder repository %q; ensure repositories.json is populated and re-run with --force", t.Repository)
					}
				}
				if report, vErr := ValidateTaskPlan(workspaceRoot, tasks, true); vErr == nil && !report.Valid {
					for _, e := range report.Errors {
						if e.Code == "invalid_repository" || e.Code == "unknown_requirement_id" || e.Code == ErrCodeVerificationCmdMissing {
							return fmt.Errorf("scaffold validation failed: %s", e.Message)
						}
					}
				}
			}

			nextSteps := []string{
				"Run: feature-dev schema show tasks --json (and schema show workspace-verify --json)",
				"Read PLAN.md and populate requirements.json with source_section/source_ref fields",
				"Map each requirement to tasks via requirement_ids in tasks.json",
				"Declare workspace-verify.json commands or cross_repo_verification_deferral in risks if cross-repo deps exist",
				fmt.Sprintf("Run: feature-dev plan coverage --from %s --json", from),
				"Run: feature-dev task preview --json",
				"Run: feature-dev plan submit --from-tasks",
			}
			manifest := map[string]any{
				"required_files": []string{
					".feature/plans/draft/requirements.json",
					".feature/plans/draft/assumptions.json",
					".feature/plans/draft/risks.json",
					".feature/plans/draft/impact.json",
					".feature/plans/draft/repo-analysis.json",
					".feature/plans/draft/workspace-verify.json",
					".feature/tasks/tasks.json",
				},
				"schema_artifacts": ListSchemaArtifacts(),
				"next_agent_steps": nextSteps,
			}
			manifestPath := filepath.Join(PlanDraftDir(workspaceRoot), "manifest.json")
			data, _ := json.MarshalIndent(manifest, "", "  ")
			_ = WriteFileAtomically(manifestPath, append(data, '\n'))

			if jsonFlag {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]any{
					"draft_written": draftWritten,
					"tasks_path":    taskPath,
					"coverage":      coverage,
					"manifest":      manifestPath,
					"next_steps":    nextSteps,
				})
			}
			fmt.Println("Scaffold complete. Calling agent: read PLAN.md, resolve unmapped items, then run task preview.")
			for _, step := range nextSteps {
				fmt.Printf("- %s\n", step)
			}
			return nil
		},
	}
	cmd.Flags().String("from", "PLAN.md", "Path to PLAN.md")
	cmd.Flags().Bool("force", false, "Overwrite existing draft files and tasks.json")
	cmd.Flags().Bool("json", false, "Emit scaffold result as JSON")
	return cmd
}

func buildPlanValidateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "validate",
		Aliases: []string{"check"},
		Short:   "Validate tasks and planning draft bundle before submit (alias for task preview)",
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceRoot, err := ResolveWorkspaceRoot()
			if err != nil {
				return err
			}
			jsonFlag, _ := cmd.Flags().GetBool("json")
			payload, _, report, err := PreviewTaskPlan(workspaceRoot)
			if err != nil {
				return err
			}
			if jsonFlag {
				out := map[string]any{
					"valid":  report.Valid,
					"report": report,
					"review": payload,
				}
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(out)
			}
			fmt.Printf("valid: %t\n", report.Valid)
			for _, e := range report.Errors {
				fmt.Printf("- [%s] %s\n", e.Code, e.Message)
			}
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "Emit validation report as JSON")
	return cmd
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
			fmt.Print(RenderTasksReviewText(workspaceRoot, ws, tasks, doc, report))
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
			deferUnmapped, _ := cmd.Flags().GetString("defer-unmapped")
			jsonFlag, _ := cmd.Flags().GetBool("json")
			result, _, _, err := ApprovePlan(workspaceRoot, ApprovePlanOptions{Revision: revision, DeferUnmapped: deferUnmapped})
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
	cmd.Flags().String("defer-unmapped", "", "Defer incomplete PLAN coverage with documented reason")
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
