package controlplane

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/benjaminwestern/agentic-control/pkg/contract"
)

func TestRunStructuredSessionRepairsMissingResult(t *testing.T) {
	controller := newFakeStructuredController()
	var missing int
	var eventCount int

	result, err := RunStructuredSession(context.Background(), controller, "fake", StartSessionRequest{
		SessionID: "session-1",
		Prompt:    "work",
		Metadata:  map[string]any{"model": "test/model"},
	}, StructuredSessionOptions{
		TickEvery:      time.Millisecond,
		Extract:        fakeStructuredExtractor,
		RepairPrompt:   "repair",
		RepairMetadata: MetadataWithDisabledTools(map[string]any{MetadataKeyModel: "test/model"}, "read", "bash"),
		MaxRepairTurns: 1,
		OnMissingStructuredResult: func(context.Context) error {
			missing++
			return nil
		},
		OnEvent: func(context.Context, contract.RuntimeEvent) error {
			eventCount++
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Repaired || result.JSON != `{"ok":true}` || missing != 1 || eventCount == 0 {
		t.Fatalf("unexpected result: %+v missing=%d eventCount=%d", result, missing, eventCount)
	}
	if len(controller.sentInputs) != 1 {
		t.Fatalf("repair turn not sent: %+v", controller.sentInputs)
	}
	tools, _ := controller.sentInputs[0].Metadata[MetadataKeyTools].(map[string]any)
	if tools["read"] != false || tools["bash"] != false {
		t.Fatalf("repair tools not disabled: %+v", controller.sentInputs[0].Metadata)
	}
}

func TestRunStructuredSessionReturnsMissingAfterRepairLimit(t *testing.T) {
	controller := newFakeStructuredController()
	controller.repairOutput = "still plain"

	_, err := RunStructuredSession(context.Background(), controller, "fake", StartSessionRequest{
		SessionID: "session-1",
		Prompt:    "work",
	}, StructuredSessionOptions{
		TickEvery:      time.Millisecond,
		Extract:        fakeStructuredExtractor,
		RepairPrompt:   "repair",
		MaxRepairTurns: 1,
	})
	if !errors.Is(err, ErrMissingStructuredResult) {
		t.Fatalf("expected missing structured result, got %v", err)
	}
}

func TestRunStructuredSessionPrefersTerminalFinalResult(t *testing.T) {
	controller := newFakeStructuredController()
	controller.startOutput = "plain"
	controller.startFinalResult = `{"ok":true}`

	result, err := RunStructuredSession(context.Background(), controller, "fake", StartSessionRequest{
		SessionID: "session-1",
		Prompt:    "work",
	}, StructuredSessionOptions{
		TickEvery:      time.Millisecond,
		Extract:        fakeStructuredExtractor,
		RepairPrompt:   "repair",
		MaxRepairTurns: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Repaired || result.JSON != `{"ok":true}` || len(controller.sentInputs) != 0 {
		t.Fatalf("terminal final result was not used directly: %+v sent=%+v", result, controller.sentInputs)
	}
}

type fakeStructuredController struct {
	events           chan contract.RuntimeEvent
	sentInputs       []SendInputRequest
	startOutput      string
	startFinalResult string
	repairOutput     string
}

func newFakeStructuredController() *fakeStructuredController {
	return &fakeStructuredController{
		events:       make(chan contract.RuntimeEvent, 16),
		repairOutput: `{"ok":true}`,
	}
}

func (f *fakeStructuredController) SubscribeEvents(int) (<-chan contract.RuntimeEvent, func()) {
	return f.events, func() {}
}

func (f *fakeStructuredController) StartSession(context.Context, string, StartSessionRequest) (*contract.RuntimeSession, error) {
	session := &contract.RuntimeSession{
		SchemaVersion: contract.ControlPlaneSchemaVersion,
		SessionID:     "session-1",
		Status:        contract.SessionRunning,
	}
	output := f.startOutput
	if output == "" {
		output = "plain"
	}
	f.events <- contract.NewRuntimeEvent(*session, contract.EventAssistantMessageDelta, "fake.delta", "turn-1", output, map[string]any{"delta": output})
	payload := map[string]any(nil)
	if f.startFinalResult != "" {
		payload = map[string]any{"final_structured_result": f.startFinalResult}
	}
	f.events <- contract.NewRuntimeEvent(*session, contract.EventTurnCompleted, "fake.done", "turn-1", "done", payload)
	return session, nil
}

func (f *fakeStructuredController) SendInput(_ context.Context, request SendInputRequest) (*contract.RuntimeEvent, error) {
	f.sentInputs = append(f.sentInputs, request)
	session := contract.RuntimeSession{
		SchemaVersion: contract.ControlPlaneSchemaVersion,
		SessionID:     request.SessionID,
		Status:        contract.SessionRunning,
	}
	f.events <- contract.NewRuntimeEvent(session, contract.EventAssistantMessageDelta, "fake.delta", "turn-2", f.repairOutput, map[string]any{"delta": f.repairOutput})
	event := contract.NewRuntimeEvent(session, contract.EventTurnCompleted, "fake.done", "turn-2", "done", nil)
	f.events <- event
	return &event, nil
}

func fakeStructuredExtractor(values ...string) (string, string) {
	for _, value := range values {
		if value == `{"ok":true}` {
			return "ok", value
		}
	}
	return "", ""
}
