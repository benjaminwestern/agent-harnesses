package opencode

import (
	"testing"
	"time"

	"github.com/benjaminwestern/agentic-control/pkg/contract"
)

func TestSessionIdentifiesActivePromptMessage(t *testing.T) {
	sess := newSession(nil, "launch-test", "provider-test", t.TempDir(), "test", "openai/gpt-5.4", remoteTimeRange{})
	sess.setActiveTurnID("msg-user")

	if !sess.isActivePromptMessage("msg-user") {
		t.Fatal("active prompt message was not identified")
	}
	if sess.isActivePromptMessage("msg-assistant") {
		t.Fatal("assistant message was incorrectly treated as the prompt echo")
	}
}

func TestSessionCanHaveStaleRunningStatusWithoutActiveTurn(t *testing.T) {
	sess := newSession(nil, "launch-test", "provider-test", t.TempDir(), "test", "openai/gpt-5.4", remoteTimeRange{})
	sess.setActiveTurnID("msg-user")
	sess.clearActiveTurnID()
	sess.setStatus("running")

	if sess.activeTurnIDSnapshot() != "" {
		t.Fatal("active turn was not cleared")
	}
	if sess.statusSnapshot() != "running" {
		t.Fatal("test did not create a stale running status")
	}
	if sess.pendingRequestCount() != 0 {
		t.Fatal("new session should not have pending requests")
	}
}

func TestCompletedTurnIsNotReopenedByLateMessageUpdate(t *testing.T) {
	p := NewProvider(func(contract.RuntimeEvent) {})
	sess := newSession(p, "launch-test", "provider-test", t.TempDir(), "test", "openai/gpt-5.4", remoteTimeRange{})
	p.storeSession(sess)
	sess.setActiveTurnID("turn-1")
	sess.setStatus(contract.SessionRunning)
	p.completeTurnFromAssistantMessage(sess, "turn-1")

	p.handleMessageUpdated(remoteMessage{
		ID:        "msg-late",
		SessionID: "provider-test",
		Role:      "assistant",
		ParentID:  "turn-1",
	})

	if sess.activeTurnIDSnapshot() != "" {
		t.Fatalf("late message update reopened completed turn: %q", sess.activeTurnIDSnapshot())
	}
}

func TestAssistantMessageCompletionWithToolActivityIsDebounced(t *testing.T) {
	var events []contract.RuntimeEvent
	p := NewProvider(func(event contract.RuntimeEvent) {
		events = append(events, event)
	})
	sess := newSession(p, "launch-test", "provider-test", t.TempDir(), "test", "openai/gpt-5.4", remoteTimeRange{})
	p.storeSession(sess)
	sess.setActiveTurnID("turn-1")
	sess.storeMessageTurn("msg-assistant", "turn-1")
	sess.setStatus(contract.SessionRunning)

	p.handleToolPartUpdated(sess, "turn-1", remotePart{
		ID:        "part-tool",
		SessionID: "provider-test",
		MessageID: "msg-assistant",
		Type:      "tool",
		Tool:      "grep",
		State:     &remoteToolState{Status: "completed"},
	})
	message := remoteMessage{
		ID:        "msg-assistant",
		SessionID: "provider-test",
		Role:      "assistant",
		ParentID:  "turn-1",
	}
	message.Time.Completed = time.Now().UnixMilli()
	p.handleMessageUpdated(message)

	if sess.activeTurnIDSnapshot() != "turn-1" {
		t.Fatalf("tool-using turn completed before idle: %q", sess.activeTurnIDSnapshot())
	}
	if countEvents(events, contract.EventTurnCompleted) != 0 {
		t.Fatalf("tool-using message update emitted immediate turn completion: %+v", events)
	}

	time.Sleep(toolCompletionDebounceDelay + 100*time.Millisecond)

	if sess.activeTurnIDSnapshot() != "" {
		t.Fatalf("debounce did not complete turn: %q", sess.activeTurnIDSnapshot())
	}
	if countEvents(events, contract.EventTurnCompleted) != 1 {
		t.Fatalf("debounce did not emit exactly one turn completion: %+v", events)
	}
}

func TestOpenCodeAgentNameMapsCommonSurfaceAliases(t *testing.T) {
	tests := map[string]string{
		"default_readonly":  "explore",
		"default_reviewer":  "build",
		"review_reporter":   "build",
		"security_reporter": "build",
		"plan":              "plan",
	}
	for input, want := range tests {
		if got := opencodeAgentName(input); got != want {
			t.Fatalf("opencodeAgentName(%q) = %q, want %q", input, got, want)
		}
	}
}

func countEvents(events []contract.RuntimeEvent, eventType string) int {
	var count int
	for _, event := range events {
		if event.EventType == eventType {
			count++
		}
	}
	return count
}
