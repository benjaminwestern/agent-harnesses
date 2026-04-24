// Package court provides Court runtime functionality.
package court

import (
	"context"
	"fmt"
)

// CancelWorker provides Court runtime functionality.
func (e *Engine) CancelWorker(ctx context.Context, workerID string) (WorkerControlResult, error) {
	worker, run, err := e.workerAndRun(ctx, workerID)
	if err != nil {
		return WorkerControlResult{}, err
	}
	if worker.Status == WorkerCompleted || worker.Status == WorkerFailed || worker.Status == WorkerCancelled {
		return WorkerControlResult{
			Worker:                   worker,
			Action:                   WorkerControlCancel,
			Status:                   WorkerControlCompleted,
			RuntimeSessionID:         worker.RuntimeSessionID,
			RuntimeProviderSessionID: worker.RuntimeProviderSessionID,
			Message:                  fmt.Sprintf("worker is already %s", worker.Status),
		}, nil
	}

	control, err := e.store.AddWorkerControl(ctx, run.ID, worker.ID, WorkerControlCancel)
	if err != nil {
		return WorkerControlResult{}, err
	}

	message := "worker cancellation requested"
	status := WorkerControlCompleted
	var stopErr error
	if worker.RuntimeSessionID != "" || worker.RuntimeProviderSessionID != "" {
		var sessionID string
		sessionID, stopErr = e.stopWorkerRuntime(ctx, run, worker)
		if stopErr == nil {
			worker.RuntimeSessionID = firstNonEmpty(sessionID, worker.RuntimeSessionID)
			message = "worker runtime stopped"
		} else if worker.Status == WorkerRunning {
			status = WorkerControlPending
			message = "worker cancellation persisted; running worker will stop itself on its next control poll"
		} else {
			status = WorkerControlFailed
			message = stopErr.Error()
		}
	}
	if status != WorkerControlPending {
		errText := ""
		if stopErr != nil {
			errText = stopErr.Error()
		}
		_ = e.store.CompleteWorkerControl(ctx, control.ID, status, errText)
	}

	_ = e.store.CompleteWorker(ctx, worker.ID, WorkerCancelled, worker.Result, worker.ResultJSON, "worker cancelled")
	e.emit(ctx, run.ID, worker.ID, "worker.cancelled", message, "")
	_ = e.store.UpdateRunStatus(ctx, run.ID, RunCancelled, "")
	_ = e.store.UpdateRunPhase(ctx, run.ID, PhaseBlocked)
	e.emit(ctx, run.ID, "", "run.cancelled", "run cancelled by worker control", "")

	updated, _ := e.store.GetWorker(ctx, worker.ID)
	result := WorkerControlResult{
		Worker:                   updated,
		Action:                   WorkerControlCancel,
		Status:                   status,
		RuntimeSessionID:         updated.RuntimeSessionID,
		RuntimeProviderSessionID: updated.RuntimeProviderSessionID,
		Message:                  message,
	}
	if stopErr != nil && status == WorkerControlFailed {
		result.Error = stopErr.Error()
		return result, stopErr
	}
	return result, nil
}

// InterruptWorker provides Court runtime functionality.
func (e *Engine) InterruptWorker(ctx context.Context, workerID string) (WorkerControlResult, error) {
	worker, run, err := e.workerAndRun(ctx, workerID)
	if err != nil {
		return WorkerControlResult{}, err
	}
	if worker.Status != WorkerRunning {
		return WorkerControlResult{}, fmt.Errorf("worker %s is %s; only running workers can be interrupted", worker.ID, worker.Status)
	}
	control, err := e.store.AddWorkerControl(ctx, run.ID, worker.ID, WorkerControlInterrupt)
	if err != nil {
		return WorkerControlResult{}, err
	}

	status := WorkerControlCompleted
	message := "worker interruption requested"
	var interruptErr error
	if worker.RuntimeSessionID == "" && worker.RuntimeProviderSessionID == "" {
		status = WorkerControlPending
		message = "worker interruption persisted; running worker will handle it after session identity is available"
	} else {
		var session *RuntimeIdentity
		session, interruptErr = e.interruptWorkerRuntime(ctx, run, worker)
		if interruptErr == nil && session != nil {
			worker.RuntimeSessionID = firstNonEmpty(session.SessionID, worker.RuntimeSessionID)
			worker.RuntimeProviderSessionID = firstNonEmpty(session.ProviderSessionID, worker.RuntimeProviderSessionID)
		}
	}
	if interruptErr != nil {
		status = WorkerControlPending
		message = "worker interruption persisted; running worker will handle it on its next control poll"
	} else {
		_ = e.store.CompleteWorkerControl(ctx, control.ID, WorkerControlCompleted, "")
		e.emit(ctx, run.ID, worker.ID, "worker.interrupted", message, "")
	}

	updated, _ := e.store.GetWorker(ctx, worker.ID)
	return WorkerControlResult{
		Worker:                   updated,
		Action:                   WorkerControlInterrupt,
		Status:                   status,
		RuntimeSessionID:         updated.RuntimeSessionID,
		RuntimeProviderSessionID: updated.RuntimeProviderSessionID,
		Message:                  message,
	}, nil
}

// ResumeWorker provides Court runtime functionality.
func (e *Engine) ResumeWorker(ctx context.Context, workerID string) (WorkerControlResult, error) {
	worker, run, err := e.workerAndRun(ctx, workerID)
	if err != nil {
		return WorkerControlResult{}, err
	}
	control, err := e.store.AddWorkerControl(ctx, run.ID, worker.ID, WorkerControlResume)
	if err != nil {
		return WorkerControlResult{}, err
	}
	session, err := e.ensureControlPlaneSession(ctx, run, worker)
	if err != nil {
		_ = e.store.CompleteWorkerControl(ctx, control.ID, WorkerControlFailed, err.Error())
		return WorkerControlResult{}, err
	}
	_ = e.store.CompleteWorkerControl(ctx, control.ID, WorkerControlCompleted, "")
	e.emit(ctx, run.ID, worker.ID, "worker.resumed", "worker runtime session adopted", "")

	updated, _ := e.store.GetWorker(ctx, worker.ID)
	return WorkerControlResult{
		Worker:                   updated,
		Action:                   WorkerControlResume,
		Status:                   WorkerControlCompleted,
		RuntimeSessionID:         session.SessionID,
		RuntimeProviderSessionID: session.ProviderSessionID,
		Message:                  "worker runtime session adopted",
	}, nil
}

// RetryWorker provides Court runtime functionality.
func (e *Engine) RetryWorker(ctx context.Context, workerID string) (WorkerControlResult, error) {
	worker, run, err := e.workerAndRun(ctx, workerID)
	if err != nil {
		return WorkerControlResult{}, err
	}
	if worker.Status != WorkerFailed && worker.Status != WorkerCancelled {
		return WorkerControlResult{}, fmt.Errorf("worker %s is %s; only failed or cancelled workers can be retried", worker.ID, worker.Status)
	}
	_ = e.store.CompletePendingWorkerControls(ctx, worker.ID, WorkerControlFailed, "superseded by retry")
	control, err := e.store.AddWorkerControl(ctx, run.ID, worker.ID, WorkerControlRetry)
	if err != nil {
		return WorkerControlResult{}, err
	}

	if err := e.store.ArchiveWorkerAttempt(ctx, worker); err != nil {
		_ = e.store.CompleteWorkerControl(ctx, control.ID, WorkerControlFailed, err.Error())
		return WorkerControlResult{}, err
	}
	launchID := "launch-" + randomID()
	if err := e.store.ResetWorkerForRetry(ctx, worker.ID, launchID); err != nil {
		_ = e.store.CompleteWorkerControl(ctx, control.ID, WorkerControlFailed, err.Error())
		return WorkerControlResult{}, err
	}
	if err := e.store.ReactivateRun(ctx, run.ID, retryPhaseForWorker(worker)); err != nil {
		_ = e.store.CompleteWorkerControl(ctx, control.ID, WorkerControlFailed, err.Error())
		return WorkerControlResult{}, err
	}
	_ = e.store.CompleteWorkerControl(ctx, control.ID, WorkerControlCompleted, "")
	e.emit(ctx, run.ID, worker.ID, "worker.retried", "worker retry queued", "")
	e.emit(ctx, run.ID, worker.ID, "worker.queued", fmt.Sprintf("queued %s", worker.RoleTitle), "")
	if err := e.spawnWorker(ctx, worker.ID); err != nil {
		_ = e.store.CompleteWorker(ctx, worker.ID, WorkerFailed, "", "", err.Error())
		_ = e.store.UpdateRunStatus(ctx, run.ID, RunFailed, "")
		_ = e.store.UpdateRunPhase(ctx, run.ID, PhaseBlocked)
		_ = e.store.CompleteWorkerControl(ctx, control.ID, WorkerControlFailed, err.Error())
		return WorkerControlResult{}, err
	}

	updated, _ := e.store.GetWorker(ctx, worker.ID)
	return WorkerControlResult{
		Worker:                   updated,
		Action:                   WorkerControlRetry,
		Status:                   WorkerControlCompleted,
		RuntimeSessionID:         updated.RuntimeSessionID,
		RuntimeProviderSessionID: updated.RuntimeProviderSessionID,
		Message:                  "worker retry queued",
	}, nil
}

func (e *Engine) workerAndRun(ctx context.Context, workerID string) (Worker, Run, error) {
	worker, err := e.store.GetWorker(ctx, workerID)
	if err != nil {
		return Worker{}, Run{}, err
	}
	run, err := e.store.GetRun(ctx, worker.RunID)
	if err != nil {
		return Worker{}, Run{}, err
	}
	return worker, run, nil
}

func (e *Engine) stopWorkerRuntime(ctx context.Context, run Run, worker Worker) (string, error) {
	session, err := e.ensureControlPlaneSession(ctx, run, worker)
	if err != nil {
		return "", err
	}
	if _, err := e.controlPlane.StopSession(ctx, session.SessionID); err != nil {
		return session.SessionID, wrapErr("stop runtime session", err)
	}
	return session.SessionID, nil
}

func (e *Engine) interruptWorkerRuntime(ctx context.Context, run Run, worker Worker) (*RuntimeIdentity, error) {
	session, err := e.ensureControlPlaneSession(ctx, run, worker)
	if err != nil {
		return nil, err
	}
	if _, err := e.controlPlane.Interrupt(ctx, session.SessionID); err != nil {
		return nil, wrapErr("interrupt runtime session", err)
	}
	return &RuntimeIdentity{SessionID: session.SessionID, ProviderSessionID: session.ProviderSessionID}, nil
}

func retryPhaseForWorker(worker Worker) Phase {
	switch worker.RoleKind {
	case RoleClerk:
		return PhaseClerk
	case RoleJuror:
		return PhaseJurors
	case RoleJudge:
		return PhaseVerdict
	default:
		return PhaseBlocked
	}
}
