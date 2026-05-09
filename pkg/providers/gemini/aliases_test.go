package gemini

import (
	"testing"

	api "github.com/benjaminwestern/agentic-control/pkg/controlplane"
)

func TestPrepareGeminiModelAliasRejectsInvalidThinkingOptions(t *testing.T) {
	tests := []struct {
		name    string
		model   string
		options api.ModelOptions
	}{
		{
			name:    "invalid gemini 3 level",
			model:   "gemini-3-flash-preview",
			options: api.ModelOptions{ThinkingLevel: "turbo"},
		},
		{
			name:    "level on gemini 2.5",
			model:   "gemini-2.5-pro",
			options: api.ModelOptions{ThinkingLevel: "HIGH"},
		},
		{
			name:    "invalid gemini 2.5 budget",
			model:   "gemini-2.5-flash",
			options: api.ModelOptions{ThinkingBudget: intPtr(999)},
		},
		{
			name:    "budget on gemini 3",
			model:   "gemini-3-flash-preview",
			options: api.ModelOptions{ThinkingBudget: intPtr(512)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := prepareGeminiModelAlias("test", tt.model, tt.options); err == nil {
				t.Fatal("prepareGeminiModelAlias succeeded, want error")
			}
		})
	}
}

func TestGetGeminiThinkingModelAliasAcceptsValidOptions(t *testing.T) {
	if alias, _ := getGeminiThinkingModelAlias("gemini-3-flash-preview", api.ModelOptions{ThinkingLevel: "HIGH"}); alias == "" {
		t.Fatal("expected Gemini 3 thinking-level alias")
	}
	if alias, _ := getGeminiThinkingModelAlias("gemini-2.5-pro", api.ModelOptions{ThinkingBudget: intPtr(512)}); alias == "" {
		t.Fatal("expected Gemini 2.5 thinking-budget alias")
	}
}

func intPtr(value int) *int {
	return &value
}
