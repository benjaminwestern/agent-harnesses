package contract

const (
	ControlPlaneSchemaVersion = "agentic-control.control-plane.v1"
	HarnessSchemaVersion      = "agentic-control.harness.v1"
	WireProtocolVersion       = "agentic-control.rpc.v1"
)

type Ownership string

const (
	OwnershipControlled Ownership = "controlled"
	OwnershipObserved   Ownership = "observed"
)

type Transport string

const (
	TransportNativeHook Transport = "native_hook"
	TransportPlugin     Transport = "native_plugin"
	TransportAppServer  Transport = "app_server"
	TransportAgentSDK   Transport = "agent_sdk"
	TransportACP        Transport = "acp"
	TransportHTTPServer Transport = "http_server"
	TransportRPC        Transport = "rpc"
)

type SessionStatus string

const (
	SessionStarting         SessionStatus = "starting"
	SessionIdle             SessionStatus = "idle"
	SessionRunning          SessionStatus = "running"
	SessionWaitingApproval  SessionStatus = "waiting_approval"
	SessionWaitingUserInput SessionStatus = "waiting_user_input"
	SessionInterrupted      SessionStatus = "interrupted"
	SessionStopped          SessionStatus = "stopped"
	SessionErrored          SessionStatus = "errored"
)

type RequestKind string

const (
	RequestApprovalTool        RequestKind = "approval.tool"
	RequestApprovalCommand     RequestKind = "approval.command"
	RequestApprovalFileChange  RequestKind = "approval.file_change"
	RequestApprovalPermissions RequestKind = "approval.permissions"
	RequestUserInputTool       RequestKind = "user_input.tool"
	RequestUserInputMCP        RequestKind = "user_input.mcp"
	RequestGeneric             RequestKind = "request"
)

type RequestStatus string

const (
	RequestStatusOpen      RequestStatus = "open"
	RequestStatusResponded RequestStatus = "responded"
	RequestStatusResolved  RequestStatus = "resolved"
	RequestStatusClosed    RequestStatus = "closed"
)

type RespondAction string

const (
	RespondActionAllow  RespondAction = "allow"
	RespondActionDeny   RespondAction = "deny"
	RespondActionSubmit RespondAction = "submit"
	RespondActionCancel RespondAction = "cancel"
	RespondActionChoose RespondAction = "choose"
)

type RequestOption struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Kind        string `json:"kind,omitempty"`
	Description string `json:"description,omitempty"`
	IsDefault   bool   `json:"is_default,omitempty"`
}

type RequestQuestion struct {
	ID          string          `json:"id"`
	Prompt      string          `json:"prompt"`
	Description string          `json:"description,omitempty"`
	Required    bool            `json:"required,omitempty"`
	Options     []RequestOption `json:"options,omitempty"`
}

type RequestToolContext struct {
	Name        string `json:"name,omitempty"`
	Title       string `json:"title,omitempty"`
	Kind        string `json:"kind,omitempty"`
	Command     string `json:"command,omitempty"`
	Description string `json:"description,omitempty"`
}

type RequestAnswer struct {
	QuestionID string `json:"question_id,omitempty"`
	OptionID   string `json:"option_id,omitempty"`
	Text       string `json:"text,omitempty"`
}

type RuntimeCapabilities struct {
	StartSession             bool `json:"start_session"`
	ResumeSession            bool `json:"resume_session"`
	SendInput                bool `json:"send_input"`
	Interrupt                bool `json:"interrupt"`
	Respond                  bool `json:"respond"`
	StopSession              bool `json:"stop_session"`
	ListSessions             bool `json:"list_sessions"`
	StreamEvents             bool `json:"stream_events"`
	ApprovalRequests         bool `json:"approval_requests"`
	UserInputRequests        bool `json:"user_input_requests"`
	ImmediateProviderSession bool `json:"immediate_provider_session"`
	ResumeByProviderID       bool `json:"resume_by_provider_id"`
	AdoptExternalSessions    bool `json:"adopt_external_sessions"`
}

type RuntimeDescriptor struct {
	SchemaVersion string              `json:"schema_version"`
	Runtime       string              `json:"runtime"`
	Ownership     Ownership           `json:"ownership"`
	Transport     Transport           `json:"transport"`
	Capabilities  RuntimeCapabilities `json:"capabilities"`
}

type SystemDescriptor struct {
	SchemaVersion       string              `json:"schema_version"`
	WireProtocolVersion string              `json:"wire_protocol_version"`
	Methods             []string            `json:"methods"`
	Runtimes            []RuntimeDescriptor `json:"runtimes"`
}

type SessionState struct {
	Status       SessionStatus `json:"status,omitempty"`
	ActiveTurnID string        `json:"active_turn_id,omitempty"`
	LastError    string        `json:"last_error,omitempty"`
	CWD          string        `json:"cwd,omitempty"`
	Model        string        `json:"model,omitempty"`
	Title        string        `json:"title,omitempty"`
}

type RuntimeSession struct {
	SchemaVersion     string         `json:"schema_version"`
	SessionID         string         `json:"session_id"`
	Runtime           string         `json:"runtime"`
	Ownership         Ownership      `json:"ownership"`
	Transport         Transport      `json:"transport"`
	Status            SessionStatus  `json:"status"`
	ProviderSessionID string         `json:"provider_session_id,omitempty"`
	ActiveTurnID      string         `json:"active_turn_id,omitempty"`
	CWD               string         `json:"cwd,omitempty"`
	Model             string         `json:"model,omitempty"`
	Title             string         `json:"title,omitempty"`
	CreatedAtMS       int64          `json:"created_at_ms"`
	UpdatedAtMS       int64          `json:"updated_at_ms"`
	LastActivityAtMS  int64          `json:"last_activity_at_ms,omitempty"`
	LastError         string         `json:"last_error,omitempty"`
	Metadata          map[string]any `json:"metadata,omitempty"`
}

type PendingRequest struct {
	SchemaVersion string              `json:"schema_version"`
	RequestID     string              `json:"request_id"`
	SessionID     string              `json:"session_id"`
	Runtime       string              `json:"runtime"`
	Kind          RequestKind         `json:"kind"`
	NativeMethod  string              `json:"native_method"`
	Status        RequestStatus       `json:"status"`
	Summary       string              `json:"summary,omitempty"`
	TurnID        string              `json:"turn_id,omitempty"`
	CreatedAtMS   int64               `json:"created_at_ms"`
	Tool          *RequestToolContext `json:"tool,omitempty"`
	Options       []RequestOption     `json:"options,omitempty"`
	Questions     []RequestQuestion   `json:"questions,omitempty"`
	Extensions    map[string]any      `json:"extensions,omitempty"`
}

type RuntimeEvent struct {
	SchemaVersion     string          `json:"schema_version"`
	EventID           string          `json:"event_id"`
	RecordedAtMS      int64           `json:"recorded_at_ms"`
	Runtime           string          `json:"runtime"`
	SessionID         string          `json:"session_id"`
	ProviderSessionID string          `json:"provider_session_id,omitempty"`
	Transport         Transport       `json:"transport"`
	Ownership         Ownership       `json:"ownership"`
	EventType         string          `json:"event_type"`
	NativeEventName   string          `json:"native_event_name,omitempty"`
	Summary           string          `json:"summary"`
	TurnID            string          `json:"turn_id,omitempty"`
	RequestID         string          `json:"request_id,omitempty"`
	SessionState      *SessionState   `json:"session_state,omitempty"`
	Payload           map[string]any  `json:"payload,omitempty"`
	Request           *PendingRequest `json:"request,omitempty"`
}
