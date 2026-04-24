package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	api "github.com/benjaminwestern/agentic-control/pkg/controlplane"
	"github.com/spf13/cobra"
)

type listedModel struct {
	Runtime  string `json:"runtime"`
	Provider string `json:"provider,omitempty"`
	ID       string `json:"id"`
	Label    string `json:"label,omitempty"`
	Default  bool   `json:"default,omitempty"`
	Custom   bool   `json:"custom,omitempty"`
}

func newModelsCmd() *cobra.Command {
	var socketPath string
	var runtime string
	var provider string
	cmd := &cobra.Command{
		Use:   "models",
		Short: "List available models via the unified registry.",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newSocketRPCClient(socketPath)
			registry, err := client.Models(context.Background())
			if err != nil {
				return err
			}
			rows := make([]listedModel, 0)
			for _, item := range registry.Backends {
				if strings.TrimSpace(runtime) != "" && item.Backend != strings.TrimSpace(runtime) {
					continue
				}
				for _, model := range item.Models {
					if strings.TrimSpace(provider) != "" && !strings.EqualFold(model.Provider, strings.TrimSpace(provider)) {
						continue
					}
					rows = append(rows, listedModel{Runtime: item.Backend, Provider: model.Provider, ID: model.ID, Label: model.Label, Default: model.Default, Custom: model.Custom})
				}
			}
			sort.Slice(rows, func(i, j int) bool {
				if rows[i].Runtime != rows[j].Runtime {
					return rows[i].Runtime < rows[j].Runtime
				}
				if rows[i].Provider != rows[j].Provider {
					return rows[i].Provider < rows[j].Provider
				}
				if rows[i].Default != rows[j].Default {
					return rows[i].Default
				}
				return rows[i].ID < rows[j].ID
			})
			return writeJSON(rows)
		},
	}
	cmd.Flags().StringVar(&socketPath, "socket-path", "", "Unix socket path for a running agent_control daemon")
	cmd.Flags().StringVar(&runtime, "runtime", "", "Filter by runtime/backend")
	cmd.Flags().StringVar(&provider, "provider", "", "Filter by upstream provider")

	var normSocketPath string
	selectionFlags := selectionFlags{}
	normalizeCmd := &cobra.Command{
		Use:   "normalize",
		Short: "Normalize a typed model selection against the shared registry.",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newSocketRPCClient(normSocketPath)
			registry, err := client.Models(context.Background())
			if err != nil {
				return err
			}
			selection := selectionFlags.build()
			if selection == nil {
				return fmt.Errorf("a typed selection is required; set --provider and --model-selection")
			}
			normalized := api.NormalizeModelSelection(registry, *selection)
			target := api.RuntimeTargetFromSelection(normalized)
			validation := api.ValidateSessionTargetWithRegistry(registry, target)
			return writeJSON(modelSelectionNormalizationResult{Selection: normalized, Target: validation.Target, Issues: validation.Issues})
		},
	}
	normalizeCmd.Flags().StringVar(&normSocketPath, "socket-path", "", "Unix socket path for a running agent_control daemon")
	selectionFlags = bindSelectionFlagsCobra(normalizeCmd, "", "normalize")
	cmd.AddCommand(normalizeCmd)
	return cmd
}
