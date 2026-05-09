package controlplane

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/benjaminwestern/agentic-control/pkg/contract"
	_ "github.com/mattn/go-sqlite3"
)

type WorkspaceStore struct {
	db *sql.DB
}

func closeRows(rows *sql.Rows) {
	_ = rows.Close()
}

func NewWorkspaceStoreFromEnv() (*WorkspaceStore, error) {
	path := strings.TrimSpace(os.Getenv("AGENTIC_CONTROL_WORKSPACE_DB"))
	if path == "" {
		configDir, err := os.UserConfigDir()
		if err != nil {
			return nil, err
		}
		path = filepath.Join(configDir, "agentic-control", "workspace.db")
	}
	return NewWorkspaceStore(path)
}

func NewWorkspaceStore(path string) (*WorkspaceStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON; PRAGMA busy_timeout=5000;`); err != nil {
		_ = db.Close()
		return nil, err
	}
	store := &WorkspaceStore{db: db}
	if err := store.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *WorkspaceStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *WorkspaceStore) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS memory (
  workspace_id TEXT NOT NULL,
  key TEXT NOT NULL,
  value TEXT NOT NULL,
  created_at_ms INTEGER NOT NULL,
  updated_at_ms INTEGER NOT NULL,
  expires_at_ms INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (workspace_id, key)
);

CREATE TABLE IF NOT EXISTS documents (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL,
  name TEXT NOT NULL,
  content TEXT NOT NULL,
  revision INTEGER NOT NULL DEFAULT 1,
  tags_json TEXT NOT NULL DEFAULT '[]',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  archived INTEGER NOT NULL DEFAULT 0,
  created_at_ms INTEGER NOT NULL,
  updated_at_ms INTEGER NOT NULL,
  UNIQUE(workspace_id, name)
);

CREATE TABLE IF NOT EXISTS tasks (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL,
  title TEXT NOT NULL,
  body TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'open',
  tags_json TEXT NOT NULL DEFAULT '[]',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  locked_by TEXT NOT NULL DEFAULT '',
  blocker_ids_json TEXT NOT NULL DEFAULT '[]',
  created_at_ms INTEGER NOT NULL,
  updated_at_ms INTEGER NOT NULL,
  completed_at_ms INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS task_comments (
  id TEXT PRIMARY KEY,
  task_id TEXT NOT NULL,
  author TEXT NOT NULL,
  body TEXT NOT NULL,
  created_at_ms INTEGER NOT NULL,
  FOREIGN KEY(task_id) REFERENCES tasks(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS wakeups (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL,
  owner_id TEXT NOT NULL,
  body TEXT NOT NULL,
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at_ms INTEGER NOT NULL,
  due_at_ms INTEGER NOT NULL,
  fired_at_ms INTEGER NOT NULL DEFAULT 0,
  cancelled_at_ms INTEGER NOT NULL DEFAULT 0,
  paused_at_ms INTEGER NOT NULL DEFAULT 0,
  paused_remaining_ms INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS leases (
  workspace_id TEXT NOT NULL,
  lock_key TEXT NOT NULL,
  owner_id TEXT NOT NULL,
  acquired_at_ms INTEGER NOT NULL,
  expires_at_ms INTEGER NOT NULL,
  PRIMARY KEY (workspace_id, lock_key)
);
`)
	return err
}

// Memory
func (s *WorkspaceStore) SetMemory(ctx context.Context, entry contract.MemoryEntry) error {
	now := time.Now().UnixMilli()
	if entry.CreatedAtMS == 0 {
		entry.CreatedAtMS = now
	}
	entry.UpdatedAtMS = now
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO memory (workspace_id, key, value, created_at_ms, updated_at_ms, expires_at_ms)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(workspace_id, key) DO UPDATE SET
			value = excluded.value,
			updated_at_ms = excluded.updated_at_ms,
			expires_at_ms = excluded.expires_at_ms`,
		entry.WorkspaceID, entry.Key, entry.Value, entry.CreatedAtMS, entry.UpdatedAtMS, entry.ExpiresAtMS)
	return err
}

func (s *WorkspaceStore) GetMemory(ctx context.Context, workspaceID, key string) (contract.MemoryEntry, error) {
	var entry contract.MemoryEntry
	row := s.db.QueryRowContext(ctx, `SELECT workspace_id, key, value, created_at_ms, updated_at_ms, expires_at_ms FROM memory WHERE workspace_id = ? AND key = ?`, workspaceID, key)
	if err := row.Scan(&entry.WorkspaceID, &entry.Key, &entry.Value, &entry.CreatedAtMS, &entry.UpdatedAtMS, &entry.ExpiresAtMS); err != nil {
		return contract.MemoryEntry{}, err
	}
	return entry, nil
}

func (s *WorkspaceStore) DeleteMemory(ctx context.Context, workspaceID, key string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM memory WHERE workspace_id = ? AND key = ?`, workspaceID, key)
	return err
}

func (s *WorkspaceStore) ListMemory(ctx context.Context, workspaceID string) ([]contract.MemoryEntry, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT workspace_id, key, value, created_at_ms, updated_at_ms, expires_at_ms FROM memory WHERE workspace_id = ?`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	var out []contract.MemoryEntry
	for rows.Next() {
		var entry contract.MemoryEntry
		if err := rows.Scan(&entry.WorkspaceID, &entry.Key, &entry.Value, &entry.CreatedAtMS, &entry.UpdatedAtMS, &entry.ExpiresAtMS); err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	return out, rows.Err()
}

// Documents
func (s *WorkspaceStore) WriteDocument(ctx context.Context, doc contract.Document) (contract.Document, error) {
	now := time.Now().UnixMilli()
	if doc.CreatedAtMS == 0 {
		doc.CreatedAtMS = now
	}
	doc.UpdatedAtMS = now
	if doc.Revision == 0 {
		doc.Revision = 1
	}

	tagsJSON, _ := json.Marshal(doc.Tags)
	if len(doc.Tags) == 0 {
		tagsJSON = []byte("[]")
	}

	metadataJSON, _ := json.Marshal(doc.Metadata)
	if len(doc.Metadata) == 0 {
		metadataJSON = []byte("{}")
	}

	archived := 0
	if doc.Archived {
		archived = 1
	}

	// Optimistic concurrency check if revision > 1
	if doc.Revision > 1 {
		var currentRev int64
		err := s.db.QueryRowContext(ctx, `SELECT revision FROM documents WHERE id = ? AND workspace_id = ?`, doc.ID, doc.WorkspaceID).Scan(&currentRev)
		if err == nil && currentRev != doc.Revision-1 {
			return contract.Document{}, errors.New("revision mismatch")
		}
	}

	result, err := s.db.ExecContext(ctx, `
		INSERT INTO documents (id, workspace_id, name, content, revision, tags_json, metadata_json, archived, created_at_ms, updated_at_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			content = excluded.content,
			revision = excluded.revision,
			tags_json = excluded.tags_json,
			metadata_json = excluded.metadata_json,
			archived = excluded.archived,
			updated_at_ms = excluded.updated_at_ms
		WHERE documents.revision = excluded.revision - 1`,
		doc.ID, doc.WorkspaceID, doc.Name, doc.Content, doc.Revision, string(tagsJSON), string(metadataJSON), archived, doc.CreatedAtMS, doc.UpdatedAtMS)

	if err != nil {
		return contract.Document{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return contract.Document{}, err
	}
	if affected == 0 {
		return contract.Document{}, errors.New("revision mismatch")
	}

	return doc, nil
}

func (s *WorkspaceStore) GetDocument(ctx context.Context, workspaceID, id string) (contract.Document, error) {
	var doc contract.Document
	var tagsJSON string
	var metadataJSON string
	var archived int

	row := s.db.QueryRowContext(ctx, `SELECT id, workspace_id, name, content, revision, tags_json, metadata_json, archived, created_at_ms, updated_at_ms FROM documents WHERE workspace_id = ? AND id = ?`, workspaceID, id)
	if err := row.Scan(&doc.ID, &doc.WorkspaceID, &doc.Name, &doc.Content, &doc.Revision, &tagsJSON, &metadataJSON, &archived, &doc.CreatedAtMS, &doc.UpdatedAtMS); err != nil {
		return contract.Document{}, err
	}
	doc.Archived = archived != 0
	_ = json.Unmarshal([]byte(tagsJSON), &doc.Tags)
	_ = json.Unmarshal([]byte(metadataJSON), &doc.Metadata)
	return doc, nil
}

func (s *WorkspaceStore) ListDocuments(ctx context.Context, workspaceID string) ([]contract.Document, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, workspace_id, name, revision, tags_json, metadata_json, archived, created_at_ms, updated_at_ms FROM documents WHERE workspace_id = ? AND archived = 0 ORDER BY updated_at_ms DESC`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	var out []contract.Document
	for rows.Next() {
		var doc contract.Document
		var tagsJSON string
		var metadataJSON string
		var archived int
		if err := rows.Scan(&doc.ID, &doc.WorkspaceID, &doc.Name, &doc.Revision, &tagsJSON, &metadataJSON, &archived, &doc.CreatedAtMS, &doc.UpdatedAtMS); err != nil {
			return nil, err
		}
		doc.Archived = archived != 0
		_ = json.Unmarshal([]byte(tagsJSON), &doc.Tags)
		_ = json.Unmarshal([]byte(metadataJSON), &doc.Metadata)
		out = append(out, doc)
	}
	return out, rows.Err()
}

func (s *WorkspaceStore) AddDocumentMetadata(ctx context.Context, workspaceID, id string, metadata map[string]any) error {
	doc, err := s.GetDocument(ctx, workspaceID, id)
	if err != nil {
		return err
	}
	if doc.Metadata == nil {
		doc.Metadata = make(map[string]any)
	}
	for k, v := range metadata {
		doc.Metadata[k] = v
	}
	doc.Revision++
	_, err = s.WriteDocument(ctx, doc)
	return err
}

func (s *WorkspaceStore) AppendDocument(ctx context.Context, workspaceID, id, content string) error {
	doc, err := s.GetDocument(ctx, workspaceID, id)
	if err != nil {
		return err
	}
	doc.Content += content
	doc.Revision++
	_, err = s.WriteDocument(ctx, doc)
	return err
}

func (s *WorkspaceStore) DeleteDocument(ctx context.Context, workspaceID, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM documents WHERE workspace_id = ? AND id = ?`, workspaceID, id)
	return err
}

func (s *WorkspaceStore) RenameDocument(ctx context.Context, workspaceID, id, name string) error {
	doc, err := s.GetDocument(ctx, workspaceID, id)
	if err != nil {
		return err
	}
	doc.Name = name
	doc.Revision++
	_, err = s.WriteDocument(ctx, doc)
	return err
}

func (s *WorkspaceStore) ArchiveDocument(ctx context.Context, workspaceID, id string, archived bool) error {
	doc, err := s.GetDocument(ctx, workspaceID, id)
	if err != nil {
		return err
	}
	doc.Archived = archived
	doc.Revision++
	_, err = s.WriteDocument(ctx, doc)
	return err
}

func (s *WorkspaceStore) ClearDocument(ctx context.Context, workspaceID, id string) error {
	doc, err := s.GetDocument(ctx, workspaceID, id)
	if err != nil {
		return err
	}
	doc.Content = ""
	doc.Revision++
	_, err = s.WriteDocument(ctx, doc)
	return err
}

// Tasks
func (s *WorkspaceStore) CreateTask(ctx context.Context, task contract.Task) (contract.Task, error) {
	now := time.Now().UnixMilli()
	if task.CreatedAtMS == 0 {
		task.CreatedAtMS = now
	}
	task.UpdatedAtMS = now
	if task.Status == "" {
		task.Status = contract.TaskStatusOpen
	}

	tagsJSON, _ := json.Marshal(task.Tags)
	if len(task.Tags) == 0 {
		tagsJSON = []byte("[]")
	}
	blockersJSON, _ := json.Marshal(task.BlockerIDs)
	if len(task.BlockerIDs) == 0 {
		blockersJSON = []byte("[]")
	}
	metadataJSON, _ := json.Marshal(task.Metadata)
	if len(task.Metadata) == 0 {
		metadataJSON = []byte("{}")
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO tasks (id, workspace_id, title, body, status, tags_json, metadata_json, locked_by, blocker_ids_json, created_at_ms, updated_at_ms, completed_at_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		task.ID, task.WorkspaceID, task.Title, task.Body, task.Status, string(tagsJSON), string(metadataJSON), task.LockedBy, string(blockersJSON), task.CreatedAtMS, task.UpdatedAtMS, task.CompletedAtMS)
	return task, err
}

func (s *WorkspaceStore) GetTask(ctx context.Context, workspaceID, id string) (contract.Task, error) {
	var task contract.Task
	var tagsJSON, metadataJSON, blockersJSON string

	row := s.db.QueryRowContext(ctx, `SELECT id, workspace_id, title, body, status, tags_json, metadata_json, locked_by, blocker_ids_json, created_at_ms, updated_at_ms, completed_at_ms FROM tasks WHERE workspace_id = ? AND id = ?`, workspaceID, id)
	if err := row.Scan(&task.ID, &task.WorkspaceID, &task.Title, &task.Body, &task.Status, &tagsJSON, &metadataJSON, &task.LockedBy, &blockersJSON, &task.CreatedAtMS, &task.UpdatedAtMS, &task.CompletedAtMS); err != nil {
		return contract.Task{}, err
	}
	_ = json.Unmarshal([]byte(tagsJSON), &task.Tags)
	_ = json.Unmarshal([]byte(metadataJSON), &task.Metadata)
	_ = json.Unmarshal([]byte(blockersJSON), &task.BlockerIDs)
	return task, nil
}

func (s *WorkspaceStore) UpdateTask(ctx context.Context, task contract.Task) (contract.Task, error) {
	task.UpdatedAtMS = time.Now().UnixMilli()
	if task.Status == contract.TaskStatusCompleted && task.CompletedAtMS == 0 {
		task.CompletedAtMS = task.UpdatedAtMS
	}

	tagsJSON, _ := json.Marshal(task.Tags)
	if len(task.Tags) == 0 {
		tagsJSON = []byte("[]")
	}
	blockersJSON, _ := json.Marshal(task.BlockerIDs)
	if len(task.BlockerIDs) == 0 {
		blockersJSON = []byte("[]")
	}
	metadataJSON, _ := json.Marshal(task.Metadata)
	if len(task.Metadata) == 0 {
		metadataJSON = []byte("{}")
	}

	result, err := s.db.ExecContext(ctx, `
		UPDATE tasks SET title = ?, body = ?, status = ?, tags_json = ?, metadata_json = ?, locked_by = ?, blocker_ids_json = ?, updated_at_ms = ?, completed_at_ms = ?
		WHERE id = ? AND workspace_id = ?`,
		task.Title, task.Body, task.Status, string(tagsJSON), string(metadataJSON), task.LockedBy, string(blockersJSON), task.UpdatedAtMS, task.CompletedAtMS, task.ID, task.WorkspaceID)
	if err != nil {
		return contract.Task{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return contract.Task{}, err
	}
	if affected == 0 {
		return contract.Task{}, sql.ErrNoRows
	}
	return task, err
}

func (s *WorkspaceStore) ListTasks(ctx context.Context, workspaceID string) ([]contract.Task, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, workspace_id, title, body, status, tags_json, metadata_json, locked_by, blocker_ids_json, created_at_ms, updated_at_ms, completed_at_ms FROM tasks WHERE workspace_id = ? ORDER BY updated_at_ms DESC`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	var out []contract.Task
	for rows.Next() {
		var task contract.Task
		var tagsJSON, metadataJSON, blockersJSON string
		if err := rows.Scan(&task.ID, &task.WorkspaceID, &task.Title, &task.Body, &task.Status, &tagsJSON, &metadataJSON, &task.LockedBy, &blockersJSON, &task.CreatedAtMS, &task.UpdatedAtMS, &task.CompletedAtMS); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(tagsJSON), &task.Tags)
		_ = json.Unmarshal([]byte(metadataJSON), &task.Metadata)
		_ = json.Unmarshal([]byte(blockersJSON), &task.BlockerIDs)
		out = append(out, task)
	}
	return out, rows.Err()
}

func (s *WorkspaceStore) AddTaskMetadata(ctx context.Context, workspaceID, id string, metadata map[string]any) error {
	task, err := s.GetTask(ctx, workspaceID, id)
	if err != nil {
		return err
	}
	if task.Metadata == nil {
		task.Metadata = make(map[string]any)
	}
	for k, v := range metadata {
		task.Metadata[k] = v
	}
	_, err = s.UpdateTask(ctx, task)
	return err
}

func (s *WorkspaceStore) AddTaskTag(ctx context.Context, workspaceID, id, tag string) error {
	task, err := s.GetTask(ctx, workspaceID, id)
	if err != nil {
		return err
	}
	for _, t := range task.Tags {
		if t == tag {
			return nil // already has it
		}
	}
	task.Tags = append(task.Tags, tag)
	_, err = s.UpdateTask(ctx, task)
	return err
}

func (s *WorkspaceStore) RemoveTaskTag(ctx context.Context, workspaceID, id, tag string) error {
	task, err := s.GetTask(ctx, workspaceID, id)
	if err != nil {
		return err
	}
	var newTags []string
	for _, t := range task.Tags {
		if t != tag {
			newTags = append(newTags, t)
		}
	}
	task.Tags = newTags
	_, err = s.UpdateTask(ctx, task)
	return err
}

func (s *WorkspaceStore) SetTaskBlockers(ctx context.Context, workspaceID, id string, blockerIDs []string) error {
	task, err := s.GetTask(ctx, workspaceID, id)
	if err != nil {
		return err
	}
	task.BlockerIDs = blockerIDs
	if len(blockerIDs) > 0 {
		task.Status = contract.TaskStatusBlocked
	} else if task.Status == contract.TaskStatusBlocked {
		task.Status = contract.TaskStatusOpen
	}
	_, err = s.UpdateTask(ctx, task)
	return err
}

func (s *WorkspaceStore) AddTaskBlocker(ctx context.Context, workspaceID, id, blockerID string) error {
	task, err := s.GetTask(ctx, workspaceID, id)
	if err != nil {
		return err
	}
	for _, b := range task.BlockerIDs {
		if b == blockerID {
			return nil
		}
	}
	task.BlockerIDs = append(task.BlockerIDs, blockerID)
	task.Status = contract.TaskStatusBlocked
	_, err = s.UpdateTask(ctx, task)
	return err
}

func (s *WorkspaceStore) RemoveTaskBlocker(ctx context.Context, workspaceID, id, blockerID string) error {
	task, err := s.GetTask(ctx, workspaceID, id)
	if err != nil {
		return err
	}
	var newBlockers []string
	for _, b := range task.BlockerIDs {
		if b != blockerID {
			newBlockers = append(newBlockers, b)
		}
	}
	task.BlockerIDs = newBlockers
	if len(newBlockers) == 0 && task.Status == contract.TaskStatusBlocked {
		task.Status = contract.TaskStatusOpen
	}
	_, err = s.UpdateTask(ctx, task)
	return err
}

func (s *WorkspaceStore) LockTask(ctx context.Context, workspaceID, id, actorID string) error {
	if strings.TrimSpace(actorID) == "" {
		return errors.New("actor_id is required")
	}
	now := time.Now().UnixMilli()
	result, err := s.db.ExecContext(ctx, `
		UPDATE tasks SET locked_by = ?, updated_at_ms = ?
		WHERE workspace_id = ? AND id = ? AND (locked_by = '' OR locked_by = ?)`,
		actorID, now, workspaceID, id, actorID)
	if err != nil {
		return err
	}
	return requireRowsAffected(result, "task is already locked by someone else")
}

func (s *WorkspaceStore) UnlockTask(ctx context.Context, workspaceID, id, actorID string) error {
	if strings.TrimSpace(actorID) == "" {
		return errors.New("actor_id is required")
	}
	now := time.Now().UnixMilli()
	result, err := s.db.ExecContext(ctx, `
		UPDATE tasks SET locked_by = '', updated_at_ms = ?
		WHERE workspace_id = ? AND id = ? AND locked_by = ?`,
		now, workspaceID, id, actorID)
	if err != nil {
		return err
	}
	return requireRowsAffected(result, "task is locked by another actor")
}

func (s *WorkspaceStore) DeleteTask(ctx context.Context, workspaceID, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM tasks WHERE workspace_id = ? AND id = ?`, workspaceID, id)
	return err
}

func (s *WorkspaceStore) CreateTaskComment(ctx context.Context, comment contract.TaskComment) (contract.TaskComment, error) {
	now := time.Now().UnixMilli()
	if comment.CreatedAtMS == 0 {
		comment.CreatedAtMS = now
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO task_comments (id, task_id, author, body, created_at_ms)
		VALUES (?, ?, ?, ?, ?)`,
		comment.ID, comment.TaskID, comment.Author, comment.Body, comment.CreatedAtMS)
	return comment, err
}

func (s *WorkspaceStore) UpdateTaskComment(ctx context.Context, id, body string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE task_comments SET body = ? WHERE id = ?`, body, id)
	return err
}

func (s *WorkspaceStore) DeleteTaskComment(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM task_comments WHERE id = ?`, id)
	return err
}

func (s *WorkspaceStore) ListTaskComments(ctx context.Context, taskID string) ([]contract.TaskComment, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, task_id, author, body, created_at_ms FROM task_comments WHERE task_id = ? ORDER BY created_at_ms ASC`, taskID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	var out []contract.TaskComment
	for rows.Next() {
		var comment contract.TaskComment
		if err := rows.Scan(&comment.ID, &comment.TaskID, &comment.Author, &comment.Body, &comment.CreatedAtMS); err != nil {
			return nil, err
		}
		out = append(out, comment)
	}
	return out, rows.Err()
}

// Wakeups
func (s *WorkspaceStore) SetWakeup(ctx context.Context, wakeup contract.Wakeup) error {
	now := time.Now().UnixMilli()
	if wakeup.CreatedAtMS == 0 {
		wakeup.CreatedAtMS = now
	}
	metadataJSON, _ := json.Marshal(wakeup.Metadata)
	if len(wakeup.Metadata) == 0 {
		metadataJSON = []byte("{}")
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO wakeups (id, workspace_id, owner_id, body, metadata_json, created_at_ms, due_at_ms, fired_at_ms, cancelled_at_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		wakeup.ID, wakeup.WorkspaceID, wakeup.OwnerID, wakeup.Body, string(metadataJSON), wakeup.CreatedAtMS, wakeup.DueAtMS, wakeup.FiredAtMS, wakeup.CancelledAtMS)
	return err
}

func (s *WorkspaceStore) ListPendingWakeups(ctx context.Context, workspaceID string) ([]contract.Wakeup, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, workspace_id, owner_id, body, metadata_json, created_at_ms, due_at_ms, fired_at_ms, cancelled_at_ms FROM wakeups WHERE workspace_id = ? AND fired_at_ms = 0 AND cancelled_at_ms = 0 ORDER BY due_at_ms ASC`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	var out []contract.Wakeup
	for rows.Next() {
		var w contract.Wakeup
		var metadataJSON string
		if err := rows.Scan(&w.ID, &w.WorkspaceID, &w.OwnerID, &w.Body, &metadataJSON, &w.CreatedAtMS, &w.DueAtMS, &w.FiredAtMS, &w.CancelledAtMS); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(metadataJSON), &w.Metadata)
		out = append(out, w)
	}
	return out, rows.Err()
}

func (s *WorkspaceStore) GetWakeup(ctx context.Context, workspaceID, id string) (contract.Wakeup, error) {
	var w contract.Wakeup
	var metadataJSON string
	row := s.db.QueryRowContext(ctx, `SELECT id, workspace_id, owner_id, body, metadata_json, created_at_ms, due_at_ms, fired_at_ms, cancelled_at_ms, paused_at_ms, paused_remaining_ms FROM wakeups WHERE workspace_id = ? AND id = ?`, workspaceID, id)
	if err := row.Scan(&w.ID, &w.WorkspaceID, &w.OwnerID, &w.Body, &metadataJSON, &w.CreatedAtMS, &w.DueAtMS, &w.FiredAtMS, &w.CancelledAtMS, &w.PausedAtMS, &w.PausedRemainingMS); err != nil {
		return contract.Wakeup{}, err
	}
	_ = json.Unmarshal([]byte(metadataJSON), &w.Metadata)
	return w, nil
}

func (s *WorkspaceStore) CancelWakeup(ctx context.Context, workspaceID, id string) error {
	now := time.Now().UnixMilli()
	_, err := s.db.ExecContext(ctx, `UPDATE wakeups SET cancelled_at_ms = ? WHERE workspace_id = ? AND id = ? AND cancelled_at_ms = 0 AND fired_at_ms = 0`, now, workspaceID, id)
	return err
}

func (s *WorkspaceStore) PauseWakeup(ctx context.Context, workspaceID, id string) error {
	now := time.Now().UnixMilli()
	w, err := s.GetWakeup(ctx, workspaceID, id)
	if err != nil {
		return err
	}
	if w.FiredAtMS != 0 || w.CancelledAtMS != 0 || w.PausedAtMS != 0 {
		return errors.New("wakeup cannot be paused in its current state")
	}
	remaining := w.DueAtMS - now
	if remaining < 0 {
		remaining = 0
	}
	_, err = s.db.ExecContext(ctx, `UPDATE wakeups SET paused_at_ms = ?, paused_remaining_ms = ? WHERE workspace_id = ? AND id = ?`, now, remaining, workspaceID, id)
	return err
}

func (s *WorkspaceStore) ResumeWakeup(ctx context.Context, workspaceID, id string) error {
	now := time.Now().UnixMilli()
	w, err := s.GetWakeup(ctx, workspaceID, id)
	if err != nil {
		return err
	}
	if w.PausedAtMS == 0 {
		return errors.New("wakeup is not paused")
	}
	newDueAt := now + w.PausedRemainingMS
	_, err = s.db.ExecContext(ctx, `UPDATE wakeups SET paused_at_ms = 0, paused_remaining_ms = 0, due_at_ms = ? WHERE workspace_id = ? AND id = ?`, newDueAt, workspaceID, id)
	return err
}

func (s *WorkspaceStore) ResetWakeup(ctx context.Context, workspaceID, id string, dueAtMS int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE wakeups SET due_at_ms = ?, fired_at_ms = 0, cancelled_at_ms = 0, paused_at_ms = 0, paused_remaining_ms = 0 WHERE workspace_id = ? AND id = ?`, dueAtMS, workspaceID, id)
	return err
}

// Leases
func (s *WorkspaceStore) AcquireLease(ctx context.Context, lease contract.Lease) (bool, error) {
	now := time.Now().UnixMilli()
	// First, clean up expired lease
	_, _ = s.db.ExecContext(ctx, `DELETE FROM leases WHERE workspace_id = ? AND lock_key = ? AND expires_at_ms < ?`, lease.WorkspaceID, lease.LockKey, now)

	// Try to insert
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO leases (workspace_id, lock_key, owner_id, acquired_at_ms, expires_at_ms)
		VALUES (?, ?, ?, ?, ?) ON CONFLICT(workspace_id, lock_key) DO NOTHING`,
		lease.WorkspaceID, lease.LockKey, lease.OwnerID, lease.AcquiredAtMS, lease.ExpiresAtMS)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (s *WorkspaceStore) ReleaseLease(ctx context.Context, workspaceID, lockKey, ownerID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM leases WHERE workspace_id = ? AND lock_key = ? AND owner_id = ?`, workspaceID, lockKey, ownerID)
	return err
}

func (s *WorkspaceStore) GetLease(ctx context.Context, workspaceID, lockKey string) (*contract.Lease, error) {
	var lease contract.Lease
	row := s.db.QueryRowContext(ctx, `SELECT workspace_id, lock_key, owner_id, acquired_at_ms, expires_at_ms FROM leases WHERE workspace_id = ? AND lock_key = ?`, workspaceID, lockKey)
	if err := row.Scan(&lease.WorkspaceID, &lease.LockKey, &lease.OwnerID, &lease.AcquiredAtMS, &lease.ExpiresAtMS); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	// Check if expired
	if lease.ExpiresAtMS < time.Now().UnixMilli() {
		return nil, nil
	}
	return &lease, nil
}

func (s *WorkspaceStore) ResetLease(ctx context.Context, workspaceID, lockKey string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM leases WHERE workspace_id = ? AND lock_key = ?`, workspaceID, lockKey)
	return err
}

func requireRowsAffected(result sql.Result, message string) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return errors.New(message)
	}
	return nil
}
