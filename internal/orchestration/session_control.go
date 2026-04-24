package orchestration

import (
	"context"
	"fmt"

	"github.com/benjaminwestern/agentic-control/pkg/contract"
	api "github.com/benjaminwestern/agentic-control/pkg/controlplane"
)

type SessionController interface {
	StopSession(context.Context, string) (*contract.RuntimeEvent, error)
	Interrupt(context.Context, string) (*contract.RuntimeEvent, error)
	Respond(context.Context, api.RespondRequest) (*contract.RuntimeEvent, error)
}

type PendingSessionControlAction string

const (
	PendingSessionControlCancel    PendingSessionControlAction = "cancel"
	PendingSessionControlInterrupt PendingSessionControlAction = "interrupt"
)

type PendingSessionControl struct {
	ID     int64
	Action PendingSessionControlAction
}

type SessionControlHooks struct {
	Complete func(PendingSessionControl, *contract.RuntimeEvent, error) error
}

type SessionControlResult struct {
	Cancelled bool
	Errors    []error
}

type QueuedRuntimeResponse struct {
	ID        int64
	RequestID string
	Action    contract.RespondAction
	Text      string
	OptionID  string
	Answers   []contract.RequestAnswer
	Metadata  map[string]any
}

type RuntimeResponseHooks struct {
	Complete func(QueuedRuntimeResponse, *contract.RuntimeEvent, error) error
}

func HandlePendingSessionControls(
	ctx context.Context,
	controller SessionController,
	sessionID string,
	controls []PendingSessionControl,
	hooks SessionControlHooks,
) (SessionControlResult, error) {
	var result SessionControlResult
	for _, control := range controls {
		switch control.Action {
		case PendingSessionControlCancel:
			event, err := controller.StopSession(ctx, sessionID)
			if hooks.Complete != nil {
				_ = hooks.Complete(control, event, err)
			}
			if err != nil {
				result.Errors = append(result.Errors, err)
				return result, fmt.Errorf("stop runtime session: %w", err)
			}
			result.Cancelled = true
			return result, nil
		case PendingSessionControlInterrupt:
			event, err := controller.Interrupt(ctx, sessionID)
			if hooks.Complete != nil {
				_ = hooks.Complete(control, event, err)
			}
			if err != nil {
				result.Errors = append(result.Errors, err)
			}
		}
	}
	return result, nil
}

func FlushQueuedRuntimeResponses(
	ctx context.Context,
	controller SessionController,
	sessionID string,
	responses []QueuedRuntimeResponse,
	hooks RuntimeResponseHooks,
) error {
	for _, response := range responses {
		event, err := controller.Respond(ctx, api.RespondRequest{
			SessionID: sessionID,
			RequestID: response.RequestID,
			Action:    response.Action,
			Text:      response.Text,
			OptionID:  response.OptionID,
			Answers:   response.Answers,
			Metadata:  response.Metadata,
		})
		if hooks.Complete != nil {
			_ = hooks.Complete(response, event, err)
		}
		if err != nil {
			return fmt.Errorf("respond to runtime request: %w", err)
		}
	}
	return nil
}
