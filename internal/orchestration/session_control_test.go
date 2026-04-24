package orchestration

import (
	"context"
	"errors"
	"testing"

	"github.com/benjaminwestern/agentic-control/pkg/contract"
	api "github.com/benjaminwestern/agentic-control/pkg/controlplane"
)

type testSessionController struct {
	stopCount      int
	interruptCount int
	respondCount   int
	stopErr        error
	interruptErr   error
	respondErr     error
}

func (t *testSessionController) StopSession(context.Context, string) (*contract.RuntimeEvent, error) {
	t.stopCount++
	return &contract.RuntimeEvent{EventType: contract.EventSessionStopped}, t.stopErr
}

func (t *testSessionController) Interrupt(context.Context, string) (*contract.RuntimeEvent, error) {
	t.interruptCount++
	return &contract.RuntimeEvent{EventType: contract.EventTurnInterrupted}, t.interruptErr
}

func (t *testSessionController) Respond(context.Context, api.RespondRequest) (*contract.RuntimeEvent, error) {
	t.respondCount++
	return &contract.RuntimeEvent{EventType: contract.EventRequestResponded}, t.respondErr
}

func TestHandlePendingSessionControlsCancelStopsSession(t *testing.T) {
	controller := &testSessionController{}
	result, err := HandlePendingSessionControls(context.Background(), controller, "session-1", []PendingSessionControl{{ID: 1, Action: PendingSessionControlCancel}}, SessionControlHooks{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Cancelled {
		t.Fatal("expected cancelled result")
	}
	if controller.stopCount != 1 {
		t.Fatalf("stop count = %d, want 1", controller.stopCount)
	}
}

func TestHandlePendingSessionControlsInterruptFailureIsBestEffort(t *testing.T) {
	controller := &testSessionController{interruptErr: errors.New("boom")}
	result, err := HandlePendingSessionControls(context.Background(), controller, "session-1", []PendingSessionControl{{ID: 1, Action: PendingSessionControlInterrupt}}, SessionControlHooks{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) != 1 {
		t.Fatalf("error count = %d, want 1", len(result.Errors))
	}
	if controller.interruptCount != 1 {
		t.Fatalf("interrupt count = %d, want 1", controller.interruptCount)
	}
}

func TestFlushQueuedRuntimeResponses(t *testing.T) {
	controller := &testSessionController{}
	err := FlushQueuedRuntimeResponses(context.Background(), controller, "session-1", []QueuedRuntimeResponse{{ID: 1, RequestID: "req-1", Action: contract.RespondActionAllow}}, RuntimeResponseHooks{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if controller.respondCount != 1 {
		t.Fatalf("respond count = %d, want 1", controller.respondCount)
	}
}
