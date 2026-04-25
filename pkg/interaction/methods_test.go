package interaction

import "testing"

func TestAllMethodsHasCurrentAgenticInteractionSurface(t *testing.T) {
	expected := []string{
		"system.describe",
		"system.status",
		"system.health",
		"devices.list",
		"diagnostics.status",
		"diagnostics.list",
		"diagnostics.log",
		"diagnostics.acknowledge",
		"diagnostics.resolve",
		"diagnostics.clear",
		"diagnostics.events.subscribe",
		"diagnostics.events.unsubscribe",
		"permissions.status",
		"permissions.request_microphone",
		"permissions.request_accessibility",
		"permissions.open_accessibility_settings",
		"models.catalog",
		"models.parakeet_eou.status",
		"models.parakeet_eou.ensure",
		"models.parakeet_eou.download",
		"models.parakeet_eou.delete",
		"models.download_job",
		"models.download_cancel",
		"transcript.get",
		"transcript.set",
		"transcript.copy",
		"transcript.insert",
		"transcript.process",
		"stt.start",
		"stt.stop",
		"stt.reset",
		"stt.status",
		"stt.events.subscribe",
		"stt.events.unsubscribe",
		"stt.models.list",
		"stt.model.get",
		"stt.model.set",
		"stt.model.download",
		"stt.realtime.models.list",
		"stt.realtime.model.get",
		"stt.realtime.model.set",
		"stt.realtime.model.download",
		"stt.realtime.prewarm",
		"stt.realtime.file_transcribe.start",
		"stt.realtime.file_transcribe.status",
		"stt.realtime.file_transcribe.result",
		"stt.realtime.file_transcribe.cancel",
		"stt.realtime.file_transcribe.list",
		"stt.batch.models.list",
		"stt.batch.model.get",
		"stt.batch.model.set",
		"stt.batch.model.download",
		"stt.batch.model.ensure",
		"stt.batch.transcribe.start",
		"stt.batch.transcribe.status",
		"stt.batch.transcribe.result",
		"stt.batch.transcribe.cancel",
		"stt.batch.transcribe.list",
		"tts.speak",
		"tts.save",
		"tts.play",
		"tts.stop",
		"tts.pause",
		"tts.resume",
		"tts.restart",
		"tts.status",
		"tts.voices.list",
		"tts.config.get",
		"tts.config.set",
		"tts.speak_selected",
		"tts.speak_transcript",
		"notification.audio.catalog",
		"notification.audio.play",
		"notification.audio.stop",
		"notification.audio.status",
		"accessibility.status",
		"accessibility.request_permission",
		"accessibility.open_settings",
		"accessibility.tree.focused",
		"accessibility.targets.list",
		"accessibility.context.capture",
		"accessibility.context.screen",
		"accessibility.find",
		"accessibility.action.perform",
		"accessibility.selection.set",
		"accessibility.text.replace",
		"accessibility.text.inspect",
		"accessibility.apps.list",
		"accessibility.insert",
		"accessibility.click_insert",
		"screen.click",
		"observation.permission_status",
		"observation.request_permission",
		"observation.screenshot",
		"observation.recording.start",
		"observation.recording.stop",
		"observation.recording.status",
		"observation.recordings.list",
		"observation.events.subscribe",
		"observation.events.unsubscribe",
		"apps.open",
		"apps.activate",
		"apps.installed.list",
		"apps.find",
	}

	if len(AllMethods) != len(expected) {
		t.Fatalf("AllMethods has %d methods, want %d", len(AllMethods), len(expected))
	}

	expectedSet := make(map[string]struct{}, len(expected))
	for _, method := range expected {
		expectedSet[method] = struct{}{}
	}

	seen := make(map[string]struct{}, len(AllMethods))
	for _, method := range AllMethods {
		if method == "" {
			t.Fatal("AllMethods contains an empty method")
		}
		if _, ok := seen[method]; ok {
			t.Fatalf("AllMethods contains duplicate method %q", method)
		}
		if _, ok := expectedSet[method]; !ok {
			t.Fatalf("AllMethods contains unexpected method %q", method)
		}
		seen[method] = struct{}{}
	}

	for _, method := range expected {
		if _, ok := seen[method]; !ok {
			t.Fatalf("AllMethods is missing method %q", method)
		}
	}
}
