package orchestration

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type CommandLaunchRequest struct {
	Command string
	Args    []string
	Env     []string
	LogPath string
}

func LaunchDetachedCommand(ctx context.Context, request CommandLaunchRequest) error {
	if request.Command == "" {
		return fmt.Errorf("command is required")
	}
	if request.LogPath == "" {
		return fmt.Errorf("log path is required")
	}
	if err := os.MkdirAll(filepath.Dir(request.LogPath), 0o750); err != nil {
		return fmt.Errorf("create command log directory: %w", err)
	}
	logFile, err := os.OpenFile(request.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open command log: %w", err)
	}
	defer func() {
		_ = logFile.Close()
	}()

	//nolint:gosec // Agentic Control intentionally starts configured worker commands.
	command := exec.CommandContext(ctx, request.Command, request.Args...)
	command.Env = append(os.Environ(), request.Env...)
	command.Stdout = logFile
	command.Stderr = logFile
	if err := command.Start(); err != nil {
		return fmt.Errorf("start command: %w", err)
	}
	if err := command.Process.Release(); err != nil {
		return fmt.Errorf("release command process: %w", err)
	}
	return nil
}
