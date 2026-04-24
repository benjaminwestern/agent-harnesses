package orchestration

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type SQLiteLedgerStore struct {
	db *sql.DB
}

func NewSQLiteLedgerStore(db *sql.DB) *SQLiteLedgerStore {
	return &SQLiteLedgerStore{db: db}
}

func (s *SQLiteLedgerStore) AddWorkerControl(ctx context.Context, runID string, workerID string, action WorkerControlAction) (WorkerControlRequest, error) {
	now := time.Now()
	result, err := s.db.ExecContext(ctx, `INSERT INTO worker_controls (run_id, worker_id, action, status, error, created_at, updated_at)
		VALUES (?, ?, ?, ?, '', ?, ?)`,
		runID, workerID, action, WorkerControlPending, formatLedgerTime(now), formatLedgerTime(now))
	if err != nil {
		return WorkerControlRequest{}, fmt.Errorf("insert worker control: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return WorkerControlRequest{}, fmt.Errorf("read worker control insert id: %w", err)
	}
	return WorkerControlRequest{ID: id, RunID: runID, WorkerID: workerID, Action: action, Status: WorkerControlPending, CreatedAt: now, UpdatedAt: now}, nil
}

func (s *SQLiteLedgerStore) CompleteWorkerControl(ctx context.Context, id int64, status WorkerControlStatus, errText string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE worker_controls SET status = ?, error = ?, updated_at = ? WHERE id = ?`, status, errText, formatLedgerTime(time.Now()), id)
	if err != nil {
		return fmt.Errorf("update worker control: %w", err)
	}
	return nil
}

func (s *SQLiteLedgerStore) CompletePendingWorkerControls(ctx context.Context, workerID string, status WorkerControlStatus, errText string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE worker_controls SET status = ?, error = ?, updated_at = ? WHERE worker_id = ? AND status = ?`, status, errText, formatLedgerTime(time.Now()), workerID, WorkerControlPending)
	if err != nil {
		return fmt.Errorf("update pending worker controls: %w", err)
	}
	return nil
}

func (s *SQLiteLedgerStore) ListPendingWorkerControls(ctx context.Context, workerID string) ([]WorkerControlRequest, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, run_id, worker_id, action, status, error, created_at, updated_at FROM worker_controls WHERE worker_id = ? AND status = ? ORDER BY id ASC`, workerID, WorkerControlPending)
	if err != nil {
		return nil, fmt.Errorf("query pending worker controls: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []WorkerControlRequest
	for rows.Next() {
		item, err := scanWorkerControlRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pending worker controls: %w", err)
	}
	return out, nil
}

func (s *SQLiteLedgerStore) ListWorkerControls(ctx context.Context, runID string) ([]WorkerControlRequest, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, run_id, worker_id, action, status, error, created_at, updated_at FROM worker_controls WHERE run_id = ? ORDER BY id ASC`, runID)
	if err != nil {
		return nil, fmt.Errorf("query worker controls: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []WorkerControlRequest
	for rows.Next() {
		item, err := scanWorkerControlRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate worker controls: %w", err)
	}
	return out, nil
}

func (s *SQLiteLedgerStore) UpsertRuntimeRequest(ctx context.Context, request RuntimeRequestRecord) error {
	now := time.Now()
	if request.CreatedAt.IsZero() {
		request.CreatedAt = now
	}
	if request.UpdatedAt.IsZero() {
		request.UpdatedAt = now
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO runtime_requests
		(run_id, worker_id, request_id, runtime_session_id, runtime_provider_session_id, runtime, kind, native_method, status, summary, turn_id, request_json, response_status, response_action, response_text, response_option_id, response_answers_json, response_error, response_json, created_at, updated_at, responded_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(worker_id, request_id) DO UPDATE SET
			runtime_session_id = excluded.runtime_session_id,
			runtime_provider_session_id = excluded.runtime_provider_session_id,
			runtime = excluded.runtime,
			kind = excluded.kind,
			native_method = excluded.native_method,
			status = excluded.status,
			summary = excluded.summary,
			turn_id = excluded.turn_id,
			request_json = CASE WHEN excluded.request_json != '' THEN excluded.request_json ELSE runtime_requests.request_json END,
			updated_at = excluded.updated_at`,
		request.RunID, request.WorkerID, request.RequestID, request.RuntimeSessionID, request.RuntimeProviderSessionID, request.Runtime, request.Kind, request.NativeMethod, request.Status, request.Summary, request.TurnID, request.RequestJSON,
		request.ResponseStatus, request.ResponseAction, request.ResponseText, request.ResponseOptionID, request.ResponseAnswersJSON, request.ResponseError, request.ResponseJSON,
		formatLedgerTime(request.CreatedAt), formatLedgerTime(request.UpdatedAt), formatLedgerTime(request.RespondedAt))
	if err != nil {
		return fmt.Errorf("upsert runtime request: %w", err)
	}
	return nil
}

func (s *SQLiteLedgerStore) GetRuntimeRequest(ctx context.Context, id int64) (RuntimeRequestRecord, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, run_id, worker_id, request_id, runtime_session_id, runtime_provider_session_id, runtime, kind, native_method, status, summary, turn_id, request_json, response_status, response_action, response_text, response_option_id, response_answers_json, response_error, response_json, created_at, updated_at, responded_at FROM runtime_requests WHERE id = ?`, id)
	return scanRuntimeRequestRecord(row)
}

func (s *SQLiteLedgerStore) ListRuntimeRequests(ctx context.Context, runID string, status RuntimeRequestStatus) ([]RuntimeRequestRecord, error) {
	query := `SELECT id, run_id, worker_id, request_id, runtime_session_id, runtime_provider_session_id, runtime, kind, native_method, status, summary, turn_id, request_json, response_status, response_action, response_text, response_option_id, response_answers_json, response_error, response_json, created_at, updated_at, responded_at FROM runtime_requests WHERE run_id = ?`
	args := []any{runID}
	if status != "" {
		query += ` AND status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY id ASC`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query runtime requests: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []RuntimeRequestRecord
	for rows.Next() {
		item, err := scanRuntimeRequestRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate runtime requests: %w", err)
	}
	return out, nil
}

func (s *SQLiteLedgerStore) ListQueuedRuntimeRequestResponses(ctx context.Context, workerID string) ([]RuntimeRequestRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, run_id, worker_id, request_id, runtime_session_id, runtime_provider_session_id, runtime, kind, native_method, status, summary, turn_id, request_json, response_status, response_action, response_text, response_option_id, response_answers_json, response_error, response_json, created_at, updated_at, responded_at FROM runtime_requests WHERE worker_id = ? AND response_status = ? ORDER BY id ASC`, workerID, RuntimeResponseQueued)
	if err != nil {
		return nil, fmt.Errorf("query queued runtime responses: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []RuntimeRequestRecord
	for rows.Next() {
		item, err := scanRuntimeRequestRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate queued runtime responses: %w", err)
	}
	return out, nil
}

func (s *SQLiteLedgerStore) QueueRuntimeRequestResponse(ctx context.Context, id int64, response RuntimeRequestResponse) error {
	answers, err := json.Marshal(response.Answers)
	if err != nil {
		return fmt.Errorf("marshal runtime request answers: %w", err)
	}
	result, err := s.db.ExecContext(ctx, `UPDATE runtime_requests SET response_status = ?, response_action = ?, response_text = ?, response_option_id = ?, response_answers_json = ?, response_error = '', updated_at = ? WHERE id = ? AND status = ?`, RuntimeResponseQueued, response.Action, response.Text, response.OptionID, string(answers), formatLedgerTime(time.Now()), id, RuntimeRequestOpen)
	if err != nil {
		return fmt.Errorf("queue runtime request response: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read queued runtime response row count: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("runtime request %d is not open or does not exist", id)
	}
	return nil
}

func (s *SQLiteLedgerStore) CompleteRuntimeRequestResponse(ctx context.Context, id int64, status RuntimeResponseStatus, responseJSON string, errText string) error {
	respondedAt := ""
	if status == RuntimeResponseCompleted {
		respondedAt = formatLedgerTime(time.Now())
	}
	_, err := s.db.ExecContext(ctx, `UPDATE runtime_requests SET response_status = ?, response_json = ?, response_error = ?, updated_at = ?, responded_at = ? WHERE id = ?`, status, responseJSON, errText, formatLedgerTime(time.Now()), respondedAt, id)
	if err != nil {
		return fmt.Errorf("complete runtime request response: %w", err)
	}
	return nil
}

func (s *SQLiteLedgerStore) AddEvent(ctx context.Context, event EventRecord) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO events (run_id, worker_id, type, message, payload, created_at) VALUES (?, ?, ?, ?, ?, ?)`, event.RunID, event.WorkerID, event.Type, event.Message, event.Payload, formatLedgerTime(event.CreatedAt))
	if err != nil {
		return fmt.Errorf("insert event: %w", err)
	}
	return nil
}

func (s *SQLiteLedgerStore) ListEvents(ctx context.Context, runID string, after int64) ([]EventRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, run_id, worker_id, type, message, payload, created_at FROM events WHERE run_id = ? AND id > ? ORDER BY id ASC LIMIT 200`, runID, after)
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []EventRecord
	for rows.Next() {
		item, err := scanEventRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate events: %w", err)
	}
	return out, nil
}

func (s *SQLiteLedgerStore) AddArtifact(ctx context.Context, artifact ArtifactRecord) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO artifacts (run_id, worker_id, kind, format, content, created_at) VALUES (?, ?, ?, ?, ?, ?)`, artifact.RunID, artifact.WorkerID, artifact.Kind, artifact.Format, artifact.Content, formatLedgerTime(artifact.CreatedAt))
	if err != nil {
		return fmt.Errorf("insert artifact: %w", err)
	}
	return nil
}

func (s *SQLiteLedgerStore) ListArtifacts(ctx context.Context, runID string) ([]ArtifactRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, run_id, worker_id, kind, format, content, created_at FROM artifacts WHERE run_id = ? ORDER BY id ASC`, runID)
	if err != nil {
		return nil, fmt.Errorf("query artifacts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []ArtifactRecord
	for rows.Next() {
		item, err := scanArtifactRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate artifacts: %w", err)
	}
	return out, nil
}

type ledgerScanner interface{ Scan(dest ...any) error }

func scanWorkerControlRecord(row ledgerScanner) (WorkerControlRequest, error) {
	var control WorkerControlRequest
	var action, status string
	var created, updated string
	if err := row.Scan(&control.ID, &control.RunID, &control.WorkerID, &action, &status, &control.Error, &created, &updated); err != nil {
		return WorkerControlRequest{}, fmt.Errorf("scan worker control: %w", err)
	}
	control.Action = WorkerControlAction(action)
	control.Status = WorkerControlStatus(status)
	control.CreatedAt = parseLedgerTime(created)
	control.UpdatedAt = parseLedgerTime(updated)
	return control, nil
}

func scanRuntimeRequestRecord(row ledgerScanner) (RuntimeRequestRecord, error) {
	var request RuntimeRequestRecord
	var status, responseStatus string
	var created, updated, responded string
	if err := row.Scan(&request.ID, &request.RunID, &request.WorkerID, &request.RequestID, &request.RuntimeSessionID, &request.RuntimeProviderSessionID, &request.Runtime, &request.Kind, &request.NativeMethod, &status, &request.Summary, &request.TurnID, &request.RequestJSON, &responseStatus, &request.ResponseAction, &request.ResponseText, &request.ResponseOptionID, &request.ResponseAnswersJSON, &request.ResponseError, &request.ResponseJSON, &created, &updated, &responded); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return RuntimeRequestRecord{}, fmt.Errorf("runtime request not found")
		}
		return RuntimeRequestRecord{}, fmt.Errorf("scan runtime request: %w", err)
	}
	request.Status = RuntimeRequestStatus(status)
	request.ResponseStatus = RuntimeResponseStatus(responseStatus)
	request.CreatedAt = parseLedgerTime(created)
	request.UpdatedAt = parseLedgerTime(updated)
	request.RespondedAt = parseLedgerTime(responded)
	return request, nil
}

func scanEventRecord(row ledgerScanner) (EventRecord, error) {
	var event EventRecord
	var created string
	if err := row.Scan(&event.ID, &event.RunID, &event.WorkerID, &event.Type, &event.Message, &event.Payload, &created); err != nil {
		return EventRecord{}, fmt.Errorf("scan event: %w", err)
	}
	event.CreatedAt = parseLedgerTime(created)
	return event, nil
}

func scanArtifactRecord(row ledgerScanner) (ArtifactRecord, error) {
	var artifact ArtifactRecord
	var created string
	if err := row.Scan(&artifact.ID, &artifact.RunID, &artifact.WorkerID, &artifact.Kind, &artifact.Format, &artifact.Content, &created); err != nil {
		return ArtifactRecord{}, fmt.Errorf("scan artifact: %w", err)
	}
	artifact.CreatedAt = parseLedgerTime(created)
	return artifact, nil
}

func formatLedgerTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func parseLedgerTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	t, _ := time.Parse(time.RFC3339Nano, value)
	return t
}
