package controlplane

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/benjaminwestern/agentic-control/pkg/contract"
	_ "github.com/mattn/go-sqlite3"
)

type ThreadStore struct {
	db *sql.DB
}

func NewThreadStoreFromEnv() (*ThreadStore, error) {
	path := stringsTrimSpace(os.Getenv("AGENTIC_CONTROL_STATE_DB"))
	if path == "" {
		configDir, err := os.UserConfigDir()
		if err != nil {
			return nil, err
		}
		path = filepath.Join(configDir, "agentic-control", "controlplane.db")
	}
	return NewThreadStore(path)
}

func NewThreadStore(path string) (*ThreadStore, error) {
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
	store := &ThreadStore{db: db}
	if err := store.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *ThreadStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *ThreadStore) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS threads (
  thread_id TEXT PRIMARY KEY,
  parent_thread_id TEXT NOT NULL DEFAULT '',
  name TEXT NOT NULL DEFAULT '',
  archived INTEGER NOT NULL DEFAULT 0,
  metadata_json TEXT NOT NULL DEFAULT '{}',
  runtime TEXT NOT NULL DEFAULT '',
  provider_session_id TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT '',
  model TEXT NOT NULL DEFAULT '',
  title TEXT NOT NULL DEFAULT '',
  updated_at_ms INTEGER NOT NULL DEFAULT 0,
  tracked_json TEXT NOT NULL,
  created_at_ms INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_threads_runtime_updated ON threads(runtime, updated_at_ms DESC);
CREATE INDEX IF NOT EXISTS idx_threads_provider_session_id ON threads(provider_session_id);
CREATE TABLE IF NOT EXISTS thread_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  thread_id TEXT NOT NULL,
  recorded_at_ms INTEGER NOT NULL DEFAULT 0,
  event_type TEXT NOT NULL DEFAULT '',
  summary TEXT NOT NULL DEFAULT '',
  turn_id TEXT NOT NULL DEFAULT '',
  request_id TEXT NOT NULL DEFAULT '',
  event_json TEXT NOT NULL,
  FOREIGN KEY(thread_id) REFERENCES threads(thread_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_thread_events_thread_id_id ON thread_events(thread_id, id ASC);
`)
	if err != nil {
		return err
	}
	if err := s.ensureThreadColumn("name", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := s.ensureThreadColumn("metadata_json", "TEXT NOT NULL DEFAULT '{}'"); err != nil {
		return err
	}
	return nil
}

func (s *ThreadStore) ensureThreadColumn(name string, definition string) error {
	rows, err := s.db.Query(`PRAGMA table_info(threads)`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var cid int
		var columnName, columnType string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &columnName, &columnType, &notNull, &defaultValue, &pk); err != nil {
			return err
		}
		if columnName == name {
			return nil
		}
	}
	_, err = s.db.Exec(`ALTER TABLE threads ADD COLUMN ` + name + ` ` + definition)
	return err
}

func (s *ThreadStore) UpsertTrackedSession(ctx context.Context, tracked contract.TrackedSession) error {
	encoded, err := json.Marshal(tracked)
	if err != nil {
		return err
	}
	metadata := threadMetadataFromSession(tracked.Session)
	metadataJSONBytes, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	metadataJSON := string(metadataJSONBytes)
	name := threadNameFromSession(tracked.Session)
	created := tracked.StartedAtMS
	if created == 0 {
		created = tracked.Session.CreatedAtMS
	}
	if created == 0 {
		created = time.Now().UnixMilli()
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO threads (thread_id, parent_thread_id, name, archived, metadata_json, runtime, provider_session_id, status, model, title, updated_at_ms, tracked_json, created_at_ms)
		VALUES (?, '', ?, 0, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(thread_id) DO UPDATE SET
			name = CASE WHEN threads.name = '' THEN excluded.name ELSE threads.name END,
			metadata_json = CASE WHEN threads.metadata_json = '{}' THEN excluded.metadata_json ELSE threads.metadata_json END,
			runtime = excluded.runtime,
			provider_session_id = excluded.provider_session_id,
			status = excluded.status,
			model = excluded.model,
			title = excluded.title,
			updated_at_ms = excluded.updated_at_ms,
			tracked_json = excluded.tracked_json`,
		tracked.Session.SessionID, name, metadataJSON, tracked.Session.Runtime, tracked.Session.ProviderSessionID, tracked.Session.Status, tracked.Session.Model, tracked.Session.Title, tracked.Session.UpdatedAtMS, string(encoded), created)
	return err
}

func (s *ThreadStore) AddEvent(ctx context.Context, event contract.RuntimeEvent) error {
	if event.SessionID == "" {
		return nil
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO thread_events (thread_id, recorded_at_ms, event_type, summary, turn_id, request_id, event_json) VALUES (?, ?, ?, ?, ?, ?, ?)`, event.SessionID, event.RecordedAtMS, event.EventType, event.Summary, event.TurnID, event.RequestID, string(encoded))
	return err
}

func (s *ThreadStore) ListThreads(ctx context.Context, runtime string, archived *bool) ([]contract.TrackedThread, error) {
	query := `SELECT thread_id, parent_thread_id, name, archived, metadata_json, tracked_json FROM threads`
	args := []any{}
	var clauses []string
	if stringsTrimSpace(runtime) != "" {
		clauses = append(clauses, "runtime = ?")
		args = append(args, stringsTrimSpace(runtime))
	}
	if archived != nil {
		clauses = append(clauses, "archived = ?")
		if *archived {
			args = append(args, 1)
		} else {
			args = append(args, 0)
		}
	}
	if len(clauses) > 0 {
		query += " WHERE " + joinClauses(clauses)
	}
	query += ` ORDER BY updated_at_ms DESC, thread_id ASC`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []contract.TrackedThread
	for rows.Next() {
		thread, err := scanTrackedThread(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, thread)
	}
	return out, rows.Err()
}

func (s *ThreadStore) GetThread(ctx context.Context, threadID string, providerSessionID string) (contract.TrackedThread, error) {
	query := `SELECT thread_id, parent_thread_id, name, archived, metadata_json, tracked_json FROM threads WHERE `
	args := []any{}
	if stringsTrimSpace(threadID) != "" {
		query += `thread_id = ?`
		args = append(args, stringsTrimSpace(threadID))
	} else {
		query += `provider_session_id = ?`
		args = append(args, stringsTrimSpace(providerSessionID))
	}
	row := s.db.QueryRowContext(ctx, query, args...)
	return scanTrackedThread(row)
}

func (s *ThreadStore) SetName(ctx context.Context, threadID string, name string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE threads SET name = ? WHERE thread_id = ?`, stringsTrimSpace(name), threadID)
	return err
}

func (s *ThreadStore) SetMetadata(ctx context.Context, threadID string, metadata contract.ThreadMetadata) error {
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE threads SET metadata_json = ? WHERE thread_id = ?`, string(encoded), threadID)
	return err
}

func (s *ThreadStore) SetParent(ctx context.Context, threadID string, parentThreadID string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE threads SET parent_thread_id = ? WHERE thread_id = ?`, stringsTrimSpace(parentThreadID), threadID)
	return err
}

func (s *ThreadStore) ForkThread(ctx context.Context, sourceThreadID string, newThreadID string, preserveProviderSession bool) (contract.TrackedThread, error) {
	thread, err := s.GetThread(ctx, sourceThreadID, "")
	if err != nil {
		return contract.TrackedThread{}, err
	}
	thread.ParentThreadID = sourceThreadID
	thread.ThreadID = newThreadID
	thread.Archived = false
	if thread.Name != "" {
		thread.Name = thread.Name + " (fork)"
	}
	thread.TrackedSession.Session.SessionID = newThreadID
	if !preserveProviderSession {
		thread.TrackedSession.Session.ProviderSessionID = ""
	}
	thread.TrackedSession.Session.Status = contract.SessionIdle
	thread.TrackedSession.Session.ActiveTurnID = ""
	thread.TrackedSession.Session.CreatedAtMS = time.Now().UnixMilli()
	thread.TrackedSession.Session.UpdatedAtMS = thread.TrackedSession.Session.CreatedAtMS
	thread.TrackedSession.StartedAtMS = thread.TrackedSession.Session.CreatedAtMS
	encoded, err := json.Marshal(thread.TrackedSession)
	if err != nil {
		return contract.TrackedThread{}, err
	}
	metadataJSON, err := json.Marshal(thread.Metadata)
	if err != nil {
		return contract.TrackedThread{}, err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO threads (thread_id, parent_thread_id, name, archived, metadata_json, runtime, provider_session_id, status, model, title, updated_at_ms, tracked_json, created_at_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		thread.ThreadID, thread.ParentThreadID, thread.Name, 0, string(metadataJSON), thread.TrackedSession.Session.Runtime, thread.TrackedSession.Session.ProviderSessionID, thread.TrackedSession.Session.Status, thread.TrackedSession.Session.Model, thread.TrackedSession.Session.Title, thread.TrackedSession.Session.UpdatedAtMS, string(encoded), thread.TrackedSession.Session.CreatedAtMS)
	if err != nil {
		return contract.TrackedThread{}, err
	}
	return thread, nil
}

func (s *ThreadStore) SetArchived(ctx context.Context, threadID string, archived bool) error {
	value := 0
	if archived {
		value = 1
	}
	_, err := s.db.ExecContext(ctx, `UPDATE threads SET archived = ? WHERE thread_id = ?`, value, threadID)
	return err
}

func (s *ThreadStore) ListThreadEvents(ctx context.Context, threadID string, afterID int64, limit int) ([]contract.ThreadEvent, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, thread_id, recorded_at_ms, event_type, summary, turn_id, request_id, event_json FROM thread_events WHERE thread_id = ? AND id > ? ORDER BY id ASC LIMIT ?`, threadID, afterID, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []contract.ThreadEvent
	for rows.Next() {
		item, err := scanThreadEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

type threadScanner interface{ Scan(dest ...any) error }

func scanTrackedThread(row threadScanner) (contract.TrackedThread, error) {
	var thread contract.TrackedThread
	var archived int
	var metadataJSON string
	var trackedJSON string
	if err := row.Scan(&thread.ThreadID, &thread.ParentThreadID, &thread.Name, &archived, &metadataJSON, &trackedJSON); err != nil {
		return contract.TrackedThread{}, err
	}
	thread.Archived = archived != 0
	if stringsTrimSpace(metadataJSON) != "" {
		_ = json.Unmarshal([]byte(metadataJSON), &thread.Metadata)
	}
	if err := json.Unmarshal([]byte(trackedJSON), &thread.TrackedSession); err != nil {
		return contract.TrackedThread{}, err
	}
	if stringsTrimSpace(thread.Name) == "" {
		thread.Name = threadNameFromSession(thread.TrackedSession.Session)
	}
	if thread.Metadata == (contract.ThreadMetadata{}) {
		thread.Metadata = threadMetadataFromSession(thread.TrackedSession.Session)
	}
	return thread, nil
}

func scanThreadEvent(row threadScanner) (contract.ThreadEvent, error) {
	var item contract.ThreadEvent
	var eventJSON string
	if err := row.Scan(&item.ID, &item.ThreadID, &item.RecordedAtMS, &item.EventType, &item.Summary, &item.TurnID, &item.RequestID, &eventJSON); err != nil {
		return contract.ThreadEvent{}, err
	}
	if err := json.Unmarshal([]byte(eventJSON), &item.Event); err != nil {
		return contract.ThreadEvent{}, err
	}
	return item, nil
}

func stringsTrimSpace(value string) string { return strings.TrimSpace(value) }

func joinClauses(values []string) string {
	return strings.Join(values, " AND ")
}

func threadNameFromSession(session contract.RuntimeSession) string {
	if session.Metadata != nil {
		if value, ok := session.Metadata["thread_name"].(string); ok && stringsTrimSpace(value) != "" {
			return stringsTrimSpace(value)
		}
	}
	if stringsTrimSpace(session.Title) != "" {
		return stringsTrimSpace(session.Title)
	}
	return ""
}

func threadMetadataFromSession(session contract.RuntimeSession) contract.ThreadMetadata {
	if len(session.Metadata) == 0 {
		return contract.ThreadMetadata{}
	}
	return contract.ThreadMetadata{
		Kind:                          contract.ThreadKind(stringValueFromMetadata(session.Metadata, "thread_kind")),
		Workflow:                      stringValueFromMetadata(session.Metadata, "workflow"),
		WorkflowMode:                  stringValueFromMetadata(session.Metadata, "workflow_mode"),
		Task:                          stringValueFromMetadata(session.Metadata, "task"),
		TargetLabel:                   stringValueFromMetadata(session.Metadata, "target_label"),
		ReductionMode:                 stringValueFromMetadata(session.Metadata, "reduction_mode"),
		CourtRunID:                    stringValueFromMetadata(session.Metadata, "court_run_id"),
		CourtWorkerID:                 stringValueFromMetadata(session.Metadata, "court_worker_id"),
		CourtRoleID:                   stringValueFromMetadata(session.Metadata, "court_role_id"),
		CourtRoleKind:                 stringValueFromMetadata(session.Metadata, "court_role_kind"),
		CourtAgent:                    stringValueFromMetadata(session.Metadata, "court_agent"),
		CourtBackend:                  stringValueFromMetadata(session.Metadata, "court_backend"),
		ForkMode:                      stringValueFromMetadata(session.Metadata, "fork_mode"),
		ForkedFromThreadID:            stringValueFromMetadata(session.Metadata, "forked_from_thread_id"),
		ForkedFromProviderSessionID:   stringValueFromMetadata(session.Metadata, "forked_from_provider_session_id"),
		RollbackMode:                  stringValueFromMetadata(session.Metadata, "rollback_mode"),
		RollbackTurns:                 intValueFromMetadata(session.Metadata, "rollback_turns"),
		RollbackFromThreadID:          stringValueFromMetadata(session.Metadata, "rollback_from_thread_id"),
		RollbackFromProviderSessionID: stringValueFromMetadata(session.Metadata, "rollback_from_provider_session_id"),
	}
}

func stringValueFromMetadata(values map[string]any, key string) string {
	value, ok := values[key]
	if !ok {
		return ""
	}
	text, _ := value.(string)
	return stringsTrimSpace(text)
}

func intValueFromMetadata(values map[string]any, key string) int {
	value, ok := values[key]
	if !ok {
		return 0
	}
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}
