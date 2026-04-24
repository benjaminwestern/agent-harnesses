package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/benjaminwestern/agentic-control/internal/court"
	"github.com/benjaminwestern/agentic-control/pkg/contract"
	"github.com/spf13/cobra"
)

func newCourtCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "court", Short: "Run Court semantic review workflows."}
	cmd.AddCommand(newCourtInitCmd())
	cmd.AddCommand(newCourtPresetsCmd())
	cmd.AddCommand(newCourtListRunsCmd())
	cmd.AddCommand(newCourtRunCmd())
	cmd.AddCommand(newCourtStatusCmd())
	cmd.AddCommand(newCourtMonitorCmd())
	cmd.AddCommand(newCourtTraceCmd())
	cmd.AddCommand(newCourtVerdictCmd())
	cmd.AddCommand(newCourtPromoteCmd())
	cmd.AddCommand(newCourtContinueCmd())
	cmd.AddCommand(newCourtRequestsCmd())
	cmd.AddCommand(newCourtRespondCmd())
	return cmd
}

func newCourtInitCmd() *cobra.Command {
	var workspace, scope, backend, model string
	var force, dryRun bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize Court defaults in a workspace or global scope.",
		RunE: func(cmd *cobra.Command, args []string) error {
			setupScope := court.SetupScopeProject
			if strings.EqualFold(scope, string(court.SetupScopeGlobal)) {
				setupScope = court.SetupScopeGlobal
			}
			result, err := court.InitDefaults(court.InitDefaultsRequest{Scope: setupScope, Workspace: workspace, Backend: backend, Model: model, Force: force, DryRun: dryRun})
			if err != nil {
				return err
			}
			return writeJSON(result)
		},
	}
	cmd.Flags().StringVar(&workspace, "workspace", ".", "Workspace to initialize")
	cmd.Flags().StringVar(&scope, "scope", "project", "Initialization scope: project or global")
	cmd.Flags().StringVar(&backend, "backend", "opencode", "Default backend")
	cmd.Flags().StringVar(&model, "model", "", "Default model override")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite existing files")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview changes without writing")
	return cmd
}

func newCourtPresetsCmd() *cobra.Command {
	var socketPath, workspace string
	cmd := &cobra.Command{
		Use:   "presets",
		Short: "List available Court presets for a workspace.",
		RunE: func(cmd *cobra.Command, args []string) error {
			engine, err := newCourtEngine(socketPath)
			if err != nil {
				return err
			}
			defer func() { _ = engine.Close() }()
			presets, err := engine.ListAvailablePresets(workspace)
			if err != nil {
				return err
			}
			return writeJSON(presets)
		},
	}
	cmd.Flags().StringVar(&socketPath, "socket-path", "", "Optional daemon socket path")
	cmd.Flags().StringVar(&workspace, "workspace", ".", "Workspace to inspect")
	return cmd
}

func newCourtListRunsCmd() *cobra.Command {
	var socketPath string
	cmd := &cobra.Command{
		Use:   "list-runs",
		Short: "List Court runs.",
		RunE: func(cmd *cobra.Command, args []string) error {
			engine, err := newCourtEngine(socketPath)
			if err != nil {
				return err
			}
			defer func() { _ = engine.Close() }()
			runs, err := engine.ListRuns(context.Background())
			if err != nil {
				return err
			}
			return writeJSON(runs)
		},
	}
	cmd.Flags().StringVar(&socketPath, "socket-path", "", "Optional daemon socket path")
	return cmd
}

func newCourtRunCmd() *cobra.Command {
	var socketPath, task, preset, workspace, workflow, delegationScope, backend, model, selectionJSON string
	var reasoningEffort, thinkingLevel string
	var thinkingBudget int
	var watch bool
	selectionFlags := selectionFlags{}
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Start a Court review run.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(task) == "" {
				return fmt.Errorf("--task is required")
			}
			engine, err := newCourtEngine(socketPath)
			if err != nil {
				return err
			}
			defer func() { _ = engine.Close() }()
			options := court.StartRunOptions{Task: task, Preset: preset, Workflow: workflow, DelegationScope: delegationScope, Backend: backend, Workspace: workspace, Model: model, ModelOptions: courtModelOptions(reasoningEffort, thinkingLevel, thinkingBudget), Selection: firstNonNilSelection(parseOptionalCourtSelectionJSON(selectionJSON), selectionFlags.build())}
			run, err := engine.StartRunWithOptions(context.Background(), options)
			if err != nil {
				return err
			}
			if err := writeJSON(run); err != nil {
				return err
			}
			if !watch {
				return nil
			}
			return engine.WatchRun(context.Background(), run.ID, court.WatchOptions{StopOnTerminal: true, PollInterval: time.Second, OnUpdate: func(update court.WatchUpdate) error { return writeJSON(update) }})
		},
	}
	cmd.Flags().StringVar(&socketPath, "socket-path", "", "Optional daemon socket path")
	cmd.Flags().StringVar(&task, "task", "", "Task or prompt to run")
	cmd.Flags().StringVar(&preset, "preset", "review", "Preset to use")
	cmd.Flags().StringVar(&workspace, "workspace", ".", "Workspace to review")
	cmd.Flags().StringVar(&workflow, "workflow", "", "Workflow override")
	cmd.Flags().StringVar(&delegationScope, "delegation-scope", "", "Delegation scope override")
	cmd.Flags().StringVar(&backend, "backend", "opencode", "Backend override")
	cmd.Flags().StringVar(&model, "model", "", "Model override")
	cmd.Flags().StringVar(&selectionJSON, "selection-json", "", "Typed model selection JSON")
	cmd.Flags().StringVar(&reasoningEffort, "reasoning-effort", "", "Advanced model option")
	cmd.Flags().StringVar(&thinkingLevel, "thinking-level", "", "Advanced model option")
	cmd.Flags().IntVar(&thinkingBudget, "thinking-budget", 0, "Advanced model option")
	cmd.Flags().BoolVar(&watch, "watch", false, "Watch the run until terminal")
	selectionFlags = bindSelectionFlagsCobra(cmd, "", "court run")
	return cmd
}

func newCourtStatusCmd() *cobra.Command {
	var socketPath string
	cmd := &cobra.Command{Use: "status <run-id>", Short: "Show Court run status and usage.", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		engine, err := newCourtEngine(socketPath)
		if err != nil {
			return err
		}
		defer func() { _ = engine.Close() }()
		status, err := engine.RunStatus(context.Background(), args[0])
		if err != nil {
			return err
		}
		usage, err := engine.UsageSummary(context.Background(), args[0])
		if err != nil {
			return err
		}
		return writeJSON(courtStatusResult{Status: status, Usage: usage})
	}}
	cmd.Flags().StringVar(&socketPath, "socket-path", "", "Optional daemon socket path")
	return cmd
}

func newCourtMonitorCmd() *cobra.Command {
	var socketPath string
	var eventLimit int
	var poll time.Duration
	var once bool
	cmd := &cobra.Command{Use: "monitor <run-id>", Short: "Watch a Court run with snapshots and usage.", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		engine, err := newCourtEngine(socketPath)
		if err != nil {
			return err
		}
		defer func() { _ = engine.Close() }()
		printSnapshot := func() error {
			snapshot, err := engine.MonitorSnapshot(context.Background(), args[0], eventLimit)
			if err != nil {
				return err
			}
			usage, err := engine.UsageSummary(context.Background(), args[0])
			if err != nil {
				return err
			}
			return writeJSON(courtMonitorResult{Snapshot: snapshot, Usage: usage})
		}
		if err := printSnapshot(); err != nil {
			return err
		}
		if once {
			return nil
		}
		return engine.WatchRun(context.Background(), args[0], court.WatchOptions{StopOnTerminal: true, PollInterval: poll, OnUpdate: func(update court.WatchUpdate) error {
			if update.TerminalRun == nil && update.Event == nil {
				return nil
			}
			return printSnapshot()
		}})
	}}
	cmd.Flags().StringVar(&socketPath, "socket-path", "", "Optional daemon socket path")
	cmd.Flags().IntVar(&eventLimit, "events", 16, "Recent events to include in each snapshot")
	cmd.Flags().DurationVar(&poll, "poll", time.Second, "Polling interval while watching")
	cmd.Flags().BoolVar(&once, "once", false, "Return a single snapshot instead of watching")
	return cmd
}

func newCourtTraceCmd() *cobra.Command {
	var socketPath string
	cmd := &cobra.Command{Use: "trace <run-id>", Short: "Show the full Court trace and usage summary.", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		engine, err := newCourtEngine(socketPath)
		if err != nil {
			return err
		}
		defer func() { _ = engine.Close() }()
		trace, err := engine.TraceRun(context.Background(), args[0])
		if err != nil {
			return err
		}
		return writeJSON(courtTraceResult{Trace: trace, Usage: court.UsageSummaryFromTrace(trace)})
	}}
	cmd.Flags().StringVar(&socketPath, "socket-path", "", "Optional daemon socket path")
	return cmd
}

func newCourtVerdictCmd() *cobra.Command {
	var socketPath string
	cmd := &cobra.Command{Use: "verdict <run-id>", Short: "Print the final Court verdict text.", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		engine, err := newCourtEngine(socketPath)
		if err != nil {
			return err
		}
		defer func() { _ = engine.Close() }()
		verdict, err := engine.CompletedVerdict(context.Background(), args[0])
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(os.Stdout, "%s\n", verdict)
		return err
	}}
	cmd.Flags().StringVar(&socketPath, "socket-path", "", "Optional daemon socket path")
	return cmd
}

func newCourtPromoteCmd() *cobra.Command {
	var socketPath, workerID string
	var best bool
	cmd := &cobra.Command{Use: "promote <run-id>", Short: "Promote a Court worker thread as the new base.", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		engine, err := newCourtEngine(socketPath)
		if err != nil {
			return err
		}
		defer func() { _ = engine.Close() }()
		if best {
			workerID, err = engine.BestPromotableWorkerID(context.Background(), args[0])
			if err != nil {
				return err
			}
		} else if strings.TrimSpace(workerID) == "" {
			return fmt.Errorf("--worker-id is required unless --best is set")
		}
		result, err := engine.PromoteWorkerThread(context.Background(), args[0], strings.TrimSpace(workerID))
		if err != nil {
			return err
		}
		return writeJSON(result)
	}}
	cmd.Flags().StringVar(&socketPath, "socket-path", "", "Optional daemon socket path")
	cmd.Flags().StringVar(&workerID, "worker-id", "", "Worker id to promote as the new base thread")
	cmd.Flags().BoolVar(&best, "best", false, "Promote the best completed worker only when Court can determine a clear semantic winner; otherwise this command fails and asks for --worker-id")
	return cmd
}

func newCourtContinueCmd() *cobra.Command {
	var socketPath, workerID, cwd, model, selectionJSON, reasoningEffort, thinkingLevel, text string
	var thinkingBudget int
	var best bool
	selectionFlags := selectionFlags{}
	cmd := &cobra.Command{Use: "continue <run-id>", Short: "Promote a Court worker thread and continue from it immediately.", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		engine, err := newCourtEngine(socketPath)
		if err != nil {
			return err
		}
		defer func() { _ = engine.Close() }()
		if best {
			workerID, err = engine.BestPromotableWorkerID(context.Background(), args[0])
			if err != nil {
				return err
			}
		} else if strings.TrimSpace(workerID) == "" {
			return fmt.Errorf("--worker-id is required unless --best is set")
		}
		promotion, err := engine.PromoteWorkerThread(context.Background(), args[0], strings.TrimSpace(workerID))
		if err != nil {
			return err
		}
		client := newSocketRPCClient(socketPath)
		selection := firstNonNilSelection(parseOptionalCourtSelectionJSON(selectionJSON), selectionFlags.build())
		resume, err := resumeThread(context.Background(), client, &promotion.Thread, resumeOptions{CWD: cwd, Model: model, ReasoningEffort: reasoningEffort, ThinkingLevel: thinkingLevel, ThinkingBudget: thinkingBudget, Selection: selection, Text: text})
		if err != nil {
			return err
		}
		return writeJSON(courtContinueResult{Promotion: promotion, Resume: &resume})
	}}
	cmd.Flags().StringVar(&socketPath, "socket-path", "", "Unix socket path for a running agent_control daemon")
	cmd.Flags().StringVar(&workerID, "worker-id", "", "Worker id to promote as the new base thread")
	cmd.Flags().BoolVar(&best, "best", false, "Promote and continue only when Court can determine a clear semantic winner; otherwise this command fails and asks for --worker-id")
	cmd.Flags().StringVar(&cwd, "cwd", "", "Optional working directory override")
	cmd.Flags().StringVar(&model, "model", "", "Optional model override")
	cmd.Flags().StringVar(&selectionJSON, "selection-json", "", "Typed model selection JSON")
	cmd.Flags().StringVar(&reasoningEffort, "reasoning-effort", "", "Advanced model option")
	cmd.Flags().StringVar(&thinkingLevel, "thinking-level", "", "Advanced model option")
	cmd.Flags().IntVar(&thinkingBudget, "thinking-budget", 0, "Advanced model option")
	cmd.Flags().StringVar(&text, "text", "", "Optional message to send immediately after resuming the promoted thread")
	selectionFlags = bindSelectionFlagsCobra(cmd, "", "continue")
	return cmd
}

func newCourtRequestsCmd() *cobra.Command {
	var socketPath, status string
	cmd := &cobra.Command{Use: "requests <run-id>", Short: "List Court runtime requests.", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		engine, err := newCourtEngine(socketPath)
		if err != nil {
			return err
		}
		defer func() { _ = engine.Close() }()
		requests, err := engine.ListRuntimeRequests(context.Background(), args[0], court.RuntimeRequestStatus(status))
		if err != nil {
			return err
		}
		return writeJSON(requests)
	}}
	cmd.Flags().StringVar(&socketPath, "socket-path", "", "Optional daemon socket path")
	cmd.Flags().StringVar(&status, "status", string(court.RuntimeRequestOpen), "Request status filter")
	return cmd
}

func newCourtRespondCmd() *cobra.Command {
	var socketPath, action, text, optionID, answersJSON string
	cmd := &cobra.Command{Use: "respond <request-id>", Short: "Respond to a pending Court runtime request.", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		requestID, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid request id %q: %w", args[0], err)
		}
		answers, err := court.RuntimeRequestAnswersFromJSON(answersJSON)
		if err != nil {
			return err
		}
		engine, err := newCourtEngine(socketPath)
		if err != nil {
			return err
		}
		defer func() { _ = engine.Close() }()
		result, err := engine.RespondToRuntimeRequest(context.Background(), requestID, court.RuntimeRequestResponse{Action: strings.TrimSpace(action), Text: text, OptionID: optionID, Answers: answers})
		if err != nil {
			return err
		}
		return writeJSON(result)
	}}
	cmd.Flags().StringVar(&socketPath, "socket-path", "", "Optional daemon socket path")
	cmd.Flags().StringVar(&action, "action", "", "Response action: allow, deny, submit, cancel, choose")
	cmd.Flags().StringVar(&text, "text", "", "Response text")
	cmd.Flags().StringVar(&optionID, "option-id", "", "Selected option id")
	cmd.Flags().StringVar(&answersJSON, "answers-json", "", "Structured answers JSON")
	return cmd
}

func newCourtEngine(socketPath string) (*court.Engine, error) {
	options := court.EngineOptionsFromEnvironment()
	if strings.TrimSpace(socketPath) != "" {
		_ = os.Setenv("AGENTIC_CONTROL_SOCKET_PATH", strings.TrimSpace(socketPath))
		options.ControlPlane = newSocketRPCClient(socketPath)
	} else if envSocket := strings.TrimSpace(os.Getenv("AGENTIC_CONTROL_SOCKET_PATH")); envSocket != "" {
		options.ControlPlane = newSocketRPCClient(envSocket)
	}
	return court.NewEngine(options)
}

func courtModelOptions(reasoningEffort string, thinkingLevel string, thinkingBudget int) court.RuntimeModelOptions {
	options := court.RuntimeModelOptions{
		ReasoningEffort: strings.TrimSpace(reasoningEffort),
		ThinkingLevel:   strings.TrimSpace(thinkingLevel),
	}
	if thinkingBudget > 0 {
		options.ThinkingBudget = &thinkingBudget
	}
	return options
}

func writeJSON(value any) error {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(os.Stdout, "%s\n", encoded)
	return err
}

func parseOptionalCourtSelectionJSON(value string) *contract.ModelSelection {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	selection, err := parseSelectionJSON(value)
	if err != nil {
		return nil
	}
	return &selection
}
