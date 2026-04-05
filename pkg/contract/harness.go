package contract

type HarnessEvent struct {
	SchemaVersion   string            `json:"schema_version"`
	RecordedAtMS    int64             `json:"recorded_at_ms"`
	Runtime         string            `json:"runtime"`
	Provenance      string            `json:"provenance"`
	NativeEventName string            `json:"native_event_name"`
	EventType       string            `json:"event_type"`
	Summary         string            `json:"summary"`
	SessionID       *string           `json:"session_id,omitempty"`
	TurnID          *string           `json:"turn_id,omitempty"`
	ToolCallID      *string           `json:"tool_call_id,omitempty"`
	ToolName        *string           `json:"tool_name,omitempty"`
	Command         *string           `json:"command,omitempty"`
	PromptText      *string           `json:"prompt_text,omitempty"`
	CWD             *string           `json:"cwd,omitempty"`
	Model           *string           `json:"model,omitempty"`
	TranscriptPath  *string           `json:"transcript_path,omitempty"`
	SessionSource   *string           `json:"session_source,omitempty"`
	PermissionMode  *string           `json:"permission_mode,omitempty"`
	Reason          *string           `json:"reason,omitempty"`
	ExitCode        *int              `json:"exit_code,omitempty"`
	StopHookActive  *bool             `json:"stop_hook_active,omitempty"`
	RuntimePID      *int              `json:"runtime_pid,omitempty"`
	Bindings        map[string]string `json:"bindings,omitempty"`
}
