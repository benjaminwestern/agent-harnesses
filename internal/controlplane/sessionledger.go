package controlplane

import (
	"slices"
	"sync"

	"github.com/benjaminwestern/agentic-control/pkg/contract"
)

type trackedSessionState struct {
	record            contract.TrackedSession
	lastObservedUsage contract.TokenUsage
	lastObservedCost  float64
	usageObserved     bool
	costObserved      bool
	usageByModel      map[string]contract.TokenUsage
	usageByMode       map[string]contract.TokenUsage
	costByModel       map[string]float64
	costByMode        map[string]float64
}

type SessionLedger struct {
	mu           sync.RWMutex
	records      map[string]*trackedSessionState
	providerToID map[string]string
}

func NewSessionLedger() *SessionLedger {
	return &SessionLedger{
		records:      make(map[string]*trackedSessionState),
		providerToID: make(map[string]string),
	}
}

func (l *SessionLedger) Upsert(session contract.RuntimeSession) {
	if session.SessionID == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	state := l.getOrCreateLocked(session.SessionID)
	state.record.Session = mergeRuntimeSession(state.record.Session, session)
	if state.record.StartedAtMS == 0 {
		state.record.StartedAtMS = session.CreatedAtMS
	}
	if session.ProviderSessionID != "" {
		l.providerToID[session.ProviderSessionID] = session.SessionID
	}
	state.record.UsageByModel = tokenBreakdowns(state.usageByModel)
	state.record.UsageByMode = tokenBreakdowns(state.usageByMode)
	state.record.CostByModel = costBreakdowns(state.costByModel)
	state.record.CostByMode = costBreakdowns(state.costByMode)
	state.record.Session.SchemaVersion = contract.ControlPlaneSchemaVersion
	state.record.Session.Runtime = session.Runtime
	state.record.Session.Transport = session.Transport
	state.record.Session.Ownership = session.Ownership
	state.record.Session.SessionID = session.SessionID
	state.record.Session.ProviderSessionID = session.ProviderSessionID
}

func (l *SessionLedger) Get(sessionID string, providerSessionID string) (contract.TrackedSession, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	state, ok := l.lookupLocked(sessionID, providerSessionID)
	if !ok {
		return contract.TrackedSession{}, false
	}
	return cloneTrackedSession(state), true
}

func (l *SessionLedger) List() []contract.TrackedSession {
	l.mu.RLock()
	defer l.mu.RUnlock()
	records := make([]contract.TrackedSession, 0, len(l.records))
	for _, state := range l.records {
		records = append(records, cloneTrackedSession(state))
	}
	slices.SortFunc(records, func(left, right contract.TrackedSession) int {
		switch {
		case left.Session.UpdatedAtMS < right.Session.UpdatedAtMS:
			return 1
		case left.Session.UpdatedAtMS > right.Session.UpdatedAtMS:
			return -1
		case left.Session.SessionID < right.Session.SessionID:
			return -1
		case left.Session.SessionID > right.Session.SessionID:
			return 1
		default:
			return 0
		}
	})
	return records
}

func (l *SessionLedger) UpdateFromEvent(event contract.RuntimeEvent) {
	if event.SessionID == "" {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	state := l.getOrCreateLocked(event.SessionID)
	session := state.record.Session
	session.SchemaVersion = contract.ControlPlaneSchemaVersion
	if session.SessionID == "" {
		session.SessionID = event.SessionID
		session.Runtime = event.Runtime
		session.Transport = event.Transport
		session.Ownership = event.Ownership
		session.CreatedAtMS = event.RecordedAtMS
		state.record.StartedAtMS = event.RecordedAtMS
	}
	session.UpdatedAtMS = event.RecordedAtMS
	session.LastActivityAtMS = event.RecordedAtMS
	state.record.LastEventType = event.EventType
	if event.ProviderSessionID != "" {
		session.ProviderSessionID = event.ProviderSessionID
		l.providerToID[event.ProviderSessionID] = event.SessionID
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
		if event.SessionState.Mode != "" {
			session.Mode = event.SessionState.Mode
		}
		if usage, ok := contract.EventTokenUsage(event); ok {
			session.Usage = usage
		}
		if cost, ok := contract.EventCostUSD(event); ok {
			session.CostUSD = cost
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
	if mode := contract.EventMode(event); mode != "" {
		session.Mode = mode
	}
	if cost, ok := contract.EventCostUSD(event); ok {
		delta := costDelta(cost, state.lastObservedCost, state.costObserved)
		state.lastObservedCost = cost
		state.costObserved = true
		session.CostUSD = cost
		if session.Model != "" && delta > 0 {
			state.costByModel[session.Model] += delta
		}
		if session.Mode != "" && delta > 0 {
			state.costByMode[session.Mode] += delta
		}
	}
	if usage, ok := contract.EventTokenUsage(event); ok {
		delta := usageDelta(usage, state.lastObservedUsage, state.usageObserved)
		state.lastObservedUsage = usage
		state.usageObserved = true
		session.Usage = usage
		if session.Model != "" && delta != (contract.TokenUsage{}) {
			state.usageByModel[session.Model] = addTokenUsage(state.usageByModel[session.Model], delta)
		}
		if session.Mode != "" && delta != (contract.TokenUsage{}) {
			state.usageByMode[session.Mode] = addTokenUsage(state.usageByMode[session.Mode], delta)
		}
	}

	switch event.EventType {
	case contract.EventSessionStopped:
		session.Status = contract.SessionStopped
		state.record.EndedAtMS = event.RecordedAtMS
	case contract.EventSessionErrored:
		session.Status = contract.SessionErrored
		state.record.EndedAtMS = event.RecordedAtMS
	case contract.EventTurnInterrupted:
		session.Status = contract.SessionInterrupted
	}

	state.record.Session = session
	state.record.UsageByModel = tokenBreakdowns(state.usageByModel)
	state.record.UsageByMode = tokenBreakdowns(state.usageByMode)
	state.record.CostByModel = costBreakdowns(state.costByModel)
	state.record.CostByMode = costBreakdowns(state.costByMode)
}

func (l *SessionLedger) getOrCreateLocked(sessionID string) *trackedSessionState {
	state, ok := l.records[sessionID]
	if ok {
		return state
	}
	state = &trackedSessionState{
		usageByModel: make(map[string]contract.TokenUsage),
		usageByMode:  make(map[string]contract.TokenUsage),
		costByModel:  make(map[string]float64),
		costByMode:   make(map[string]float64),
	}
	l.records[sessionID] = state
	return state
}

func (l *SessionLedger) lookupLocked(sessionID string, providerSessionID string) (*trackedSessionState, bool) {
	if sessionID != "" {
		state, ok := l.records[sessionID]
		return state, ok
	}
	if providerSessionID == "" {
		return nil, false
	}
	sessionID, ok := l.providerToID[providerSessionID]
	if !ok {
		return nil, false
	}
	state, ok := l.records[sessionID]
	return state, ok
}

func cloneTrackedSession(state *trackedSessionState) contract.TrackedSession {
	record := state.record
	record.UsageByModel = tokenBreakdowns(state.usageByModel)
	record.UsageByMode = tokenBreakdowns(state.usageByMode)
	record.CostByModel = costBreakdowns(state.costByModel)
	record.CostByMode = costBreakdowns(state.costByMode)
	return record
}

func mergeRuntimeSession(current contract.RuntimeSession, next contract.RuntimeSession) contract.RuntimeSession {
	if next.SchemaVersion != "" {
		current.SchemaVersion = next.SchemaVersion
	}
	if next.SessionID != "" {
		current.SessionID = next.SessionID
	}
	if next.Runtime != "" {
		current.Runtime = next.Runtime
	}
	if next.Ownership != "" {
		current.Ownership = next.Ownership
	}
	if next.Transport != "" {
		current.Transport = next.Transport
	}
	if next.Status != "" {
		current.Status = next.Status
	}
	if next.ProviderSessionID != "" {
		current.ProviderSessionID = next.ProviderSessionID
	}
	if next.ActiveTurnID != "" {
		current.ActiveTurnID = next.ActiveTurnID
	}
	if next.CWD != "" {
		current.CWD = next.CWD
	}
	if next.Model != "" {
		current.Model = next.Model
	}
	if next.Mode != "" {
		current.Mode = next.Mode
	}
	if next.Usage != (contract.TokenUsage{}) {
		current.Usage = next.Usage
	}
	if next.CostUSD > 0 {
		current.CostUSD = next.CostUSD
	}
	if next.Title != "" {
		current.Title = next.Title
	}
	if next.CreatedAtMS != 0 {
		current.CreatedAtMS = next.CreatedAtMS
	}
	if next.UpdatedAtMS != 0 {
		current.UpdatedAtMS = next.UpdatedAtMS
	}
	if next.LastActivityAtMS != 0 {
		current.LastActivityAtMS = next.LastActivityAtMS
	}
	if next.LastError != "" {
		current.LastError = next.LastError
	}
	if len(next.Metadata) > 0 {
		current.Metadata = next.Metadata
	}
	return current
}

func costDelta(current float64, previous float64, previousSeen bool) float64 {
	if !previousSeen {
		return current
	}
	if current >= previous {
		return current - previous
	}
	return current
}

func usageDelta(current contract.TokenUsage, previous contract.TokenUsage, previousSeen bool) contract.TokenUsage {
	if !previousSeen {
		return current
	}
	return contract.TokenUsage{
		InputTokens:     deltaInt64(current.InputTokens, previous.InputTokens),
		OutputTokens:    deltaInt64(current.OutputTokens, previous.OutputTokens),
		ReasoningTokens: deltaInt64(current.ReasoningTokens, previous.ReasoningTokens),
		CachedTokens:    deltaInt64(current.CachedTokens, previous.CachedTokens),
		TotalTokens:     deltaInt64(current.TotalTokens, previous.TotalTokens),
	}
}

func deltaInt64(current int64, previous int64) int64 {
	if current >= previous {
		return current - previous
	}
	return current
}

func addTokenUsage(left contract.TokenUsage, right contract.TokenUsage) contract.TokenUsage {
	return contract.TokenUsage{
		InputTokens:     left.InputTokens + right.InputTokens,
		OutputTokens:    left.OutputTokens + right.OutputTokens,
		ReasoningTokens: left.ReasoningTokens + right.ReasoningTokens,
		CachedTokens:    left.CachedTokens + right.CachedTokens,
		TotalTokens:     left.TotalTokens + right.TotalTokens,
	}
}

func tokenBreakdowns(values map[string]contract.TokenUsage) []contract.TokenUsageBreakdown {
	breakdowns := make([]contract.TokenUsageBreakdown, 0, len(values))
	for key, usage := range values {
		if key == "" {
			continue
		}
		breakdowns = append(breakdowns, contract.TokenUsageBreakdown{Key: key, Usage: usage})
	}
	slices.SortFunc(breakdowns, func(left, right contract.TokenUsageBreakdown) int {
		switch {
		case left.Usage.TotalTokens > right.Usage.TotalTokens:
			return -1
		case left.Usage.TotalTokens < right.Usage.TotalTokens:
			return 1
		case left.Key < right.Key:
			return -1
		case left.Key > right.Key:
			return 1
		default:
			return 0
		}
	})
	return breakdowns
}

func costBreakdowns(values map[string]float64) []contract.CostBreakdown {
	breakdowns := make([]contract.CostBreakdown, 0, len(values))
	for key, cost := range values {
		if key == "" {
			continue
		}
		breakdowns = append(breakdowns, contract.CostBreakdown{Key: key, CostUSD: cost})
	}
	slices.SortFunc(breakdowns, func(left, right contract.CostBreakdown) int {
		switch {
		case left.CostUSD > right.CostUSD:
			return -1
		case left.CostUSD < right.CostUSD:
			return 1
		case left.Key < right.Key:
			return -1
		case left.Key > right.Key:
			return 1
		default:
			return 0
		}
	})
	return breakdowns
}
