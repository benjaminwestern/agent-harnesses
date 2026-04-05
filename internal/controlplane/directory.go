package controlplane

import (
	"slices"
	"sync"

	"github.com/benjaminwestern/agentic-control/pkg/contract"
)

type SessionDirectory struct {
	mu       sync.RWMutex
	sessions map[string]contract.RuntimeSession
}

func NewSessionDirectory() *SessionDirectory {
	return &SessionDirectory{
		sessions: make(map[string]contract.RuntimeSession),
	}
}

func (d *SessionDirectory) Upsert(session contract.RuntimeSession) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.sessions[session.SessionID] = session
}

func (d *SessionDirectory) Delete(sessionID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.sessions, sessionID)
}

func (d *SessionDirectory) Get(sessionID string) (contract.RuntimeSession, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	session, ok := d.sessions[sessionID]
	return session, ok
}

func (d *SessionDirectory) List() []contract.RuntimeSession {
	d.mu.RLock()
	defer d.mu.RUnlock()

	sessions := make([]contract.RuntimeSession, 0, len(d.sessions))
	for _, session := range d.sessions {
		sessions = append(sessions, session)
	}
	slices.SortFunc(sessions, func(left, right contract.RuntimeSession) int {
		switch {
		case left.UpdatedAtMS < right.UpdatedAtMS:
			return 1
		case left.UpdatedAtMS > right.UpdatedAtMS:
			return -1
		default:
			if left.SessionID < right.SessionID {
				return -1
			}
			if left.SessionID > right.SessionID {
				return 1
			}
			return 0
		}
	})
	return sessions
}

func (d *SessionDirectory) UpdateFromEvent(event contract.RuntimeEvent) {
	if event.SessionID == "" {
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	session, ok := d.sessions[event.SessionID]
	if !ok {
		return
	}

	session.UpdatedAtMS = event.RecordedAtMS
	session.LastActivityAtMS = event.RecordedAtMS
	if event.ProviderSessionID != "" {
		session.ProviderSessionID = event.ProviderSessionID
	}
	if event.TurnID != "" {
		session.ActiveTurnID = event.TurnID
	}

	if event.SessionState != nil {
		if event.SessionState.Status != "" {
			session.Status = event.SessionState.Status
		}
		if event.SessionState.ActiveTurnID != "" {
			session.ActiveTurnID = event.SessionState.ActiveTurnID
		}
		if event.SessionState.LastError != "" {
			session.LastError = event.SessionState.LastError
		}
		if event.SessionState.CWD != "" {
			session.CWD = event.SessionState.CWD
		}
		if event.SessionState.Model != "" {
			session.Model = event.SessionState.Model
		}
		if event.SessionState.Title != "" {
			session.Title = event.SessionState.Title
		}
	}

	if event.Payload != nil {
		if status, ok := event.Payload["status"].(string); ok && status != "" {
			session.Status = contract.SessionStatus(status)
		}
		if lastError, ok := event.Payload["last_error"].(string); ok && lastError != "" {
			session.LastError = lastError
		}
	}

	d.sessions[event.SessionID] = session
}
