package contract

import (
	"encoding/json"
	"fmt"
	"strconv"
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
	if event.NativeEventName == "message.part.updated" {
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

func EventMode(event RuntimeEvent) string {
	if event.SessionState != nil && strings.TrimSpace(event.SessionState.Mode) != "" {
		return strings.TrimSpace(event.SessionState.Mode)
	}
	return firstNonEmpty(
		EventPayloadString(event, "mode"),
		EventPayloadString(event, "current_mode"),
		EventPayloadString(event, "current_mode_id"),
	)
}

func EventTokenUsage(event RuntimeEvent) (TokenUsage, bool) {
	if event.SessionState != nil {
		if usage, ok := tokenUsageIfSet(event.SessionState.Usage); ok {
			return usage, true
		}
	}
	if usage, ok := tokenUsageFromValue(event.Payload["usage"]); ok {
		return usage, true
	}
	if usage, ok := tokenUsageFromValue(event.Payload["token_count"]); ok {
		return usage, true
	}
	if usage, ok := tokenUsageFromPath(event.Payload, "meta", "quota", "token_count"); ok {
		return usage, true
	}
	if usage, ok := tokenUsageFromPath(event.Payload, "meta", "token_count"); ok {
		return usage, true
	}
	if usage, ok := tokenUsageFromPath(event.Payload, "tokenUsage", "total"); ok {
		return usage, true
	}
	if usage, ok := tokenUsageFromPath(event.Payload, "tokenUsage", "last"); ok {
		return usage, true
	}
	return tokenUsageFromValue(event.Payload)
}

func EventCostUSD(event RuntimeEvent) (float64, bool) {
	if event.SessionState != nil && event.SessionState.CostUSD > 0 {
		return event.SessionState.CostUSD, true
	}
	if cost, ok := float64Value(event.Payload["cost"]); ok {
		return cost, true
	}
	if cost, ok := float64Value(event.Payload["cost_usd"]); ok {
		return cost, true
	}
	if cost, ok := float64ValueFromPath(event.Payload, "meta", "quota", "cost"); ok {
		return cost, true
	}
	if cost, ok := float64ValueFromPath(event.Payload, "meta", "cost"); ok {
		return cost, true
	}
	return 0, false
}

func tokenUsageFromPath(root map[string]any, keys ...string) (TokenUsage, bool) {
	var current any = root
	for _, key := range keys {
		values, ok := current.(map[string]any)
		if !ok {
			return TokenUsage{}, false
		}
		current = values[key]
	}
	return tokenUsageFromValue(current)
}

func float64ValueFromPath(root map[string]any, keys ...string) (float64, bool) {
	var current any = root
	for _, key := range keys {
		values, ok := current.(map[string]any)
		if !ok {
			return 0, false
		}
		current = values[key]
	}
	return float64Value(current)
}

func tokenUsageIfSet(usage TokenUsage) (TokenUsage, bool) {
	if usage != (TokenUsage{}) {
		return usage, true
	}
	return TokenUsage{}, false
}

func tokenUsageFromValue(value any) (TokenUsage, bool) {
	if value == nil {
		return TokenUsage{}, false
	}
	if usage, ok := value.(TokenUsage); ok {
		return tokenUsageIfSet(usage)
	}
	values, ok := value.(map[string]any)
	if !ok {
		return TokenUsage{}, false
	}
	var usage TokenUsage
	seen := false
	if usage.InputTokens, ok = int64Value(values["input_tokens"]); ok {
		seen = true
	} else if usage.InputTokens, ok = int64Value(values["inputTokens"]); ok {
		seen = true
	}
	if usage.OutputTokens, ok = int64Value(values["output_tokens"]); ok {
		seen = true
	} else if usage.OutputTokens, ok = int64Value(values["outputTokens"]); ok {
		seen = true
	}
	if usage.ReasoningTokens, ok = int64Value(values["reasoning_tokens"]); ok {
		seen = true
	} else if usage.ReasoningTokens, ok = int64Value(values["reasoningOutputTokens"]); ok {
		seen = true
	}
	if usage.CachedTokens, ok = int64Value(values["cached_tokens"]); ok {
		seen = true
	} else if usage.CachedTokens, ok = int64Value(values["cachedInputTokens"]); ok {
		seen = true
	}
	if usage.TotalTokens, ok = int64Value(values["total_tokens"]); ok {
		seen = true
	} else if usage.TotalTokens, ok = int64Value(values["totalTokens"]); ok {
		seen = true
	}
	if !seen {
		if used, ok := int64Value(values["used"]); ok {
			usage.TotalTokens = used
			seen = true
		}
	}
	if !seen {
		return TokenUsage{}, false
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.InputTokens + usage.OutputTokens + usage.ReasoningTokens
	}
	return usage, true
}

func int64Value(value any) (int64, bool) {
	switch typed := value.(type) {
	case int:
		return int64(typed), true
	case int32:
		return int64(typed), true
	case int64:
		return typed, true
	case float32:
		return int64(typed), true
	case float64:
		return int64(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func float64Value(value any) (float64, bool) {
	switch typed := value.(type) {
	case float32:
		return float64(typed), true
	case float64:
		return typed, true
	case int:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return parsed, err == nil
	default:
		return 0, false
	}
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
