// Package court provides Court runtime functionality.
package court

import (
	"encoding/json"
	"time"

	"github.com/benjaminwestern/agentic-control/internal/orchestration"
	"github.com/benjaminwestern/agentic-control/pkg/contract"
	api "github.com/benjaminwestern/agentic-control/pkg/controlplane"
)

type RunStatus = orchestration.RunStatus

const (
	RunQueued    = orchestration.RunQueued
	RunRunning   = orchestration.RunRunning
	RunCompleted = orchestration.RunCompleted
	RunFailed    = orchestration.RunFailed
	RunCancelled = orchestration.RunCancelled
)

type WorkerStatus = orchestration.WorkerStatus

const (
	WorkerQueued    = orchestration.WorkerQueued
	WorkerRunning   = orchestration.WorkerRunning
	WorkerCompleted = orchestration.WorkerCompleted
	WorkerFailed    = orchestration.WorkerFailed
	WorkerCancelled = orchestration.WorkerCancelled
)

type WorkerControlAction = orchestration.WorkerControlAction

const (
	WorkerControlCancel    = orchestration.WorkerControlCancel
	WorkerControlInterrupt = orchestration.WorkerControlInterrupt
	WorkerControlRetry     = orchestration.WorkerControlRetry
	WorkerControlResume    = orchestration.WorkerControlResume
)

type WorkerControlStatus = orchestration.WorkerControlStatus

const (
	WorkerControlPending   = orchestration.WorkerControlPending
	WorkerControlCompleted = orchestration.WorkerControlCompleted
	WorkerControlFailed    = orchestration.WorkerControlFailed
)

// RoleKind defines Court runtime data.
type RoleKind string

const (
	// RoleClerk defines a Court runtime value.
	RoleClerk RoleKind = "clerk"
	// RoleJuror defines a Court runtime value.
	RoleJuror RoleKind = "juror"
	// RoleJudge defines a Court runtime value.
	RoleJudge RoleKind = "judge"
)

// WorkflowMode defines Court runtime data.
type WorkflowMode string

const (
	// WorkflowParallelConsensus defines a Court runtime value.
	WorkflowParallelConsensus WorkflowMode = "parallel_consensus"
	// WorkflowRouted defines a Court runtime value.
	WorkflowRouted WorkflowMode = "routed"
	// WorkflowRoleScoped defines a Court runtime value.
	WorkflowRoleScoped WorkflowMode = "role_scoped"
	// WorkflowBoundedCorrection defines a Court runtime value.
	WorkflowBoundedCorrection WorkflowMode = "bounded_correction"
	// WorkflowReviewOnly defines a Court runtime value.
	WorkflowReviewOnly WorkflowMode = "review_only"
)

// DelegationScope defines Court runtime data.
type DelegationScope string

const (
	// DelegationScopePreset defines a Court runtime value.
	DelegationScopePreset DelegationScope = "preset"
	// DelegationScopeWorkspace defines a Court runtime value.
	DelegationScopeWorkspace DelegationScope = "workspace"
	// DelegationScopeGlobal defines a Court runtime value.
	DelegationScopeGlobal DelegationScope = "global"
)

// Phase defines Court runtime data.
type Phase string

const (
	// PhaseIdle defines a Court runtime value.
	PhaseIdle Phase = "idle"
	// PhaseBlocked defines a Court runtime value.
	PhaseBlocked Phase = "blocked"
	// PhaseClerk defines a Court runtime value.
	PhaseClerk Phase = "clerk"
	// PhaseJurors defines a Court runtime value.
	PhaseJurors Phase = "jurors"
	// PhaseInlineJudge defines a Court runtime value.
	PhaseInlineJudge Phase = "inline_judge"
	// PhaseCorrections defines a Court runtime value.
	PhaseCorrections Phase = "corrections"
	// PhaseVerdict defines a Court runtime value.
	PhaseVerdict Phase = "verdict"
	// PhaseComplete defines a Court runtime value.
	PhaseComplete Phase = "complete"
)

// Role defines Court runtime data.
type Role struct {
	ID           string                          `json:"id"`
	Kind         RoleKind                        `json:"kind"`
	Title        string                          `json:"title"`
	Brief        string                          `json:"brief"`
	Backend      string                          `json:"backend,omitempty"`
	Provider     string                          `json:"provider,omitempty"`
	Model        string                          `json:"model,omitempty"`
	ModelOptions RuntimeModelOptions             `json:"model_options,omitempty"`
	Agent        string                          `json:"agent,omitempty"`
	Backends     map[string]RuntimeBackendConfig `json:"backends,omitempty"`
}

// RuntimeBackendConfig defines Court runtime data.
type RuntimeBackendConfig struct {
	Provider     string              `json:"provider,omitempty"`
	Model        string              `json:"model,omitempty"`
	ModelOptions RuntimeModelOptions `json:"model_options,omitempty"`
}

type RuntimeModelOptions = api.ModelOptions

// Preset defines Court runtime data.
type Preset struct {
	ID          string       `json:"id"`
	Title       string       `json:"title"`
	Description string       `json:"description"`
	Workflow    WorkflowMode `json:"workflow"`
	Roles       []Role       `json:"roles"`
}

// Run defines Court runtime data.
type Run struct {
	ID                  string              `json:"id"`
	CourtID             string              `json:"court_id,omitempty"`
	Task                string              `json:"task"`
	Preset              string              `json:"preset"`
	Workflow            WorkflowMode        `json:"workflow"`
	DelegationScope     DelegationScope     `json:"delegation_scope"`
	Backend             string              `json:"backend"`
	Model               string              `json:"model,omitempty"`
	ModelOptions        RuntimeModelOptions `json:"model_options,omitempty"`
	DefaultProvider     string              `json:"default_provider,omitempty"`
	DefaultModel        string              `json:"default_model,omitempty"`
	DefaultModelOptions RuntimeModelOptions `json:"default_model_options,omitempty"`
	Workspace           string              `json:"workspace"`
	Status              RunStatus           `json:"status"`
	Phase               Phase               `json:"phase"`
	Verdict             string              `json:"verdict,omitempty"`
	CreatedAt           time.Time           `json:"created_at"`
	UpdatedAt           time.Time           `json:"updated_at"`
	CompletedAt         time.Time           `json:"completed_at,omitempty"`
}

// Worker defines Court runtime data.
type Worker struct {
	ID                       string              `json:"id"`
	RunID                    string              `json:"run_id"`
	LaunchID                 string              `json:"launch_id"`
	Attempt                  int                 `json:"attempt"`
	RoleID                   string              `json:"role_id"`
	RoleKind                 RoleKind            `json:"role_kind"`
	RoleTitle                string              `json:"role_title"`
	Backend                  string              `json:"backend"`
	Provider                 string              `json:"provider,omitempty"`
	Model                    string              `json:"model,omitempty"`
	ModelOptions             RuntimeModelOptions `json:"model_options,omitempty"`
	Agent                    string              `json:"agent,omitempty"`
	Status                   WorkerStatus        `json:"status"`
	RuntimeSessionID         string              `json:"runtime_session_id,omitempty"`
	RuntimeProviderSessionID string              `json:"runtime_provider_session_id,omitempty"`
	RuntimeTranscriptPath    string              `json:"runtime_transcript_path,omitempty"`
	RuntimePID               int                 `json:"runtime_pid,omitempty"`
	Result                   string              `json:"result,omitempty"`
	ResultJSON               string              `json:"result_json,omitempty"`
	Error                    string              `json:"error,omitempty"`
	CreatedAt                time.Time           `json:"created_at"`
	UpdatedAt                time.Time           `json:"updated_at"`
	CompletedAt              time.Time           `json:"completed_at,omitempty"`
}

type RuntimeIdentity = orchestration.RuntimeIdentity

type WorkerAttempt struct {
	ID                       int64               `json:"id"`
	WorkerID                 string              `json:"worker_id"`
	RunID                    string              `json:"run_id"`
	Attempt                  int                 `json:"attempt"`
	LaunchID                 string              `json:"launch_id"`
	RoleID                   string              `json:"role_id"`
	RoleKind                 RoleKind            `json:"role_kind"`
	RoleTitle                string              `json:"role_title"`
	Backend                  string              `json:"backend"`
	Provider                 string              `json:"provider,omitempty"`
	Model                    string              `json:"model,omitempty"`
	ModelOptions             RuntimeModelOptions `json:"model_options,omitempty"`
	Agent                    string              `json:"agent,omitempty"`
	Status                   WorkerStatus        `json:"status"`
	RuntimeSessionID         string              `json:"runtime_session_id,omitempty"`
	RuntimeProviderSessionID string              `json:"runtime_provider_session_id,omitempty"`
	RuntimeTranscriptPath    string              `json:"runtime_transcript_path,omitempty"`
	RuntimePID               int                 `json:"runtime_pid,omitempty"`
	Result                   string              `json:"result,omitempty"`
	ResultJSON               string              `json:"result_json,omitempty"`
	Error                    string              `json:"error,omitempty"`
	CreatedAt                time.Time           `json:"created_at"`
	UpdatedAt                time.Time           `json:"updated_at"`
	CompletedAt              time.Time           `json:"completed_at,omitempty"`
	ArchivedAt               time.Time           `json:"archived_at"`
}

// WorkerControlRequest defines Court runtime data.
type WorkerControlRequest = orchestration.WorkerControlRequest

// WorkerControlResult defines Court runtime data.
type WorkerControlResult struct {
	Worker                   Worker              `json:"worker"`
	Action                   WorkerControlAction `json:"action"`
	Status                   WorkerControlStatus `json:"status"`
	RuntimeSessionID         string              `json:"runtime_session_id,omitempty"`
	RuntimeProviderSessionID string              `json:"runtime_provider_session_id,omitempty"`
	Message                  string              `json:"message,omitempty"`
	Error                    string              `json:"error,omitempty"`
}

type RuntimeRequestStatus = orchestration.RuntimeRequestStatus

const (
	RuntimeRequestOpen      = orchestration.RuntimeRequestOpen
	RuntimeRequestResponded = orchestration.RuntimeRequestResponded
	RuntimeRequestResolved  = orchestration.RuntimeRequestResolved
	RuntimeRequestClosed    = orchestration.RuntimeRequestClosed
)

type RuntimeResponseStatus = orchestration.RuntimeResponseStatus

const (
	RuntimeResponseNone      = orchestration.RuntimeResponseNone
	RuntimeResponseQueued    = orchestration.RuntimeResponseQueued
	RuntimeResponseCompleted = orchestration.RuntimeResponseCompleted
	RuntimeResponseFailed    = orchestration.RuntimeResponseFailed
)

type RuntimeRequest = orchestration.RuntimeRequestRecord

type RuntimeRequestAnswer = orchestration.RuntimeRequestAnswer

type RuntimeRequestResponse = orchestration.RuntimeRequestResponse

// RuntimeRequestResponseResult defines Court runtime data.
type RuntimeRequestResponseResult struct {
	Request RuntimeRequest `json:"request"`
	Message string         `json:"message,omitempty"`
}

// WorkerResult defines Court runtime data.
type WorkerResult struct {
	SchemaVersion int      `json:"schema_version"`
	Summary       string   `json:"summary"`
	Findings      []string `json:"findings"`
	Risks         []string `json:"risks"`
	NextActions   []string `json:"next_actions"`
	Verdict       string   `json:"verdict"`
	Confidence    string   `json:"confidence"`
}

// WorkerResultRequiredFields provides Court runtime functionality.
func WorkerResultRequiredFields() []string {
	return []string{"schema_version", "summary", "findings", "risks", "next_actions", "verdict", "confidence"}
}

// WorkerResultConfidenceValues provides Court runtime functionality.
func WorkerResultConfidenceValues() []string {
	return []string{"low", "medium", "high"}
}

// ValidWorkerResultConfidence provides Court runtime functionality.
func ValidWorkerResultConfidence(value string) bool {
	for _, allowed := range WorkerResultConfidenceValues() {
		if value == allowed {
			return true
		}
	}
	return false
}

// WorkerResultSchemaExample provides Court runtime functionality.
func WorkerResultSchemaExample() string {
	data, _ := json.Marshal(WorkerResult{
		SchemaVersion: 1,
		Summary:       "<summary>",
		Findings:      []string{"<finding>"},
		Risks:         []string{"<risk>"},
		NextActions:   []string{"<next_action>"},
		Verdict:       "<verdict>",
		Confidence:    "medium",
	})
	return string(data)
}

type Event = orchestration.EventRecord

type Artifact = orchestration.ArtifactRecord

type WorkerTrace = orchestration.WorkerTrace[Worker, WorkerAttempt, WorkerResult, WorkerControlRequest, RuntimeRequest, Event, Artifact, contract.TrackedSession]

type RunTrace = orchestration.RunTrace[Run, WorkerTrace, Event, Artifact]

// StartRunRequest defines Court runtime data.
type StartRunRequest struct {
	Task            string
	Preset          string
	CourtID         string
	Workflow        WorkflowMode
	DelegationScope DelegationScope
	Backend         string
	Workspace       string
	Model           string
	ModelOptions    RuntimeModelOptions
	Selection       *contract.ModelSelection
}

// EngineOptions defines Court runtime data.
type EngineOptions struct {
	ConfigDir     string
	DataDir       string
	RootDir       string
	DBPath        string
	WorkerCommand string
	ControlPlane  RuntimeControlPlane
}
