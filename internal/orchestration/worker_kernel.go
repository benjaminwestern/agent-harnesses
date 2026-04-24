package orchestration

import "context"

type WorkerStatusSummary struct {
	HasFailed          bool
	HasCancelled       bool
	HasRunningOrQueued bool
}

func SummarizeWorkerStatuses[T any](workers []T, status func(T) WorkerStatus) WorkerStatusSummary {
	var summary WorkerStatusSummary
	for _, worker := range workers {
		switch status(worker) {
		case WorkerFailed:
			summary.HasFailed = true
		case WorkerCancelled:
			summary.HasCancelled = true
		case WorkerQueued, WorkerRunning:
			summary.HasRunningOrQueued = true
		}
	}
	return summary
}

type QueueAndLaunchHooks[T any] struct {
	Persist        func(context.Context, T) error
	OnPersistError func(context.Context, T, error) (skip bool, err error)
	AfterPersist   func(context.Context, T) error
	Launch         func(context.Context, T) error
	OnLaunchError  func(context.Context, T, error) error
}

func QueueAndLaunch[T any](ctx context.Context, items []T, hooks QueueAndLaunchHooks[T]) error {
	for _, item := range items {
		if hooks.Persist != nil {
			if err := hooks.Persist(ctx, item); err != nil {
				if hooks.OnPersistError == nil {
					return err
				}
				skip, nextErr := hooks.OnPersistError(ctx, item, err)
				if nextErr != nil {
					return nextErr
				}
				if skip {
					continue
				}
			}
		}
		if hooks.AfterPersist != nil {
			if err := hooks.AfterPersist(ctx, item); err != nil {
				return err
			}
		}
		if hooks.Launch != nil {
			if err := hooks.Launch(ctx, item); err != nil {
				if hooks.OnLaunchError != nil {
					if nextErr := hooks.OnLaunchError(ctx, item, err); nextErr != nil {
						return nextErr
					}
					continue
				}
				return err
			}
		}
	}
	return nil
}
