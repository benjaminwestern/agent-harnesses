package orchestration

import "context"

type WorkerExecutionHooks[RunT any, WorkerT any, IdentityT any] struct {
	Load             func(context.Context, string) (WorkerT, RunT, error)
	IsTerminal       func(WorkerT) bool
	MarkRunning      func(context.Context, string) error
	OnStarted        func(context.Context, RunT, WorkerT)
	Execute          func(context.Context, RunT, WorkerT) (string, string, IdentityT, error)
	HasIdentity      func(IdentityT) bool
	PersistIdentity  func(context.Context, string, IdentityT) error
	IsCancelledError func(error) bool
	MarkCancelled    func(context.Context, string, string, string) error
	OnCancelled      func(context.Context, RunT, WorkerT, string, string)
	MarkFailed       func(context.Context, string, string, string, string) error
	OnFailed         func(context.Context, RunT, WorkerT, string, string, string)
	Reload           func(context.Context, string) (WorkerT, error)
	IsCancelled      func(WorkerT) bool
	MarkCompleted    func(context.Context, string, string, string) error
	OnCompleted      func(context.Context, RunT, WorkerT, string, string)
	Reconcile        func(context.Context, RunT) error
}

func ExecuteWorker[RunT any, WorkerT any, IdentityT any](ctx context.Context, workerID string, hooks WorkerExecutionHooks[RunT, WorkerT, IdentityT]) error {
	worker, run, err := hooks.Load(ctx, workerID)
	if err != nil {
		return err
	}
	if hooks.IsTerminal != nil && hooks.IsTerminal(worker) {
		return nil
	}
	if hooks.MarkRunning != nil {
		if err := hooks.MarkRunning(ctx, workerID); err != nil {
			return err
		}
	}
	if hooks.OnStarted != nil {
		hooks.OnStarted(ctx, run, worker)
	}

	result, resultJSON, identity, runErr := hooks.Execute(ctx, run, worker)
	if hooks.HasIdentity != nil && hooks.HasIdentity(identity) && hooks.PersistIdentity != nil {
		_ = hooks.PersistIdentity(ctx, workerID, identity)
	}

	if hooks.IsCancelledError != nil && hooks.IsCancelledError(runErr) {
		if hooks.MarkCancelled != nil {
			_ = hooks.MarkCancelled(ctx, workerID, result, resultJSON)
		}
		if hooks.OnCancelled != nil {
			hooks.OnCancelled(ctx, run, worker, result, resultJSON)
		}
		if hooks.Reconcile != nil {
			_ = hooks.Reconcile(ctx, run)
		}
		return nil
	}
	if runErr != nil {
		errText := runErr.Error()
		if hooks.MarkFailed != nil {
			_ = hooks.MarkFailed(ctx, workerID, result, resultJSON, errText)
		}
		if hooks.OnFailed != nil {
			hooks.OnFailed(ctx, run, worker, result, resultJSON, errText)
		}
		if hooks.Reconcile != nil {
			_ = hooks.Reconcile(ctx, run)
		}
		return runErr
	}
	if hooks.Reload != nil && hooks.IsCancelled != nil {
		if current, err := hooks.Reload(ctx, workerID); err == nil && hooks.IsCancelled(current) {
			if hooks.OnCancelled != nil {
				hooks.OnCancelled(ctx, run, worker, result, resultJSON)
			}
			if hooks.Reconcile != nil {
				_ = hooks.Reconcile(ctx, run)
			}
			return nil
		}
	}
	if hooks.MarkCompleted != nil {
		if err := hooks.MarkCompleted(ctx, workerID, result, resultJSON); err != nil {
			return err
		}
	}
	if hooks.OnCompleted != nil {
		hooks.OnCompleted(ctx, run, worker, result, resultJSON)
	}
	if hooks.Reconcile != nil {
		return hooks.Reconcile(ctx, run)
	}
	return nil
}
