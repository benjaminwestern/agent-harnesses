package contract

type MemoryEntry struct {
	WorkspaceID string `json:"workspace_id"`
	Key         string `json:"key"`
	Value       string `json:"value"`
	CreatedAtMS int64  `json:"created_at_ms"`
	UpdatedAtMS int64  `json:"updated_at_ms"`
	ExpiresAtMS int64  `json:"expires_at_ms,omitempty"`
}

type Document struct {
	ID          string         `json:"id"`
	WorkspaceID string         `json:"workspace_id"`
	Name        string         `json:"name"`
	Content     string         `json:"content,omitempty"`
	Revision    int64          `json:"revision"`
	Tags        []string       `json:"tags,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	Archived    bool           `json:"archived"`
	CreatedAtMS int64          `json:"created_at_ms"`
	UpdatedAtMS int64          `json:"updated_at_ms"`
}

type TaskStatus string

const (
	TaskStatusOpen       TaskStatus = "open"
	TaskStatusInProgress TaskStatus = "in_progress"
	TaskStatusBlocked    TaskStatus = "blocked"
	TaskStatusCompleted  TaskStatus = "completed"
)

type Task struct {
	ID            string         `json:"id"`
	WorkspaceID   string         `json:"workspace_id"`
	Title         string         `json:"title"`
	Body          string         `json:"body"`
	Status        TaskStatus     `json:"status"`
	Tags          []string       `json:"tags,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
	LockedBy      string         `json:"locked_by,omitempty"`
	BlockerIDs    []string       `json:"blocker_ids,omitempty"`
	CreatedAtMS   int64          `json:"created_at_ms"`
	UpdatedAtMS   int64          `json:"updated_at_ms"`
	CompletedAtMS int64          `json:"completed_at_ms,omitempty"`
}

type TaskComment struct {
	ID          string `json:"id"`
	TaskID      string `json:"task_id"`
	Author      string `json:"author"`
	Body        string `json:"body"`
	CreatedAtMS int64  `json:"created_at_ms"`
}

type Wakeup struct {
	ID                string         `json:"id"`
	WorkspaceID       string         `json:"workspace_id"`
	OwnerID           string         `json:"owner_id"`
	Body              string         `json:"body"`
	Metadata          map[string]any `json:"metadata,omitempty"`
	CreatedAtMS       int64          `json:"created_at_ms"`
	DueAtMS           int64          `json:"due_at_ms"`
	FiredAtMS         int64          `json:"fired_at_ms,omitempty"`
	CancelledAtMS     int64          `json:"cancelled_at_ms,omitempty"`
	PausedAtMS        int64          `json:"paused_at_ms,omitempty"`
	PausedRemainingMS int64          `json:"paused_remaining_ms,omitempty"`
}

type Lease struct {
	WorkspaceID  string `json:"workspace_id"`
	LockKey      string `json:"lock_key"`
	OwnerID      string `json:"owner_id"`
	AcquiredAtMS int64  `json:"acquired_at_ms"`
	ExpiresAtMS  int64  `json:"expires_at_ms"`
}
