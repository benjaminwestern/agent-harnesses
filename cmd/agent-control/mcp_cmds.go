package main

import (
	"context"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

func newMCPCmd() *cobra.Command {
	var socketPath string
	var workspaceID string

	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Run the native Model Context Protocol (MCP) server over stdio",
		RunE: func(cmd *cobra.Command, args []string) error {
			server := newMCPServer(socketPath, workspaceID)
			return server.ServeStdio(context.Background())
		},
	}

	cmd.Flags().StringVar(&socketPath, "socket-path", filepath.Join(os.TempDir(), defaultSocketPath), "Unix socket path to agentic-control daemon")
	cmd.Flags().StringVar(&workspaceID, "workspace", "default", "Workspace ID to bind MCP operations to")

	return cmd
}
