package orchestration

import (
	"context"
	"errors"
	"testing"
)

func TestSummarizeWorkerStatuses(t *testing.T) {
	workers := []WorkerStatus{WorkerQueued, WorkerFailed, WorkerCancelled}
	summary := SummarizeWorkerStatuses(workers, func(status WorkerStatus) WorkerStatus { return status })
	if !summary.HasFailed || !summary.HasCancelled || !summary.HasRunningOrQueued {
		t.Fatalf("unexpected summary: %#v", summary)
	}
}

func TestQueueAndLaunchSkipsPersistConflicts(t *testing.T) {
	items := []int{1, 2, 3}
	var launched []int
	err := QueueAndLaunch(context.Background(), items, QueueAndLaunchHooks[int]{
		Persist: func(_ context.Context, item int) error {
			if item == 2 {
				return errors.New("duplicate")
			}
			return nil
		},
		OnPersistError: func(_ context.Context, item int, err error) (bool, error) {
			if item == 2 {
				return true, nil
			}
			return false, err
		},
		Launch: func(_ context.Context, item int) error {
			launched = append(launched, item)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(launched) != 2 || launched[0] != 1 || launched[1] != 3 {
		t.Fatalf("launched = %#v, want [1 3]", launched)
	}
}
