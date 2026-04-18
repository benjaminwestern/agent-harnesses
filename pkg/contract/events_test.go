package contract

import "testing"

func TestEventHelpers(t *testing.T) {
	event := RuntimeEvent{
		EventType: EventAssistantMessageDelta,
		Payload:   map[string]any{"delta": " hello ", "nested": map[string]any{"ok": true}},
	}
	if got := EventDeltaText(event); got != " hello " {
		t.Fatalf("delta = %q", got)
	}
	if got := EventPayloadString(event, "nested"); got != `{"ok":true}` {
		t.Fatalf("nested payload = %q", got)
	}
	if !IsTurnCompletedEvent(RuntimeEvent{EventType: EventTurnCompleted}) {
		t.Fatal("expected completed event")
	}
	if !IsTurnErroredEvent(RuntimeEvent{EventType: EventSessionErrored}) {
		t.Fatal("expected errored event")
	}
	if RequestStatusFromEvent(RuntimeEvent{EventType: EventRequestOpened}) != RequestStatusOpen {
		t.Fatal("expected opened request status")
	}
	if !IsRequestEvent(RuntimeEvent{RequestID: "req-1"}) {
		t.Fatal("expected request event")
	}
}

func TestEventErrorText(t *testing.T) {
	if got := EventErrorText(RuntimeEvent{Summary: "summary"}); got != "summary" {
		t.Fatalf("summary error = %q", got)
	}
	if got := EventErrorText(RuntimeEvent{Payload: map[string]any{"last_error": "specific"}, Summary: "summary"}); got != "specific" {
		t.Fatalf("payload error = %q", got)
	}
}
