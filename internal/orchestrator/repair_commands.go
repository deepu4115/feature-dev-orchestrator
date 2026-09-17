package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
)

type RepairReport struct {
	Valid    bool                  `json:"valid"`
	Errors   []PlanValidationIssue `json:"errors,omitempty"`
	Warnings []PlanValidationIssue `json:"warnings,omitempty"`
	Applied  bool                  `json:"applied"`
	Backup   string                `json:"backup,omitempty"`
}

func RepairTasks(workspaceRoot string, apply bool) (RepairReport, error) {
	report := RepairReport{Valid: true}
	load, err := LoadTasksWithReport(workspaceRoot)
	if err != nil {
		return report, err
	}
	report.Errors = load.Report.Errors
	report.Warnings = load.Report.Warnings
	report.Valid = load.Report.Valid
	if report.Valid || !apply {
		return report, nil
	}

	path := TaskStoragePath(workspaceRoot)
	backupDir := filepath.Join(ResolveFeatureDir(workspaceRoot), "backups", time.Now().UTC().Format("20060102T150405Z"))
	if err := EnsureDir(backupDir); err != nil {
		return report, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return report, err
	}
	backupPath := filepath.Join(backupDir, "tasks.json")
	if err := WriteFileAtomically(backupPath, data); err != nil {
		return report, err
	}
	report.Backup = backupPath

	if hasCode(load.Report.Errors, "invalid_verification_type") {
		for i := range load.Tasks {
			for j := range load.Tasks[i].Verification {
				load.Tasks[i].Verification[j].Command = NormalizeVerificationCommand(load.Tasks[i], load.Tasks[i].Verification[j].Command)
			}
		}
	}
	if err := SaveTasks(workspaceRoot, load.Tasks); err != nil {
		return report, err
	}
	reload, err := LoadTasksWithReport(workspaceRoot)
	if err != nil {
		return report, err
	}
	report.Valid = reload.Report.Valid
	report.Errors = reload.Report.Errors
	report.Applied = true
	return report, nil
}

func BuildRepairCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "repair",
		Short: "Diagnose and repair planning artifacts",
	}
	cmd.AddCommand(buildRepairTasksCommand())
	return cmd
}

func buildRepairTasksCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tasks",
		Short: "Validate or repair tasks.json",
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceRoot, err := ResolveWorkspaceRoot()
			if err != nil {
				return err
			}
			apply, _ := cmd.Flags().GetBool("apply")
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			if dryRun {
				apply = false
			}
			report, err := RepairTasks(workspaceRoot, apply)
			if err != nil {
				return err
			}
			jsonFlag, _ := cmd.Flags().GetBool("json")
			if jsonFlag {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(report)
			}
			fmt.Printf("valid: %t applied: %t\n", report.Valid, report.Applied)
			if report.Backup != "" {
				fmt.Printf("backup: %s\n", report.Backup)
			}
			if !report.Valid {
				return fmt.Errorf("tasks repair could not produce valid tasks.json")
			}
			return nil
		},
	}
	cmd.Flags().Bool("dry-run", false, "Diagnose without writing")
	cmd.Flags().Bool("apply", false, "Apply safe repairs with backup")
	cmd.Flags().Bool("json", false, "Emit repair report as JSON")
	return cmd
}
