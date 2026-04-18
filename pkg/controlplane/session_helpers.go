package controlplane

import (
	"context"
	"fmt"
	"strings"

	"github.com/benjaminwestern/agentic-control/pkg/contract"
)

type SessionController interface {
	ListSessions(context.Context, string) ([]contract.RuntimeSession, error)
	ResumeSession(context.Context, string, ResumeSessionRequest) (*contract.RuntimeSession, error)
}

func FindSessionByIDOrProviderID(
	sessions []contract.RuntimeSession,
	sessionID string,
	providerSessionID string,
) (*contract.RuntimeSession, bool) {
	sessionID = strings.TrimSpace(sessionID)
	providerSessionID = strings.TrimSpace(providerSessionID)
	for i := range sessions {
		session := sessions[i]
		if sessionID != "" && session.SessionID == sessionID {
			return &session, true
		}
		if providerSessionID != "" && session.ProviderSessionID == providerSessionID {
			return &session, true
		}
	}
	return nil, false
}

func AdoptOrResumeSession(
	ctx context.Context,
	controller SessionController,
	runtime string,
	request ResumeSessionRequest,
) (*contract.RuntimeSession, error) {
	sessions, err := controller.ListSessions(ctx, runtime)
	if err == nil {
		if session, ok := FindSessionByIDOrProviderID(sessions, request.SessionID, request.ProviderSessionID); ok {
			return session, nil
		}
	}
	if strings.TrimSpace(request.ProviderSessionID) == "" {
		return nil, fmt.Errorf("provider_session_id is required to resume a session not present in the control plane")
	}
	return controller.ResumeSession(ctx, runtime, request)
}
