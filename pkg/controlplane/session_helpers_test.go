package controlplane

import (
	"context"
	"strings"
	"testing"

	"github.com/benjaminwestern/agentic-control/pkg/contract"
)

func TestFindSessionByIDOrProviderID(t *testing.T) {
	sessions := []contract.RuntimeSession{
		{SessionID: "session-1", ProviderSessionID: "provider-1"},
		{SessionID: "session-2", ProviderSessionID: "provider-2"},
	}
	session, ok := FindSessionByIDOrProviderID(sessions, "", "provider-2")
	if !ok || session.SessionID != "session-2" {
		t.Fatalf("session = %+v ok=%v", session, ok)
	}
	session, ok = FindSessionByIDOrProviderID(sessions, "session-1", "")
	if !ok || session.ProviderSessionID != "provider-1" {
		t.Fatalf("session = %+v ok=%v", session, ok)
	}
}

func TestAdoptOrResumeSession(t *testing.T) {
	controller := &fakeSessionController{
		sessions: []contract.RuntimeSession{{SessionID: "session-1", ProviderSessionID: "provider-1"}},
	}
	session, err := AdoptOrResumeSession(context.Background(), controller, "codex", ResumeSessionRequest{
		SessionID:         "session-1",
		ProviderSessionID: "provider-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if session.SessionID != "session-1" || controller.resumed {
		t.Fatalf("expected adopted session, got %+v resumed=%v", session, controller.resumed)
	}

	session, err = AdoptOrResumeSession(context.Background(), controller, "codex", ResumeSessionRequest{
		SessionID:         "session-2",
		ProviderSessionID: "provider-2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if session.SessionID != "session-2" || !controller.resumed {
		t.Fatalf("expected resumed session, got %+v resumed=%v", session, controller.resumed)
	}
}

func TestAdoptOrResumeSessionRequiresProviderSessionID(t *testing.T) {
	_, err := AdoptOrResumeSession(context.Background(), &fakeSessionController{}, "codex", ResumeSessionRequest{SessionID: "session-1"})
	if err == nil || !strings.Contains(err.Error(), "provider_session_id is required") {
		t.Fatalf("expected provider id error, got %v", err)
	}
}

func TestTurnAccumulator(t *testing.T) {
	var turn TurnAccumulator
	turn.Add(contract.RuntimeEvent{
		EventType: contract.EventAssistantMessageDelta,
		Payload:   map[string]any{"delta": "hello "},
	})
	turn.Add(contract.RuntimeEvent{
		EventType: contract.EventAssistantMessageDelta,
		Payload:   map[string]any{"delta": "world"},
	})
	if got := turn.JoinedDelta(); got != "hello world" {
		t.Fatalf("joined delta = %q", got)
	}
	if turn.LatestDelta != "world" || !turn.HasEvents() || !strings.Contains(turn.EventsJSONL(), contract.EventAssistantMessageDelta) {
		t.Fatalf("unexpected turn state: latest=%q events=%q", turn.LatestDelta, turn.EventsJSONL())
	}
}

type fakeSessionController struct {
	sessions []contract.RuntimeSession
	resumed  bool
}

func (f *fakeSessionController) ListSessions(context.Context, string) ([]contract.RuntimeSession, error) {
	return f.sessions, nil
}

func (f *fakeSessionController) ResumeSession(_ context.Context, runtime string, request ResumeSessionRequest) (*contract.RuntimeSession, error) {
	f.resumed = true
	return &contract.RuntimeSession{
		SessionID:         request.SessionID,
		ProviderSessionID: request.ProviderSessionID,
		Runtime:           runtime,
	}, nil
}
