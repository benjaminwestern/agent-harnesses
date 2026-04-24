package controlplane

import (
	"testing"

	"github.com/benjaminwestern/agentic-control/pkg/contract"
)

func TestNormalizeModelSelectionOpenCodeAliasAndOptions(t *testing.T) {
	registry := contract.ModelRegistry{
		SchemaVersion: contract.ControlPlaneSchemaVersion,
		Backends: []contract.RuntimeBackendRegistry{{
			Backend:         "opencode",
			DefaultModel:    "opencode/gemini-3-flash",
			DefaultProvider: "google",
			Aliases: []contract.ModelAlias{{
				Alias: "gemini-3-flash",
				Model: "opencode/gemini-3-flash",
			}},
			Models: []contract.RuntimeModel{{
				ID:       "opencode/gemini-3-flash",
				Provider: "google",
				Capabilities: contract.RuntimeModelCapabilities{
					VariantOptions: []contract.RuntimeModelOption{{Value: "safe", IsDefault: true}, {Value: "fast"}},
					AgentOptions:   []contract.RuntimeModelOption{{Value: "coder", IsDefault: true}, {Value: "reviewer"}},
				},
			}},
		}},
	}
	selection := NormalizeModelSelection(registry, contract.ModelSelection{
		Provider: contract.ProviderKindOpenCode,
		Model:    "gemini-3-flash",
		OpenCode: &contract.OpenCodeModelOptions{Variant: "unknown", Agent: "reviewer"},
	})
	if selection.Model != "opencode/gemini-3-flash" {
		t.Fatalf("model = %q, want opencode/gemini-3-flash", selection.Model)
	}
	if selection.OpenCode == nil || selection.OpenCode.Variant != "safe" || selection.OpenCode.Agent != "reviewer" {
		t.Fatalf("opencode options = %#v, want normalized options", selection.OpenCode)
	}
}
