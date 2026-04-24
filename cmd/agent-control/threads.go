package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/benjaminwestern/agentic-control/pkg/contract"
	"github.com/spf13/cobra"
)

func newThreadsCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "threads", Short: "Inspect and mutate durable threads."}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List durable threads.",
		RunE: func(cmd *cobra.Command, args []string) error {
			socketPath, _ := cmd.Flags().GetString("socket-path")
			runtime, _ := cmd.Flags().GetString("runtime")
			archived, _ := cmd.Flags().GetBool("archived")
			all, _ := cmd.Flags().GetBool("all")
			client := newSocketRPCClient(socketPath)
			var archivedFilter *bool
			if !all {
				archivedFilter = &archived
			}
			threads, err := client.ListThreads(context.Background(), runtime, archivedFilter)
			if err != nil {
				return err
			}
			return writeJSON(threads)
		},
	}
	listCmd.Flags().String("socket-path", "", "Unix socket path for a running agent_control daemon")
	listCmd.Flags().String("runtime", "", "Runtime/backend filter")
	listCmd.Flags().Bool("archived", false, "Show archived threads instead of active ones")
	listCmd.Flags().Bool("all", false, "Show both active and archived threads")
	cmd.AddCommand(listCmd)

	getCmd := &cobra.Command{
		Use:   "get <thread-id>",
		Short: "Get one durable thread.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			socketPath, _ := cmd.Flags().GetString("socket-path")
			client := newSocketRPCClient(socketPath)
			thread, err := client.GetThread(context.Background(), args[0], "")
			if err != nil {
				return err
			}
			return writeJSON(thread)
		},
	}
	getCmd.Flags().String("socket-path", "", "Unix socket path for a running agent_control daemon")
	cmd.AddCommand(getCmd)

	eventsCmd := &cobra.Command{
		Use:   "events <thread-id>",
		Short: "List raw persisted thread events.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			socketPath, _ := cmd.Flags().GetString("socket-path")
			after, _ := cmd.Flags().GetInt64("after-id")
			limit, _ := cmd.Flags().GetInt("limit")
			client := newSocketRPCClient(socketPath)
			events, err := client.ThreadEvents(context.Background(), args[0], after, limit)
			if err != nil {
				return err
			}
			return writeJSON(events)
		},
	}
	eventsCmd.Flags().String("socket-path", "", "Unix socket path for a running agent_control daemon")
	eventsCmd.Flags().Int64("after-id", 0, "Only show events after this id")
	eventsCmd.Flags().Int("limit", 200, "Maximum events to return")
	cmd.AddCommand(eventsCmd)

	readCmd := &cobra.Command{
		Use:   "read <thread-id>",
		Short: "Read a durable thread as grouped turns.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			socketPath, _ := cmd.Flags().GetString("socket-path")
			client := newSocketRPCClient(socketPath)
			read, err := client.ReadThread(context.Background(), args[0])
			if err != nil {
				return err
			}
			return writeJSON(read)
		},
	}
	readCmd.Flags().String("socket-path", "", "Unix socket path for a running agent_control daemon")
	cmd.AddCommand(readCmd)

	nameCmd := &cobra.Command{
		Use:   "name <thread-id>",
		Short: "Rename a thread.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			socketPath, _ := cmd.Flags().GetString("socket-path")
			name, _ := cmd.Flags().GetString("value")
			if strings.TrimSpace(name) == "" {
				return fmt.Errorf("--value is required")
			}
			client := newSocketRPCClient(socketPath)
			if err := client.SetThreadName(context.Background(), args[0], name); err != nil {
				return err
			}
			return writeJSON(renameThreadResult{OK: true, ThreadID: args[0], Name: strings.TrimSpace(name)})
		},
	}
	nameCmd.Flags().String("socket-path", "", "Unix socket path for a running agent_control daemon")
	nameCmd.Flags().String("value", "", "Thread name")
	cmd.AddCommand(nameCmd)

	metadataCmd := &cobra.Command{
		Use:   "metadata <thread-id>",
		Short: "Replace thread metadata with a typed metadata object.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			socketPath, _ := cmd.Flags().GetString("socket-path")
			jsonValue, _ := cmd.Flags().GetString("json")
			metadata, err := parseMetadataJSON(jsonValue)
			if err != nil {
				return err
			}
			client := newSocketRPCClient(socketPath)
			if err := client.SetThreadMetadata(context.Background(), args[0], metadata); err != nil {
				return err
			}
			return writeJSON(threadMetadataResult{OK: true, ThreadID: args[0], Metadata: metadata})
		},
	}
	metadataCmd.Flags().String("socket-path", "", "Unix socket path for a running agent_control daemon")
	metadataCmd.Flags().String("json", "{}", "Thread metadata as JSON object")
	cmd.AddCommand(metadataCmd)

	forkCmd := &cobra.Command{
		Use:   "fork <thread-id>",
		Short: "Create a logical child thread from an existing thread.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			socketPath, _ := cmd.Flags().GetString("socket-path")
			name, _ := cmd.Flags().GetString("name")
			jsonValue, _ := cmd.Flags().GetString("metadata-json")
			metadata, err := parseMetadataJSON(jsonValue)
			if err != nil {
				return err
			}
			client := newSocketRPCClient(socketPath)
			thread, err := client.ForkThread(context.Background(), args[0], name, metadata)
			if err != nil {
				return err
			}
			return writeJSON(thread)
		},
	}
	forkCmd.Flags().String("socket-path", "", "Unix socket path for a running agent_control daemon")
	forkCmd.Flags().String("name", "", "Optional child thread name")
	forkCmd.Flags().String("metadata-json", "{}", "Optional child thread metadata JSON object")
	cmd.AddCommand(forkCmd)

	rollbackCmd := &cobra.Command{
		Use:   "rollback <thread-id>",
		Short: "Create a logical rollback child thread.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			socketPath, _ := cmd.Flags().GetString("socket-path")
			turns, _ := cmd.Flags().GetInt("turns")
			client := newSocketRPCClient(socketPath)
			thread, err := client.RollbackThread(context.Background(), args[0], turns)
			if err != nil {
				return err
			}
			return writeJSON(thread)
		},
	}
	rollbackCmd.Flags().String("socket-path", "", "Unix socket path for a running agent_control daemon")
	rollbackCmd.Flags().Int("turns", 1, "Number of turns to roll back")
	cmd.AddCommand(rollbackCmd)

	archiveCmd := func(name string, archived bool) *cobra.Command {
		c := &cobra.Command{
			Use:   name + " <thread-id>",
			Short: strings.Title(name) + " a thread.",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				socketPath, _ := cmd.Flags().GetString("socket-path")
				client := newSocketRPCClient(socketPath)
				if err := client.ArchiveThread(context.Background(), args[0], archived); err != nil {
					return err
				}
				return writeJSON(archiveThreadResult{OK: true, ThreadID: args[0], Archived: archived})
			},
		}
		c.Flags().String("socket-path", "", "Unix socket path for a running agent_control daemon")
		return c
	}
	cmd.AddCommand(archiveCmd("archive", true))
	cmd.AddCommand(archiveCmd("unarchive", false))

	return cmd
}

func parseMetadataJSON(value string) (contract.ThreadMetadata, error) {
	if strings.TrimSpace(value) == "" {
		return contract.ThreadMetadata{}, nil
	}
	var metadata contract.ThreadMetadata
	if err := json.Unmarshal([]byte(value), &metadata); err != nil {
		return contract.ThreadMetadata{}, err
	}
	return metadata, nil
}
