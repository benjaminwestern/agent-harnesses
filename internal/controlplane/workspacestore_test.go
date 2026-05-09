package controlplane

import (
	"context"
	"testing"

	"github.com/benjaminwestern/agentic-control/pkg/contract"
)

func newTestWorkspaceStore(t *testing.T) *WorkspaceStore {
	t.Helper()
	store, err := NewWorkspaceStore(t.TempDir() + "/workspace.db")
	if err != nil {
		t.Fatalf("NewWorkspaceStore failed: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestWorkspaceStoreTaskMetadataRoundTrips(t *testing.T) {
	store := newTestWorkspaceStore(t)
	ctx := context.Background()

	created, err := store.CreateTask(ctx, contract.Task{
		ID:          "task-1",
		WorkspaceID: "ws",
		Title:       "Task",
		Metadata:    map[string]any{"source": "test"},
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}
	if created.Metadata["source"] != "test" {
		t.Fatalf("created metadata = %#v", created.Metadata)
	}

	if err := store.AddTaskMetadata(ctx, "ws", "task-1", map[string]any{"priority": "high"}); err != nil {
		t.Fatalf("AddTaskMetadata failed: %v", err)
	}
	got, err := store.GetTask(ctx, "ws", "task-1")
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}
	if got.Metadata["source"] != "test" || got.Metadata["priority"] != "high" {
		t.Fatalf("metadata = %#v, want source and priority", got.Metadata)
	}
	listed, err := store.ListTasks(ctx, "ws")
	if err != nil {
		t.Fatalf("ListTasks failed: %v", err)
	}
	if len(listed) != 1 || listed[0].Metadata["priority"] != "high" {
		t.Fatalf("listed tasks = %#v", listed)
	}
}

func TestWorkspaceStoreRejectsStaleDocumentWrite(t *testing.T) {
	store := newTestWorkspaceStore(t)
	ctx := context.Background()

	if _, err := store.WriteDocument(ctx, contract.Document{
		ID:          "doc-1",
		WorkspaceID: "ws",
		Name:        "Doc",
		Content:     "first",
		Revision:    1,
	}); err != nil {
		t.Fatalf("initial WriteDocument failed: %v", err)
	}
	if _, err := store.WriteDocument(ctx, contract.Document{
		ID:          "doc-1",
		WorkspaceID: "ws",
		Name:        "Doc",
		Content:     "stale overwrite",
		Revision:    1,
	}); err == nil {
		t.Fatal("stale WriteDocument succeeded, want revision error")
	}
}

func TestWorkspaceStoreTaskLocksRequireOwningActor(t *testing.T) {
	store := newTestWorkspaceStore(t)
	ctx := context.Background()
	if _, err := store.CreateTask(ctx, contract.Task{ID: "task-1", WorkspaceID: "ws", Title: "Task"}); err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}
	if err := store.LockTask(ctx, "ws", "task-1", "actor-a"); err != nil {
		t.Fatalf("LockTask actor-a failed: %v", err)
	}
	if err := store.LockTask(ctx, "ws", "task-1", "actor-b"); err == nil {
		t.Fatal("LockTask actor-b succeeded, want locked error")
	}
	if err := store.UnlockTask(ctx, "ws", "task-1", "actor-b"); err == nil {
		t.Fatal("UnlockTask actor-b succeeded, want ownership error")
	}
	if err := store.UnlockTask(ctx, "ws", "task-1", "actor-a"); err != nil {
		t.Fatalf("UnlockTask actor-a failed: %v", err)
	}
	got, err := store.GetTask(ctx, "ws", "task-1")
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}
	if got.LockedBy != "" {
		t.Fatalf("LockedBy = %q, want unlocked", got.LockedBy)
	}
}
