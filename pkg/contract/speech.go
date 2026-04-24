package contract

const (
	EventSpeechTTSRequested = "speech.tts.requested"
	EventSpeechTTSStarted   = "speech.tts.started"
	EventSpeechTTSCompleted = "speech.tts.completed"
	EventSpeechTTSCancelled = "speech.tts.cancelled"
	EventSpeechTTSFailed    = "speech.tts.failed"

	EventSpeechSTTStarted   = "speech.stt.started"
	EventSpeechSTTPartial   = "speech.stt.partial"
	EventSpeechSTTFinal     = "speech.stt.final"
	EventSpeechSTTCancelled = "speech.stt.cancelled"
	EventSpeechSTTFailed    = "speech.stt.failed"

	EventAppOpenRequested = "app.open.requested"
	EventAppOpenCompleted = "app.open.completed"
	EventAppOpenFailed    = "app.open.failed"

	EventScreenObserveRequested = "screen.observe.requested"
	EventScreenObserveCompleted = "screen.observe.completed"
	EventScreenObserveFailed    = "screen.observe.failed"

	EventScreenClickRequested = "screen.click.requested"
	EventScreenClickCompleted = "screen.click.completed"
	EventScreenClickFailed    = "screen.click.failed"

	EventInsertTargetsRequested = "insert.targets.requested"
	EventInsertTargetsCompleted = "insert.targets.completed"
	EventInsertTargetsFailed    = "insert.targets.failed"

	EventSpeechInsertRequested = "speech.insert.requested"
	EventSpeechInsertStarted   = "speech.insert.started"
	EventSpeechInsertCompleted = "speech.insert.completed"
	EventSpeechInsertFailed    = "speech.insert.failed"

	EventAttentionItemCreated   = "attention.item.created"
	EventAttentionItemUpdated   = "attention.item.updated"
	EventAttentionItemCompleted = "attention.item.completed"
	EventAttentionItemFailed    = "attention.item.failed"
)

type AttentionAction string

const (
	AttentionActionSpeak     AttentionAction = "speak"
	AttentionActionAsk       AttentionAction = "ask"
	AttentionActionInsert    AttentionAction = "insert"
	AttentionActionSummarise AttentionAction = "summarise"
	AttentionActionIgnore    AttentionAction = "ignore"
	AttentionActionDefer     AttentionAction = "defer"
)

type AttentionStatus string

const (
	AttentionStatusQueued    AttentionStatus = "queued"
	AttentionStatusReady     AttentionStatus = "ready"
	AttentionStatusActive    AttentionStatus = "active"
	AttentionStatusCompleted AttentionStatus = "completed"
	AttentionStatusSpoken    AttentionStatus = "spoken"
	AttentionStatusInserted  AttentionStatus = "inserted"
	AttentionStatusMuted     AttentionStatus = "muted"
	AttentionStatusDeferred  AttentionStatus = "deferred"
	AttentionStatusCancelled AttentionStatus = "cancelled"
	AttentionStatusFailed    AttentionStatus = "failed"
)

type AttentionItem struct {
	SchemaVersion   string          `json:"schema_version"`
	ID              string          `json:"id"`
	Status          AttentionStatus `json:"status"`
	Action          AttentionAction `json:"action"`
	Source          string          `json:"source,omitempty"`
	Runtime         string          `json:"runtime,omitempty"`
	SessionID       string          `json:"session_id,omitempty"`
	TurnID          string          `json:"turn_id,omitempty"`
	Priority        int             `json:"priority,omitempty"`
	Text            string          `json:"text,omitempty"`
	SpeakableText   string          `json:"speakable_text,omitempty"`
	Target          map[string]any  `json:"target,omitempty"`
	Voice           string          `json:"voice,omitempty"`
	InterruptPolicy string          `json:"interrupt_policy,omitempty"`
	Metadata        map[string]any  `json:"metadata,omitempty"`
	Result          map[string]any  `json:"result,omitempty"`
	Error           string          `json:"error,omitempty"`
	CreatedAtMS     int64           `json:"created_at_ms"`
	UpdatedAtMS     int64           `json:"updated_at_ms"`
}
