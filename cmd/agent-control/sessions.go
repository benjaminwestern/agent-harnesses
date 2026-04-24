package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/benjaminwestern/agentic-control/pkg/contract"
	api "github.com/benjaminwestern/agentic-control/pkg/controlplane"
	"github.com/spf13/cobra"
)

func newSessionsCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "sessions", Short: "Inspect and resume tracked sessions."}
	resumeSelectionFlags := selectionFlags{}
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List tracked sessions/threads.",
		RunE: func(cmd *cobra.Command, args []string) error {
			socketPath, _ := cmd.Flags().GetString("socket-path")
			runtime, _ := cmd.Flags().GetString("runtime")
			client := newSocketRPCClient(socketPath)
			threads, err := client.ListThreads(context.Background(), strings.TrimSpace(runtime), nil)
			if err != nil {
				return err
			}
			return writeJSON(threads)
		},
	}
	listCmd.Flags().String("socket-path", "", "Unix socket path for a running agent_control daemon")
	listCmd.Flags().String("runtime", "", "Runtime/backend filter")
	cmd.AddCommand(listCmd)

	getCmd := &cobra.Command{
		Use:   "get <session-id-or-provider-session-id>",
		Short: "Get one tracked session/thread.",
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

	resumeCmd := &cobra.Command{
		Use:   "resume <session-id-or-provider-session-id>",
		Short: "Resume or continue from a tracked session/thread.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			socketPath, _ := cmd.Flags().GetString("socket-path")
			cwd, _ := cmd.Flags().GetString("cwd")
			model, _ := cmd.Flags().GetString("model")
			selectionJSON, _ := cmd.Flags().GetString("selection-json")
			reasoningEffort, _ := cmd.Flags().GetString("reasoning-effort")
			thinkingLevel, _ := cmd.Flags().GetString("thinking-level")
			thinkingBudget, _ := cmd.Flags().GetInt("thinking-budget")
			text, _ := cmd.Flags().GetString("text")
			client := newSocketRPCClient(socketPath)
			thread, err := client.GetThread(context.Background(), args[0], "")
			if err != nil {
				return err
			}
			selection := firstNonNilSelection(parseOptionalSelectionJSON(selectionJSON), resumeSelectionFlags.build())
			result, err := resumeThread(context.Background(), client, thread, resumeOptions{CWD: cwd, Model: model, ReasoningEffort: reasoningEffort, ThinkingLevel: thinkingLevel, ThinkingBudget: thinkingBudget, Selection: selection, Text: text})
			if err != nil {
				return err
			}
			return writeJSON(result)
		},
	}
	resume := resumeCmd
	resume.Flags().String("socket-path", "", "Unix socket path for a running agent_control daemon")
	resume.Flags().String("cwd", "", "Optional working directory override")
	resume.Flags().String("model", "", "Optional model override")
	resume.Flags().String("selection-json", "", "Typed model selection JSON")
	resume.Flags().String("reasoning-effort", "", "Advanced model option")
	resume.Flags().String("thinking-level", "", "Advanced model option")
	resume.Flags().Int("thinking-budget", 0, "Advanced model option")
	resume.Flags().String("text", "", "Optional message to send immediately after resume")
	resumeSelectionFlags = bindSelectionFlagsCobra(resume, "", "resume")
	cmd.AddCommand(resumeCmd)
	return cmd
}

type resumeOptions struct {
	CWD             string
	Model           string
	ReasoningEffort string
	ThinkingLevel   string
	ThinkingBudget  int
	Selection       *contract.ModelSelection
	Text            string
}

func resumeThread(ctx context.Context, client *socketRPCClient, thread *contract.TrackedThread, opts resumeOptions) (sessionResumeResult, error) {
	tracked := thread.TrackedSession
	var sessionID string
	var resumed *contract.RuntimeSession
	var err error
	overrideModel := firstNonEmptyString(opts.Model, tracked.Session.Model)
	overrideOptions := buildSharedModelOptions(opts.ReasoningEffort, opts.ThinkingLevel, opts.ThinkingBudget)
	if opts.Selection != nil {
		registry, err := client.Models(ctx)
		if err != nil {
			return sessionResumeResult{}, err
		}
		normalized := api.NormalizeModelSelection(registry, *opts.Selection)
		if strings.TrimSpace(string(normalized.Provider)) != "" && string(normalized.Provider) != tracked.Session.Runtime {
			return sessionResumeResult{}, fmt.Errorf("typed selection provider %q does not match resumable runtime %q", normalized.Provider, tracked.Session.Runtime)
		}
		target := api.RuntimeTargetFromSelection(normalized)
		overrideModel = firstNonEmptyString(target.Model, overrideModel)
		overrideOptions = api.MergeModelOptions(target.Options, overrideOptions)
	}
	if tracked.Session.Status == contract.SessionIdle || tracked.Session.Status == contract.SessionRunning || tracked.Session.Status == contract.SessionWaitingApproval || tracked.Session.Status == contract.SessionWaitingUserInput {
		if _, lookupErr := client.GetTrackedSession(ctx, tracked.Session.SessionID, tracked.Session.ProviderSessionID); lookupErr == nil {
			sessionID = tracked.Session.SessionID
		} else if tracked.Session.ProviderSessionID != "" {
			resumed, err = client.ResumeSession(ctx, tracked.Session.Runtime, api.ResumeSessionRequest{
				ProviderSessionID: tracked.Session.ProviderSessionID,
				CWD:               firstNonEmptyString(opts.CWD, tracked.Session.CWD),
				Model:             overrideModel,
				ModelOptions:      overrideOptions,
				Metadata: map[string]any{
					"rehydrated_from_session_id": tracked.Session.SessionID,
				},
			})
			if err != nil {
				return sessionResumeResult{}, err
			}
			sessionID = resumed.SessionID
		} else {
			sessionID = tracked.Session.SessionID
		}
	} else {
		resumed, err = client.ResumeSession(ctx, tracked.Session.Runtime, api.ResumeSessionRequest{
			ProviderSessionID: tracked.Session.ProviderSessionID,
			CWD:               firstNonEmptyString(opts.CWD, tracked.Session.CWD),
			Model:             overrideModel,
			ModelOptions:      overrideOptions,
			Metadata: map[string]any{
				"rehydrated_from_session_id": tracked.Session.SessionID,
			},
		})
		if err != nil {
			return sessionResumeResult{}, err
		}
		sessionID = resumed.SessionID
	}
	result := sessionResumeResult{}
	if resumed == nil {
		result.Session = tracked.Session
		result.Tracked = tracked
		if strings.TrimSpace(opts.Text) != "" {
			event, err := client.SendInput(ctx, api.SendInputRequest{
				SessionID: sessionID,
				Text:      opts.Text,
				Metadata: map[string]any{
					"rehydrated_from_session_id": tracked.Session.SessionID,
				},
			})
			if err != nil {
				return sessionResumeResult{}, err
			}
			result.Event = event
		}
		return result, nil
	}
	result.Session = *resumed
	result.Tracked = tracked
	if strings.TrimSpace(opts.Text) != "" {
		event, err := client.SendInput(ctx, api.SendInputRequest{
			SessionID: sessionID,
			Text:      opts.Text,
			Metadata: map[string]any{
				"rehydrated_from_session_id": tracked.Session.SessionID,
			},
		})
		if err != nil {
			return sessionResumeResult{}, err
		}
		result.Event = event
	}
	return result, nil
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
