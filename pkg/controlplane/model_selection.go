package controlplane

import (
	"strings"

	"github.com/benjaminwestern/agentic-control/pkg/contract"
)

func NormalizeModelSelection(registry contract.ModelRegistry, selection contract.ModelSelection) contract.ModelSelection {
	selection.Provider = normalizeProviderKind(selection.Provider)
	if selection.Model == "" {
		if backend, ok := registryBackend(registry, string(selection.Provider)); ok {
			selection.Model = backend.DefaultModel
		}
	}
	if backend, ok := registryBackend(registry, string(selection.Provider)); ok {
		selection.Model = resolveRegistryModelAlias(backend, selection.Model)
	}
	switch selection.Provider {
	case contract.ProviderKindCodex:
		selection.Claude = nil
		selection.Gemini = nil
		selection.OpenCode = nil
		selection.Pi = nil
	case contract.ProviderKindClaude:
		selection.Codex = nil
		selection.Gemini = nil
		selection.OpenCode = nil
		selection.Pi = nil
	case contract.ProviderKindGemini:
		selection.Codex = nil
		selection.Claude = nil
		selection.OpenCode = nil
		selection.Pi = nil
	case contract.ProviderKindOpenCode:
		selection.Codex = nil
		selection.Claude = nil
		selection.Gemini = nil
		selection.Pi = nil
	case contract.ProviderKindPi:
		selection.Codex = nil
		selection.Claude = nil
		selection.Gemini = nil
		selection.OpenCode = nil
	}
	if backend, ok := registryBackend(registry, string(selection.Provider)); ok {
		if model, ok := registryModel(backend, selection.Model); ok {
			normalizeProviderSpecificOptions(&selection, *model)
		}
	}
	return selection
}

func RuntimeTargetFromSelection(selection contract.ModelSelection) RuntimeTarget {
	target := RuntimeTarget{Backend: string(selection.Provider), Model: selection.Model}
	switch selection.Provider {
	case contract.ProviderKindCodex:
		if selection.Codex != nil {
			target.Options.ReasoningEffort = strings.TrimSpace(selection.Codex.ReasoningEffort)
		}
	case contract.ProviderKindClaude:
		if selection.Claude != nil {
			target.Options.ReasoningEffort = strings.TrimSpace(selection.Claude.Effort)
		}
	case contract.ProviderKindGemini:
		if selection.Gemini != nil {
			target.Options.ThinkingLevel = strings.TrimSpace(selection.Gemini.ThinkingLevel)
			target.Options.ThinkingBudget = selection.Gemini.ThinkingBudget
		}
	case contract.ProviderKindOpenCode:
		// OpenCode-specific options are retained in the typed selection for future provider-specific surfaces.
	case contract.ProviderKindPi:
	}
	return target
}

func normalizeProviderSpecificOptions(selection *contract.ModelSelection, model contract.RuntimeModel) {
	capabilities := model.Capabilities
	switch selection.Provider {
	case contract.ProviderKindCodex:
		if selection.Codex == nil {
			return
		}
		selection.Codex.ReasoningEffort = resolveEffort(capabilities, selection.Codex.ReasoningEffort)
		if !capabilities.SupportsFastMode {
			selection.Codex.FastMode = nil
		}
	case contract.ProviderKindClaude:
		if selection.Claude == nil {
			return
		}
		selection.Claude.Effort = resolveEffort(capabilities, selection.Claude.Effort)
		if !capabilities.SupportsThinkingToggle {
			selection.Claude.Thinking = nil
		}
		if !capabilities.SupportsFastMode {
			selection.Claude.FastMode = nil
		}
		selection.Claude.ContextWindow = resolveContextWindow(capabilities, selection.Claude.ContextWindow)
	case contract.ProviderKindGemini:
		if selection.Gemini == nil {
			return
		}
		if !capabilities.SupportsThinkingLevel {
			selection.Gemini.ThinkingLevel = ""
		}
		if !capabilities.SupportsThinkingBudget {
			selection.Gemini.ThinkingBudget = nil
		}
	case contract.ProviderKindOpenCode:
		if selection.OpenCode == nil {
			return
		}
		selection.OpenCode.Variant = resolveLabeledOption(capabilities.VariantOptions, trimToNull(selection.OpenCode.Variant))
		selection.OpenCode.Agent = resolveLabeledOption(capabilities.AgentOptions, trimToNull(selection.OpenCode.Agent))
	case contract.ProviderKindPi:
	}
}

func resolveEffort(caps contract.RuntimeModelCapabilities, raw string) string {
	trimmed := strings.TrimSpace(raw)
	defaultValue := ""
	for _, option := range caps.ReasoningEffortLevels {
		if option.IsDefault {
			defaultValue = option.Value
		}
		if trimmed != "" && option.Value == trimmed {
			return trimmed
		}
	}
	return defaultValue
}

func resolveContextWindow(caps contract.RuntimeModelCapabilities, raw string) string {
	trimmed := strings.TrimSpace(raw)
	defaultValue := ""
	for _, option := range caps.ContextWindowOptions {
		if option.IsDefault {
			defaultValue = option.Value
		}
		if trimmed != "" && option.Value == trimmed {
			return trimmed
		}
	}
	return defaultValue
}

func resolveLabeledOption(options []contract.RuntimeModelOption, raw string) string {
	defaultValue := ""
	for _, option := range options {
		if option.IsDefault {
			defaultValue = option.Value
		}
		if raw != "" && option.Value == raw {
			return raw
		}
	}
	return defaultValue
}

func trimToNull(value string) string {
	return strings.TrimSpace(value)
}

func normalizeProviderKind(value contract.ProviderKind) contract.ProviderKind {
	switch NormalizeRuntimeBackend(string(value)) {
	case "codex":
		return contract.ProviderKindCodex
	case "claude":
		return contract.ProviderKindClaude
	case "gemini":
		return contract.ProviderKindGemini
	case "opencode":
		return contract.ProviderKindOpenCode
	case "pi":
		return contract.ProviderKindPi
	default:
		return value
	}
}
