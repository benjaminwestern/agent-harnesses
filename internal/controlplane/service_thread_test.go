package controlplane

import (
	"testing"
	"time"

	"github.com/benjaminwestern/agentic-control/pkg/contract"
)

func TestPublishEventToolRepairBuffersAreScopedBySessionAndTurn(t *testing.T) {
	service := NewService()
	events, unsubscribe := service.SubscribeEvents(8)
	defer unsubscribe()

	service.PublishEvent(contract.RuntimeEvent{
		Runtime:   "test",
		SessionID: "session-a",
		TurnID:    "turn-1",
		EventType: contract.EventAssistantMessageDelta,
		Payload:   map[string]any{"delta": `{"name":"first","arguments":{"x":1}}`},
	})
	service.PublishEvent(contract.RuntimeEvent{
		Runtime:   "test",
		SessionID: "session-b",
		TurnID:    "turn-1",
		EventType: contract.EventAssistantMessageDelta,
		Payload:   map[string]any{"delta": "plain text"},
	})
	service.PublishEvent(contract.RuntimeEvent{
		Runtime:   "test",
		SessionID: "session-b",
		TurnID:    "turn-1",
		EventType: contract.EventTurnCompleted,
		Payload:   map[string]any{},
	})
	service.PublishEvent(contract.RuntimeEvent{
		Runtime:   "test",
		SessionID: "session-a",
		TurnID:    "turn-1",
		EventType: contract.EventTurnCompleted,
		Payload:   map[string]any{},
	})

	var completedA, completedB contract.RuntimeEvent
	deadline := time.After(time.Second)
	for completedA.EventType == "" || completedB.EventType == "" {
		select {
		case event := <-events:
			if event.EventType != contract.EventTurnCompleted {
				continue
			}
			switch event.SessionID {
			case "session-a":
				completedA = event
			case "session-b":
				completedB = event
			}
		case <-deadline:
			t.Fatal("timeout waiting for completed events")
		}
	}

	if _, ok := completedB.Payload[contract.PayloadExtractedTools]; ok {
		t.Fatalf("session-b completed event leaked session-a tools: %#v", completedB.Payload)
	}
	tools, ok := completedA.Payload[contract.PayloadExtractedTools].([]contract.ToolCall)
	if !ok || len(tools) != 1 || tools[0].Function.Name != "first" {
		t.Fatalf("session-a extracted tools = %#v, want first tool", completedA.Payload[contract.PayloadExtractedTools])
	}
}

func TestDeriveThreadTurnsPreservesInputPartsToolMessagesAndReasoningOrder(t *testing.T) {
	events := []contract.ThreadEvent{
		{
			ThreadID:     "thread-1",
			RecordedAtMS: 1,
			EventType:    contract.EventTurnStarted,
			Summary:      "Started turn: fallback",
			TurnID:       "turn-1",
			Event: contract.RuntimeEvent{
				EventType: contract.EventTurnStarted,
				Payload: map[string]any{
					contract.PayloadInputText: "look",
					contract.PayloadInputParts: []contract.ContentPart{
						{Type: contract.ContentPartTypeImage, URL: "https://example.com/image.png"},
					},
				},
			},
		},
		{
			ThreadID:     "thread-1",
			RecordedAtMS: 2,
			EventType:    contract.EventAssistantThoughtDelta,
			TurnID:       "turn-1",
			Event: contract.RuntimeEvent{
				EventType: contract.EventAssistantThoughtDelta,
				Payload:   map[string]any{"delta": "thinking"},
			},
		},
		{
			ThreadID:     "thread-1",
			RecordedAtMS: 3,
			EventType:    contract.EventAssistantMessageDelta,
			TurnID:       "turn-1",
			Event: contract.RuntimeEvent{
				EventType: contract.EventAssistantMessageDelta,
				Payload:   map[string]any{"delta": "answer"},
			},
		},
		{
			ThreadID:     "thread-1",
			RecordedAtMS: 4,
			EventType:    contract.EventToolCompleted,
			Summary:      "Tool completed",
			TurnID:       "turn-1",
			RequestID:    "tool-1",
			Event: contract.RuntimeEvent{
				EventType: contract.EventToolCompleted,
				RequestID: "tool-1",
				Payload: map[string]any{
					"name":   "search",
					"result": "ok",
				},
			},
		},
		{
			ThreadID:     "thread-1",
			RecordedAtMS: 5,
			EventType:    contract.EventTurnCompleted,
			Summary:      "done",
			TurnID:       "turn-1",
			Event: contract.RuntimeEvent{
				EventType: contract.EventTurnCompleted,
				Payload: map[string]any{
					contract.PayloadExtractedTools: []contract.ToolCall{{
						ID:   "tool-1",
						Type: "function",
						Function: contract.FunctionCall{
							Name:      "search",
							Arguments: map[string]any{"q": "agent"},
						},
					}},
				},
			},
		},
	}

	turns := deriveThreadTurns(events)
	if len(turns) != 1 {
		t.Fatalf("turn count = %d, want 1", len(turns))
	}
	messages := turns[0].Messages
	if len(messages) != 3 {
		t.Fatalf("message count = %d, want 3: %#v", len(messages), messages)
	}
	if messages[0].Role != contract.MessageRoleUser || len(messages[0].Parts) != 2 {
		t.Fatalf("user message = %#v, want text plus image parts", messages[0])
	}
	if messages[0].Parts[0].Text != "look" || messages[0].Parts[1].URL != "https://example.com/image.png" {
		t.Fatalf("user parts = %#v", messages[0].Parts)
	}
	if messages[1].Role != contract.MessageRoleAssistant || len(messages[1].Parts) != 2 {
		t.Fatalf("assistant message = %#v, want reasoning plus text", messages[1])
	}
	if messages[1].Parts[0].Type != contract.ContentPartTypeReasoning || messages[1].Parts[1].Type != contract.ContentPartTypeText {
		t.Fatalf("assistant parts order = %#v", messages[1].Parts)
	}
	if len(messages[1].ToolCalls) != 1 || messages[1].ToolCalls[0].Function.Name != "search" {
		t.Fatalf("assistant tool calls = %#v", messages[1].ToolCalls)
	}
	if messages[2].Role != contract.MessageRoleTool || messages[2].ToolCallID != "tool-1" || messages[2].Parts[0].Text != "ok" {
		t.Fatalf("tool message = %#v, want tool result", messages[2])
	}
}

func TestDeriveThreadTurnsGroupsEventsWithoutProviderTurnID(t *testing.T) {
	events := []contract.ThreadEvent{
		{
			ID:           1,
			ThreadID:     "thread-blank",
			RecordedAtMS: 1,
			EventType:    contract.EventTurnStarted,
			Summary:      "Started turn: hello",
			Event: contract.RuntimeEvent{
				EventType: contract.EventTurnStarted,
				SessionID: "session-blank",
			},
		},
		{
			ID:           2,
			ThreadID:     "thread-blank",
			RecordedAtMS: 2,
			EventType:    contract.EventAssistantMessageDelta,
			Event: contract.RuntimeEvent{
				EventType: contract.EventAssistantMessageDelta,
				SessionID: "session-blank",
				Payload:   map[string]any{"delta": "world"},
			},
		},
		{
			ID:           3,
			ThreadID:     "thread-blank",
			RecordedAtMS: 3,
			EventType:    contract.EventTurnCompleted,
			Event: contract.RuntimeEvent{
				EventType: contract.EventTurnCompleted,
				SessionID: "session-blank",
				Payload: map[string]any{
					contract.PayloadExtractedTools: []contract.ToolCall{{
						ID:   "tool-blank",
						Type: "function",
						Function: contract.FunctionCall{
							Name:      "lookup",
							Arguments: map[string]any{"q": "x"},
						},
					}},
				},
			},
		},
	}

	turns := deriveThreadTurns(events)
	if len(turns) != 1 {
		t.Fatalf("turn count = %d, want 1", len(turns))
	}
	if turns[0].TurnID == "" {
		t.Fatal("synthetic turn ID is empty")
	}
	if turns[0].AssistantText != "world" {
		t.Fatalf("AssistantText = %q, want world", turns[0].AssistantText)
	}
	if len(turns[0].Messages) < 2 || len(turns[0].Messages[1].ToolCalls) != 1 {
		t.Fatalf("messages = %#v, want assistant tool call", turns[0].Messages)
	}
}
