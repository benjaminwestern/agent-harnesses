package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/benjaminwestern/agentic-control/pkg/contract"
	api "github.com/benjaminwestern/agentic-control/pkg/controlplane"
)

func modelSelectionCommand(args []string) error {
	flags := flag.NewFlagSet("models normalize", flag.ExitOnError)
	flags.SetOutput(os.Stdout)
	socketPath := flags.String("socket-path", "", "Unix socket path for a running agent_control daemon")
	provider := flags.String("provider", "", "Provider/runtime kind: codex, claude, gemini, opencode, pi")
	model := flags.String("model", "", "Model id or alias")
	reasoningEffort := flags.String("reasoning-effort", "", "Codex or Claude effort")
	fastMode := flags.Bool("fast-mode", false, "Codex/Claude fast mode")
	thinking := flags.Bool("thinking", false, "Claude thinking toggle")
	contextWindow := flags.String("context-window", "", "Claude context window option")
	thinkingLevel := flags.String("thinking-level", "", "Gemini thinking level")
	thinkingBudget := flags.Int("thinking-budget", 0, "Gemini thinking budget")
	variant := flags.String("variant", "", "OpenCode variant")
	agent := flags.String("agent", "", "OpenCode agent")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*provider) == "" {
		return fmt.Errorf("--provider is required")
	}
	client := newSocketRPCClient(*socketPath)
	registry, err := client.Models(context.Background())
	if err != nil {
		return err
	}
	selection := contract.ModelSelection{
		Provider: contract.ProviderKind(strings.TrimSpace(*provider)),
		Model:    strings.TrimSpace(*model),
	}
	switch selection.Provider {
	case contract.ProviderKindCodex:
		selection.Codex = &contract.CodexModelOptions{ReasoningEffort: strings.TrimSpace(*reasoningEffort)}
		if *fastMode {
			selection.Codex.FastMode = fastMode
		}
	case contract.ProviderKindClaude:
		selection.Claude = &contract.ClaudeModelOptions{Effort: strings.TrimSpace(*reasoningEffort), ContextWindow: strings.TrimSpace(*contextWindow)}
		if *thinking {
			selection.Claude.Thinking = thinking
		}
		if *fastMode {
			selection.Claude.FastMode = fastMode
		}
	case contract.ProviderKindGemini:
		selection.Gemini = &contract.GeminiModelOptions{ThinkingLevel: strings.TrimSpace(*thinkingLevel)}
		if *thinkingBudget > 0 {
			selection.Gemini.ThinkingBudget = thinkingBudget
		}
	case contract.ProviderKindOpenCode:
		selection.OpenCode = &contract.OpenCodeModelOptions{Variant: strings.TrimSpace(*variant), Agent: strings.TrimSpace(*agent)}
	case contract.ProviderKindPi:
		selection.Pi = &contract.PiModelOptions{}
	}
	normalized := api.NormalizeModelSelection(registry, selection)
	resolved := api.RuntimeTargetFromSelection(normalized)
	validation := api.ValidateSessionTargetWithRegistry(registry, resolved)
	return writeJSON(modelSelectionNormalizationResult{Selection: normalized, Target: validation.Target, Issues: validation.Issues})
}
