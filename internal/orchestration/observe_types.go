package orchestration

import "time"

type WorkerTrace[WorkerT any, AttemptT any, ResultT any, ControlT any, RequestT any, EventT any, ArtifactT any, SessionT any] struct {
	Worker           WorkerT     `json:"worker"`
	RuntimeSession   *SessionT   `json:"runtime_session,omitempty"`
	Attempts         []AttemptT  `json:"attempts,omitempty"`
	StructuredResult *ResultT    `json:"structured_result,omitempty"`
	Events           []EventT    `json:"events"`
	Artifacts        []ArtifactT `json:"artifacts"`
	Controls         []ControlT  `json:"controls,omitempty"`
	Requests         []RequestT  `json:"requests,omitempty"`
}

type RunTrace[RunT any, WorkerTraceT any, EventT any, ArtifactT any] struct {
	Run       RunT           `json:"run"`
	Workers   []WorkerTraceT `json:"workers"`
	Events    []EventT       `json:"events"`
	Artifacts []ArtifactT    `json:"artifacts"`
}

type RunStatusView[RunT any, WorkerT any] struct {
	Run     RunT      `json:"run"`
	Workers []WorkerT `json:"workers"`
}

type MonitorSnapshot[RunT any, WorkerT any, RequestT any, EventT any] struct {
	Run          RunT       `json:"run"`
	Workers      []WorkerT  `json:"workers"`
	OpenRequests []RequestT `json:"open_requests"`
	RecentEvents []EventT   `json:"recent_events"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type WatchUpdate[RunT any, EventT any] struct {
	Event       *EventT
	TerminalRun *RunT
}

type WatchOptions[RunT any, EventT any] struct {
	StopOnTerminal bool
	PollInterval   time.Duration
	OnUpdate       func(WatchUpdate[RunT, EventT]) error
}
