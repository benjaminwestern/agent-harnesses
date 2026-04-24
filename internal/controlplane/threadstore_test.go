package controlplane

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/benjaminwestern/agentic-control/pkg/contract"
)

func TestThreadStorePersistsThreadsAndEvents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "controlplane.db")
	store, err := NewThreadStore(path)
	if err != nil {
		t.Fatalf("new thread store: %v", err)
	}
	ctx := context.Background()
	tracked := contract.TrackedSession{
		Session: contract.RuntimeSession{
			SchemaVersion:     contract.ControlPlaneSchemaVersion,
			SessionID:         "thread-1",
			Runtime:           "opencode",
			ProviderSessionID: "provider-1",
			Status:            contract.SessionIdle,
			Model:             "opencode/gemini-3-flash",
			CreatedAtMS:       100,
			UpdatedAtMS:       200,
		},
		StartedAtMS:   100,
		LastEventType: "session.started",
	}
	if err := store.UpsertTrackedSession(ctx, tracked); err != nil {
		t.Fatalf("upsert tracked session: %v", err)
	}
	if err := store.AddEvent(ctx, contract.RuntimeEvent{
		SchemaVersion: contract.ControlPlaneSchemaVersion,
		SessionID:     "thread-1",
		Runtime:       "opencode",
		EventType:     "turn.completed",
		Summary:       "done",
		RecordedAtMS:  300,
	}); err != nil {
		t.Fatalf("add event: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	reopened, err := NewThreadStore(path)
	if err != nil {
		t.Fatalf("reopen thread store: %v", err)
	}
	defer func() { _ = reopened.Close() }()
	threads, err := reopened.ListThreads(ctx, "", nil)
	if err != nil {
		t.Fatalf("list threads: %v", err)
	}
	if len(threads) != 1 || threads[0].ThreadID != "thread-1" {
		t.Fatalf("unexpected threads: %#v", threads)
	}
	events, err := reopened.ListThreadEvents(ctx, "thread-1", 0, 10)
	if err != nil {
		t.Fatalf("list thread events: %v", err)
	}
	if len(events) != 1 || events[0].EventType != "turn.completed" {
		t.Fatalf("unexpected events: %#v", events)
	}
	if err := reopened.SetArchived(ctx, "thread-1", true); err != nil {
		t.Fatalf("archive thread: %v", err)
	}
	archived := true
	archivedThreads, err := reopened.ListThreads(ctx, "", &archived)
	if err != nil {
		t.Fatalf("list archived threads: %v", err)
	}
	if len(archivedThreads) != 1 || !archivedThreads[0].Archived {
		t.Fatalf("unexpected archived threads: %#v", archivedThreads)
	}
}
