package orchestration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	api "github.com/benjaminwestern/agentic-control/pkg/controlplane"
)

func (s *SQLiteLedgerStore) CreateWorker(ctx context.Context, worker WorkerRecord) error {
	if worker.Attempt == 0 {
		worker.Attempt = 1
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO workers
		(id, run_id, launch_id, attempt, role_id, role_kind, role_title, backend, provider, model, model_options, agent, status, runtime_session_id, runtime_provider_session_id, runtime_transcript_path, runtime_pid, result, result_json, error, created_at, updated_at, completed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		worker.ID, worker.RunID, worker.LaunchID, worker.Attempt, worker.RoleID, worker.RoleKind, worker.RoleTitle, worker.Backend,
		worker.Provider, worker.Model, api.MarshalModelOptionsJSON(worker.ModelOptions), worker.Agent, worker.Status, worker.RuntimeSessionID, worker.RuntimeProviderSessionID, worker.RuntimeTranscriptPath, worker.RuntimePID, worker.Result, worker.ResultJSON, worker.Error,
		formatLedgerTime(worker.CreatedAt), formatLedgerTime(worker.UpdatedAt), formatLedgerTime(worker.CompletedAt))
	if err != nil {
		return fmt.Errorf("create worker: %w", err)
	}
	return nil
}

func (s *SQLiteLedgerStore) GetWorker(ctx context.Context, id string) (WorkerRecord, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, run_id, launch_id, attempt, role_id, role_kind, role_title, backend, provider, model, model_options, agent, status, runtime_session_id, runtime_provider_session_id, runtime_transcript_path, runtime_pid, result, result_json, error, created_at, updated_at, completed_at FROM workers WHERE id = ?`, id)
	return scanWorkerRecord(row)
}

func (s *SQLiteLedgerStore) ListWorkers(ctx context.Context, runID string) ([]WorkerRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, run_id, launch_id, attempt, role_id, role_kind, role_title, backend, provider, model, model_options, agent, status, runtime_session_id, runtime_provider_session_id, runtime_transcript_path, runtime_pid, result, result_json, error, created_at, updated_at, completed_at FROM workers WHERE run_id = ? ORDER BY created_at ASC`, runID)
	if err != nil {
		return nil, fmt.Errorf("query workers: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []WorkerRecord
	for rows.Next() {
		worker, err := scanWorkerRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, worker)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workers: %w", err)
	}
	return out, nil
}

func (s *SQLiteLedgerStore) ArchiveWorkerAttempt(ctx context.Context, worker WorkerRecord) error {
	if worker.Attempt == 0 {
		worker.Attempt = 1
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO worker_attempts
		(worker_id, run_id, attempt, launch_id, role_id, role_kind, role_title, backend, provider, model, model_options, agent, status, runtime_session_id, runtime_provider_session_id, runtime_transcript_path, runtime_pid, result, result_json, error, created_at, updated_at, completed_at, archived_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(worker_id, attempt) DO UPDATE SET
			launch_id = excluded.launch_id,
			role_id = excluded.role_id,
			role_kind = excluded.role_kind,
			role_title = excluded.role_title,
			backend = excluded.backend,
			provider = excluded.provider,
			model = excluded.model,
			model_options = excluded.model_options,
			agent = excluded.agent,
			status = excluded.status,
			runtime_session_id = excluded.runtime_session_id,
			runtime_provider_session_id = excluded.runtime_provider_session_id,
			runtime_transcript_path = excluded.runtime_transcript_path,
			runtime_pid = excluded.runtime_pid,
			result = excluded.result,
			result_json = excluded.result_json,
			error = excluded.error,
			created_at = excluded.created_at,
			updated_at = excluded.updated_at,
			completed_at = excluded.completed_at,
			archived_at = excluded.archived_at`,
		worker.ID, worker.RunID, worker.Attempt, worker.LaunchID, worker.RoleID, worker.RoleKind, worker.RoleTitle, worker.Backend, worker.Provider, worker.Model, api.MarshalModelOptionsJSON(worker.ModelOptions), worker.Agent, worker.Status,
		worker.RuntimeSessionID, worker.RuntimeProviderSessionID, worker.RuntimeTranscriptPath, worker.RuntimePID, worker.Result, worker.ResultJSON, worker.Error,
		formatLedgerTime(worker.CreatedAt), formatLedgerTime(worker.UpdatedAt), formatLedgerTime(worker.CompletedAt), formatLedgerTime(nowOrTime(worker.UpdatedAt)))
	if err != nil {
		return fmt.Errorf("archive worker attempt: %w", err)
	}
	return nil
}

func (s *SQLiteLedgerStore) ListWorkerAttempts(ctx context.Context, runID string) ([]WorkerAttemptRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, worker_id, run_id, attempt, launch_id, role_id, role_kind, role_title, backend, provider, model, model_options, agent, status, runtime_session_id, runtime_provider_session_id, runtime_transcript_path, runtime_pid, result, result_json, error, created_at, updated_at, completed_at, archived_at FROM worker_attempts WHERE run_id = ? ORDER BY worker_id ASC, attempt ASC`, runID)
	if err != nil {
		return nil, fmt.Errorf("query worker attempts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []WorkerAttemptRecord
	for rows.Next() {
		attempt, err := scanWorkerAttemptRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, attempt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate worker attempts: %w", err)
	}
	return out, nil
}

func scanWorkerRecord(row ledgerScanner) (WorkerRecord, error) {
	var worker WorkerRecord
	var status string
	var modelOptions string
	var created, updated, completed string
	if err := row.Scan(&worker.ID, &worker.RunID, &worker.LaunchID, &worker.Attempt, &worker.RoleID, &worker.RoleKind, &worker.RoleTitle, &worker.Backend, &worker.Provider, &worker.Model, &modelOptions, &worker.Agent, &status, &worker.RuntimeSessionID, &worker.RuntimeProviderSessionID, &worker.RuntimeTranscriptPath, &worker.RuntimePID, &worker.Result, &worker.ResultJSON, &worker.Error, &created, &updated, &completed); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return WorkerRecord{}, fmt.Errorf("worker not found")
		}
		return WorkerRecord{}, fmt.Errorf("scan worker: %w", err)
	}
	if worker.Attempt == 0 {
		worker.Attempt = 1
	}
	worker.ModelOptions = api.ParseModelOptionsJSON(modelOptions)
	worker.Status = WorkerStatus(status)
	worker.CreatedAt = parseLedgerTime(created)
	worker.UpdatedAt = parseLedgerTime(updated)
	worker.CompletedAt = parseLedgerTime(completed)
	return worker, nil
}

func scanWorkerAttemptRecord(row ledgerScanner) (WorkerAttemptRecord, error) {
	var attempt WorkerAttemptRecord
	var status string
	var modelOptions string
	var created, updated, completed, archived string
	if err := row.Scan(&attempt.ID, &attempt.WorkerID, &attempt.RunID, &attempt.Attempt, &attempt.LaunchID, &attempt.RoleID, &attempt.RoleKind, &attempt.RoleTitle, &attempt.Backend, &attempt.Provider, &attempt.Model, &modelOptions, &attempt.Agent, &status, &attempt.RuntimeSessionID, &attempt.RuntimeProviderSessionID, &attempt.RuntimeTranscriptPath, &attempt.RuntimePID, &attempt.Result, &attempt.ResultJSON, &attempt.Error, &created, &updated, &completed, &archived); err != nil {
		return WorkerAttemptRecord{}, fmt.Errorf("scan worker attempt: %w", err)
	}
	attempt.ModelOptions = api.ParseModelOptionsJSON(modelOptions)
	attempt.Status = WorkerStatus(status)
	attempt.CreatedAt = parseLedgerTime(created)
	attempt.UpdatedAt = parseLedgerTime(updated)
	attempt.CompletedAt = parseLedgerTime(completed)
	attempt.ArchivedAt = parseLedgerTime(archived)
	return attempt, nil
}

func nowOrTime(value time.Time) time.Time {
	if value.IsZero() {
		return time.Now()
	}
	return value
}
