package controlplane

import (
	"encoding/json"
	"strings"
)

func HasModelOptions(options ModelOptions) bool {
	return strings.TrimSpace(options.ReasoningEffort) != "" ||
		strings.TrimSpace(options.ThinkingLevel) != "" ||
		options.ThinkingBudget != nil ||
		options.MaxOutputTokens > 0 ||
		options.Temperature != nil ||
		options.TopP != nil ||
		strings.TrimSpace(options.BaseURL) != "" ||
		strings.TrimSpace(options.APIKey) != "" ||
		strings.TrimSpace(options.OAuthTokenURL) != "" ||
		strings.TrimSpace(options.OAuthClientID) != "" ||
		strings.TrimSpace(options.OAuthClientSecret) != "" ||
		options.ResponseSchema != nil ||
		options.Logprobs ||
		options.TopLogprobs > 0
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
		if value.MaxOutputTokens > 0 {
			out.MaxOutputTokens = value.MaxOutputTokens
		}
		if value.Temperature != nil {
			out.Temperature = value.Temperature
		}
		if value.TopP != nil {
			out.TopP = value.TopP
		}
		if strings.TrimSpace(value.BaseURL) != "" {
			out.BaseURL = strings.TrimSpace(value.BaseURL)
		}
		if strings.TrimSpace(value.APIKey) != "" {
			out.APIKey = strings.TrimSpace(value.APIKey)
		}
		if strings.TrimSpace(value.OAuthTokenURL) != "" {
			out.OAuthTokenURL = strings.TrimSpace(value.OAuthTokenURL)
		}
		if strings.TrimSpace(value.OAuthClientID) != "" {
			out.OAuthClientID = strings.TrimSpace(value.OAuthClientID)
		}
		if strings.TrimSpace(value.OAuthClientSecret) != "" {
			out.OAuthClientSecret = strings.TrimSpace(value.OAuthClientSecret)
		}
		if value.ResponseSchema != nil {
			out.ResponseSchema = value.ResponseSchema
		}
		if value.Logprobs {
			out.Logprobs = true
		}
		if value.TopLogprobs > 0 {
			out.TopLogprobs = value.TopLogprobs
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
