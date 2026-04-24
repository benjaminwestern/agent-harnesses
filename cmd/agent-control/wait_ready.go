package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"
)

func waitReadyCommand(args []string) error {
	flags := flag.NewFlagSet("wait-ready", flag.ExitOnError)
	flags.SetOutput(os.Stdout)
	socketPath := flags.String("socket-path", "", "Unix socket path")
	timeout := flags.Duration("timeout", 20*time.Second, "Maximum wait time")
	interval := flags.Duration("interval", 200*time.Millisecond, "Polling interval")
	if err := flags.Parse(args); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	client := newSocketRPCClient(*socketPath)
	if err := client.WaitReady(ctx, *interval); err != nil {
		return err
	}
	_, err := fmt.Fprintln(os.Stdout, "ready")
	return err
}
