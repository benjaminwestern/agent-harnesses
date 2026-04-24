package orchestration

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type SQLiteRunStateStore struct {
	db *sql.DB
}

func NewSQLiteRunStateStore(db *sql.DB) *SQLiteRunStateStore {
	return &SQLiteRunStateStore{db: db}
}

func (s *SQLiteRunStateStore) UpdateRunStatus(ctx context.Context, runID string, status RunStatus, output string) error {
	now := time.Now()
	completed := ""
	if IsTerminalRunStatus(status) {
		completed = formatLedgerTime(now)
	}
	_, err := s.db.ExecContext(ctx, `UPDATE runs SET status = ?, verdict = ?, updated_at = ?, completed_at = ? WHERE id = ?`, status, output, formatLedgerTime(now), completed, runID)
	if err != nil {
		return fmt.Errorf("update run status: %w", err)
	}
	return nil
}

func (s *SQLiteRunStateStore) ReactivateRun(ctx context.Context, runID string, stage string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE runs SET status = ?, phase = ?, verdict = '', updated_at = ?, completed_at = '' WHERE id = ?`, RunRunning, stage, formatLedgerTime(time.Now()), runID)
	if err != nil {
		return fmt.Errorf("reactivate run: %w", err)
	}
	return nil
}

func (s *SQLiteRunStateStore) UpdateRunStage(ctx context.Context, runID string, stage string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE runs SET phase = ?, updated_at = ? WHERE id = ?`, stage, formatLedgerTime(time.Now()), runID)
	if err != nil {
		return fmt.Errorf("update run stage: %w", err)
	}
	return nil
}
