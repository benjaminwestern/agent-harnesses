package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/benjaminwestern/agentic-control/internal/orchestration"
	"github.com/benjaminwestern/agentic-control/pkg/contract"
	api "github.com/benjaminwestern/agentic-control/pkg/controlplane"
	"github.com/benjaminwestern/agentic-control/pkg/controlplane/embedded"
	"github.com/spf13/cobra"
)

type repeatedTargets []string

func (r *repeatedTargets) String() string { return strings.Join(*r, ",") }

func (r *repeatedTargets) Set(value string) error {
	*r = append(*r, value)
	return nil
}

type repeatedSelections []contract.ModelSelection

func (r *repeatedSelections) String() string { return "" }

func (r *repeatedSelections) Set(value string) error {
	selection, err := parseSelectionJSON(value)
	if err != nil {
		return err
	}
	*r = append(*r, selection)
	return nil
}

func newOrchestrateCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "orchestrate", Short: "Run generic orchestration flows."}
	cmd.AddCommand(newReductionCmd("fanout", "Run the same prompt across one or more targets.", "", true))
	cmd.AddCommand(newReductionCmd("compare", "Compare multiple attempts or targets.", orchestration.ReductionModeCompare, false))
	cmd.AddCommand(newReductionCmd("summarize", "Summarize multiple attempts or targets.", orchestration.ReductionModeSummarize, false))
	cmd.AddCommand(newReductionCmd("best-of-n", "Pick the best result across multiple attempts or targets.", orchestration.ReductionModeBestOfN, false))
	return cmd
}

func newReductionCmd(use string, short string, mode orchestration.ReductionMode, fanoutOnly bool) *cobra.Command {
	var socketPath string
	var task string
	var repeat int
	var keepSessions bool
	var reasoningEffort string
	var thinkingLevel string
	var thinkingBudget int
	var targets []string
	var selections []string
	targetSelectionFlags := selectionFlags{}
	var reduceBackend string
	var reduceModel string
	var reduceSelectionJSON string
	var reduceReasoningEffort string
	var reduceThinkingLevel string
	var reduceThinkingBudget int
	reducerSelectionFlags := selectionFlags{}

	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(task) == "" {
				return fmt.Errorf("--task is required")
			}
			selectionValues := make([]contract.ModelSelection, 0, len(selections)+1)
			for _, raw := range selections {
				selection, err := parseSelectionJSON(raw)
				if err != nil {
					return err
				}
				selectionValues = append(selectionValues, selection)
			}
			if selection := targetSelectionFlags.build(); selection != nil {
				selectionValues = append(selectionValues, *selection)
			}
			controller := fanoutController(socketPath)
			fanoutOptions := orchestration.FanoutOptions{
				Prompt:       task,
				Targets:      parseFanoutTargets(targets, selectionValues),
				Repeat:       repeat,
				ModelOptions: buildSharedModelOptions(reasoningEffort, thinkingLevel, thinkingBudget),
				KeepSessions: keepSessions,
				Metadata: map[string]any{
					"workflow": "fanout",
				},
			}
			if fanoutOnly {
				result, err := orchestration.RunFanout(context.Background(), controller, fanoutOptions)
				if err != nil {
					return err
				}
				return writeJSON(result)
			}
			reducerSelection := firstNonNilSelection(parseOptionalSelectionJSON(reduceSelectionJSON), reducerSelectionFlags.build())
			result, err := orchestration.RunReviewedFanout(context.Background(), controller, orchestration.ReviewedFanoutOptions{
				Mode:   mode,
				Fanout: fanoutOptions,
				ReductionTarget: orchestration.FanoutTarget{
					Backend:   reduceBackend,
					Model:     reduceModel,
					Options:   buildSharedModelOptions(reduceReasoningEffort, reduceThinkingLevel, reduceThinkingBudget),
					Selection: reducerSelection,
				},
			})
			if err != nil {
				return err
			}
			return writeJSON(result)
		},
	}
	cmd.Flags().StringVar(&socketPath, "socket-path", "", "Optional daemon socket path; use this to preserve sessions for later rehydration")
	cmd.Flags().StringVar(&task, "task", "", "Prompt to run")
	cmd.Flags().IntVar(&repeat, "repeat", 1, "Repeat each target this many times")
	cmd.Flags().BoolVar(&keepSessions, "keep-sessions", false, "Keep sessions alive after completion")
	cmd.Flags().StringVar(&reasoningEffort, "reasoning-effort", "", "Advanced model option for all targets")
	cmd.Flags().StringVar(&thinkingLevel, "thinking-level", "", "Advanced model option for all targets")
	cmd.Flags().IntVar(&thinkingBudget, "thinking-budget", 0, "Advanced model option for all targets")
	cmd.Flags().StringArrayVar(&targets, "target", nil, "Target in the form backend=model; repeat to compare explicit targets")
	cmd.Flags().StringArrayVar(&selections, "selection-json", nil, "Typed model selection JSON; repeat to compare explicit typed selections")
	targetSelectionFlags = bindSelectionFlagsCobra(cmd, "", use)
	if !fanoutOnly {
		cmd.Flags().StringVar(&reduceBackend, "reduce-backend", "", "Reducer backend override")
		cmd.Flags().StringVar(&reduceModel, "reduce-model", "", "Reducer model override")
		cmd.Flags().StringVar(&reduceSelectionJSON, "reduce-selection-json", "", "Typed reducer selection JSON")
		cmd.Flags().StringVar(&reduceReasoningEffort, "reduce-reasoning-effort", "", "Advanced model option for the reducer")
		cmd.Flags().StringVar(&reduceThinkingLevel, "reduce-thinking-level", "", "Advanced model option for the reducer")
		cmd.Flags().IntVar(&reduceThinkingBudget, "reduce-thinking-budget", 0, "Advanced model option for the reducer")
		reducerSelectionFlags = bindSelectionFlagsCobra(cmd, "reduce-", "reducer")
	}
	return cmd
}

func parseFanoutTargets(values []string, selections []contract.ModelSelection) []orchestration.FanoutTarget {
	targets := make([]orchestration.FanoutTarget, 0, len(values)+len(selections))
	for _, value := range values {
		backend := strings.TrimSpace(value)
		model := ""
		if strings.Contains(value, "=") {
			left, right, _ := strings.Cut(value, "=")
			backend = strings.TrimSpace(left)
			model = strings.TrimSpace(right)
		}
		targets = append(targets, orchestration.FanoutTarget{Backend: backend, Model: model})
	}
	for _, selection := range selections {
		selectionCopy := selection
		targets = append(targets, orchestration.FanoutTarget{Backend: string(selection.Provider), Model: selection.Model, Selection: &selectionCopy})
	}
	return targets
}

func buildSharedModelOptions(reasoningEffort string, thinkingLevel string, thinkingBudget int) api.ModelOptions {
	options := api.ModelOptions{ReasoningEffort: strings.TrimSpace(reasoningEffort), ThinkingLevel: strings.TrimSpace(thinkingLevel)}
	if thinkingBudget > 0 {
		options.ThinkingBudget = &thinkingBudget
	}
	return options
}

func fanoutController(socketPath string) orchestration.FanoutController {
	if strings.TrimSpace(socketPath) != "" {
		return newSocketRPCClient(socketPath)
	}
	return embedded.New()
}

func parseSelectionJSON(value string) (contract.ModelSelection, error) {
	var selection contract.ModelSelection
	if err := json.Unmarshal([]byte(value), &selection); err != nil {
		return contract.ModelSelection{}, err
	}
	return selection, nil
}

func parseOptionalSelectionJSON(value string) *contract.ModelSelection {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	selection, err := parseSelectionJSON(value)
	if err != nil {
		return nil
	}
	return &selection
}

func firstNonNilSelection(values ...*contract.ModelSelection) *contract.ModelSelection {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}
