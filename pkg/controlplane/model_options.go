package controlplane

import (
	"encoding/json"
	"strings"
)

func HasModelOptions(options ModelOptions) bool {
	return strings.TrimSpace(options.ReasoningEffort) != "" ||
		strings.TrimSpace(options.ThinkingLevel) != "" ||
		options.ThinkingBudget != nil
}

func MergeModelOptions(values ...ModelOptions) ModelOptions {
	var out ModelOptions
	for i := len(values) - 1; i >= 0; i-- {
		value := values[i]
		if strings.TrimSpace(value.ReasoningEffort) != "" {
			out.ReasoningEffort = strings.TrimSpace(value.ReasoningEffort)
		}
		if strings.TrimSpace(value.ThinkingLevel) != "" {
			out.ThinkingLevel = strings.TrimSpace(value.ThinkingLevel)
		}
		if value.ThinkingBudget != nil {
			out.ThinkingBudget = value.ThinkingBudget
		}
	}
	return out
}

func MarshalModelOptionsJSON(options ModelOptions) string {
	if !HasModelOptions(options) {
		return ""
	}
	encoded, err := json.Marshal(options)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func ParseModelOptionsJSON(value string) ModelOptions {
	if strings.TrimSpace(value) == "" {
		return ModelOptions{}
	}
	var options ModelOptions
	if err := json.Unmarshal([]byte(value), &options); err != nil {
		return ModelOptions{}
	}
	return options
}
