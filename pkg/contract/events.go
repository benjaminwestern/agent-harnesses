package contract

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	EventAssistantMessageDelta   = "assistant.message.delta"
	EventAssistantThoughtDelta   = "assistant.thought.delta"
	EventTurnStarted             = "turn.started"
	EventTurnCompleted           = "turn.completed"
	EventTurnErrored             = "turn.errored"
	EventTurnInterrupted         = "turn.interrupted"
	EventTurnPlanUpdated         = "turn.plan.updated"
	EventThreadTokenUsageUpdated = "thread.token-usage.updated"
	EventToolStarted             = "tool.started"
	EventToolProgress            = "tool.progress"
	EventToolCompleted           = "tool.completed"
	EventToolErrored             = "tool.errored"
	EventSessionStarted          = "session.started"
	EventSessionStopped          = "session.stopped"
	EventSessionErrored          = "session.errored"
	EventSessionModeChanged      = "session.mode.changed"
	EventRequestOpened           = "request.opened"
	EventRequestResponded        = "request.responded"
	EventRequestResolved         = "request.resolved"
	EventRequestClosed           = "request.closed"
	EventRuntimeEvent            = "runtime.event"
	EventRuntimeDecodeError      = "runtime.decode_error"
	EventRuntimeStderr           = "runtime.stderr"
)

type TokenUsage struct {
	InputTokens     int64 `json:"input_tokens,omitempty"`
	OutputTokens    int64 `json:"output_tokens,omitempty"`
	ReasoningTokens int64 `json:"reasoning_tokens,omitempty"`
	CachedTokens    int64 `json:"cached_tokens,omitempty"`
	TotalTokens     int64 `json:"total_tokens,omitempty"`
}

type PlanStep struct {
	ID      string `json:"id,omitempty"`
	Title   string `json:"title,omitempty"`
	Status  string `json:"status,omitempty"`
	Details string `json:"details,omitempty"`
}

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
