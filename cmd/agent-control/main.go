package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	internal "github.com/benjaminwestern/agentic-control/internal/controlplane"
	"github.com/benjaminwestern/agentic-control/internal/controlplane/providers/claude"
	"github.com/benjaminwestern/agentic-control/internal/controlplane/providers/codex"
	"github.com/benjaminwestern/agentic-control/internal/controlplane/providers/gemini"
	"github.com/benjaminwestern/agentic-control/internal/controlplane/providers/opencode"
	"github.com/benjaminwestern/agentic-control/pkg/contract"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}
	if os.Args[1] == "--help" || os.Args[1] == "-h" || os.Args[1] == "help" {
		usage()
		return
	}

	switch os.Args[1] {
	case "serve":
		if err := serve(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "describe":
		if err := describe(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	default:
		usage()
		os.Exit(1)
	}
}

func serve(args []string) error {
	flags := flag.NewFlagSet("serve", flag.ExitOnError)
	flags.SetOutput(os.Stdout)
	socketPath := flags.String("socket-path", filepath.Join(os.TempDir(), "agentic-control.sock"), "Unix socket path")
	flags.Parse(args)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var service *internal.Service
	emit := func(event contract.RuntimeEvent) {
		service.PublishEvent(event)
	}
	service = internal.NewService(
		codex.NewProvider(emit),
		claude.NewProvider(emit),
		gemini.NewProvider(emit),
		opencode.NewProvider(emit),
	)

	server := internal.NewRPCServer(service)
	return server.ServeUnix(ctx, *socketPath)
}

func describe(args []string) error {
	flags := flag.NewFlagSet("describe", flag.ExitOnError)
	flags.SetOutput(os.Stdout)
	socketPath := flags.String("socket-path", filepath.Join(os.TempDir(), "agentic-control.sock"), "Unix socket path")
	flags.Parse(args)

	connection, err := net.Dial("unix", *socketPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOENT) || errors.Is(err, syscall.ECONNREFUSED) {
			return fmt.Errorf("could not connect to agent_control at %q; start it first with `.artifacts/bin/agent_control serve --socket-path %q`", *socketPath, *socketPath)
		}
		return err
	}
	defer connection.Close()

	request := map[string]any{
		"id":     "describe-1",
		"method": "system.describe",
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		return err
	}
	if _, err := connection.Write(append(encoded, '\n')); err != nil {
		return err
	}

	scanner := bufio.NewScanner(connection)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return err
		}
		return fmt.Errorf("agent_control describe received no response")
	}
	var formatted any
	if err := json.Unmarshal(scanner.Bytes(), &formatted); err != nil {
		return err
	}
	pretty, err := json.MarshalIndent(formatted, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(os.Stdout, "%s\n", pretty)
	return err
}

func usage() {
	fmt.Fprint(os.Stdout,
		"Usage:\n"+
			"  agent_control serve [--socket-path <path>]\n"+
			"  agent_control describe [--socket-path <path>]\n\n"+
			"Primary use case:\n"+
			"  serve   Start the app-managed session control-plane daemon on a Unix socket.\n\n"+
			"  describe  Call system.describe against a running control-plane socket.\n\n"+
			"First success:\n"+
			"  1. Start `agent_control serve`.\n"+
			"  2. Connect over the Unix socket.\n"+
			"  3. Run `agent_control describe` to discover runtime capabilities.\n")
}
