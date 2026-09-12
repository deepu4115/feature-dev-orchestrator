package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func BuildContextCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "context <task-id>",
		Short: "Show repository-aware context for a task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			level, _ := cmd.Flags().GetString("level")
			maxFiles, _ := cmd.Flags().GetInt("max-files")
			maxChars, _ := cmd.Flags().GetInt("max-chars")
			maxFileChars, _ := cmd.Flags().GetInt("max-file-chars")

			workspaceRoot, err := ResolveWorkspaceRoot()
			if err != nil {
				return err
			}
			tasks, err := LoadTasks(workspaceRoot)
			if err != nil {
				return err
			}
			var target Task
			found := false
			for _, task := range tasks {
				if task.ID == args[0] {
					target = task
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("task %s not found", args[0])
			}

			if target.Repository == "" || target.Repository == "workspace" {
				return fmt.Errorf("context requires a repository-scoped task")
			}

			payload, err := BuildTaskContext(workspaceRoot, target, level, ContextBudget{
				MaxFiles:      maxFiles,
				MaxTotalChars: maxChars,
				MaxFileChars:  maxFileChars,
			})
			if err != nil {
				return err
			}
			jsonFlag, _ := cmd.Flags().GetBool("json")
			if jsonFlag {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(payload)
			}
			fmt.Println(RenderContextText(payload))
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "Emit repository context as JSON")
	cmd.Flags().String("level", "brief", "Context detail level: brief or full")
	cmd.Flags().Int("max-files", DefaultContextBudget().MaxFiles, "Maximum number of files to include for full context")
	cmd.Flags().Int("max-chars", DefaultContextBudget().MaxTotalChars, "Maximum total snippet characters for full context")
	cmd.Flags().Int("max-file-chars", DefaultContextBudget().MaxFileChars, "Maximum characters per snippet file")
	return cmd
}
