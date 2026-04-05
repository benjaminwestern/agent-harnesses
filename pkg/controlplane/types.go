package controlplane

import (
	"context"
	"strings"

	"github.com/benjaminwestern/agentic-control/pkg/contract"
)

type StartSessionRequest struct {
	SessionID string
	CWD       string
	Model     string
	Prompt    string
	Metadata  map[string]any
}

type ResumeSessionRequest struct {
	SessionID         string
	ProviderSessionID string
	CWD               string
	Model             string
	Metadata          map[string]any
}

type SendInputRequest struct {
	SessionID string
	Text      string
	Metadata  map[string]any
}

type RespondRequest struct {
	SessionID string
	RequestID string
	Action    contract.RespondAction
	Text      string
	OptionID  string
	Answers   []contract.RequestAnswer
	Metadata  map[string]any
}

type Provider interface {
	Runtime() string
	Describe() contract.RuntimeDescriptor
	StartSession(context.Context, StartSessionRequest) (*contract.RuntimeSession, error)
	ResumeSession(context.Context, ResumeSessionRequest) (*contract.RuntimeSession, error)
	SendInput(context.Context, SendInputRequest) (*contract.RuntimeEvent, error)
	Interrupt(context.Context, string) (*contract.RuntimeEvent, error)
	Respond(context.Context, RespondRequest) (*contract.RuntimeEvent, error)
	StopSession(context.Context, string) (*contract.RuntimeEvent, error)
	ListSessions(context.Context) ([]contract.RuntimeSession, error)
}

func (request *RespondRequest) Normalize() {
	action := canonicalRespondAction(request.Action)
	if action == "" {
		switch {
		case request.OptionID != "":
			action = contract.RespondActionChoose
		case request.Text != "" || len(request.Answers) > 0:
			action = contract.RespondActionSubmit
		}
	}
	request.Action = action
}

func (request RespondRequest) Validate() error {
	if request.SessionID == "" {
		return contextError("session_id is required")
	}
	if request.RequestID == "" {
		return contextError("request_id is required")
	}
	if request.Action == "" {
		return contextError("action is required unless option_id, text, or answers imply one")
	}
	switch request.Action {
	case contract.RespondActionChoose:
		if request.OptionID == "" {
			return contextError("option_id is required when action is choose")
		}
	case contract.RespondActionSubmit:
		if request.Text == "" && len(request.Answers) == 0 {
			return contextError("text or answers are required when action is submit")
		}
	}
	return nil
}

func canonicalRespondAction(action contract.RespondAction) contract.RespondAction {
	switch strings.ToLower(strings.TrimSpace(string(action))) {
	case "":
		return ""
	case "allow", "approve", "accept", "approve_once", "allow_once":
		return contract.RespondActionAllow
	case "deny", "reject", "decline", "block":
		return contract.RespondActionDeny
	case "submit", "answer", "respond":
		return contract.RespondActionSubmit
	case "cancel", "dismiss":
		return contract.RespondActionCancel
	case "choose", "select":
		return contract.RespondActionChoose
	default:
		return action
	}
}

type contextError string

func (e contextError) Error() string { return string(e) }
