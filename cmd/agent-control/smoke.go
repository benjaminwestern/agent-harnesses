package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/benjaminwestern/agentic-control/internal/orchestration"
	"github.com/benjaminwestern/agentic-control/pkg/contract"
	"github.com/spf13/cobra"
)

func newSmokeCmd() *cobra.Command {
	var socketPath string
	var task string
	var targets []string
	var selectionJSON []string
	selectionFlags := selectionFlags{}
	cmd := &cobra.Command{
		Use:   "smoke",
		Short: "Run the live runtime smoke matrix.",
		RunE: func(cmd *cobra.Command, args []string) error {
			selections := make([]contract.ModelSelection, 0, len(selectionJSON)+1)
			for _, raw := range selectionJSON {
				selection, err := parseSelectionJSON(raw)
				if err != nil {
					return err
				}
				selections = append(selections, selection)
			}
			if selection := selectionFlags.build(); selection != nil {
				selections = append(selections, *selection)
			}
			if len(targets) == 0 && len(selections) == 0 {
				targets = []string{"claude=claude-sonnet-4-6", "gemini=gemini-3-flash-preview", "codex=gpt-5.4", "opencode=google/gemini-3-flash-preview", "opencode=openai/gpt-5.4"}
			}
			controller := fanoutController(socketPath)
			result, err := orchestration.RunFanout(context.Background(), controller, orchestration.FanoutOptions{Prompt: task, Targets: parseFanoutTargets(targets, selections), Metadata: map[string]any{"workflow": "smoke"}})
			if err != nil {
				return err
			}
			rows := make([]smokeTargetResult, 0, len(result.Targets))
			allPassed := true
			for _, target := range result.Targets {
				text := strings.TrimSpace(target.Text)
				passed := target.Error == "" && text != ""
				if !passed {
					allPassed = false
				}
				rows = append(rows, smokeTargetResult{Target: target.Target.Label, Backend: target.Target.Backend, Model: target.Target.Model, Passed: passed, Text: text, Error: target.Error, Usage: target.RecordedUsage, CostUSD: target.RecordedCostUSD})
			}
			output := smokeResult{Passed: allPassed, TargetCount: len(rows), Targets: rows, TotalUsage: result.TotalUsage, TotalCostUSD: result.TotalCostUSD}
			if err := writeJSON(output); err != nil {
				return err
			}
			if !allPassed {
				return fmt.Errorf("one or more smoke targets failed")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&socketPath, "socket-path", "", "Optional daemon socket path")
	cmd.Flags().StringVar(&task, "task", "Reply with exactly: smoke-ok", "Prompt used for smoke validation")
	cmd.Flags().StringArrayVar(&targets, "target", nil, "Target in the form backend=model; repeat for multiple checks")
	cmd.Flags().StringArrayVar(&selectionJSON, "selection-json", nil, "Typed model selection JSON; repeat for multiple checks")
	selectionFlags = bindSelectionFlagsCobra(cmd, "", "smoke")
	return cmd
}
