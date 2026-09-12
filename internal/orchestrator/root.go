package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func BuildRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:     "feature-dev",
		Short:   "Workspace-native orchestration engine for feature work across repos",
		Version: "0.1.0",
	}

	root.AddCommand(BuildVersionCommand())
	root.AddCommand(BuildDoctorCommand())
	root.AddCommand(BuildInitCommand())
	root.AddCommand(BuildDiscoverCommand())
	root.AddCommand(BuildRepositoriesCommand())
	root.AddCommand(BuildStatusCommand())
	root.AddCommand(BuildGraphCommand())
	root.AddCommand(BuildReadyCommand())
	root.AddCommand(BuildStartTaskCommand())
	root.AddCommand(BuildTaskCommand())
	root.AddCommand(BuildContextCommand())
	root.AddCommand(BuildVerifyCommand())
	root.AddCommand(BuildCompleteTaskCommand())
	root.AddCommand(BuildFailTaskCommand())
	root.AddCommand(BuildReconcileCommand())
	root.AddCommand(BuildVerifyWorkspaceCommand())
	root.AddCommand(BuildExecuteNextCommand())
	root.AddCommand(BuildExecuteLoopCommand())

	return root
}

func BuildVersionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print the feature-dev version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("feature-dev version 0.1.0")
		},
	}
	return cmd
}

func BuildDoctorCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Validate workspace setup",
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceRoot, err := ResolveWorkspaceRoot()
			if err != nil {
				return err
			}
			if !Exists(ResolveFeatureDir(workspaceRoot)) {
				return fmt.Errorf("workspace is not initialized: missing %s", DefaultFeatureDir)
			}
			cfg, err := LoadWorkspaceConfig(workspaceRoot)
			if err != nil {
				return fmt.Errorf("load workspace config: %w", err)
			}
			if cfg.SchemaVersion == "" {
				return fmt.Errorf("invalid workspace config: schema_version missing")
			}
			fmt.Println("workspace configuration looks valid")
			return nil
		},
	}
	return cmd
}

func BuildInitCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize the workspace orchestration state",
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceRoot, err := ResolveWorkspaceRoot()
			if err != nil {
				return err
			}
			if err := InitWorkspace(workspaceRoot); err != nil {
				return err
			}
			fmt.Printf("initialized workspace at %s\n", workspaceRoot)
			return nil
		},
	}
	return cmd
}

func BuildDiscoverCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "discover",
		Short: "Discover repositories in the workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceRoot, err := ResolveWorkspaceRoot()
			if err != nil {
				return err
			}
			if !Exists(ResolveFeatureDir(workspaceRoot)) {
				return fmt.Errorf("workspace not initialized; run feature-dev init first")
			}
			repos, err := DiscoverRepos(workspaceRoot)
			if err != nil {
				return err
			}
			if err := SaveRepositories(workspaceRoot, repos); err != nil {
				return err
			}
			fmt.Printf("discovered %d repositories\n", len(repos))
			for _, repo := range repos {
				fmt.Printf("- %s: %s\n", repo.ID, repo.Path)
			}
			return nil
		},
	}
	return cmd
}

func BuildRepositoriesCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "repositories",
		Short: "List discovered repositories",
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceRoot, err := ResolveWorkspaceRoot()
			if err != nil {
				return err
			}
			jsonFlag, _ := cmd.Flags().GetBool("json")
			data, err := os.ReadFile(DefaultRepositoriesPath(workspaceRoot))
			if err != nil {
				return fmt.Errorf("read repositories file: %w", err)
			}
			if jsonFlag {
				fmt.Println(strings.TrimSpace(string(data)))
				return nil
			}
			fmt.Println(strings.TrimSpace(string(data)))
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "Emit the repository registry as JSON")
	return cmd
}

func BuildStatusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show workspace status",
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceRoot, err := ResolveWorkspaceRoot()
			if err != nil {
				return err
			}
			jsonFlag, _ := cmd.Flags().GetBool("json")
			payload := map[string]string{
				"workspace":   workspaceRoot,
				"feature_dir": ResolveFeatureDir(workspaceRoot),
			}
			if jsonFlag {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(payload)
			}
			fmt.Printf("workspace: %s\n", payload["workspace"])
			fmt.Printf("feature dir: %s\n", payload["feature_dir"])
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "Emit status as JSON")
	return cmd
}

func BuildReadyCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ready",
		Short: "Query ready work items",
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceRoot, err := ResolveWorkspaceRoot()
			if err != nil {
				return err
			}
			tasks, err := LoadTasks(workspaceRoot)
			if err != nil {
				return err
			}
			ready, err := ReadyTasks(tasks)
			if err != nil {
				return err
			}
			jsonFlag, _ := cmd.Flags().GetBool("json")
			payload := map[string]any{"ready": []string{}, "count": 0}
			if len(ready) > 0 {
				payload["ready"] = ready
				payload["count"] = len(ready)
			}
			if jsonFlag {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(payload)
			}
			if len(ready) == 0 {
				fmt.Println("ready queue is empty")
				return nil
			}
			for _, task := range ready {
				fmt.Println(task.String())
			}
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "Emit the ready queue as JSON")
	return cmd
}
