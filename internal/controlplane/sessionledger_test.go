package controlplane

import (
	"testing"

	"github.com/benjaminwestern/agentic-control/pkg/contract"
)

func TestSessionLedgerTracksUsageByModelAndMode(t *testing.T) {
	ledger := NewSessionLedger()
	ledger.Upsert(contract.RuntimeSession{
		SchemaVersion: contract.ControlPlaneSchemaVersion,
		SessionID:     "session-1",
		Runtime:       "gemini",
		Model:         "gemini-2.5-pro",
		CreatedAtMS:   100,
		UpdatedAtMS:   100,
	})

	ledger.UpdateFromEvent(contract.RuntimeEvent{
		SessionID:    "session-1",
		Runtime:      "gemini",
		RecordedAtMS: 200,
		EventType:    contract.EventSessionModeChanged,
		Payload:      map[string]any{"current_mode_id": "review"},
	})
	ledger.UpdateFromEvent(contract.RuntimeEvent{
		SessionID:    "session-1",
		Runtime:      "gemini",
		RecordedAtMS: 300,
		EventType:    contract.EventThreadTokenUsageUpdated,
		Payload: map[string]any{
			"usage": map[string]any{
				"input_tokens":  120,
				"output_tokens": 30,
				"total_tokens":  150,
			},
		},
	})
	ledger.UpdateFromEvent(contract.RuntimeEvent{
		SessionID:    "session-1",
		Runtime:      "gemini",
		RecordedAtMS: 400,
		EventType:    contract.EventThreadTokenUsageUpdated,
		SessionState: &contract.SessionState{Model: "gemini-2.5-flash", Mode: "judge"},
		Payload: map[string]any{
			"usage": map[string]any{
				"input_tokens":  200,
				"output_tokens": 50,
				"total_tokens":  250,
			},
		},
	})

	record, ok := ledger.Get("session-1", "")
	if !ok {
		t.Fatal("tracked session not found")
	}
	if record.Session.Usage.TotalTokens != 250 {
		t.Fatalf("total tokens = %d, want 250", record.Session.Usage.TotalTokens)
	}
	if record.Session.Mode != "judge" {
		t.Fatalf("mode = %q, want judge", record.Session.Mode)
	}
	if len(record.UsageByModel) != 2 {
		t.Fatalf("usage by model count = %d, want 2", len(record.UsageByModel))
	}
	if record.UsageByModel[0].Usage.TotalTokens != 150 {
		t.Fatalf("first model total = %d, want 150", record.UsageByModel[0].Usage.TotalTokens)
	}
	if len(record.UsageByMode) != 2 {
		t.Fatalf("usage by mode count = %d, want 2", len(record.UsageByMode))
	}
	if record.UsageByMode[0].Key != "review" || record.UsageByMode[0].Usage.TotalTokens != 150 {
		t.Fatalf("review mode usage = %#v, want review/150", record.UsageByMode[0])
	}
	if record.UsageByMode[1].Key != "judge" || record.UsageByMode[1].Usage.TotalTokens != 100 {
		t.Fatalf("judge mode usage = %#v, want judge/100", record.UsageByMode[1])
	}
}
