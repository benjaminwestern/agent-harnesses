package controlplane

import (
	"testing"

	"github.com/benjaminwestern/agentic-control/pkg/contract"
)

func TestBuildModelRegistryGroupsProvidersAndDefaults(t *testing.T) {
	registry := BuildModelRegistry([]contract.RuntimeDescriptor{{
		Runtime: "opencode",
		Capabilities: contract.RuntimeCapabilities{
			StartSession: true,
			StreamEvents: true,
		},
		Probe: &contract.RuntimeProbe{
			Installed:   true,
			ModelSource: "remote",
			Models: []contract.RuntimeModel{{
				ID:       "google/gemini-3-flash-preview",
				Provider: "google",
				Default:  true,
			}, {
				ID:       "openai/gpt-5",
				Provider: "openai",
			}},
		},
	}})
	if len(registry.Backends) != 1 {
		t.Fatalf("backend count = %d, want 1", len(registry.Backends))
	}
	backend := registry.Backends[0]
	if backend.DefaultModel != "google/gemini-3-flash-preview" {
		t.Fatalf("default model = %q, want google/gemini-3-flash-preview", backend.DefaultModel)
	}
	if backend.DefaultProvider != "google" {
		t.Fatalf("default provider = %q, want google", backend.DefaultProvider)
	}
	if len(backend.Providers) != 2 {
		t.Fatalf("provider count = %d, want 2", len(backend.Providers))
	}
}

func TestValidateSessionTargetWithRegistryResolvesAlias(t *testing.T) {
	registry := contract.ModelRegistry{
		SchemaVersion: contract.ControlPlaneSchemaVersion,
		Backends: []contract.RuntimeBackendRegistry{{
			Backend:         "opencode",
			Installed:       true,
			SupportsSession: true,
			DefaultModel:    "opencode/gemini-3-flash",
			DefaultProvider: "google",
			Aliases: []contract.ModelAlias{{
				Alias: "gemini-3-flash",
				Model: "opencode/gemini-3-flash",
			}},
			Models: []contract.RuntimeModel{{
				ID:       "opencode/gemini-3-flash",
				Provider: "google",
			}},
		}},
	}
	result := ValidateSessionTargetWithRegistry(registry, RuntimeTarget{Backend: "opencode", Model: "gemini-3-flash"})
	if result.HasErrors() {
		t.Fatalf("unexpected errors: %#v", result.Issues)
	}
	if result.Target.Model != "opencode/gemini-3-flash" {
		t.Fatalf("resolved model = %q, want opencode/gemini-3-flash", result.Target.Model)
	}
}
