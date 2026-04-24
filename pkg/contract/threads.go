package contract

type ThreadKind string

const (
	ThreadKindOrchestrationTarget  ThreadKind = "orchestration_target"
	ThreadKindOrchestrationReducer ThreadKind = "orchestration_reducer"
	ThreadKindCourtWorker          ThreadKind = "court_worker"
)

type ThreadMetadata struct {
	Kind                          ThreadKind `json:"kind,omitempty"`
	Workflow                      string     `json:"workflow,omitempty"`
	WorkflowMode                  string     `json:"workflow_mode,omitempty"`
	Task                          string     `json:"task,omitempty"`
	TargetLabel                   string     `json:"target_label,omitempty"`
	ReductionMode                 string     `json:"reduction_mode,omitempty"`
	PromotedFromRunID             string     `json:"promoted_from_run_id,omitempty"`
	PromotedFromWorkerID          string     `json:"promoted_from_worker_id,omitempty"`
	PromotedFromRoleID            string     `json:"promoted_from_role_id,omitempty"`
	PromotedAsBase                bool       `json:"promoted_as_base,omitempty"`
	CourtRunID                    string     `json:"court_run_id,omitempty"`
	CourtWorkerID                 string     `json:"court_worker_id,omitempty"`
	CourtRoleID                   string     `json:"court_role_id,omitempty"`
	CourtRoleKind                 string     `json:"court_role_kind,omitempty"`
	CourtAgent                    string     `json:"court_agent,omitempty"`
	CourtBackend                  string     `json:"court_backend,omitempty"`
	ForkMode                      string     `json:"fork_mode,omitempty"`
	ForkedFromThreadID            string     `json:"forked_from_thread_id,omitempty"`
	ForkedFromProviderSessionID   string     `json:"forked_from_provider_session_id,omitempty"`
	RollbackMode                  string     `json:"rollback_mode,omitempty"`
	RollbackTurns                 int        `json:"rollback_turns,omitempty"`
	RollbackFromThreadID          string     `json:"rollback_from_thread_id,omitempty"`
	RollbackFromProviderSessionID string     `json:"rollback_from_provider_session_id,omitempty"`
}

type TrackedThread struct {
	ThreadID       string         `json:"thread_id"`
	ParentThreadID string         `json:"parent_thread_id,omitempty"`
	Name           string         `json:"name,omitempty"`
	Archived       bool           `json:"archived"`
	Metadata       ThreadMetadata `json:"metadata,omitempty"`
	TrackedSession TrackedSession `json:"tracked_session"`
}

type ThreadEvent struct {
	ID           int64        `json:"id"`
	ThreadID     string       `json:"thread_id"`
	RecordedAtMS int64        `json:"recorded_at_ms,omitempty"`
	EventType    string       `json:"event_type,omitempty"`
	Summary      string       `json:"summary,omitempty"`
	TurnID       string       `json:"turn_id,omitempty"`
	RequestID    string       `json:"request_id,omitempty"`
	Event        RuntimeEvent `json:"event"`
}

type ThreadTurnStatus string

const (
	ThreadTurnRunning     ThreadTurnStatus = "running"
	ThreadTurnCompleted   ThreadTurnStatus = "completed"
	ThreadTurnErrored     ThreadTurnStatus = "errored"
	ThreadTurnInterrupted ThreadTurnStatus = "interrupted"
)

type ThreadTurn struct {
	TurnID        string           `json:"turn_id"`
	Status        ThreadTurnStatus `json:"status"`
	Summary       string           `json:"summary,omitempty"`
	AssistantText string           `json:"assistant_text,omitempty"`
	StartedAtMS   int64            `json:"started_at_ms,omitempty"`
	CompletedAtMS int64            `json:"completed_at_ms,omitempty"`
	EventCount    int              `json:"event_count,omitempty"`
	RequestIDs    []string         `json:"request_ids,omitempty"`
}

type ThreadRead struct {
	Thread     TrackedThread `json:"thread"`
	Turns      []ThreadTurn  `json:"turns,omitempty"`
	EventCount int           `json:"event_count,omitempty"`
}

type PromotedThread struct {
	RunID    string        `json:"run_id"`
	WorkerID string        `json:"worker_id"`
	Thread   TrackedThread `json:"thread"`
}
