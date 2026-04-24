package orchestration

import (
	"context"
	"errors"
	"testing"
)

func TestExecuteWorkerCompleted(t *testing.T) {
	type run struct{ id string }
	type worker struct {
		id        string
		terminal  bool
		cancelled bool
	}
	type identity struct{ ok bool }

	var started, completed, reconciled bool
	err := ExecuteWorker(context.Background(), "w1", WorkerExecutionHooks[run, worker, identity]{
		Load: func(context.Context, string) (worker, run, error) {
			return worker{id: "w1"}, run{id: "r1"}, nil
		},
		IsTerminal:  func(worker) bool { return false },
		MarkRunning: func(context.Context, string) error { return nil },
		OnStarted:   func(context.Context, run, worker) { started = true },
		Execute: func(context.Context, run, worker) (string, string, identity, error) {
			return "ok", "{}", identity{ok: true}, nil
		},
		HasIdentity:     func(id identity) bool { return id.ok },
		PersistIdentity: func(context.Context, string, identity) error { return nil },
		Reload:          func(context.Context, string) (worker, error) { return worker{id: "w1"}, nil },
		IsCancelled:     func(worker) bool { return false },
		MarkCompleted:   func(context.Context, string, string, string) error { return nil },
		OnCompleted:     func(context.Context, run, worker, string, string) { completed = true },
		Reconcile:       func(context.Context, run) error { reconciled = true; return nil },
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !started || !completed || !reconciled {
		t.Fatalf("started=%v completed=%v reconciled=%v", started, completed, reconciled)
	}
}

func TestExecuteWorkerCancelledErrorReturnsNil(t *testing.T) {
	type run struct{}
	type worker struct{}
	var cancelled bool
	err := ExecuteWorker(context.Background(), "w1", WorkerExecutionHooks[run, worker, struct{}]{
		Load:       func(context.Context, string) (worker, run, error) { return worker{}, run{}, nil },
		IsTerminal: func(worker) bool { return false },
		Execute: func(context.Context, run, worker) (string, string, struct{}, error) {
			return "", "", struct{}{}, errors.New("cancelled")
		},
		IsCancelledError: func(err error) bool { return err != nil && err.Error() == "cancelled" },
		MarkCancelled:    func(context.Context, string, string, string) error { cancelled = true; return nil },
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !cancelled {
		t.Fatal("expected cancellation hook to run")
	}
}
