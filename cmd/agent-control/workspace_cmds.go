package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/benjaminwestern/agentic-control/pkg/contract"
	"github.com/spf13/cobra"
)

func newWorkspaceMemoryCmd() *cobra.Command {
	var socketPath string
	var workspaceID string

	cmd := &cobra.Command{
		Use:   "memory",
		Short: "Manage workspace memory (KV)",
	}
	cmd.PersistentFlags().StringVar(&socketPath, "socket-path", filepath.Join(os.TempDir(), defaultSocketPath), "Unix socket path")
	cmd.PersistentFlags().StringVar(&workspaceID, "workspace", "default", "Workspace ID")

	cmd.AddCommand(&cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a memory key to a value",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newSocketRPCClient(socketPath)
			err := client.SetMemory(context.Background(), contract.MemoryEntry{
				WorkspaceID: workspaceID,
				Key:         args[0],
				Value:       args[1],
			})
			if err != nil {
				return err
			}
			fmt.Println("Memory set.")
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "get <key>",
		Short: "Get a memory value",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newSocketRPCClient(socketPath)
			entry, err := client.GetMemory(context.Background(), workspaceID, args[0])
			if err != nil {
				return err
			}
			fmt.Println(entry.Value)
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "delete <key>",
		Short: "Delete a memory key",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newSocketRPCClient(socketPath)
			err := client.DeleteMemory(context.Background(), workspaceID, args[0])
			if err != nil {
				return err
			}
			fmt.Println("Memory deleted.")
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all memory keys",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newSocketRPCClient(socketPath)
			entries, err := client.ListMemory(context.Background(), workspaceID)
			if err != nil {
				return err
			}
			return writeJSON(entries)
		},
	})

	return cmd
}

func newWorkspaceDocumentsCmd() *cobra.Command {
	var socketPath string
	var workspaceID string

	cmd := &cobra.Command{
		Use:   "documents",
		Short: "Manage workspace documents",
	}
	cmd.PersistentFlags().StringVar(&socketPath, "socket-path", filepath.Join(os.TempDir(), defaultSocketPath), "Unix socket path")
	cmd.PersistentFlags().StringVar(&workspaceID, "workspace", "default", "Workspace ID")

	cmd.AddCommand(&cobra.Command{
		Use:   "write <name> <content>",
		Short: "Write a document",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newSocketRPCClient(socketPath)
			doc, err := client.WriteDocument(context.Background(), contract.Document{
				WorkspaceID: workspaceID,
				Name:        args[0],
				Content:     args[1],
			})
			if err != nil {
				return err
			}
			fmt.Printf("Document written (ID: %s, Revision: %d)\n", doc.ID, doc.Revision)
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "append <id> <content>",
		Short: "Append to a document",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newSocketRPCClient(socketPath)
			err := client.AppendDocument(context.Background(), workspaceID, args[0], args[1])
			if err != nil {
				return err
			}
			fmt.Println("Document appended.")
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "add-metadata <id> <key> <value>",
		Short: "Add metadata to a document",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newSocketRPCClient(socketPath)
			err := client.AddDocumentMetadata(context.Background(), workspaceID, args[0], map[string]any{args[1]: args[2]})
			if err != nil {
				return err
			}
			fmt.Println("Document metadata added.")
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "rename <id> <name>",
		Short: "Rename a document",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newSocketRPCClient(socketPath)
			err := client.RenameDocument(context.Background(), workspaceID, args[0], args[1])
			if err != nil {
				return err
			}
			fmt.Println("Document renamed.")
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "archive <id> <archived_bool>",
		Short: "Archive or unarchive a document",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newSocketRPCClient(socketPath)
			archived := args[1] == "true"
			err := client.ArchiveDocument(context.Background(), workspaceID, args[0], archived)
			if err != nil {
				return err
			}
			fmt.Println("Document archive status updated.")
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "clear <id>",
		Short: "Clear a document's content",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newSocketRPCClient(socketPath)
			err := client.ClearDocument(context.Background(), workspaceID, args[0])
			if err != nil {
				return err
			}
			fmt.Println("Document cleared.")
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "get <id>",
		Short: "Get a document",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newSocketRPCClient(socketPath)
			doc, err := client.GetDocument(context.Background(), workspaceID, args[0])
			if err != nil {
				return err
			}
			return writeJSON(doc)
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a document",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newSocketRPCClient(socketPath)
			err := client.DeleteDocument(context.Background(), workspaceID, args[0])
			if err != nil {
				return err
			}
			fmt.Println("Document deleted.")
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all documents",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newSocketRPCClient(socketPath)
			docs, err := client.ListDocuments(context.Background(), workspaceID)
			if err != nil {
				return err
			}
			return writeJSON(docs)
		},
	})

	return cmd
}

func newWorkspaceTasksCmd() *cobra.Command {
	var socketPath string
	var workspaceID string

	cmd := &cobra.Command{
		Use:   "tasks",
		Short: "Manage workspace tasks",
	}
	cmd.PersistentFlags().StringVar(&socketPath, "socket-path", filepath.Join(os.TempDir(), defaultSocketPath), "Unix socket path")
	cmd.PersistentFlags().StringVar(&workspaceID, "workspace", "default", "Workspace ID")

	cmd.AddCommand(&cobra.Command{
		Use:   "create <title> [body]",
		Short: "Create a task",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newSocketRPCClient(socketPath)
			body := ""
			if len(args) > 1 {
				body = args[1]
			}
			task, err := client.CreateTask(context.Background(), contract.Task{
				WorkspaceID: workspaceID,
				Title:       args[0],
				Body:        body,
			})
			if err != nil {
				return err
			}
			fmt.Printf("Task created (ID: %s)\n", task.ID)
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "add-metadata <id> <key> <value>",
		Short: "Add metadata to a task",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newSocketRPCClient(socketPath)
			err := client.AddTaskMetadata(context.Background(), workspaceID, args[0], map[string]any{args[1]: args[2]})
			if err != nil {
				return err
			}
			fmt.Println("Task metadata added.")
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "add-tag <id> <tag>",
		Short: "Add a tag to a task",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newSocketRPCClient(socketPath)
			err := client.AddTaskTag(context.Background(), workspaceID, args[0], args[1])
			if err != nil {
				return err
			}
			fmt.Println("Task tag added.")
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "remove-tag <id> <tag>",
		Short: "Remove a tag from a task",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newSocketRPCClient(socketPath)
			err := client.RemoveTaskTag(context.Background(), workspaceID, args[0], args[1])
			if err != nil {
				return err
			}
			fmt.Println("Task tag removed.")
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "add-blocker <id> <blocker_id>",
		Short: "Add a blocker to a task",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newSocketRPCClient(socketPath)
			err := client.AddTaskBlocker(context.Background(), workspaceID, args[0], args[1])
			if err != nil {
				return err
			}
			fmt.Println("Task blocker added.")
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "remove-blocker <id> <blocker_id>",
		Short: "Remove a blocker from a task",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newSocketRPCClient(socketPath)
			err := client.RemoveTaskBlocker(context.Background(), workspaceID, args[0], args[1])
			if err != nil {
				return err
			}
			fmt.Println("Task blocker removed.")
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "lock <id> <actor_id>",
		Short: "Lock a task",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newSocketRPCClient(socketPath)
			err := client.LockTask(context.Background(), workspaceID, args[0], args[1])
			if err != nil {
				return err
			}
			fmt.Println("Task locked.")
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "unlock <id> <actor_id>",
		Short: "Unlock a task",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newSocketRPCClient(socketPath)
			err := client.UnlockTask(context.Background(), workspaceID, args[0], args[1])
			if err != nil {
				return err
			}
			fmt.Println("Task unlocked.")
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "get <id>",
		Short: "Get a task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newSocketRPCClient(socketPath)
			task, err := client.GetTask(context.Background(), workspaceID, args[0])
			if err != nil {
				return err
			}
			return writeJSON(task)
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newSocketRPCClient(socketPath)
			err := client.DeleteTask(context.Background(), workspaceID, args[0])
			if err != nil {
				return err
			}
			fmt.Println("Task deleted.")
			return nil
		},
	})

	commentsCmd := &cobra.Command{
		Use:   "comments",
		Short: "Manage task comments",
	}
	commentsCmd.AddCommand(&cobra.Command{
		Use:   "create <task_id> <author> <body>",
		Short: "Create a comment",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newSocketRPCClient(socketPath)
			comment, err := client.CreateTaskComment(context.Background(), contract.TaskComment{
				TaskID: args[0],
				Author: args[1],
				Body:   args[2],
			})
			if err != nil {
				return err
			}
			fmt.Printf("Comment created (ID: %s)\n", comment.ID)
			return nil
		},
	})
	commentsCmd.AddCommand(&cobra.Command{
		Use:   "list <task_id>",
		Short: "List comments",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newSocketRPCClient(socketPath)
			comments, err := client.ListTaskComments(context.Background(), args[0])
			if err != nil {
				return err
			}
			return writeJSON(comments)
		},
	})
	commentsCmd.AddCommand(&cobra.Command{
		Use:   "update <id> <body>",
		Short: "Update a comment",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newSocketRPCClient(socketPath)
			err := client.UpdateTaskComment(context.Background(), args[0], args[1])
			if err != nil {
				return err
			}
			fmt.Println("Comment updated.")
			return nil
		},
	})
	commentsCmd.AddCommand(&cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a comment",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newSocketRPCClient(socketPath)
			err := client.DeleteTaskComment(context.Background(), args[0])
			if err != nil {
				return err
			}
			fmt.Println("Comment deleted.")
			return nil
		},
	})
	cmd.AddCommand(commentsCmd)

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all tasks",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newSocketRPCClient(socketPath)
			tasks, err := client.ListTasks(context.Background(), workspaceID)
			if err != nil {
				return err
			}
			return writeJSON(tasks)
		},
	})

	return cmd
}

func newWorkspaceWakeupsCmd() *cobra.Command {
	var socketPath string
	var workspaceID string

	cmd := &cobra.Command{
		Use:   "wakeups",
		Short: "Manage workspace wakeups",
	}
	cmd.PersistentFlags().StringVar(&socketPath, "socket-path", filepath.Join(os.TempDir(), defaultSocketPath), "Unix socket path")
	cmd.PersistentFlags().StringVar(&workspaceID, "workspace", "default", "Workspace ID")

	cmd.AddCommand(&cobra.Command{
		Use:   "set <owner_id> <due_at_ms> <body>",
		Short: "Set a wakeup",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			dueAt, err := strconv.ParseInt(args[1], 10, 64)
			if err != nil {
				return fmt.Errorf("parse due_at_ms: %w", err)
			}
			client := newSocketRPCClient(socketPath)
			err = client.SetWakeup(context.Background(), contract.Wakeup{
				WorkspaceID: workspaceID,
				OwnerID:     args[0],
				DueAtMS:     dueAt,
				Body:        args[2],
			})
			if err != nil {
				return err
			}
			fmt.Println("Wakeup set.")
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "get <id>",
		Short: "Get a wakeup",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newSocketRPCClient(socketPath)
			wakeup, err := client.GetWakeup(context.Background(), workspaceID, args[0])
			if err != nil {
				return err
			}
			return writeJSON(wakeup)
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "cancel <id>",
		Short: "Cancel a wakeup",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newSocketRPCClient(socketPath)
			err := client.CancelWakeup(context.Background(), workspaceID, args[0])
			if err != nil {
				return err
			}
			fmt.Println("Wakeup cancelled.")
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "pause <id>",
		Short: "Pause a wakeup",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newSocketRPCClient(socketPath)
			err := client.PauseWakeup(context.Background(), workspaceID, args[0])
			if err != nil {
				return err
			}
			fmt.Println("Wakeup paused.")
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "resume <id>",
		Short: "Resume a wakeup",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newSocketRPCClient(socketPath)
			err := client.ResumeWakeup(context.Background(), workspaceID, args[0])
			if err != nil {
				return err
			}
			fmt.Println("Wakeup resumed.")
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "reset <id> <due_at_ms>",
		Short: "Reset a wakeup",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			dueAt, err := strconv.ParseInt(args[1], 10, 64)
			if err != nil {
				return fmt.Errorf("parse due_at_ms: %w", err)
			}
			client := newSocketRPCClient(socketPath)
			err = client.ResetWakeup(context.Background(), workspaceID, args[0], dueAt)
			if err != nil {
				return err
			}
			fmt.Println("Wakeup reset.")
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List pending wakeups",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newSocketRPCClient(socketPath)
			wakeups, err := client.ListPendingWakeups(context.Background(), workspaceID)
			if err != nil {
				return err
			}
			return writeJSON(wakeups)
		},
	})

	return cmd
}

func newWorkspaceLeasesCmd() *cobra.Command {
	var socketPath string
	var workspaceID string

	cmd := &cobra.Command{
		Use:   "leases",
		Short: "Manage workspace leases (locks)",
	}
	cmd.PersistentFlags().StringVar(&socketPath, "socket-path", filepath.Join(os.TempDir(), defaultSocketPath), "Unix socket path")
	cmd.PersistentFlags().StringVar(&workspaceID, "workspace", "default", "Workspace ID")

	cmd.AddCommand(&cobra.Command{
		Use:   "acquire <lock_key> <owner_id> <expires_at_ms>",
		Short: "Acquire a lease",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			expiresAt, err := strconv.ParseInt(args[2], 10, 64)
			if err != nil {
				return fmt.Errorf("parse expires_at_ms: %w", err)
			}
			client := newSocketRPCClient(socketPath)
			acquired, err := client.AcquireLease(context.Background(), contract.Lease{
				WorkspaceID: workspaceID,
				LockKey:     args[0],
				OwnerID:     args[1],
				ExpiresAtMS: expiresAt,
			})
			if err != nil {
				return err
			}
			fmt.Printf("Acquired: %v\n", acquired)
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "release <lock_key> <owner_id>",
		Short: "Release a lease",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newSocketRPCClient(socketPath)
			err := client.ReleaseLease(context.Background(), workspaceID, args[0], args[1])
			if err != nil {
				return err
			}
			fmt.Println("Lease released.")
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "get <lock_key>",
		Short: "Get a lease",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newSocketRPCClient(socketPath)
			lease, err := client.GetLease(context.Background(), workspaceID, args[0])
			if err != nil {
				return err
			}
			return writeJSON(lease)
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "reset <lock_key>",
		Short: "Reset a lease",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newSocketRPCClient(socketPath)
			err := client.ResetLease(context.Background(), workspaceID, args[0])
			if err != nil {
				return err
			}
			fmt.Println("Lease reset.")
			return nil
		},
	})

	return cmd
}
