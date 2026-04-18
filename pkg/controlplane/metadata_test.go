package controlplane

import "testing"

func TestRuntimeMetadataMapOmitsEmptyAndPreservesLabels(t *testing.T) {
	metadata := RuntimeMetadata{
		Title:    "court run worker",
		System:   "system prompt",
		Model:    "openai/gpt-5.4",
		Provider: "openai",
		Labels: map[string]string{
			"court_run_id":    "run-1",
			"court_worker_id": "wrk-1",
			"empty":           "",
		},
		Extra: map[string]any{
			"custom": "value",
			"nil":    nil,
		},
	}.Map()

	if metadata[MetadataKeyTitle] != "court run worker" ||
		metadata[MetadataKeySystem] != "system prompt" ||
		metadata[MetadataKeyModel] != "openai/gpt-5.4" ||
		metadata[MetadataKeyProvider] != "openai" ||
		metadata["court_run_id"] != "run-1" ||
		metadata["court_worker_id"] != "wrk-1" ||
		metadata["custom"] != "value" {
		t.Fatalf("metadata not mapped as expected: %+v", metadata)
	}
	if _, ok := metadata[MetadataKeyAgent]; ok {
		t.Fatalf("empty agent should be omitted: %+v", metadata)
	}
	if _, ok := metadata["empty"]; ok {
		t.Fatalf("empty label should be omitted: %+v", metadata)
	}
	if _, ok := metadata["nil"]; ok {
		t.Fatalf("nil extra should be omitted: %+v", metadata)
	}
}

func TestMetadataForNoToolTurnDisablesDefaultCodingTools(t *testing.T) {
	metadata := MetadataForNoToolTurn(map[string]any{
		MetadataKeyModel: "openai/gpt-5.4",
	})

	tools, ok := metadata[MetadataKeyTools].(map[string]any)
	if !ok {
		t.Fatalf("tools metadata was not set: %+v", metadata)
	}
	for _, tool := range DefaultNoToolTurnTools {
		if tools[tool] != false {
			t.Fatalf("tool %q was not disabled: %+v", tool, tools)
		}
	}
	if metadata[MetadataKeyModel] != "openai/gpt-5.4" {
		t.Fatalf("existing metadata was not preserved: %+v", metadata)
	}
}
