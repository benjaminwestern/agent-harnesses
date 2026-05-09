package controlplane

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/benjaminwestern/agentic-control/pkg/contract"
	"github.com/santhosh-tekuri/jsonschema/v5"
)

var ErrMissingStructuredResult = errors.New("structured result not found")

type StructuredResultExtractor func(values ...string) (rendered string, normalisedJSON string, err error)

type StructuredSessionController interface {
	SubscribeEvents(buffer int) (<-chan contract.RuntimeEvent, func())
	StartSession(context.Context, string, StartSessionRequest) (*contract.RuntimeSession, error)
	SendInput(context.Context, SendInputRequest) (*contract.RuntimeEvent, error)
}

type StructuredSessionOptions struct {
	EventBuffer int
	TickEvery   time.Duration

	Extract StructuredResultExtractor

	RepairPrompt   string
	RepairMetadata map[string]any
	MaxRepairTurns int

	OnSessionStarted          func(context.Context, *contract.RuntimeSession) error
	OnTick                    func(context.Context, string) error
	OnEvent                   func(context.Context, contract.RuntimeEvent) error
	OnTurnEvents              func(context.Context, string) error
	OnMissingStructuredResult func(context.Context) error
}

type StructuredSessionResult struct {
	Session       *contract.RuntimeSession
	Text          string
	JSON          string
	Repaired      bool
	RepairAttempt int
	Logprobs      []contract.TokenLogprob
}

func RunStructuredSession(
	ctx context.Context,
	controller StructuredSessionController,
	runtime string,
	request StartSessionRequest,
	options StructuredSessionOptions,
) (StructuredSessionResult, error) {
	if options.Extract == nil {
		return StructuredSessionResult{}, errors.New("structured result extractor is required")
	}
	request.Normalize()
	buffer := options.EventBuffer
	if buffer <= 0 {
		buffer = 512
	}
	events, unsubscribe := controller.SubscribeEvents(buffer)
	defer unsubscribe()

	session, err := controller.StartSession(ctx, runtime, request)
	if err != nil {
		return StructuredSessionResult{}, err
	}
	result := StructuredSessionResult{Session: session}
	if options.OnSessionStarted != nil {
		if err := options.OnSessionStarted(ctx, session); err != nil {
			return result, err
		}
	}

	for attempt := 0; ; attempt++ {
		turn, err := waitForStructuredTurn(ctx, session.SessionID, events, options, request)
		result.Text = turn.Text
		result.JSON = turn.JSON
		result.Logprobs = turn.Logprobs
		result.RepairAttempt = attempt
		if err == nil {
			result.Repaired = attempt > 0
			return result, nil
		}
		if !errors.Is(err, ErrMissingStructuredResult) {
			return result, err
		}
		if attempt >= options.MaxRepairTurns || options.RepairPrompt == "" {
			return result, err
		}

		repairPrompt := options.RepairPrompt
		if errText := strings.TrimSpace(err.Error()); errText != "" {
			repairPrompt = fmt.Sprintf("%s\n\nLast validation error:\n%s", repairPrompt, errText)
		}
		if request.ResponseSchema != nil {
			schemaBytes, err := json.MarshalIndent(request.ResponseSchema, "", "  ")
			if err == nil {
				repairPrompt = fmt.Sprintf("%s\n\nTarget Schema:\n```json\n%s\n```", repairPrompt, string(schemaBytes))
			}
		}

		if options.OnMissingStructuredResult != nil {
			if hookErr := options.OnMissingStructuredResult(ctx); hookErr != nil {
				return result, hookErr
			}
		}
		if _, sendErr := controller.SendInput(ctx, SendInputRequest{
			SessionID: session.SessionID,
			Text:      repairPrompt,
			Metadata:  options.RepairMetadata,
		}); sendErr != nil {
			return result, fmt.Errorf("structured result repair turn: %w", sendErr)
		}
	}
}

type structuredTurnResult struct {
	Text     string
	JSON     string
	Logprobs []contract.TokenLogprob
}

func waitForStructuredTurn(
	ctx context.Context,
	sessionID string,
	events <-chan contract.RuntimeEvent,
	options StructuredSessionOptions,
	request StartSessionRequest,
) (structuredTurnResult, error) {
	var turn TurnAccumulator
	tickEvery := options.TickEvery
	if tickEvery <= 0 {
		tickEvery = time.Second
	}
	ticker := time.NewTicker(tickEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return structuredTurnResult{Text: turn.JoinedDelta()}, ctx.Err()
		case <-ticker.C:
			if options.OnTick != nil {
				if err := options.OnTick(ctx, sessionID); err != nil {
					return structuredTurnResult{Text: turn.JoinedDelta()}, err
				}
			}
		case event, ok := <-events:
			if !ok {
				return structuredTurnResult{Text: turn.JoinedDelta()}, errors.New("control plane event stream closed")
			}
			if event.SessionID != sessionID {
				continue
			}
			if options.OnEvent != nil {
				if err := options.OnEvent(ctx, event); err != nil {
					return structuredTurnResult{Text: turn.JoinedDelta()}, err
				}
			}
			turn.Add(event)
			switch {
			case contract.IsTurnCompletedEvent(event):
				if turn.HasEvents() && options.OnTurnEvents != nil {
					if err := options.OnTurnEvents(ctx, turn.EventsJSONL()); err != nil {
						return structuredTurnResult{Text: turn.JoinedDelta()}, err
					}
				}
				rendered, resultJSON, extractErr := options.Extract(turn.FinalText, turn.LatestDelta, turn.JoinedDelta(), event.Summary)

				if resultJSON != "" && extractErr == nil && request.ResponseSchema != nil {
					schemaCompiler := jsonschema.NewCompiler()
					schemaCompiler.Draft = jsonschema.Draft7
					schemaBytes, _ := json.Marshal(request.ResponseSchema)
					if err := schemaCompiler.AddResource("schema.json", strings.NewReader(string(schemaBytes))); err == nil {
						if schema, err := schemaCompiler.Compile("schema.json"); err == nil {
							var parsedCandidate any
							if err := json.Unmarshal([]byte(resultJSON), &parsedCandidate); err == nil {
								if validationErr := schema.Validate(parsedCandidate); validationErr != nil {
									extractErr = fmt.Errorf("JSON does not match required schema: %v", validationErr)
									resultJSON = ""
								}
							}
						}
					}
				}

				if resultJSON == "" || extractErr != nil {
					if extractErr != nil {
						return structuredTurnResult{
							Text: "control plane completed without the required structured result or the structured result was invalid. Schema Validation failed.",
						}, fmt.Errorf("%w: %v", ErrMissingStructuredResult, extractErr)
					}
					return structuredTurnResult{
						Text: "control plane completed without the required structured result. Runtime events are stored as artifacts.",
					}, ErrMissingStructuredResult
				}
				return structuredTurnResult{Text: rendered, JSON: resultJSON, Logprobs: turn.Logprobs}, nil
			case contract.IsTurnErroredEvent(event):
				if turn.HasEvents() && options.OnTurnEvents != nil {
					if err := options.OnTurnEvents(ctx, turn.EventsJSONL()); err != nil {
						return structuredTurnResult{Text: turn.JoinedDelta()}, err
					}
				}
				return structuredTurnResult{Text: turn.JoinedDelta()}, fmt.Errorf("control plane session failed: %s", contract.EventErrorText(event))
			}
		}
	}
}
