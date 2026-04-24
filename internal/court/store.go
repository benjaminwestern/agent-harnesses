// Package court provides Court runtime functionality.
package court

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/benjaminwestern/agentic-control/internal/orchestration"
	api "github.com/benjaminwestern/agentic-control/pkg/controlplane"
	_ "github.com/mattn/go-sqlite3" // Registers the SQLite driver for database/sql.
)

// Store defines Court runtime data.
type Store struct {
	db       *sql.DB
	ledger   *orchestration.SQLiteLedgerStore
	runState *orchestration.SQLiteRunStateStore
}

func closeRows(rows *sql.Rows) {
	_ = rows.Close()
}

func (s *Store) execContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, wrapErr("execute sql", err)
	}
	return result, nil
}

func (s *Store) queryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, wrapErr("query sql", err)
	}
	return rows, nil
}

func rowsErr(rows *sql.Rows) error {
	return wrapErr("iterate sql rows", rows.Err())
}

func lastInsertID(result sql.Result) (int64, error) {
	id, err := result.LastInsertId()
	return id, wrapErr("read last insert id", err)
}

func rowsAffected(result sql.Result) (int64, error) {
	count, err := result.RowsAffected()
	return count, wrapErr("read affected row count", err)
}

// OpenStore provides Court runtime functionality.
func OpenStore(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, wrapErr("create store directory", err)
	}
	db, err := sql.Open("sqlite3", path+"?_busy_timeout=5000&_fk=1")
	if err != nil {
		return nil, wrapErr("open sqlite database", err)
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)

	store := &Store{db: db, ledger: orchestration.NewSQLiteLedgerStore(db), runState: orchestration.NewSQLiteRunStateStore(db)}
	if err := store.init(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

// Close provides Court runtime functionality.
func (s *Store) Close() error {
	return wrapErr("close sqlite database", s.db.Close())
}

func (s *Store) init(ctx context.Context) error {
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
	}
	for _, pragma := range pragmas {
		if _, err := s.execContext(ctx, pragma); err != nil {
			return err
		}
	}

	schema := []string{
		`CREATE TABLE IF NOT EXISTS runs (
			id TEXT PRIMARY KEY,
			court_id TEXT NOT NULL DEFAULT '',
			task TEXT NOT NULL,
			preset TEXT NOT NULL,
			workflow TEXT NOT NULL DEFAULT 'parallel_consensus',
			delegation_scope TEXT NOT NULL DEFAULT 'preset',
				backend TEXT NOT NULL,
				model TEXT NOT NULL DEFAULT '',
				model_options TEXT NOT NULL DEFAULT '',
				default_provider TEXT NOT NULL DEFAULT '',
				default_model TEXT NOT NULL DEFAULT '',
				default_model_options TEXT NOT NULL DEFAULT '',
				workspace TEXT NOT NULL,
			status TEXT NOT NULL,
			phase TEXT NOT NULL DEFAULT 'idle',
			verdict TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			completed_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS workers (
			id TEXT PRIMARY KEY,
			run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
			launch_id TEXT NOT NULL DEFAULT '',
			attempt INTEGER NOT NULL DEFAULT 1,
			role_id TEXT NOT NULL,
			role_kind TEXT NOT NULL,
			role_title TEXT NOT NULL,
			backend TEXT NOT NULL,
				provider TEXT NOT NULL DEFAULT '',
				model TEXT NOT NULL DEFAULT '',
				model_options TEXT NOT NULL DEFAULT '',
				agent TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			runtime_session_id TEXT NOT NULL DEFAULT '',
			runtime_provider_session_id TEXT NOT NULL DEFAULT '',
			runtime_transcript_path TEXT NOT NULL DEFAULT '',
			runtime_pid INTEGER NOT NULL DEFAULT 0,
			result TEXT NOT NULL DEFAULT '',
			result_json TEXT NOT NULL DEFAULT '',
			error TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			completed_at TEXT NOT NULL DEFAULT '',
			UNIQUE(run_id, role_id)
		)`,
		`CREATE TABLE IF NOT EXISTS worker_attempts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			worker_id TEXT NOT NULL,
			run_id TEXT NOT NULL,
			attempt INTEGER NOT NULL,
			launch_id TEXT NOT NULL DEFAULT '',
			role_id TEXT NOT NULL,
			role_kind TEXT NOT NULL,
			role_title TEXT NOT NULL,
			backend TEXT NOT NULL,
				provider TEXT NOT NULL DEFAULT '',
				model TEXT NOT NULL DEFAULT '',
				model_options TEXT NOT NULL DEFAULT '',
				agent TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			runtime_session_id TEXT NOT NULL DEFAULT '',
			runtime_provider_session_id TEXT NOT NULL DEFAULT '',
			runtime_transcript_path TEXT NOT NULL DEFAULT '',
			runtime_pid INTEGER NOT NULL DEFAULT 0,
			result TEXT NOT NULL DEFAULT '',
			result_json TEXT NOT NULL DEFAULT '',
			error TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			completed_at TEXT NOT NULL DEFAULT '',
			archived_at TEXT NOT NULL,
			UNIQUE(worker_id, attempt)
		)`,
		`CREATE TABLE IF NOT EXISTS events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			run_id TEXT NOT NULL,
			worker_id TEXT NOT NULL DEFAULT '',
			type TEXT NOT NULL,
			message TEXT NOT NULL,
			payload TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS artifacts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			run_id TEXT NOT NULL,
			worker_id TEXT NOT NULL DEFAULT '',
			kind TEXT NOT NULL,
			format TEXT NOT NULL,
			content TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS worker_controls (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			run_id TEXT NOT NULL,
			worker_id TEXT NOT NULL REFERENCES workers(id) ON DELETE CASCADE,
			action TEXT NOT NULL,
			status TEXT NOT NULL,
			error TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS runtime_requests (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			run_id TEXT NOT NULL,
			worker_id TEXT NOT NULL REFERENCES workers(id) ON DELETE CASCADE,
			request_id TEXT NOT NULL,
			runtime_session_id TEXT NOT NULL,
			runtime_provider_session_id TEXT NOT NULL DEFAULT '',
			runtime TEXT NOT NULL DEFAULT '',
			kind TEXT NOT NULL DEFAULT '',
			native_method TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			summary TEXT NOT NULL DEFAULT '',
			turn_id TEXT NOT NULL DEFAULT '',
			request_json TEXT NOT NULL DEFAULT '',
			response_status TEXT NOT NULL DEFAULT '',
			response_action TEXT NOT NULL DEFAULT '',
			response_text TEXT NOT NULL DEFAULT '',
			response_option_id TEXT NOT NULL DEFAULT '',
			response_answers_json TEXT NOT NULL DEFAULT '',
			response_error TEXT NOT NULL DEFAULT '',
			response_json TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			responded_at TEXT NOT NULL DEFAULT '',
			UNIQUE(worker_id, request_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_runs_status ON runs(status)`,
		`CREATE INDEX IF NOT EXISTS idx_workers_run ON workers(run_id)`,
		`CREATE INDEX IF NOT EXISTS idx_worker_attempts_worker ON worker_attempts(worker_id, attempt)`,
		`CREATE INDEX IF NOT EXISTS idx_events_run_id ON events(run_id, id)`,
		`CREATE INDEX IF NOT EXISTS idx_artifacts_run_id ON artifacts(run_id, kind)`,
		`CREATE INDEX IF NOT EXISTS idx_worker_controls_pending ON worker_controls(worker_id, status, id)`,
		`CREATE INDEX IF NOT EXISTS idx_runtime_requests_run ON runtime_requests(run_id, status, id)`,
		`CREATE INDEX IF NOT EXISTS idx_runtime_requests_response ON runtime_requests(worker_id, response_status, id)`,
	}
	for _, stmt := range schema {
		if _, err := s.execContext(ctx, stmt); err != nil {
			return err
		}
	}
	migrations := []string{
		`ALTER TABLE runs ADD COLUMN court_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE runs ADD COLUMN workflow TEXT NOT NULL DEFAULT 'parallel_consensus'`,
		`ALTER TABLE runs ADD COLUMN delegation_scope TEXT NOT NULL DEFAULT 'preset'`,
		`ALTER TABLE runs ADD COLUMN phase TEXT NOT NULL DEFAULT 'idle'`,
		`ALTER TABLE runs ADD COLUMN model TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE runs ADD COLUMN model_options TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE runs ADD COLUMN default_provider TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE runs ADD COLUMN default_model TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE runs ADD COLUMN default_model_options TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE workers ADD COLUMN launch_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE workers ADD COLUMN attempt INTEGER NOT NULL DEFAULT 1`,
		`ALTER TABLE workers ADD COLUMN provider TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE workers ADD COLUMN model_options TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE workers ADD COLUMN runtime_session_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE workers ADD COLUMN runtime_provider_session_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE workers ADD COLUMN runtime_transcript_path TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE workers ADD COLUMN runtime_pid INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE workers ADD COLUMN result_json TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE worker_attempts ADD COLUMN model_options TEXT NOT NULL DEFAULT ''`,
	}
	for _, stmt := range migrations {
		if _, err := s.execContext(ctx, stmt); err != nil && !strings.Contains(err.Error(), "duplicate column") {
			return err
		}
	}
	return nil
}

// CreateRun provides Court runtime functionality.
func (s *Store) CreateRun(ctx context.Context, run Run) error {
	_, err := s.execContext(ctx, `INSERT INTO runs
		(id, court_id, task, preset, workflow, delegation_scope, backend, model, model_options, default_provider, default_model, default_model_options, workspace, status, phase, verdict, created_at, updated_at, completed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run.ID, run.CourtID, run.Task, run.Preset, run.Workflow, run.DelegationScope, run.Backend, run.Model, api.MarshalModelOptionsJSON(run.ModelOptions), run.DefaultProvider, run.DefaultModel, api.MarshalModelOptionsJSON(run.DefaultModelOptions), run.Workspace, run.Status, run.Phase, run.Verdict,
		formatTime(run.CreatedAt), formatTime(run.UpdatedAt), formatTime(run.CompletedAt),
	)
	return err
}

// UpdateRunStatus provides Court runtime functionality.
func (s *Store) UpdateRunStatus(ctx context.Context, runID string, status RunStatus, verdict string) error {
	return s.runState.UpdateRunStatus(ctx, runID, orchestration.RunStatus(status), verdict)
}

// ReactivateRun provides Court runtime functionality.
func (s *Store) ReactivateRun(ctx context.Context, runID string, phase Phase) error {
	return s.runState.ReactivateRun(ctx, runID, string(phase))
}

// UpdateRunPhase provides Court runtime functionality.
func (s *Store) UpdateRunPhase(ctx context.Context, runID string, phase Phase) error {
	return s.runState.UpdateRunStage(ctx, runID, string(phase))
}

// GetRun provides Court runtime functionality.
func (s *Store) GetRun(ctx context.Context, id string) (Run, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, court_id, task, preset, workflow, delegation_scope, backend, model, model_options, default_provider, default_model, default_model_options, workspace, status, phase, verdict, created_at, updated_at, completed_at FROM runs WHERE id = ?`, id)
	return scanRun(row)
}

// ListRuns provides Court runtime functionality.
func (s *Store) ListRuns(ctx context.Context) ([]Run, error) {
	rows, err := s.queryContext(ctx, `SELECT id, court_id, task, preset, workflow, delegation_scope, backend, model, model_options, default_provider, default_model, default_model_options, workspace, status, phase, verdict, created_at, updated_at, completed_at FROM runs ORDER BY created_at DESC LIMIT 50`)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)

	var out []Run
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	return out, rowsErr(rows)
}

// CreateWorker provides Court runtime functionality.
func (s *Store) CreateWorker(ctx context.Context, worker Worker) error {
	return s.ledger.CreateWorker(ctx, orchestrationWorkerRecord(worker))
}

// GetWorker provides Court runtime functionality.
func (s *Store) GetWorker(ctx context.Context, id string) (Worker, error) {
	record, err := s.ledger.GetWorker(ctx, id)
	if err != nil {
		return Worker{}, err
	}
	return courtWorkerFromRecord(record), nil
}

// ListWorkers provides Court runtime functionality.
func (s *Store) ListWorkers(ctx context.Context, runID string) ([]Worker, error) {
	records, err := s.ledger.ListWorkers(ctx, runID)
	if err != nil {
		return nil, err
	}
	out := make([]Worker, 0, len(records))
	for _, record := range records {
		out = append(out, courtWorkerFromRecord(record))
	}
	return out, nil
}

// UpdateWorkerRunning provides Court runtime functionality.
func (s *Store) UpdateWorkerRunning(ctx context.Context, workerID string) error {
	return s.ledger.UpdateWorkerRunning(ctx, workerID)
}

// UpdateWorkerRuntimeIdentity provides Court runtime functionality.
func (s *Store) UpdateWorkerRuntimeIdentity(ctx context.Context, workerID string, identity RuntimeIdentity) error {
	return s.ledger.UpdateWorkerRuntimeIdentity(ctx, workerID, identity)
}

// ResetWorkerForRetry provides Court runtime functionality.
func (s *Store) ResetWorkerForRetry(ctx context.Context, workerID string, launchID string) error {
	return s.ledger.ResetWorkerForRetry(ctx, workerID, launchID)
}

// ArchiveWorkerAttempt provides Court runtime functionality.
func (s *Store) ArchiveWorkerAttempt(ctx context.Context, worker Worker) error {
	return s.ledger.ArchiveWorkerAttempt(ctx, orchestrationWorkerRecord(worker))
}

// ListWorkerAttempts provides Court runtime functionality.
func (s *Store) ListWorkerAttempts(ctx context.Context, runID string) ([]WorkerAttempt, error) {
	records, err := s.ledger.ListWorkerAttempts(ctx, runID)
	if err != nil {
		return nil, err
	}
	out := make([]WorkerAttempt, 0, len(records))
	for _, record := range records {
		out = append(out, courtWorkerAttemptFromRecord(record))
	}
	return out, nil
}

// CompleteWorker provides Court runtime functionality.
func (s *Store) CompleteWorker(ctx context.Context, workerID string, status WorkerStatus, result string, resultJSON string, errText string) error {
	return s.ledger.CompleteWorker(ctx, workerID, status, result, resultJSON, errText)
}

// AddWorkerControl provides Court runtime functionality.
func (s *Store) AddWorkerControl(ctx context.Context, runID string, workerID string, action WorkerControlAction) (WorkerControlRequest, error) {
	return s.ledger.AddWorkerControl(ctx, runID, workerID, action)
}

// CompleteWorkerControl provides Court runtime functionality.
func (s *Store) CompleteWorkerControl(ctx context.Context, id int64, status WorkerControlStatus, errText string) error {
	return s.ledger.CompleteWorkerControl(ctx, id, status, errText)
}

// CompletePendingWorkerControls provides Court runtime functionality.
func (s *Store) CompletePendingWorkerControls(ctx context.Context, workerID string, status WorkerControlStatus, errText string) error {
	return s.ledger.CompletePendingWorkerControls(ctx, workerID, status, errText)
}

// ListPendingWorkerControls provides Court runtime functionality.
func (s *Store) ListPendingWorkerControls(ctx context.Context, workerID string) ([]WorkerControlRequest, error) {
	return s.ledger.ListPendingWorkerControls(ctx, workerID)
}

// ListWorkerControls provides Court runtime functionality.
func (s *Store) ListWorkerControls(ctx context.Context, runID string) ([]WorkerControlRequest, error) {
	return s.ledger.ListWorkerControls(ctx, runID)
}

// UpsertRuntimeRequest provides Court runtime functionality.
func (s *Store) UpsertRuntimeRequest(ctx context.Context, request RuntimeRequest) error {
	return s.ledger.UpsertRuntimeRequest(ctx, request)
}

// GetRuntimeRequest provides Court runtime functionality.
func (s *Store) GetRuntimeRequest(ctx context.Context, id int64) (RuntimeRequest, error) {
	return s.ledger.GetRuntimeRequest(ctx, id)
}

// ListRuntimeRequests provides Court runtime functionality.
func (s *Store) ListRuntimeRequests(ctx context.Context, runID string, status RuntimeRequestStatus) ([]RuntimeRequest, error) {
	return s.ledger.ListRuntimeRequests(ctx, runID, status)
}

// ListQueuedRuntimeRequestResponses provides Court runtime functionality.
func (s *Store) ListQueuedRuntimeRequestResponses(ctx context.Context, workerID string) ([]RuntimeRequest, error) {
	return s.ledger.ListQueuedRuntimeRequestResponses(ctx, workerID)
}

// QueueRuntimeRequestResponse provides Court runtime functionality.
func (s *Store) QueueRuntimeRequestResponse(ctx context.Context, id int64, response RuntimeRequestResponse) error {
	return s.ledger.QueueRuntimeRequestResponse(ctx, id, response)
}

// CompleteRuntimeRequestResponse provides Court runtime functionality.
func (s *Store) CompleteRuntimeRequestResponse(ctx context.Context, id int64, status RuntimeResponseStatus, responseJSON string, errText string) error {
	return s.ledger.CompleteRuntimeRequestResponse(ctx, id, status, responseJSON, errText)
}

// AddEvent provides Court runtime functionality.
func (s *Store) AddEvent(ctx context.Context, event Event) error {
	return s.ledger.AddEvent(ctx, event)
}

// ListEvents provides Court runtime functionality.
func (s *Store) ListEvents(ctx context.Context, runID string, after int64) ([]Event, error) {
	return s.ledger.ListEvents(ctx, runID, after)
}

// AddArtifact provides Court runtime functionality.
func (s *Store) AddArtifact(ctx context.Context, artifact Artifact) error {
	return s.ledger.AddArtifact(ctx, artifact)
}

// ListArtifacts provides Court runtime functionality.
func (s *Store) ListArtifacts(ctx context.Context, runID string) ([]Artifact, error) {
	return s.ledger.ListArtifacts(ctx, runID)
}

type scanner interface {
	Scan(dest ...any) error
}

func scanRun(row scanner) (Run, error) {
	var run Run
	var status, workflow, delegationScope, phase string
	var modelOptions, defaultModelOptions string
	var created, updated, completed string
	if err := row.Scan(&run.ID, &run.CourtID, &run.Task, &run.Preset, &workflow, &delegationScope, &run.Backend, &run.Model, &modelOptions, &run.DefaultProvider, &run.DefaultModel, &defaultModelOptions, &run.Workspace, &status, &phase, &run.Verdict, &created, &updated, &completed); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Run{}, fmt.Errorf("run not found")
		}
		return Run{}, wrapErr("scan run", err)
	}
	run.Status = RunStatus(status)
	run.ModelOptions = api.ParseModelOptionsJSON(modelOptions)
	run.DefaultModelOptions = api.ParseModelOptionsJSON(defaultModelOptions)
	run.Workflow = WorkflowMode(workflow)
	run.DelegationScope = ResolveDelegationScope(delegationScope, DelegationScopePreset)
	run.Phase = Phase(phase)
	run.CreatedAt = parseTime(created)
	run.UpdatedAt = parseTime(updated)
	run.CompletedAt = parseTime(completed)
	return run, nil
}

func scanWorker(row scanner) (Worker, error) {
	var worker Worker
	var kind, status string
	var modelOptions string
	var created, updated, completed string
	if err := row.Scan(&worker.ID, &worker.RunID, &worker.LaunchID, &worker.Attempt, &worker.RoleID, &kind, &worker.RoleTitle, &worker.Backend, &worker.Provider, &worker.Model, &modelOptions, &worker.Agent, &status, &worker.RuntimeSessionID, &worker.RuntimeProviderSessionID, &worker.RuntimeTranscriptPath, &worker.RuntimePID, &worker.Result, &worker.ResultJSON, &worker.Error, &created, &updated, &completed); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Worker{}, fmt.Errorf("worker not found")
		}
		return Worker{}, wrapErr("scan worker", err)
	}
	if worker.Attempt == 0 {
		worker.Attempt = 1
	}
	worker.RoleKind = RoleKind(kind)
	worker.ModelOptions = api.ParseModelOptionsJSON(modelOptions)
	worker.Status = WorkerStatus(status)
	worker.CreatedAt = parseTime(created)
	worker.UpdatedAt = parseTime(updated)
	worker.CompletedAt = parseTime(completed)
	return worker, nil
}

func scanWorkerAttempt(row scanner) (WorkerAttempt, error) {
	var attempt WorkerAttempt
	var kind, status string
	var modelOptions string
	var created, updated, completed, archived string
	if err := row.Scan(&attempt.ID, &attempt.WorkerID, &attempt.RunID, &attempt.Attempt, &attempt.LaunchID, &attempt.RoleID, &kind, &attempt.RoleTitle, &attempt.Backend, &attempt.Provider, &attempt.Model, &modelOptions, &attempt.Agent, &status, &attempt.RuntimeSessionID, &attempt.RuntimeProviderSessionID, &attempt.RuntimeTranscriptPath, &attempt.RuntimePID, &attempt.Result, &attempt.ResultJSON, &attempt.Error, &created, &updated, &completed, &archived); err != nil {
		return WorkerAttempt{}, wrapErr("scan worker attempt", err)
	}
	attempt.RoleKind = RoleKind(kind)
	attempt.ModelOptions = api.ParseModelOptionsJSON(modelOptions)
	attempt.Status = WorkerStatus(status)
	attempt.CreatedAt = parseTime(created)
	attempt.UpdatedAt = parseTime(updated)
	attempt.CompletedAt = parseTime(completed)
	attempt.ArchivedAt = parseTime(archived)
	return attempt, nil
}

func scanWorkerControl(row scanner) (WorkerControlRequest, error) {
	var control WorkerControlRequest
	var action, status string
	var created, updated string
	if err := row.Scan(&control.ID, &control.RunID, &control.WorkerID, &action, &status, &control.Error, &created, &updated); err != nil {
		return WorkerControlRequest{}, wrapErr("scan worker control", err)
	}
	control.Action = WorkerControlAction(action)
	control.Status = WorkerControlStatus(status)
	control.CreatedAt = parseTime(created)
	control.UpdatedAt = parseTime(updated)
	return control, nil
}

func scanRuntimeRequest(row scanner) (RuntimeRequest, error) {
	var request RuntimeRequest
	var status, responseStatus string
	var created, updated, responded string
	if err := row.Scan(
		&request.ID,
		&request.RunID,
		&request.WorkerID,
		&request.RequestID,
		&request.RuntimeSessionID,
		&request.RuntimeProviderSessionID,
		&request.Runtime,
		&request.Kind,
		&request.NativeMethod,
		&status,
		&request.Summary,
		&request.TurnID,
		&request.RequestJSON,
		&responseStatus,
		&request.ResponseAction,
		&request.ResponseText,
		&request.ResponseOptionID,
		&request.ResponseAnswersJSON,
		&request.ResponseError,
		&request.ResponseJSON,
		&created,
		&updated,
		&responded,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return RuntimeRequest{}, fmt.Errorf("runtime request not found")
		}
		return RuntimeRequest{}, wrapErr("scan runtime request", err)
	}
	request.Status = RuntimeRequestStatus(status)
	request.ResponseStatus = RuntimeResponseStatus(responseStatus)
	request.CreatedAt = parseTime(created)
	request.UpdatedAt = parseTime(updated)
	request.RespondedAt = parseTime(responded)
	return request, nil
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	t, _ := time.Parse(time.RFC3339Nano, value)
	return t
}

func orchestrationWorkerRecord(worker Worker) orchestration.WorkerRecord {
	return orchestration.WorkerRecord{
		ID:                       worker.ID,
		RunID:                    worker.RunID,
		LaunchID:                 worker.LaunchID,
		Attempt:                  worker.Attempt,
		RoleID:                   worker.RoleID,
		RoleKind:                 string(worker.RoleKind),
		RoleTitle:                worker.RoleTitle,
		Backend:                  worker.Backend,
		Provider:                 worker.Provider,
		Model:                    worker.Model,
		ModelOptions:             worker.ModelOptions,
		Agent:                    worker.Agent,
		Status:                   orchestration.WorkerStatus(worker.Status),
		RuntimeSessionID:         worker.RuntimeSessionID,
		RuntimeProviderSessionID: worker.RuntimeProviderSessionID,
		RuntimeTranscriptPath:    worker.RuntimeTranscriptPath,
		RuntimePID:               worker.RuntimePID,
		Result:                   worker.Result,
		ResultJSON:               worker.ResultJSON,
		Error:                    worker.Error,
		CreatedAt:                worker.CreatedAt,
		UpdatedAt:                worker.UpdatedAt,
		CompletedAt:              worker.CompletedAt,
	}
}

func courtWorkerFromRecord(record orchestration.WorkerRecord) Worker {
	return Worker{
		ID:                       record.ID,
		RunID:                    record.RunID,
		LaunchID:                 record.LaunchID,
		Attempt:                  record.Attempt,
		RoleID:                   record.RoleID,
		RoleKind:                 RoleKind(record.RoleKind),
		RoleTitle:                record.RoleTitle,
		Backend:                  record.Backend,
		Provider:                 record.Provider,
		Model:                    record.Model,
		ModelOptions:             record.ModelOptions,
		Agent:                    record.Agent,
		Status:                   WorkerStatus(record.Status),
		RuntimeSessionID:         record.RuntimeSessionID,
		RuntimeProviderSessionID: record.RuntimeProviderSessionID,
		RuntimeTranscriptPath:    record.RuntimeTranscriptPath,
		RuntimePID:               record.RuntimePID,
		Result:                   record.Result,
		ResultJSON:               record.ResultJSON,
		Error:                    record.Error,
		CreatedAt:                record.CreatedAt,
		UpdatedAt:                record.UpdatedAt,
		CompletedAt:              record.CompletedAt,
	}
}

func courtWorkerAttemptFromRecord(record orchestration.WorkerAttemptRecord) WorkerAttempt {
	return WorkerAttempt{
		ID:                       record.ID,
		WorkerID:                 record.WorkerID,
		RunID:                    record.RunID,
		Attempt:                  record.Attempt,
		LaunchID:                 record.LaunchID,
		RoleID:                   record.RoleID,
		RoleKind:                 RoleKind(record.RoleKind),
		RoleTitle:                record.RoleTitle,
		Backend:                  record.Backend,
		Provider:                 record.Provider,
		Model:                    record.Model,
		ModelOptions:             record.ModelOptions,
		Agent:                    record.Agent,
		Status:                   WorkerStatus(record.Status),
		RuntimeSessionID:         record.RuntimeSessionID,
		RuntimeProviderSessionID: record.RuntimeProviderSessionID,
		RuntimeTranscriptPath:    record.RuntimeTranscriptPath,
		RuntimePID:               record.RuntimePID,
		Result:                   record.Result,
		ResultJSON:               record.ResultJSON,
		Error:                    record.Error,
		CreatedAt:                record.CreatedAt,
		UpdatedAt:                record.UpdatedAt,
		CompletedAt:              record.CompletedAt,
		ArchivedAt:               record.ArchivedAt,
	}
}
