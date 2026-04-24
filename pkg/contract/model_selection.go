package contract

type ProviderKind string

const (
	ProviderKindCodex   ProviderKind = "codex"
	ProviderKindClaude  ProviderKind = "claude"
	ProviderKindGemini  ProviderKind = "gemini"
	ProviderKindOpenCode ProviderKind = "opencode"
	ProviderKindPi      ProviderKind = "pi"
)

type CodexModelOptions struct {
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
	FastMode        *bool  `json:"fast_mode,omitempty"`
}

type ClaudeModelOptions struct {
	Effort        string `json:"effort,omitempty"`
	Thinking      *bool  `json:"thinking,omitempty"`
	FastMode      *bool  `json:"fast_mode,omitempty"`
	ContextWindow string `json:"context_window,omitempty"`
}

type GeminiModelOptions struct {
	ThinkingLevel  string `json:"thinking_level,omitempty"`
	ThinkingBudget *int   `json:"thinking_budget,omitempty"`
}

type OpenCodeModelOptions struct {
	Variant string `json:"variant,omitempty"`
	Agent   string `json:"agent,omitempty"`
}

type PiModelOptions struct{}

type ModelSelection struct {
	Provider ProviderKind `json:"provider"`
	Model    string       `json:"model"`
	Codex    *CodexModelOptions `json:"codex,omitempty"`
	Claude   *ClaudeModelOptions `json:"claude,omitempty"`
	Gemini   *GeminiModelOptions `json:"gemini,omitempty"`
	OpenCode *OpenCodeModelOptions `json:"opencode,omitempty"`
	Pi       *PiModelOptions `json:"pi,omitempty"`
}
