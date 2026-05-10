package modelcatalog

import "github.com/benjaminwestern/agentic-control/pkg/contract"

func Codex() []contract.RuntimeModel {
	caps := textGenerationCapabilities(contract.RuntimeModelCapabilities{
		ReasoningEffortLevels: []contract.RuntimeModelOption{
			{Value: "xhigh", Label: "Extra High"},
			{Value: "high", Label: "High", IsDefault: true},
			{Value: "medium", Label: "Medium"},
			{Value: "low", Label: "Low"},
		},
		SupportsFastMode: true,
	})
	return []contract.RuntimeModel{
		model("gpt-5.4", "GPT-5.4", "codex", true, caps),
		model("gpt-5.4-mini", "GPT-5.4 Mini", "codex", false, caps),
		model("gpt-5.3-codex", "GPT-5.3 Codex", "codex", false, caps),
		model("gpt-5.3-codex-spark", "GPT-5.3 Codex Spark", "codex", false, caps),
		model("gpt-5.2-codex", "GPT-5.2 Codex", "codex", false, caps),
		model("gpt-5.2", "GPT-5.2", "codex", false, caps),
	}
}

func Claude() []contract.RuntimeModel {
	return []contract.RuntimeModel{
		model("claude-opus-4-7", "Claude Opus 4.7", "claude", false, textGenerationCapabilities(contract.RuntimeModelCapabilities{
			ReasoningEffortLevels: []contract.RuntimeModelOption{
				{Value: "low", Label: "Low"},
				{Value: "medium", Label: "Medium"},
				{Value: "high", Label: "High"},
				{Value: "xhigh", Label: "Extra High", IsDefault: true},
				{Value: "max", Label: "Max"},
				{Value: "ultrathink", Label: "Ultrathink"},
			},
			ContextWindowOptions: []contract.RuntimeModelOption{
				{Value: "200k", Label: "200k", IsDefault: true},
				{Value: "1m", Label: "1M"},
			},
			PromptInjectedEfforts: []string{"ultrathink"},
		})),
		model("claude-opus-4-6", "Claude Opus 4.6", "claude", false, textGenerationCapabilities(contract.RuntimeModelCapabilities{
			ReasoningEffortLevels: []contract.RuntimeModelOption{
				{Value: "low", Label: "Low"},
				{Value: "medium", Label: "Medium"},
				{Value: "high", Label: "High", IsDefault: true},
				{Value: "max", Label: "Max"},
				{Value: "ultrathink", Label: "Ultrathink"},
			},
			ContextWindowOptions: []contract.RuntimeModelOption{
				{Value: "200k", Label: "200k", IsDefault: true},
				{Value: "1m", Label: "1M"},
			},
			PromptInjectedEfforts: []string{"ultrathink"},
			SupportsFastMode:      true,
		})),
		model("claude-opus-4-5", "Claude Opus 4.5", "claude", false, textGenerationCapabilities(contract.RuntimeModelCapabilities{
			ReasoningEffortLevels: []contract.RuntimeModelOption{
				{Value: "low", Label: "Low"},
				{Value: "medium", Label: "Medium"},
				{Value: "high", Label: "High", IsDefault: true},
				{Value: "max", Label: "Max"},
			},
			SupportsFastMode: true,
		})),
		model("claude-sonnet-4-6", "Claude Sonnet 4.6", "claude", true, textGenerationCapabilities(contract.RuntimeModelCapabilities{
			ReasoningEffortLevels: []contract.RuntimeModelOption{
				{Value: "low", Label: "Low"},
				{Value: "medium", Label: "Medium"},
				{Value: "high", Label: "High", IsDefault: true},
				{Value: "ultrathink", Label: "Ultrathink"},
			},
			ContextWindowOptions: []contract.RuntimeModelOption{
				{Value: "200k", Label: "200k", IsDefault: true},
				{Value: "1m", Label: "1M"},
			},
			PromptInjectedEfforts: []string{"ultrathink"},
		})),
		model("claude-haiku-4-5", "Claude Haiku 4.5", "claude", false, textGenerationCapabilities(contract.RuntimeModelCapabilities{
			SupportsThinkingToggle: true,
		})),
	}
}

func Gemini() []contract.RuntimeModel {
	levelCaps := textGenerationCapabilities(contract.RuntimeModelCapabilities{
		SupportsThinkingLevel:   true,
		SupportedThinkingLevels: []string{"HIGH", "LOW"},
		ReasoningEffortLevels: []contract.RuntimeModelOption{
			{Value: "HIGH", Label: "High", IsDefault: true},
			{Value: "LOW", Label: "Low"},
		},
	})
	budgetCaps := textGenerationCapabilities(contract.RuntimeModelCapabilities{
		SupportsThinkingBudget:   true,
		SupportedThinkingBudgets: []int{-1, 0, 512},
		ReasoningEffortLevels: []contract.RuntimeModelOption{
			{Value: "-1", Label: "Dynamic", IsDefault: true},
			{Value: "0", Label: "None"},
			{Value: "512", Label: "512 Tokens"},
		},
	})
	return []contract.RuntimeModel{
		model("auto-gemini-3", "Auto Gemini 3", "gemini", true, levelCaps),
		model("gemini-3.1-pro-preview", "Gemini 3.1 Pro Preview", "gemini", false, levelCaps),
		model("gemini-3-flash-preview", "Gemini 3 Flash Preview", "gemini", false, levelCaps),
		model("gemini-3.1-flash-lite-preview", "Gemini 3.1 Flash Lite Preview", "gemini", false, levelCaps),
		model("gemini-2.5-pro", "Gemini 2.5 Pro", "gemini", false, budgetCaps),
		model("gemini-2.5-flash", "Gemini 2.5 Flash", "gemini", false, budgetCaps),
		model("gemini-2.5-flash-lite", "Gemini 2.5 Flash Lite", "gemini", false, budgetCaps),
	}
}

func Pi() []contract.RuntimeModel {
	caps := textGenerationCapabilities(contract.RuntimeModelCapabilities{
		SupportsThinkingLevel: true,
		SupportedThinkingLevels: []string{
			"low",
			"medium",
			"high",
			"xhigh",
		},
		ReasoningEffortLevels: []contract.RuntimeModelOption{
			{Value: "low", Label: "Low"},
			{Value: "medium", Label: "Medium", IsDefault: true},
			{Value: "high", Label: "High"},
			{Value: "xhigh", Label: "Extra High"},
		},
	})
	return []contract.RuntimeModel{
		model("pi-default", "Pi Default", "pi", true, caps),
	}
}

func textGenerationCapabilities(caps contract.RuntimeModelCapabilities) contract.RuntimeModelCapabilities {
	caps.Tasks = appendUniqueTask(caps.Tasks, contract.RuntimeModelTaskTextGeneration)
	caps.InputModalities = appendUniqueModality(caps.InputModalities, contract.RuntimeModelModalityText)
	caps.OutputModalities = appendUniqueModality(caps.OutputModalities, contract.RuntimeModelModalityText)
	return caps
}

func appendUniqueTask(values []contract.RuntimeModelTask, value contract.RuntimeModelTask) []contract.RuntimeModelTask {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func appendUniqueModality(values []contract.RuntimeModelModality, value contract.RuntimeModelModality) []contract.RuntimeModelModality {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func model(id string, label string, provider string, isDefault bool, capabilities contract.RuntimeModelCapabilities) contract.RuntimeModel {
	return contract.RuntimeModel{
		ID:           id,
		Label:        label,
		Provider:     provider,
		Default:      isDefault,
		Capabilities: capabilities,
	}
}
