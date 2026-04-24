package orchestration

import (
	"context"
	"fmt"
	"time"
)

func (s *SQLiteLedgerStore) UpdateWorkerRunning(ctx context.Context, workerID string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE workers SET status = ?, updated_at = ? WHERE id = ?`, WorkerRunning, formatLedgerTime(time.Now()), workerID)
	if err != nil {
		return fmt.Errorf("update worker running status: %w", err)
	}
	return nil
}

func (s *SQLiteLedgerStore) UpdateWorkerRuntimeIdentity(ctx context.Context, workerID string, identity RuntimeIdentity) error {
	_, err := s.db.ExecContext(ctx, `UPDATE workers SET runtime_session_id = ?, runtime_provider_session_id = ?, runtime_transcript_path = ?, runtime_pid = ?, updated_at = ? WHERE id = ?`, identity.SessionID, identity.ProviderSessionID, identity.TranscriptPath, identity.PID, formatLedgerTime(time.Now()), workerID)
	if err != nil {
		return fmt.Errorf("update worker runtime identity: %w", err)
	}
	return nil
}

func (s *SQLiteLedgerStore) ResetWorkerForRetry(ctx context.Context, workerID string, launchID string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE workers SET
		launch_id = ?,
		attempt = attempt + 1,
		status = ?,
		runtime_session_id = '',
		runtime_provider_session_id = '',
		runtime_transcript_path = '',
		runtime_pid = 0,
		result = '',
		result_json = '',
		error = '',
		updated_at = ?,
		completed_at = ''
		WHERE id = ?`, launchID, WorkerQueued, formatLedgerTime(time.Now()), workerID)
	if err != nil {
		return fmt.Errorf("reset worker for retry: %w", err)
	}
	return nil
}

func (s *SQLiteLedgerStore) CompleteWorker(ctx context.Context, workerID string, status WorkerStatus, result string, resultJSON string, errText string) error {
	now := time.Now()
	_, err := s.db.ExecContext(ctx, `UPDATE workers SET status = ?, result = ?, result_json = ?, error = ?, updated_at = ?, completed_at = ? WHERE id = ? AND (status != ? OR ? = ?)`, status, result, resultJSON, errText, formatLedgerTime(now), formatLedgerTime(now), workerID, WorkerCancelled, status, WorkerCancelled)
	if err != nil {
		return fmt.Errorf("complete worker: %w", err)
	}
	return nil
}
