package main

import (
	"flag"
	"strings"

	"github.com/benjaminwestern/agentic-control/pkg/contract"
	"github.com/spf13/cobra"
)

type selectionFlags struct {
	provider        *string
	model           *string
	reasoningEffort *string
	fastMode        *bool
	thinking        *bool
	contextWindow   *string
	thinkingLevel   *string
	thinkingBudget  *int
	variant         *string
	agent           *string
}

func bindSelectionFlags(fs *flag.FlagSet, prefix string, label string) selectionFlags {
	name := func(base string) string {
		if prefix == "" {
			return base
		}
		return prefix + base
	}
	providerLabel := label
	if providerLabel == "" {
		providerLabel = "typed selection"
	}
	return selectionFlags{
		provider:        fs.String(name("provider"), "", providerLabel+" provider kind: codex, claude, gemini, opencode, pi"),
		model:           fs.String(name("model-selection"), "", providerLabel+" model id or alias"),
		reasoningEffort: fs.String(name("effort"), "", providerLabel+" reasoning effort"),
		fastMode:        fs.Bool(name("fast-mode"), false, providerLabel+" fast mode"),
		thinking:        fs.Bool(name("thinking"), false, providerLabel+" thinking toggle"),
		contextWindow:   fs.String(name("context-window"), "", providerLabel+" context window option"),
		thinkingLevel:   fs.String(name("thinking-level-selection"), "", providerLabel+" thinking level"),
		thinkingBudget:  fs.Int(name("thinking-budget-selection"), 0, providerLabel+" thinking budget"),
		variant:         fs.String(name("variant"), "", providerLabel+" OpenCode variant"),
		agent:           fs.String(name("agent"), "", providerLabel+" OpenCode agent"),
	}
}

func (s selectionFlags) build() *contract.ModelSelection {
	provider := strings.TrimSpace(*s.provider)
	if provider == "" {
		return nil
	}
	selection := &contract.ModelSelection{
		Provider: contract.ProviderKind(provider),
		Model:    strings.TrimSpace(*s.model),
	}
	switch selection.Provider {
	case contract.ProviderKindCodex:
		selection.Codex = &contract.CodexModelOptions{ReasoningEffort: strings.TrimSpace(*s.reasoningEffort)}
		if *s.fastMode {
			selection.Codex.FastMode = s.fastMode
		}
	case contract.ProviderKindClaude:
		selection.Claude = &contract.ClaudeModelOptions{Effort: strings.TrimSpace(*s.reasoningEffort), ContextWindow: strings.TrimSpace(*s.contextWindow)}
		if *s.thinking {
			selection.Claude.Thinking = s.thinking
		}
		if *s.fastMode {
			selection.Claude.FastMode = s.fastMode
		}
	case contract.ProviderKindGemini:
		selection.Gemini = &contract.GeminiModelOptions{ThinkingLevel: strings.TrimSpace(*s.thinkingLevel)}
		if *s.thinkingBudget > 0 {
			selection.Gemini.ThinkingBudget = s.thinkingBudget
		}
	case contract.ProviderKindOpenCode:
		selection.OpenCode = &contract.OpenCodeModelOptions{Variant: strings.TrimSpace(*s.variant), Agent: strings.TrimSpace(*s.agent)}
	case contract.ProviderKindPi:
		selection.Pi = &contract.PiModelOptions{}
	}
	return selection
}

func bindSelectionFlagsCobra(cmd *cobra.Command, prefix string, label string) selectionFlags {
	name := func(base string) string {
		if prefix == "" {
			return base
		}
		return prefix + base
	}
	providerLabel := label
	if providerLabel == "" {
		providerLabel = "typed selection"
	}
	result := selectionFlags{}
	result.provider = cmd.Flags().String(name("provider"), "", providerLabel+" provider kind: codex, claude, gemini, opencode, pi")
	result.model = cmd.Flags().String(name("model-selection"), "", providerLabel+" model id or alias")
	result.reasoningEffort = cmd.Flags().String(name("effort"), "", providerLabel+" reasoning effort")
	result.fastMode = cmd.Flags().Bool(name("fast-mode"), false, providerLabel+" fast mode")
	result.thinking = cmd.Flags().Bool(name("thinking"), false, providerLabel+" thinking toggle")
	result.contextWindow = cmd.Flags().String(name("context-window"), "", providerLabel+" context window option")
	result.thinkingLevel = cmd.Flags().String(name("thinking-level-selection"), "", providerLabel+" thinking level")
	result.thinkingBudget = cmd.Flags().Int(name("thinking-budget-selection"), 0, providerLabel+" thinking budget")
	result.variant = cmd.Flags().String(name("variant"), "", providerLabel+" OpenCode variant")
	result.agent = cmd.Flags().String(name("agent"), "", providerLabel+" OpenCode agent")
	return result
}
