package orchestration

import "time"

import api "github.com/benjaminwestern/agentic-control/pkg/controlplane"

type RunStatus string

const (
	RunQueued    RunStatus = "queued"
	RunRunning   RunStatus = "running"
	RunCompleted RunStatus = "completed"
	RunFailed    RunStatus = "failed"
	RunCancelled RunStatus = "cancelled"
)

func IsTerminalRunStatus(status RunStatus) bool {
	return status == RunCompleted || status == RunFailed || status == RunCancelled
}

type WorkerStatus string

const (
	WorkerQueued    WorkerStatus = "queued"
	WorkerRunning   WorkerStatus = "running"
	WorkerCompleted WorkerStatus = "completed"
	WorkerFailed    WorkerStatus = "failed"
	WorkerCancelled WorkerStatus = "cancelled"
)

type RuntimeIdentity struct {
	SessionID         string `json:"session_id,omitempty"`
	ProviderSessionID string `json:"provider_session_id,omitempty"`
	TranscriptPath    string `json:"transcript_path,omitempty"`
	PID               int    `json:"pid,omitempty"`
}

type WorkerRecord struct {
	ID                       string           `json:"id"`
	RunID                    string           `json:"run_id"`
	LaunchID                 string           `json:"launch_id"`
	Attempt                  int              `json:"attempt"`
	RoleID                   string           `json:"role_id"`
	RoleKind                 string           `json:"role_kind"`
	RoleTitle                string           `json:"role_title"`
	Backend                  string           `json:"backend"`
	Provider                 string           `json:"provider,omitempty"`
	Model                    string           `json:"model,omitempty"`
	ModelOptions             api.ModelOptions `json:"model_options,omitempty"`
	Agent                    string           `json:"agent,omitempty"`
	Status                   WorkerStatus     `json:"status"`
	RuntimeSessionID         string           `json:"runtime_session_id,omitempty"`
	RuntimeProviderSessionID string           `json:"runtime_provider_session_id,omitempty"`
	RuntimeTranscriptPath    string           `json:"runtime_transcript_path,omitempty"`
	RuntimePID               int              `json:"runtime_pid,omitempty"`
	Result                   string           `json:"result,omitempty"`
	ResultJSON               string           `json:"result_json,omitempty"`
	Error                    string           `json:"error,omitempty"`
	CreatedAt                time.Time        `json:"created_at"`
	UpdatedAt                time.Time        `json:"updated_at"`
	CompletedAt              time.Time        `json:"completed_at,omitempty"`
}

type WorkerAttemptRecord struct {
	ID                       int64            `json:"id"`
	WorkerID                 string           `json:"worker_id"`
	RunID                    string           `json:"run_id"`
	Attempt                  int              `json:"attempt"`
	LaunchID                 string           `json:"launch_id"`
	RoleID                   string           `json:"role_id"`
	RoleKind                 string           `json:"role_kind"`
	RoleTitle                string           `json:"role_title"`
	Backend                  string           `json:"backend"`
	Provider                 string           `json:"provider,omitempty"`
	Model                    string           `json:"model,omitempty"`
	ModelOptions             api.ModelOptions `json:"model_options,omitempty"`
	Agent                    string           `json:"agent,omitempty"`
	Status                   WorkerStatus     `json:"status"`
	RuntimeSessionID         string           `json:"runtime_session_id,omitempty"`
	RuntimeProviderSessionID string           `json:"runtime_provider_session_id,omitempty"`
	RuntimeTranscriptPath    string           `json:"runtime_transcript_path,omitempty"`
	RuntimePID               int              `json:"runtime_pid,omitempty"`
	Result                   string           `json:"result,omitempty"`
	ResultJSON               string           `json:"result_json,omitempty"`
	Error                    string           `json:"error,omitempty"`
	CreatedAt                time.Time        `json:"created_at"`
	UpdatedAt                time.Time        `json:"updated_at"`
	CompletedAt              time.Time        `json:"completed_at,omitempty"`
	ArchivedAt               time.Time        `json:"archived_at"`
}

type WorkerControlAction string

const (
	WorkerControlCancel    WorkerControlAction = "cancel"
	WorkerControlInterrupt WorkerControlAction = "interrupt"
	WorkerControlRetry     WorkerControlAction = "retry"
	WorkerControlResume    WorkerControlAction = "resume"
)

type WorkerControlStatus string

const (
	WorkerControlPending   WorkerControlStatus = "pending"
	WorkerControlCompleted WorkerControlStatus = "completed"
	WorkerControlFailed    WorkerControlStatus = "failed"
)

type RuntimeRequestStatus string

const (
	RuntimeRequestOpen      RuntimeRequestStatus = "open"
	RuntimeRequestResponded RuntimeRequestStatus = "responded"
	RuntimeRequestResolved  RuntimeRequestStatus = "resolved"
	RuntimeRequestClosed    RuntimeRequestStatus = "closed"
)

type RuntimeResponseStatus string

const (
	RuntimeResponseNone      RuntimeResponseStatus = ""
	RuntimeResponseQueued    RuntimeResponseStatus = "queued"
	RuntimeResponseCompleted RuntimeResponseStatus = "completed"
	RuntimeResponseFailed    RuntimeResponseStatus = "failed"
)

type RuntimeRequestAnswer struct {
	QuestionID string `json:"question_id,omitempty"`
	OptionID   string `json:"option_id,omitempty"`
	Text       string `json:"text,omitempty"`
}

type RuntimeRequestResponse struct {
	Action   string                 `json:"action,omitempty"`
	Text     string                 `json:"text,omitempty"`
	OptionID string                 `json:"option_id,omitempty"`
	Answers  []RuntimeRequestAnswer `json:"answers,omitempty"`
}

type WorkerControlRequest struct {
	ID        int64               `json:"id"`
	RunID     string              `json:"run_id"`
	WorkerID  string              `json:"worker_id"`
	Action    WorkerControlAction `json:"action"`
	Status    WorkerControlStatus `json:"status"`
	Error     string              `json:"error,omitempty"`
	CreatedAt time.Time           `json:"created_at"`
	UpdatedAt time.Time           `json:"updated_at"`
}

type RuntimeRequestRecord struct {
	ID                       int64                 `json:"id"`
	RunID                    string                `json:"run_id"`
	WorkerID                 string                `json:"worker_id"`
	RequestID                string                `json:"request_id"`
	RuntimeSessionID         string                `json:"runtime_session_id"`
	RuntimeProviderSessionID string                `json:"runtime_provider_session_id,omitempty"`
	Runtime                  string                `json:"runtime"`
	Kind                     string                `json:"kind"`
	NativeMethod             string                `json:"native_method,omitempty"`
	Status                   RuntimeRequestStatus  `json:"status"`
	Summary                  string                `json:"summary,omitempty"`
	TurnID                   string                `json:"turn_id,omitempty"`
	RequestJSON              string                `json:"request_json,omitempty"`
	ResponseStatus           RuntimeResponseStatus `json:"response_status,omitempty"`
	ResponseAction           string                `json:"response_action,omitempty"`
	ResponseText             string                `json:"response_text,omitempty"`
	ResponseOptionID         string                `json:"response_option_id,omitempty"`
	ResponseAnswersJSON      string                `json:"response_answers_json,omitempty"`
	ResponseError            string                `json:"response_error,omitempty"`
	ResponseJSON             string                `json:"response_json,omitempty"`
	CreatedAt                time.Time             `json:"created_at"`
	UpdatedAt                time.Time             `json:"updated_at"`
	RespondedAt              time.Time             `json:"responded_at,omitempty"`
}

type EventRecord struct {
	ID        int64     `json:"id"`
	RunID     string    `json:"run_id"`
	WorkerID  string    `json:"worker_id,omitempty"`
	Type      string    `json:"type"`
	Message   string    `json:"message"`
	Payload   string    `json:"payload,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type ArtifactRecord struct {
	ID        int64     `json:"id"`
	RunID     string    `json:"run_id"`
	WorkerID  string    `json:"worker_id,omitempty"`
	Kind      string    `json:"kind"`
	Format    string    `json:"format"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}
