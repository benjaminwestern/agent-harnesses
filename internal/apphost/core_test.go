package apphost

import "testing"

func TestNormalizeNotificationAudioPlayParamsAddsCatalogEvent(t *testing.T) {
	params := normalizeNotificationAudioPlayParams(map[string]any{})
	if params["event"] != defaultNotificationAudioEvent {
		t.Fatalf("event = %v, want %q", params["event"], defaultNotificationAudioEvent)
	}
	if params["interrupt"] != true {
		t.Fatalf("interrupt = %v, want true", params["interrupt"])
	}
}

func TestNormalizeNotificationAudioPlayParamsMapsLegacyVisualAttention(t *testing.T) {
	params := normalizeNotificationAudioPlayParams(map[string]any{
		"event":     "visual_attention",
		"interrupt": false,
	})
	if params["event"] != defaultNotificationAudioEvent {
		t.Fatalf("event = %v, want %q", params["event"], defaultNotificationAudioEvent)
	}
	if params["interrupt"] != false {
		t.Fatalf("interrupt = %v, want false", params["interrupt"])
	}
}

func TestNormalizeNotificationAudioPlayParamsKeepsExplicitTarget(t *testing.T) {
	params := normalizeNotificationAudioPlayParams(map[string]any{
		"systemSound": "Hero",
	})
	if params["system_sound"] != "Hero" {
		t.Fatalf("system_sound = %v, want Hero", params["system_sound"])
	}
	if _, ok := params["event"]; ok {
		t.Fatalf("event should not be set when system_sound is provided")
	}
}
