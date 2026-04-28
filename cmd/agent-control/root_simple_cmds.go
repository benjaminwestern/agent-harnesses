package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	internal "github.com/benjaminwestern/agentic-control/internal/controlplane"
	"github.com/benjaminwestern/agentic-control/internal/controlplane/providers/claude"
	"github.com/benjaminwestern/agentic-control/internal/controlplane/providers/codex"
	"github.com/benjaminwestern/agentic-control/internal/controlplane/providers/gemini"
	"github.com/benjaminwestern/agentic-control/internal/controlplane/providers/opencode"
	"github.com/benjaminwestern/agentic-control/internal/controlplane/providers/pi"
	"github.com/benjaminwestern/agentic-control/internal/court"
	"github.com/benjaminwestern/agentic-control/pkg/contract"
	"github.com/spf13/cobra"
)

func newServeCmd() *cobra.Command {
	var socketPath string
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the local control-plane daemon.",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			var service *internal.Service
			emit := func(event contract.RuntimeEvent) { service.PublishEvent(event) }
			service = internal.NewService(
				codex.NewProvider(emit),
				claude.NewProvider(emit),
				gemini.NewProvider(emit),
				opencode.NewProvider(emit),
				pi.NewProvider(emit),
			)
			defer func() { _ = service.Close() }()
			server := internal.NewRPCServer(service)
			return server.ServeUnix(ctx, socketPath)
		},
	}
	cmd.Flags().StringVar(&socketPath, "socket-path", filepath.Join(os.TempDir(), defaultSocketPath), "Unix socket path")
	return cmd
}

func newDescribeCmd() *cobra.Command {
	var socketPath string
	cmd := &cobra.Command{
		Use:   "describe",
		Short: "Describe the running control-plane daemon.",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newSocketRPCClient(socketPath)
			return writeJSON(client.Describe())
		},
	}
	cmd.Flags().StringVar(&socketPath, "socket-path", filepath.Join(os.TempDir(), defaultSocketPath), "Unix socket path")
	return cmd
}

func newWaitReadyCmd() *cobra.Command {
	var socketPath string
	var timeout time.Duration
	var interval time.Duration
	cmd := &cobra.Command{
		Use:   "wait-ready",
		Short: "Wait until the local daemon is ready.",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			client := newSocketRPCClient(socketPath)
			if err := client.WaitReady(ctx, interval); err != nil {
				return err
			}
			_, err := fmt.Fprintln(os.Stdout, "ready")
			return err
		},
	}
	cmd.Flags().StringVar(&socketPath, "socket-path", filepath.Join(os.TempDir(), defaultSocketPath), "Unix socket path")
	cmd.Flags().DurationVar(&timeout, "timeout", 20*time.Second, "Maximum wait time")
	cmd.Flags().DurationVar(&interval, "interval", 200*time.Millisecond, "Polling interval")
	return cmd
}

func newWorkerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "worker <worker-id>",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			options := court.EngineOptionsFromEnvironment()
			if socketPath := strings.TrimSpace(os.Getenv("AGENTIC_CONTROL_SOCKET_PATH")); socketPath != "" {
				options.ControlPlane = newSocketRPCClient(socketPath)
			}
			engine, err := court.NewEngine(options)
			if err != nil {
				return err
			}
			defer func() { _ = engine.Close() }()
			return engine.RunWorker(context.Background(), args[0])
		},
	}
	return cmd
}
