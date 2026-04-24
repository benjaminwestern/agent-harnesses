package controlplane

import (
	"testing"

	"github.com/benjaminwestern/agentic-control/pkg/contract"
)

func TestValidateSessionTarget(t *testing.T) {
	descriptors := []contract.RuntimeDescriptor{{
		Runtime: "opencode",
		Capabilities: contract.RuntimeCapabilities{
			StartSession: true,
			StreamEvents: true,
		},
		Probe: &contract.RuntimeProbe{
			Installed: true,
			Models: []contract.RuntimeModel{{
				ID:       "opencode/gpt-5.4",
				Provider: "opencode",
				Capabilities: contract.RuntimeModelCapabilities{
					ReasoningEffortLevels: []contract.RuntimeModelOption{{Value: "low"}, {Value: "medium"}},
				},
			}},
		},
	}}

	result := ValidateSessionTarget(descriptors, RuntimeTarget{
		Backend: "opencode",
		Model:   "opencode/gpt-5.4",
		Options: ModelOptions{ReasoningEffort: "medium"},
	})
	if result.HasErrors() {
		t.Fatalf("unexpected validation errors: %#v", result.Issues)
	}
	if result.Target.Provider != "opencode" {
		t.Fatalf("provider = %q, want opencode", result.Target.Provider)
	}
	if result.Model == nil || result.Model.ID != "opencode/gpt-5.4" {
		t.Fatalf("model descriptor = %#v, want opencode/gpt-5.4", result.Model)
	}
}

func TestValidateSessionTargetRejectsProviderMismatch(t *testing.T) {
	descriptors := []contract.RuntimeDescriptor{{
		Runtime: "opencode",
		Capabilities: contract.RuntimeCapabilities{
			StartSession: true,
			StreamEvents: true,
		},
		Probe: &contract.RuntimeProbe{
			Installed: true,
			Models: []contract.RuntimeModel{{
				ID:       "opencode/gpt-5.4",
				Provider: "opencode",
			}},
		},
	}}

	result := ValidateSessionTarget(descriptors, RuntimeTarget{
		Backend:  "opencode",
		Provider: "claude",
		Model:    "opencode/gpt-5.4",
	})
	if !result.HasErrors() {
		t.Fatal("expected provider/model mismatch error")
	}
	if result.Issues[0].Code != "provider_model_mismatch" {
		t.Fatalf("issue code = %q, want provider_model_mismatch", result.Issues[0].Code)
	}
}
