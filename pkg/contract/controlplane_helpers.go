package contract

import (
	"fmt"
	"time"
)

func NewRuntimeDescriptor(
	runtime string,
	ownership Ownership,
	transport Transport,
	capabilities RuntimeCapabilities,
) RuntimeDescriptor {
	return RuntimeDescriptor{
		SchemaVersion: ControlPlaneSchemaVersion,
		Runtime:       runtime,
		Ownership:     ownership,
		Transport:     transport,
		Capabilities:  capabilities,
	}
}

func NewRuntimeEvent(
	session RuntimeSession,
	eventType string,
	nativeEventName string,
	turnID string,
	summary string,
	payload map[string]any,
) RuntimeEvent {
	return RuntimeEvent{
		SchemaVersion:     ControlPlaneSchemaVersion,
		EventID:           fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		RecordedAtMS:      time.Now().UnixMilli(),
		Runtime:           session.Runtime,
		SessionID:         session.SessionID,
		ProviderSessionID: session.ProviderSessionID,
		Transport:         session.Transport,
		Ownership:         session.Ownership,
		EventType:         eventType,
		NativeEventName:   nativeEventName,
		Summary:           summary,
		TurnID:            turnID,
		SessionState:      session.State(),
		Payload:           payload,
	}
}

func (session RuntimeSession) State() *SessionState {
	state := &SessionState{
		Status:       session.Status,
		ActiveTurnID: session.ActiveTurnID,
		LastError:    session.LastError,
		CWD:          session.CWD,
		Model:        session.Model,
		Title:        session.Title,
	}
	if *state == (SessionState{}) {
		return nil
	}
	return state
}
