package orchestration_test

import (
	"context"
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/benjaminwestern/agentic-control/internal/orchestration"
	"github.com/benjaminwestern/agentic-control/pkg/contract"
	api "github.com/benjaminwestern/agentic-control/pkg/controlplane"
)

type mockFanoutController struct {
	lastSessionID    string
	lastStartRequest api.StartSessionRequest
	emitLogprobs     bool
}

func (m *mockFanoutController) Describe() contract.SystemDescriptor {
	return contract.SystemDescriptor{
		Runtimes: []contract.RuntimeDescriptor{
			{
				Runtime: "opencode",
				Capabilities: contract.RuntimeCapabilities{
					StartSession: true,
					StreamEvents: true,
				},
				Probe: &contract.RuntimeProbe{Installed: true},
			},
		},
	}
}

func (m *mockFanoutController) StartSession(ctx context.Context, runtime string, req api.StartSessionRequest) (*contract.RuntimeSession, error) {
	m.lastSessionID = req.SessionID
	m.lastStartRequest = req
	return &contract.RuntimeSession{
		SessionID:         req.SessionID,
		ProviderSessionID: "mock-provider-sess",
		Runtime:           runtime,
		Model:             req.Model,
	}, nil
}

func (m *mockFanoutController) SendInput(ctx context.Context, req api.SendInputRequest) (*contract.RuntimeEvent, error) {
	return nil, nil
}

func (m *mockFanoutController) StopSession(ctx context.Context, sessionID string) (*contract.RuntimeEvent, error) {
	return nil, nil
}

func (m *mockFanoutController) GetTrackedSession(ctx context.Context, sessionID string, providerID string) (*contract.TrackedSession, error) {
	return &contract.TrackedSession{
		Session: contract.RuntimeSession{
			SessionID: sessionID,
			Usage: contract.TokenUsage{
				TotalTokens: 100,
			},
			CostUSD: 0.05,
		},
	}, nil
}

func (m *mockFanoutController) SubscribeEvents(buffer int) (<-chan contract.RuntimeEvent, func()) {
	ch := make(chan contract.RuntimeEvent, 10)
	go func() {
		time.Sleep(10 * time.Millisecond)
		payload := map[string]any{
			"delta": `{"score": 0.95, "rationale": "Matches perfectly", "passed": true}`,
		}
		if m.lastStartRequest.ModelOptions.Logprobs {
			payload["delta"] = "5"
			if m.emitLogprobs {
				payload["logprobs"] = []contract.TokenLogprob{{
					Token:   "5",
					Logprob: math.Log(0.75),
					TopLogprobs: []contract.TokenLogprob{
						{Token: "4", Logprob: math.Log(0.25)},
						{Token: "5", Logprob: math.Log(0.75)},
					},
				}}
			}
		}
		ch <- contract.RuntimeEvent{
			SessionID: m.lastSessionID,
			EventType: "assistant.message.delta",
			Payload:   payload,
		}
		ch <- contract.RuntimeEvent{
			SessionID: m.lastSessionID,
			EventType: contract.EventTurnCompleted,
		}
	}()
	return ch, func() {}
}

func TestRunReductionEvaluate(t *testing.T) {
	ctrl := &mockFanoutController{}

	fanout := orchestration.FanoutResult{
		Prompt: "Grade this output using a 0.0 to 1.0 scale.",
		Targets: []orchestration.FanoutTargetResult{
			{
				Target: orchestration.FanoutTarget{Backend: "opencode", Model: "gpt-4", Label: "candidate-1"},
			},
		},
		TotalUsage: contract.TokenUsage{TotalTokens: 100},
	}

	res, err := orchestration.RunReduction(context.Background(), ctrl, orchestration.ReductionModeEvaluate, fanout, orchestration.FanoutTarget{}, false)
	if err != nil {
		t.Fatalf("RunReduction failed: %v", err)
	}

	if res.RecordedCostUSD != 0.05 {
		t.Errorf("expected CostUSD 0.05, got %v", res.RecordedCostUSD)
	}

	if res.JSON == "" {
		t.Errorf("expected JSON output")
	}

	if !res.Stopped {
		t.Errorf("expected session to be stopped")
	}
}

func TestRunReductionGEval(t *testing.T) {
	ctrl := &mockFanoutController{emitLogprobs: true}

	fanout := orchestration.FanoutResult{
		Prompt: "Grade this output using a 1 to 5 scale.",
		Targets: []orchestration.FanoutTargetResult{
			{
				Target: orchestration.FanoutTarget{Backend: "opencode", Model: "gpt-4", Label: "candidate-1"},
				Text:   "I am a good output",
			},
		},
	}

	res, err := orchestration.RunReduction(context.Background(), ctrl, orchestration.ReductionModeGEval, fanout, orchestration.FanoutTarget{}, false)
	if err != nil {
		t.Fatalf("RunReduction failed: %v", err)
	}

	var resultJSON struct {
		Score     float64 `json:"score"`
		Rationale string  `json:"rationale"`
		Passed    bool    `json:"passed"`
	}
	if err := json.Unmarshal([]byte(res.JSON), &resultJSON); err != nil {
		t.Fatalf("Failed to parse GEval JSON: %v, raw: %s", err, res.JSON)
	}

	if resultJSON.Rationale != "G-Eval logarithmic probability evaluation" {
		t.Errorf("Expected GEval rationale")
	}
	if math.Abs(resultJSON.Score-4.75) > 0.0001 {
		t.Fatalf("GEval score = %v, want 4.75 from token logprobs", resultJSON.Score)
	}
}

func TestRunReductionGEvalDoesNotFallbackToTextScore(t *testing.T) {
	ctrl := &mockFanoutController{}

	fanout := orchestration.FanoutResult{
		Prompt: "Grade this output using a 1 to 5 scale.",
		Targets: []orchestration.FanoutTargetResult{
			{
				Target: orchestration.FanoutTarget{Backend: "opencode", Model: "gpt-4", Label: "candidate-1"},
				Text:   "I am a good output",
			},
		},
	}

	res, err := orchestration.RunReduction(context.Background(), ctrl, orchestration.ReductionModeGEval, fanout, orchestration.FanoutTarget{}, false)
	if err != nil {
		t.Fatalf("RunReduction failed: %v", err)
	}
	if res.Error == "" {
		t.Fatal("GEval without logprobs succeeded, want explicit error")
	}
	if res.JSON != "" {
		t.Fatalf("GEval JSON = %s, want empty JSON when logprobs are missing", res.JSON)
	}
}
