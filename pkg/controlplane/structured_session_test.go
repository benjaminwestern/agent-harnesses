package controlplane

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/benjaminwestern/agentic-control/pkg/contract"
)

type fakeStructuredController struct {
	events chan contract.RuntimeEvent

	mu           sync.Mutex
	startRequest StartSessionRequest
	sentInputs   []SendInputRequest

	onStart func()
	onSend  func(SendInputRequest)
}

func newFakeStructuredController() *fakeStructuredController {
	return &fakeStructuredController{events: make(chan contract.RuntimeEvent, 8)}
}

func (f *fakeStructuredController) SubscribeEvents(int) (<-chan contract.RuntimeEvent, func()) {
	return f.events, func() {}
}

func (f *fakeStructuredController) StartSession(_ context.Context, runtime string, request StartSessionRequest) (*contract.RuntimeSession, error) {
	f.mu.Lock()
	f.startRequest = request
	f.mu.Unlock()
	if f.onStart != nil {
		go f.onStart()
	}
	return &contract.RuntimeSession{
		SchemaVersion: contract.ControlPlaneSchemaVersion,
		SessionID:     request.SessionID,
		Runtime:       runtime,
		Status:        contract.SessionRunning,
	}, nil
}

func (f *fakeStructuredController) SendInput(_ context.Context, request SendInputRequest) (*contract.RuntimeEvent, error) {
	f.mu.Lock()
	f.sentInputs = append(f.sentInputs, request)
	f.mu.Unlock()
	if f.onSend != nil {
		go f.onSend(request)
	}
	event := contract.RuntimeEvent{SessionID: request.SessionID, EventType: contract.EventTurnStarted}
	return &event, nil
}

func (f *fakeStructuredController) emit(event contract.RuntimeEvent) {
	f.events <- event
}

func (f *fakeStructuredController) startedRequest() StartSessionRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.startRequest
}

func (f *fakeStructuredController) inputs() []SendInputRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]SendInputRequest, len(f.sentInputs))
	copy(out, f.sentInputs)
	return out
}

func extractFirstJSON(values ...string) (string, string, error) {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value, value, nil
		}
	}
	return "", "", ErrMissingStructuredResult
}

func TestRunStructuredSessionMirrorsSchemaAndCapturesLogprobs(t *testing.T) {
	controller := newFakeStructuredController()
	logprobs := []contract.TokenLogprob{{
		Token:   "5",
		Logprob: -0.1,
		TopLogprobs: []contract.TokenLogprob{
			{Token: "5", Logprob: -0.1},
			{Token: "4", Logprob: -2},
		},
	}}
	controller.onStart = func() {
		controller.emit(contract.RuntimeEvent{
			SchemaVersion: contract.ControlPlaneSchemaVersion,
			Runtime:       "test",
			SessionID:     "structured-1",
			EventType:     contract.EventTurnCompleted,
			Summary:       `{"score":5}`,
			Payload: map[string]any{
				"final_structured_result": `{"score":5}`,
				"logprobs":                logprobs,
			},
		})
	}

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"score": map[string]any{"type": "number"},
		},
		"required": []string{"score"},
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	result, err := RunStructuredSession(ctx, controller, "test", StartSessionRequest{
		SessionID: "structured-1",
		ModelOptions: ModelOptions{
			ResponseSchema: schema,
		},
	}, StructuredSessionOptions{
		Extract: extractFirstJSON,
	})
	if err != nil {
		t.Fatalf("RunStructuredSession failed: %v", err)
	}
	if got := controller.startedRequest(); got.ResponseSchema == nil || got.ModelOptions.ResponseSchema == nil {
		t.Fatalf("schema was not mirrored before StartSession: %#v", got)
	}
	if len(result.Logprobs) != 1 || result.Logprobs[0].Token != "5" {
		t.Fatalf("result logprobs = %#v, want token 5", result.Logprobs)
	}
}

func TestRunStructuredSessionRepairPromptIncludesValidationError(t *testing.T) {
	controller := newFakeStructuredController()
	controller.onStart = func() {
		controller.emit(contract.RuntimeEvent{
			SchemaVersion: contract.ControlPlaneSchemaVersion,
			Runtime:       "test",
			SessionID:     "structured-2",
			EventType:     contract.EventTurnCompleted,
			Summary:       `{"score":"bad"}`,
			Payload:       map[string]any{"final_structured_result": `{"score":"bad"}`},
		})
	}
	controller.onSend = func(request SendInputRequest) {
		controller.emit(contract.RuntimeEvent{
			SchemaVersion: contract.ControlPlaneSchemaVersion,
			Runtime:       "test",
			SessionID:     request.SessionID,
			EventType:     contract.EventTurnCompleted,
			Summary:       `{"score":4}`,
			Payload:       map[string]any{"final_structured_result": `{"score":4}`},
		})
	}

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"score": map[string]any{"type": "number"},
		},
		"required": []string{"score"},
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err := RunStructuredSession(ctx, controller, "test", StartSessionRequest{
		SessionID:      "structured-2",
		ResponseSchema: schema,
	}, StructuredSessionOptions{
		Extract:        extractFirstJSON,
		RepairPrompt:   "Return valid JSON.",
		MaxRepairTurns: 1,
	})
	if err != nil {
		t.Fatalf("RunStructuredSession failed: %v", err)
	}
	inputs := controller.inputs()
	if len(inputs) != 1 {
		t.Fatalf("repair input count = %d, want 1", len(inputs))
	}
	if !strings.Contains(inputs[0].Text, "JSON does not match required schema") {
		t.Fatalf("repair prompt did not include validation error:\n%s", inputs[0].Text)
	}
}
