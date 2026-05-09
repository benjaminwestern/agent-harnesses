package controlplane

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/benjaminwestern/agentic-control/pkg/contract"
)

type workspaceIDParams struct {
	WorkspaceID string `json:"workspace_id"`
}

type workspaceKeyParams struct {
	WorkspaceID string `json:"workspace_id"`
	Key         string `json:"key"`
}

type taskIDParams struct {
	TaskID string `json:"task_id"`
}

type wakeupResetParams struct {
	WorkspaceID string `json:"workspace_id"`
	Key         string `json:"key"`
	DueAtMS     int64  `json:"due_at_ms"`
}

type documentAppendParams struct {
	WorkspaceID string `json:"workspace_id"`
	Key         string `json:"key"`
	Content     string `json:"content"`
}

type documentRenameParams struct {
	WorkspaceID string `json:"workspace_id"`
	Key         string `json:"key"`
	Name        string `json:"name"`
}

type documentArchiveParams struct {
	WorkspaceID string `json:"workspace_id"`
	Key         string `json:"key"`
	Archived    bool   `json:"archived"`
}

type metadataParams struct {
	WorkspaceID string         `json:"workspace_id"`
	Key         string         `json:"key"`
	Metadata    map[string]any `json:"metadata"`
}

type taskTagParams struct {
	WorkspaceID string `json:"workspace_id"`
	Key         string `json:"key"`
	Tag         string `json:"tag"`
}

type taskBlockersParams struct {
	WorkspaceID string   `json:"workspace_id"`
	Key         string   `json:"key"`
	BlockerIDs  []string `json:"blocker_ids"`
}

type taskBlockerParams struct {
	WorkspaceID string `json:"workspace_id"`
	Key         string `json:"key"`
	BlockerID   string `json:"blocker_id"`
}

type taskLockParams struct {
	WorkspaceID string `json:"workspace_id"`
	Key         string `json:"key"`
	ActorID     string `json:"actor_id"`
}

type taskCommentUpdateParams struct {
	ID   string `json:"id"`
	Body string `json:"body"`
}

type taskCommentDeleteParams struct {
	ID string `json:"id"`
}

func (s *RPCServer) handleWorkspaceRequest(ctx context.Context, method string, params json.RawMessage) (any, bool, error) {
	switch method {
	case "memory.set":
		var entry contract.MemoryEntry
		if err := unmarshalParams(params, &entry); err != nil {
			return nil, true, err
		}
		if entry.WorkspaceID == "" || entry.Key == "" {
			return nil, true, errors.New("workspace_id and key are required")
		}
		if s.service.Workspace() == nil {
			return nil, true, errors.New("workspace store is not configured")
		}
		err := s.service.Workspace().SetMemory(ctx, entry)
		return map[string]any{"ok": err == nil}, true, err
	case "memory.get":
		var p workspaceKeyParams
		if err := unmarshalParams(params, &p); err != nil {
			return nil, true, err
		}
		if s.service.Workspace() == nil {
			return nil, true, errors.New("workspace store is not configured")
		}
		result, err := s.service.Workspace().GetMemory(ctx, p.WorkspaceID, p.Key)
		return result, true, err
	case "memory.delete":
		var p workspaceKeyParams
		if err := unmarshalParams(params, &p); err != nil {
			return nil, true, err
		}
		if s.service.Workspace() == nil {
			return nil, true, errors.New("workspace store is not configured")
		}
		err := s.service.Workspace().DeleteMemory(ctx, p.WorkspaceID, p.Key)
		return map[string]any{"ok": err == nil}, true, err
	case "memory.list":
		var p workspaceIDParams
		if err := unmarshalParams(params, &p); err != nil {
			return nil, true, err
		}
		if s.service.Workspace() == nil {
			return nil, true, errors.New("workspace store is not configured")
		}
		result, err := s.service.Workspace().ListMemory(ctx, p.WorkspaceID)
		return result, true, err
	case "documents.write":
		var doc contract.Document
		if err := unmarshalParams(params, &doc); err != nil {
			return nil, true, err
		}
		if doc.WorkspaceID == "" || doc.Name == "" {
			return nil, true, errors.New("workspace_id and name are required")
		}
		if doc.ID == "" {
			doc.ID = newIdentifier("doc")
		}
		if s.service.Workspace() == nil {
			return nil, true, errors.New("workspace store is not configured")
		}
		result, err := s.service.Workspace().WriteDocument(ctx, doc)
		return result, true, err
	case "documents.get":
		var p workspaceKeyParams // Key is ID
		if err := unmarshalParams(params, &p); err != nil {
			return nil, true, err
		}
		if s.service.Workspace() == nil {
			return nil, true, errors.New("workspace store is not configured")
		}
		result, err := s.service.Workspace().GetDocument(ctx, p.WorkspaceID, p.Key)
		return result, true, err
	case "documents.list":
		var p workspaceIDParams
		if err := unmarshalParams(params, &p); err != nil {
			return nil, true, err
		}
		if s.service.Workspace() == nil {
			return nil, true, errors.New("workspace store is not configured")
		}
		result, err := s.service.Workspace().ListDocuments(ctx, p.WorkspaceID)
		return result, true, err
	case "tasks.create":
		var task contract.Task
		if err := unmarshalParams(params, &task); err != nil {
			return nil, true, err
		}
		if task.WorkspaceID == "" || task.Title == "" {
			return nil, true, errors.New("workspace_id and title are required")
		}
		if task.ID == "" {
			task.ID = newIdentifier("task")
		}
		if s.service.Workspace() == nil {
			return nil, true, errors.New("workspace store is not configured")
		}
		result, err := s.service.Workspace().CreateTask(ctx, task)
		return result, true, err
	case "tasks.update":
		var task contract.Task
		if err := unmarshalParams(params, &task); err != nil {
			return nil, true, err
		}
		if s.service.Workspace() == nil {
			return nil, true, errors.New("workspace store is not configured")
		}
		result, err := s.service.Workspace().UpdateTask(ctx, task)
		return result, true, err
	case "tasks.get":
		var p workspaceKeyParams // Key is ID
		if err := unmarshalParams(params, &p); err != nil {
			return nil, true, err
		}
		if s.service.Workspace() == nil {
			return nil, true, errors.New("workspace store is not configured")
		}
		result, err := s.service.Workspace().GetTask(ctx, p.WorkspaceID, p.Key)
		return result, true, err
	case "tasks.list":
		var p workspaceIDParams
		if err := unmarshalParams(params, &p); err != nil {
			return nil, true, err
		}
		if s.service.Workspace() == nil {
			return nil, true, errors.New("workspace store is not configured")
		}
		result, err := s.service.Workspace().ListTasks(ctx, p.WorkspaceID)
		return result, true, err
	case "wakeups.set":
		var wakeup contract.Wakeup
		if err := unmarshalParams(params, &wakeup); err != nil {
			return nil, true, err
		}
		if wakeup.WorkspaceID == "" {
			return nil, true, errors.New("workspace_id is required")
		}
		if wakeup.ID == "" {
			wakeup.ID = newIdentifier("wakeup")
		}
		if s.service.Workspace() == nil {
			return nil, true, errors.New("workspace store is not configured")
		}
		err := s.service.Workspace().SetWakeup(ctx, wakeup)
		return map[string]any{"ok": err == nil, "id": wakeup.ID}, true, err
	case "wakeups.list_pending":
		var p workspaceIDParams
		if err := unmarshalParams(params, &p); err != nil {
			return nil, true, err
		}
		if s.service.Workspace() == nil {
			return nil, true, errors.New("workspace store is not configured")
		}
		result, err := s.service.Workspace().ListPendingWakeups(ctx, p.WorkspaceID)
		return result, true, err
	case "leases.acquire":
		var lease contract.Lease
		if err := unmarshalParams(params, &lease); err != nil {
			return nil, true, err
		}
		if lease.WorkspaceID == "" || lease.LockKey == "" || lease.OwnerID == "" {
			return nil, true, errors.New("workspace_id, lock_key, and owner_id are required")
		}
		if s.service.Workspace() == nil {
			return nil, true, errors.New("workspace store is not configured")
		}
		acquired, err := s.service.Workspace().AcquireLease(ctx, lease)
		return map[string]any{"acquired": acquired}, true, err
	case "leases.release":
		var p struct {
			WorkspaceID string `json:"workspace_id"`
			LockKey     string `json:"lock_key"`
			OwnerID     string `json:"owner_id"`
		}
		if err := unmarshalParams(params, &p); err != nil {
			return nil, true, err
		}
		if s.service.Workspace() == nil {
			return nil, true, errors.New("workspace store is not configured")
		}
		err := s.service.Workspace().ReleaseLease(ctx, p.WorkspaceID, p.LockKey, p.OwnerID)
		return map[string]any{"ok": err == nil}, true, err
	case "leases.get":
		var p workspaceKeyParams // Key is lock_key
		if err := unmarshalParams(params, &p); err != nil {
			return nil, true, err
		}
		if s.service.Workspace() == nil {
			return nil, true, errors.New("workspace store is not configured")
		}
		result, err := s.service.Workspace().GetLease(ctx, p.WorkspaceID, p.Key)
		return result, true, err
	case "documents.delete":
		var p workspaceKeyParams
		if err := unmarshalParams(params, &p); err != nil {
			return nil, true, err
		}
		if s.service.Workspace() == nil {
			return nil, true, errors.New("workspace store is not configured")
		}
		err := s.service.Workspace().DeleteDocument(ctx, p.WorkspaceID, p.Key)
		return map[string]any{"ok": err == nil}, true, err
	case "documents.append":
		var p documentAppendParams
		if err := unmarshalParams(params, &p); err != nil {
			return nil, true, err
		}
		if s.service.Workspace() == nil {
			return nil, true, errors.New("workspace store is not configured")
		}
		err := s.service.Workspace().AppendDocument(ctx, p.WorkspaceID, p.Key, p.Content)
		return map[string]any{"ok": err == nil}, true, err
	case "documents.rename":
		var p documentRenameParams
		if err := unmarshalParams(params, &p); err != nil {
			return nil, true, err
		}
		if s.service.Workspace() == nil {
			return nil, true, errors.New("workspace store is not configured")
		}
		err := s.service.Workspace().RenameDocument(ctx, p.WorkspaceID, p.Key, p.Name)
		return map[string]any{"ok": err == nil}, true, err
	case "documents.archive":
		var p documentArchiveParams
		if err := unmarshalParams(params, &p); err != nil {
			return nil, true, err
		}
		if s.service.Workspace() == nil {
			return nil, true, errors.New("workspace store is not configured")
		}
		err := s.service.Workspace().ArchiveDocument(ctx, p.WorkspaceID, p.Key, p.Archived)
		return map[string]any{"ok": err == nil}, true, err
	case "documents.clear":
		var p workspaceKeyParams
		if err := unmarshalParams(params, &p); err != nil {
			return nil, true, err
		}
		if s.service.Workspace() == nil {
			return nil, true, errors.New("workspace store is not configured")
		}
		err := s.service.Workspace().ClearDocument(ctx, p.WorkspaceID, p.Key)
		return map[string]any{"ok": err == nil}, true, err
	case "documents.add_metadata":
		var p metadataParams
		if err := unmarshalParams(params, &p); err != nil {
			return nil, true, err
		}
		if s.service.Workspace() == nil {
			return nil, true, errors.New("workspace store is not configured")
		}
		err := s.service.Workspace().AddDocumentMetadata(ctx, p.WorkspaceID, p.Key, p.Metadata)
		return map[string]any{"ok": err == nil}, true, err
	case "tasks.delete":
		var p workspaceKeyParams
		if err := unmarshalParams(params, &p); err != nil {
			return nil, true, err
		}
		if s.service.Workspace() == nil {
			return nil, true, errors.New("workspace store is not configured")
		}
		err := s.service.Workspace().DeleteTask(ctx, p.WorkspaceID, p.Key)
		return map[string]any{"ok": err == nil}, true, err
	case "tasks.add_metadata":
		var p metadataParams
		if err := unmarshalParams(params, &p); err != nil {
			return nil, true, err
		}
		if s.service.Workspace() == nil {
			return nil, true, errors.New("workspace store is not configured")
		}
		err := s.service.Workspace().AddTaskMetadata(ctx, p.WorkspaceID, p.Key, p.Metadata)
		return map[string]any{"ok": err == nil}, true, err
	case "tasks.add_tag":
		var p taskTagParams
		if err := unmarshalParams(params, &p); err != nil {
			return nil, true, err
		}
		if s.service.Workspace() == nil {
			return nil, true, errors.New("workspace store is not configured")
		}
		err := s.service.Workspace().AddTaskTag(ctx, p.WorkspaceID, p.Key, p.Tag)
		return map[string]any{"ok": err == nil}, true, err
	case "tasks.remove_tag":
		var p taskTagParams
		if err := unmarshalParams(params, &p); err != nil {
			return nil, true, err
		}
		if s.service.Workspace() == nil {
			return nil, true, errors.New("workspace store is not configured")
		}
		err := s.service.Workspace().RemoveTaskTag(ctx, p.WorkspaceID, p.Key, p.Tag)
		return map[string]any{"ok": err == nil}, true, err
	case "tasks.set_blockers":
		var p taskBlockersParams
		if err := unmarshalParams(params, &p); err != nil {
			return nil, true, err
		}
		if s.service.Workspace() == nil {
			return nil, true, errors.New("workspace store is not configured")
		}
		err := s.service.Workspace().SetTaskBlockers(ctx, p.WorkspaceID, p.Key, p.BlockerIDs)
		return map[string]any{"ok": err == nil}, true, err
	case "tasks.add_blocker":
		var p taskBlockerParams
		if err := unmarshalParams(params, &p); err != nil {
			return nil, true, err
		}
		if s.service.Workspace() == nil {
			return nil, true, errors.New("workspace store is not configured")
		}
		err := s.service.Workspace().AddTaskBlocker(ctx, p.WorkspaceID, p.Key, p.BlockerID)
		return map[string]any{"ok": err == nil}, true, err
	case "tasks.remove_blocker":
		var p taskBlockerParams
		if err := unmarshalParams(params, &p); err != nil {
			return nil, true, err
		}
		if s.service.Workspace() == nil {
			return nil, true, errors.New("workspace store is not configured")
		}
		err := s.service.Workspace().RemoveTaskBlocker(ctx, p.WorkspaceID, p.Key, p.BlockerID)
		return map[string]any{"ok": err == nil}, true, err
	case "tasks.lock":
		var p taskLockParams
		if err := unmarshalParams(params, &p); err != nil {
			return nil, true, err
		}
		if s.service.Workspace() == nil {
			return nil, true, errors.New("workspace store is not configured")
		}
		err := s.service.Workspace().LockTask(ctx, p.WorkspaceID, p.Key, p.ActorID)
		return map[string]any{"ok": err == nil}, true, err
	case "tasks.unlock":
		var p taskLockParams
		if err := unmarshalParams(params, &p); err != nil {
			return nil, true, err
		}
		if s.service.Workspace() == nil {
			return nil, true, errors.New("workspace store is not configured")
		}
		err := s.service.Workspace().UnlockTask(ctx, p.WorkspaceID, p.Key, p.ActorID)
		return map[string]any{"ok": err == nil}, true, err
	case "tasks.comments.create":
		var comment contract.TaskComment
		if err := unmarshalParams(params, &comment); err != nil {
			return nil, true, err
		}
		if comment.TaskID == "" || comment.Body == "" {
			return nil, true, errors.New("task_id and body are required")
		}
		if comment.ID == "" {
			comment.ID = newIdentifier("comment")
		}
		if s.service.Workspace() == nil {
			return nil, true, errors.New("workspace store is not configured")
		}
		result, err := s.service.Workspace().CreateTaskComment(ctx, comment)
		return result, true, err
	case "tasks.comments.list":
		var p taskIDParams
		if err := unmarshalParams(params, &p); err != nil {
			return nil, true, err
		}
		if s.service.Workspace() == nil {
			return nil, true, errors.New("workspace store is not configured")
		}
		result, err := s.service.Workspace().ListTaskComments(ctx, p.TaskID)
		return result, true, err
	case "tasks.comments.update":
		var p taskCommentUpdateParams
		if err := unmarshalParams(params, &p); err != nil {
			return nil, true, err
		}
		if s.service.Workspace() == nil {
			return nil, true, errors.New("workspace store is not configured")
		}
		err := s.service.Workspace().UpdateTaskComment(ctx, p.ID, p.Body)
		return map[string]any{"ok": err == nil}, true, err
	case "tasks.comments.delete":
		var p taskCommentDeleteParams
		if err := unmarshalParams(params, &p); err != nil {
			return nil, true, err
		}
		if s.service.Workspace() == nil {
			return nil, true, errors.New("workspace store is not configured")
		}
		err := s.service.Workspace().DeleteTaskComment(ctx, p.ID)
		return map[string]any{"ok": err == nil}, true, err
	case "wakeups.cancel":
		var p workspaceKeyParams
		if err := unmarshalParams(params, &p); err != nil {
			return nil, true, err
		}
		if s.service.Workspace() == nil {
			return nil, true, errors.New("workspace store is not configured")
		}
		err := s.service.Workspace().CancelWakeup(ctx, p.WorkspaceID, p.Key)
		return map[string]any{"ok": err == nil}, true, err
	case "wakeups.pause":
		var p workspaceKeyParams
		if err := unmarshalParams(params, &p); err != nil {
			return nil, true, err
		}
		if s.service.Workspace() == nil {
			return nil, true, errors.New("workspace store is not configured")
		}
		err := s.service.Workspace().PauseWakeup(ctx, p.WorkspaceID, p.Key)
		return map[string]any{"ok": err == nil}, true, err
	case "wakeups.resume":
		var p workspaceKeyParams
		if err := unmarshalParams(params, &p); err != nil {
			return nil, true, err
		}
		if s.service.Workspace() == nil {
			return nil, true, errors.New("workspace store is not configured")
		}
		err := s.service.Workspace().ResumeWakeup(ctx, p.WorkspaceID, p.Key)
		return map[string]any{"ok": err == nil}, true, err
	case "wakeups.reset":
		var p wakeupResetParams
		if err := unmarshalParams(params, &p); err != nil {
			return nil, true, err
		}
		if s.service.Workspace() == nil {
			return nil, true, errors.New("workspace store is not configured")
		}
		err := s.service.Workspace().ResetWakeup(ctx, p.WorkspaceID, p.Key, p.DueAtMS)
		return map[string]any{"ok": err == nil}, true, err
	case "leases.reset":
		var p workspaceKeyParams // Key is LockKey
		if err := unmarshalParams(params, &p); err != nil {
			return nil, true, err
		}
		if s.service.Workspace() == nil {
			return nil, true, errors.New("workspace store is not configured")
		}
		err := s.service.Workspace().ResetLease(ctx, p.WorkspaceID, p.Key)
		return map[string]any{"ok": err == nil}, true, err
	}
	return nil, false, nil
}
