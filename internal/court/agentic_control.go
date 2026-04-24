// Package court provides Court runtime functionality.
package court

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/benjaminwestern/agentic-control/internal/orchestration"
	"github.com/benjaminwestern/agentic-control/pkg/contract"
	api "github.com/benjaminwestern/agentic-control/pkg/controlplane"
	"github.com/benjaminwestern/agentic-control/pkg/features"
)

// RuntimeControlPlane defines the runtime boundary Court depends on.
type RuntimeControlPlane interface {
	Describe() contract.SystemDescriptor
	SubscribeEvents(buffer int) (<-chan contract.RuntimeEvent, func())
	StartSession(context.Context, string, api.StartSessionRequest) (*contract.RuntimeSession, error)
	ResumeSession(context.Context, string, api.ResumeSessionRequest) (*contract.RuntimeSession, error)
	SendInput(context.Context, api.SendInputRequest) (*contract.RuntimeEvent, error)
	Interrupt(context.Context, string) (*contract.RuntimeEvent, error)
	Respond(context.Context, api.RespondRequest) (*contract.RuntimeEvent, error)
	StopSession(context.Context, string) (*contract.RuntimeEvent, error)
	ListSessions(context.Context, string) ([]contract.RuntimeSession, error)
	GetTrackedSession(context.Context, string, string) (*contract.TrackedSession, error)
	GetThread(context.Context, string, string) (*contract.TrackedThread, error)
	SetThreadMetadata(context.Context, string, contract.ThreadMetadata) error
}

var errMissingStructuredResult = api.ErrMissingStructuredResult

func (e *Engine) runAgenticControlWorker(ctx context.Context, run Run, worker Worker) (string, string, RuntimeIdentity, error) {
	if features.Check(features.ExperimentalUI) {
		// Example feature flag check
	}
	role, err := e.roleForWorker(run, worker.RoleID)
	if err != nil {
		return "", "", RuntimeIdentity{}, err
	}
	prompt, err := e.buildWorkerPrompt(run, worker, role)
	if err != nil {
		return "", "", RuntimeIdentity{}, err
	}

	var identity RuntimeIdentity
	result, err := api.RunStructuredSession(ctx, e.controlPlane, worker.Backend, api.StartSessionRequest{
		SessionID:    worker.LaunchID,
		CWD:          run.Workspace,
		Model:        worker.Model,
		ModelOptions: worker.ModelOptions,
		Prompt:       prompt,
		Metadata:     workerRuntimeMetadata(run, worker),
	}, api.StructuredSessionOptions{
		Extract:        firstStructuredResult,
		RepairPrompt:   structuredResultRepairPrompt(worker),
		RepairMetadata: workerRepairRuntimeMetadata(run, worker),
		MaxRepairTurns: 1,
		OnSessionStarted: func(ctx context.Context, session *contract.RuntimeSession) error {
			identity = RuntimeIdentity{
				SessionID:         session.SessionID,
				ProviderSessionID: session.ProviderSessionID,
			}
			return e.store.UpdateWorkerRuntimeIdentity(ctx, worker.ID, identity)
		},
		OnTick: func(ctx context.Context, sessionID string) error {
			return e.handleAgenticControlWorkerTick(ctx, run, worker, sessionID)
		},
		OnEvent: func(ctx context.Context, event contract.RuntimeEvent) error {
			if contract.IsRequestEvent(event) {
				e.persistRuntimeRequestEvent(ctx, run, worker, event)
			}
			return nil
		},
		OnTurnEvents: func(ctx context.Context, eventsJSONL string) error {
			e.addArtifact(ctx, run.ID, worker.ID, "runtime_events", "jsonl", eventsJSONL)
			return nil
		},
		OnMissingStructuredResult: func(ctx context.Context) error {
			e.emit(ctx, run.ID, worker.ID, "worker.result_repair", "worker completed without structured JSON; requesting repair turn", "")
			return nil
		},
	})
	if err != nil {
		if errors.Is(err, errMissingStructuredResult) {
			return result.Text, result.JSON, identity, errors.New("worker did not return required structured JSON result")
		}
		return result.Text, result.JSON, identity, fmt.Errorf("agentic-control structured %s session: %w", worker.Backend, err)
	}
	return result.Text, result.JSON, identity, nil
}

func (e *Engine) handleAgenticControlWorkerTick(
	ctx context.Context,
	run Run,
	worker Worker,
	sessionID string,
) error {
	current, err := e.store.GetWorker(ctx, worker.ID)
	if err != nil {
		return err
	}
	if err := e.handlePendingWorkerControls(ctx, run, current, sessionID); err != nil {
		if err == errWorkerCancelled {
			return err
		}
		e.emit(ctx, run.ID, worker.ID, "worker.control_failed", err.Error(), "")
	}
	if err := e.handleQueuedRuntimeRequestResponses(ctx, run, current, sessionID); err != nil {
		e.emit(ctx, run.ID, worker.ID, "runtime_request.response_failed", err.Error(), "")
	}
	if current.Status == WorkerCancelled {
		_, _ = e.controlPlane.StopSession(ctx, sessionID)
		return errWorkerCancelled
	}
	return nil
}

func structuredResultRepairPrompt(worker Worker) string {
	var b strings.Builder
	b.WriteString("Your previous turn completed without the required Court worker result JSON.\n")
	b.WriteString("Do not use tools. Return only one JSON object with no markdown fence and no surrounding prose.\n")
	b.WriteString("Use concrete values from the work you just performed as ")
	b.WriteString(worker.RoleTitle)
	b.WriteString(". Do not return placeholders such as \"...\" or angle-bracket examples.\n")
	b.WriteString("Each JSON key must appear exactly once. Arrays must contain strings only; do not repeat keys inside arrays.\n")
	b.WriteString("Required shape:\n")
	b.WriteString(WorkerResultSchemaExample())
	b.WriteString("\n")
	return b.String()
}

func (e *Engine) ensureControlPlaneSession(ctx context.Context, run Run, worker Worker) (*contract.RuntimeSession, error) {
	sessionID := firstNonEmpty(worker.RuntimeSessionID, worker.LaunchID)
	session, err := api.AdoptOrResumeSession(ctx, e.controlPlane, worker.Backend, api.ResumeSessionRequest{
		SessionID:         sessionID,
		ProviderSessionID: worker.RuntimeProviderSessionID,
		CWD:               run.Workspace,
		Model:             worker.Model,
		ModelOptions:      worker.ModelOptions,
		Metadata:          workerRuntimeMetadata(run, worker),
	})
	if err != nil {
		return nil, fmt.Errorf("agentic-control resume %s session for worker %s: %w", worker.Backend, worker.ID, err)
	}
	identity := RuntimeIdentity{
		SessionID:         session.SessionID,
		ProviderSessionID: session.ProviderSessionID,
	}
	_ = e.store.UpdateWorkerRuntimeIdentity(ctx, worker.ID, identity)
	return session, nil
}

func workerRuntimeMetadata(run Run, worker Worker) map[string]any {
	return api.RuntimeMetadata{
		Title:    fmt.Sprintf("court %s %s", run.ID, worker.RoleID),
		System:   courtWorkerSystemPrompt(),
		Agent:    worker.Agent,
		Model:    worker.Model,
		Provider: worker.Provider,
		Labels: map[string]string{
			"thread_name":     fmt.Sprintf("court %s %s", run.ID, worker.RoleTitle),
			"thread_kind":     "court_worker",
			"workflow":        "court",
			"workflow_mode":   string(run.Workflow),
			"court_agent":     worker.Agent,
			"court_run_id":    run.ID,
			"court_worker_id": worker.ID,
			"court_launch_id": worker.LaunchID,
			"court_role_id":   worker.RoleID,
			"court_role_kind": string(worker.RoleKind),
			"court_backend":   worker.Backend,
		},
	}.Map()
}

func workerRepairRuntimeMetadata(run Run, worker Worker) map[string]any {
	return api.MetadataForNoToolTurn(workerRuntimeMetadata(run, worker))
}

func firstStructuredResult(values ...string) (string, string) {
	var fallback string
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		if fallback == "" {
			fallback = value
		}
		result, resultJSON := readResultFromOutput(value)
		if resultJSON != "" {
			return result, resultJSON
		}
	}
	return strings.TrimSpace(fallback), ""
}

func (e *Engine) handlePendingWorkerControls(ctx context.Context, run Run, worker Worker, sessionID string) error {
	controls, err := e.store.ListPendingWorkerControls(ctx, worker.ID)
	if err != nil {
		return err
	}
	pending := make([]orchestration.PendingSessionControl, 0, len(controls))
	for _, control := range controls {
		switch control.Action {
		case WorkerControlCancel:
			pending = append(pending, orchestration.PendingSessionControl{ID: control.ID, Action: orchestration.PendingSessionControlCancel})
		case WorkerControlInterrupt:
			pending = append(pending, orchestration.PendingSessionControl{ID: control.ID, Action: orchestration.PendingSessionControlInterrupt})
		}
	}
	result, err := orchestration.HandlePendingSessionControls(ctx, e.controlPlane, sessionID, pending, orchestration.SessionControlHooks{
		Complete: func(control orchestration.PendingSessionControl, event *contract.RuntimeEvent, actionErr error) error {
			switch control.Action {
			case orchestration.PendingSessionControlCancel:
				if actionErr != nil {
					_ = e.store.CompleteWorkerControl(ctx, control.ID, WorkerControlFailed, actionErr.Error())
					e.emit(ctx, run.ID, worker.ID, "worker.cancel_failed", actionErr.Error(), "")
					return nil
				}
				_ = e.store.CompleteWorkerControl(ctx, control.ID, WorkerControlCompleted, "")
				e.emit(ctx, run.ID, worker.ID, "worker.cancelled", "worker cancelled", eventPayload(event))
			case orchestration.PendingSessionControlInterrupt:
				if actionErr != nil {
					_ = e.store.CompleteWorkerControl(ctx, control.ID, WorkerControlFailed, actionErr.Error())
					e.emit(ctx, run.ID, worker.ID, "worker.interrupt_failed", actionErr.Error(), "")
					return nil
				}
				_ = e.store.CompleteWorkerControl(ctx, control.ID, WorkerControlCompleted, "")
				e.emit(ctx, run.ID, worker.ID, "worker.interrupted", "worker interruption requested", eventPayload(event))
			}
			return nil
		},
	})
	if err != nil {
		return err
	}
	if result.Cancelled {
		return errWorkerCancelled
	}
	return nil
}

func eventPayload(event *contract.RuntimeEvent) string {
	if event == nil {
		return ""
	}
	data, err := json.Marshal(event)
	if err != nil {
		return ""
	}
	return string(data)
}
