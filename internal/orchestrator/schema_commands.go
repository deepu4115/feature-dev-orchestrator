package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

func BuildSchemaCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schema",
		Short: "Show canonical JSON schemas and examples for planning artifacts",
	}
	cmd.AddCommand(buildSchemaShowCommand())
	cmd.AddCommand(buildSchemaListCommand())
	return cmd
}

func buildSchemaListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available schema artifacts",
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonFlag, _ := cmd.Flags().GetBool("json")
			artifacts := ListSchemaArtifacts()
			sort.Strings(artifacts)
			if jsonFlag {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]any{"artifacts": artifacts})
			}
			fmt.Println("Available schema artifacts:")
			for _, name := range artifacts {
				fmt.Printf("- %s\n", name)
			}
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "Emit artifact list as JSON")
	return cmd
}

func buildSchemaShowCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <artifact>",
		Short: "Show canonical example and field contract for a planning artifact",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			payload, err := SchemaShow(args[0])
			if err != nil {
				return err
			}
			jsonFlag, _ := cmd.Flags().GetBool("json")
			if jsonFlag {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(payload)
			}
			fmt.Printf("artifact: %s\n", payload["artifact"])
			fmt.Printf("description: %s\n", payload["description"])
			fmt.Printf("root_key: %s\n", payload["root_key"])
			fmt.Printf("init_command: %s\n", payload["init_command"])
			fmt.Println("required_fields:")
			for _, field := range payload["required_fields"].([]string) {
				fmt.Printf("  - %s\n", field)
			}
			fmt.Println("example:")
			fmt.Println(payload["example"])
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "Emit schema contract as JSON")
	return cmd
}

func BuildPlanDraftInitCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "draft",
		Short: "Manage planning draft bundle files",
	}
	cmd.AddCommand(buildPlanDraftInitSubcommand())
	return cmd
}

func buildPlanDraftInitSubcommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Scaffold valid planning draft bundle files from built-in templates",
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceRoot, err := ResolveWorkspaceRoot()
			if err != nil {
				return err
			}
			if !Exists(ResolveFeatureDir(workspaceRoot)) {
				return fmt.Errorf("workspace not initialized; run feature-dev init first")
			}
			force, _ := cmd.Flags().GetBool("force")
			jsonFlag, _ := cmd.Flags().GetBool("json")

			written, err := InitPlanningDraftFromTemplates(workspaceRoot, force)
			if err != nil {
				return err
			}
			if jsonFlag {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]any{
					"written": written,
					"count":   len(written),
				})
			}
			if len(written) == 0 {
				fmt.Println("planning draft files already exist (use --force to overwrite)")
				return nil
			}
			fmt.Printf("wrote %d planning draft file(s):\n", len(written))
			for _, path := range written {
				fmt.Printf("- %s\n", path)
			}
			fmt.Println("next: edit draft files with domain content, then run feature-dev task preview --json")
			return nil
		},
	}
	cmd.Flags().Bool("force", false, "Overwrite existing draft files")
	cmd.Flags().Bool("json", false, "Emit result as JSON")
	return cmd
}

func BuildTaskInitCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Scaffold a valid tasks.json from built-in template",
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceRoot, err := ResolveWorkspaceRoot()
			if err != nil {
				return err
			}
			if !Exists(ResolveFeatureDir(workspaceRoot)) {
				return fmt.Errorf("workspace not initialized; run feature-dev init first")
			}
			force, _ := cmd.Flags().GetBool("force")
			jsonFlag, _ := cmd.Flags().GetBool("json")

			path, err := InitTasksFromTemplate(workspaceRoot, force)
			if err != nil {
				if !force && strings.Contains(err.Error(), "already exists") {
					if jsonFlag {
						enc := json.NewEncoder(os.Stdout)
						enc.SetIndent("", "  ")
						return enc.Encode(map[string]any{
							"written": false,
							"path":    TaskStoragePath(workspaceRoot),
							"message": err.Error(),
						})
					}
				}
				return err
			}
			if jsonFlag {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]any{
					"written": true,
					"path":    path,
				})
			}
			fmt.Printf("wrote %s\n", path)
			fmt.Println("next: edit tasks with domain content, then run feature-dev task preview --json")
			return nil
		},
	}
	cmd.Flags().Bool("force", false, "Overwrite existing tasks.json")
	cmd.Flags().Bool("json", false, "Emit result as JSON")
	return cmd
}
