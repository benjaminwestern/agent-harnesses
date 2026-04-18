package contract

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	EventAssistantMessageDelta = "assistant.message.delta"
	EventTurnCompleted         = "turn.completed"
	EventTurnErrored           = "turn.errored"
	EventSessionErrored        = "session.errored"
	EventRequestOpened         = "request.opened"
	EventRequestResponded      = "request.responded"
	EventRequestResolved       = "request.resolved"
	EventRequestClosed         = "request.closed"
)

func IsRequestEvent(event RuntimeEvent) bool {
	return event.Request != nil || strings.TrimSpace(event.RequestID) != ""
}

func IsTerminalTurnEvent(event RuntimeEvent) bool {
	return IsTurnCompletedEvent(event) || IsTurnErroredEvent(event)
}

func IsTurnCompletedEvent(event RuntimeEvent) bool {
	return event.EventType == EventTurnCompleted
}

func IsTurnErroredEvent(event RuntimeEvent) bool {
	return event.EventType == EventTurnErrored || event.EventType == EventSessionErrored
}

func RequestStatusFromEvent(event RuntimeEvent) RequestStatus {
	if event.Request != nil && event.Request.Status != "" {
		return event.Request.Status
	}
	switch event.EventType {
	case EventRequestOpened:
		return RequestStatusOpen
	case EventRequestResponded:
		return RequestStatusResponded
	case EventRequestResolved:
		return RequestStatusResolved
	case EventRequestClosed:
		return RequestStatusClosed
	default:
		return ""
	}
}

func EventPayloadString(event RuntimeEvent, key string) string {
	return PayloadString(event.Payload, key)
}

func PayloadString(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	switch value := payload[key].(type) {
	case string:
		return strings.TrimSpace(value)
	case fmt.Stringer:
		return strings.TrimSpace(value.String())
	default:
		if value == nil {
			return ""
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(encoded))
	}
}

func EventDeltaText(event RuntimeEvent) string {
	if event.EventType != EventAssistantMessageDelta {
		return ""
	}
	if event.Payload == nil {
		return ""
	}
	switch value := event.Payload["delta"].(type) {
	case string:
		return value
	case fmt.Stringer:
		return value.String()
	default:
		return PayloadString(event.Payload, "delta")
	}
}

func EventFinalText(event RuntimeEvent) string {
	return firstNonEmpty(
		EventPayloadString(event, "final_structured_result"),
		EventPayloadString(event, "final_result"),
		EventPayloadString(event, "final_text"),
		EventPayloadString(event, "assistant_text"),
		EventPayloadString(event, "result"),
		EventPayloadString(event, "text"),
	)
}

func EventErrorText(event RuntimeEvent) string {
	return firstNonEmpty(
		EventPayloadString(event, "last_error"),
		EventPayloadString(event, "error"),
		event.Summary,
	)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
