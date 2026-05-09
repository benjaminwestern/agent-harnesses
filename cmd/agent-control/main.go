package main

import (
	"fmt"
	"github.com/spf13/cobra"
	"os"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "agent_control",
		Short: "Local agent runtime, orchestration, and Court control plane",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	root.AddCommand(newServeCmd())
	root.AddCommand(newSetupCmd())
	root.AddCommand(newWaitReadyCmd())
	root.AddCommand(newWorkerCmd())
	root.AddCommand(newDescribeCmd())
	root.AddCommand(newModelsCmd())
	root.AddCommand(newThreadsCmd())
	root.AddCommand(newSessionsCmd())
	root.AddCommand(newSmokeCmd())
	root.AddCommand(newOrchestrateCmd())
	root.AddCommand(newCourtCmd())
	root.AddCommand(newWorkspaceMemoryCmd())
	root.AddCommand(newWorkspaceDocumentsCmd())
	root.AddCommand(newWorkspaceTasksCmd())
	root.AddCommand(newWorkspaceWakeupsCmd())
	root.AddCommand(newWorkspaceLeasesCmd())
	root.AddCommand(newMCPCmd())
	root.AddCommand(newDatasetCmd())
	return root
}
