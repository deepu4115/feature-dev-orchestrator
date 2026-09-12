package main

import (
	"fmt"
	"os"

	orchestrator "feature-dev-orchestrator/internal/orchestrator"
)

func main() {
	root := orchestrator.BuildRootCommand()
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
