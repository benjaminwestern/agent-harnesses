package controlplane

type ProviderUsage struct {
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
	VectorCount      int `json:"vector_count,omitempty"`
}

type ProviderResultMetadata struct {
	Provider      string        `json:"provider,omitempty"`
	Model         string        `json:"model,omitempty"`
	StatusCode    int           `json:"status_code,omitempty"`
	LatencyMillis int64         `json:"latency_millis,omitempty"`
	FinishReason  string        `json:"finish_reason,omitempty"`
	Usage         ProviderUsage `json:"usage,omitempty"`
}

func mergeProviderMetadata(metadata map[string]any, result ProviderResultMetadata) map[string]any {
	if metadata == nil {
		metadata = map[string]any{}
	}
	if result.Provider != "" {
		metadata["provider"] = result.Provider
	}
	if result.Model != "" {
		metadata["model"] = result.Model
	}
	metadata["usage"] = result.Usage
	metadata["provider_result"] = result
	return metadata
}
