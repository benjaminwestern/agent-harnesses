// Package court provides Court runtime functionality.
package court

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/benjaminwestern/agentic-control/internal/orchestration"
	"github.com/benjaminwestern/agentic-control/pkg/contract"
)

// ListRuntimeRequests provides Court runtime functionality.
func (e *Engine) ListRuntimeRequests(ctx context.Context, runID string, status RuntimeRequestStatus) ([]RuntimeRequest, error) {
	if _, err := e.store.GetRun(ctx, runID); err != nil {
		return nil, err
	}
	return e.store.ListRuntimeRequests(ctx, runID, status)
}

// RespondToRuntimeRequest provides Court runtime functionality.
func (e *Engine) RespondToRuntimeRequest(ctx context.Context, id int64, response RuntimeRequestResponse) (RuntimeRequestResponseResult, error) {
	request, err := e.store.GetRuntimeRequest(ctx, id)
	if err != nil {
		return RuntimeRequestResponseResult{}, err
	}
	if request.Status != RuntimeRequestOpen {
		return RuntimeRequestResponseResult{}, fmt.Errorf("runtime request %d is %s; only open requests can be responded to", id, request.Status)
	}
	if response.Action == "" && response.Text == "" && response.OptionID == "" && len(response.Answers) == 0 {
		response.Action = string(contract.RespondActionAllow)
	}
	if err := e.store.QueueRuntimeRequestResponse(ctx, id, response); err != nil {
		return RuntimeRequestResponseResult{}, err
	}
	updated, _ := e.store.GetRuntimeRequest(ctx, id)
	e.emit(ctx, request.RunID, request.WorkerID, "runtime_request.response_queued", fmt.Sprintf("queued response for runtime request %s", request.RequestID), "")
	return RuntimeRequestResponseResult{
		Request: updated,
		Message: "runtime request response queued",
	}, nil
}

func (e *Engine) persistRuntimeRequestEvent(ctx context.Context, run Run, worker Worker, event contract.RuntimeEvent) {
	request, ok := runtimeRequestFromEvent(run, worker, event)
	if !ok {
		return
	}
	_ = e.store.UpsertRuntimeRequest(ctx, request)
}

func runtimeRequestFromEvent(run Run, worker Worker, event contract.RuntimeEvent) (RuntimeRequest, bool) {
	if !contract.IsRequestEvent(event) {
		return RuntimeRequest{}, false
	}

	now := time.Now()
	status := RuntimeRequestStatus(contract.RequestStatusFromEvent(event))
	requestID := event.RequestID
	kind := ""
	nativeMethod := event.NativeEventName
	summary := event.Summary
	turnID := event.TurnID
	var requestJSON string
	if event.Request != nil {
		requestID = event.Request.RequestID
		kind = string(event.Request.Kind)
		nativeMethod = event.Request.NativeMethod
		status = RuntimeRequestStatus(event.Request.Status)
		summary = firstNonEmpty(event.Request.Summary, summary)
		turnID = firstNonEmpty(event.Request.TurnID, turnID)
		if encoded, err := json.Marshal(event.Request); err == nil {
			requestJSON = string(encoded)
		}
	}
	if requestID == "" {
		return RuntimeRequest{}, false
	}
	if status == "" {
		status = RuntimeRequestOpen
	}

	return RuntimeRequest{
		RunID:                    run.ID,
		WorkerID:                 worker.ID,
		RequestID:                requestID,
		RuntimeSessionID:         event.SessionID,
		RuntimeProviderSessionID: event.ProviderSessionID,
		Runtime:                  event.Runtime,
		Kind:                     kind,
		NativeMethod:             nativeMethod,
		Status:                   status,
		Summary:                  summary,
		TurnID:                   turnID,
		RequestJSON:              requestJSON,
		CreatedAt:                now,
		UpdatedAt:                now,
	}, true
}

func (e *Engine) handleQueuedRuntimeRequestResponses(ctx context.Context, run Run, worker Worker, sessionID string) error {
	requests, err := e.store.ListQueuedRuntimeRequestResponses(ctx, worker.ID)
	if err != nil {
		return err
	}
	queued := make([]orchestration.QueuedRuntimeResponse, 0, len(requests))
	for _, request := range requests {
		queued = append(queued, orchestration.QueuedRuntimeResponse{
			ID:        request.ID,
			RequestID: request.RequestID,
			Action:    contract.RespondAction(request.ResponseAction),
			Text:      request.ResponseText,
			OptionID:  request.ResponseOptionID,
			Answers:   contractAnswers(request.ResponseAnswersJSON),
			Metadata: map[string]any{
				"court_run_id":     run.ID,
				"court_worker_id":  worker.ID,
				"court_request_id": request.ID,
			},
		})
	}
	return orchestration.FlushQueuedRuntimeResponses(ctx, e.controlPlane, sessionID, queued, orchestration.RuntimeResponseHooks{
		Complete: func(response orchestration.QueuedRuntimeResponse, event *contract.RuntimeEvent, actionErr error) error {
			if actionErr != nil {
				_ = e.store.CompleteRuntimeRequestResponse(ctx, response.ID, RuntimeResponseFailed, "", actionErr.Error())
				return nil
			}
			_ = e.store.CompleteRuntimeRequestResponse(ctx, response.ID, RuntimeResponseCompleted, eventPayload(event), "")
			if event != nil {
				e.persistRuntimeRequestEvent(ctx, run, worker, *event)
			}
			e.emit(ctx, run.ID, worker.ID, "runtime_request.responded", fmt.Sprintf("responded to runtime request %s", response.RequestID), eventPayload(event))
			return nil
		},
	})
}

func contractAnswers(value string) []contract.RequestAnswer {
	if value == "" {
		return nil
	}
	var answers []RuntimeRequestAnswer
	if err := json.Unmarshal([]byte(value), &answers); err != nil {
		return nil
	}
	out := make([]contract.RequestAnswer, 0, len(answers))
	for _, answer := range answers {
		out = append(out, contract.RequestAnswer{
			QuestionID: answer.QuestionID,
			OptionID:   answer.OptionID,
			Text:       answer.Text,
		})
	}
	return out
}
