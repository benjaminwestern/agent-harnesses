package apphost

import (
	"reflect"
	"testing"
)

func TestExtractVoiceNamesReadsProviderVoiceGroups(t *testing.T) {
	value := map[string]any{
		"schema_version": "agentic-interaction.rpc.v1",
		"groups": []any{
			map[string]any{
				"id":    "kokoro-en",
				"title": "Kokoro English",
				"voices": []any{
					map[string]any{"code": "af_bella", "name": "Bella"},
				},
			},
		},
		"providers": []any{
			map[string]any{
				"provider":     "apple_speech_synthesis",
				"display_name": "Apple Speech",
				"groups": []any{
					map[string]any{
						"id":    "en-AU",
						"title": "English Australia",
						"voices": []any{
							map[string]any{"code": "com.apple.voice.compact.en-AU.Karen", "name": "Karen"},
						},
					},
				},
			},
		},
	}

	got := extractVoiceNames(value)
	want := []string{"af_bella", "com.apple.voice.compact.en-AU.Karen"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("voices = %#v, want %#v", got, want)
	}
}
