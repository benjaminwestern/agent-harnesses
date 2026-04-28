package embedded

import (
	"context"

	internal "github.com/benjaminwestern/agentic-control/internal/controlplane"
	"github.com/benjaminwestern/agentic-control/internal/controlplane/providers/claude"
	"github.com/benjaminwestern/agentic-control/internal/controlplane/providers/codex"
	"github.com/benjaminwestern/agentic-control/internal/controlplane/providers/gemini"
	"github.com/benjaminwestern/agentic-control/internal/controlplane/providers/opencode"
	"github.com/benjaminwestern/agentic-control/internal/controlplane/providers/pi"
	"github.com/benjaminwestern/agentic-control/pkg/contract"
	api "github.com/benjaminwestern/agentic-control/pkg/controlplane"
)

// ControlPlane exposes the app-managed runtime control plane as an importable
// Go API. It intentionally reuses the public contract and controlplane types so
// callers do not need to shell out or duplicate the RPC protocol.
type ControlPlane struct {
	service *internal.Service
}

func New() *ControlPlane {
	var service *internal.Service
	emit := func(event contract.RuntimeEvent) {
		service.PublishEvent(event)
	}
	service = internal.NewService(
		codex.NewProvider(emit),
		claude.NewProvider(emit),
		gemini.NewProvider(emit),
		opencode.NewProvider(emit),
		pi.NewProvider(emit),
	)
	return &ControlPlane{service: service}
}

func (c *ControlPlane) Close() error {
	if c == nil || c.service == nil {
		return nil
	}
	return c.service.Close()
}

func (c *ControlPlane) Describe() contract.SystemDescriptor {
	return c.service.Describe()
}

func (c *ControlPlane) SubscribeEvents(buffer int) (<-chan contract.RuntimeEvent, func()) {
	return c.service.SubscribeEvents(buffer)
}

func (c *ControlPlane) StartSession(ctx context.Context, runtime string, request api.StartSessionRequest) (*contract.RuntimeSession, error) {
	return c.service.StartSession(ctx, runtime, request)
}

func (c *ControlPlane) ResumeSession(ctx context.Context, runtime string, request api.ResumeSessionRequest) (*contract.RuntimeSession, error) {
	return c.service.ResumeSession(ctx, runtime, request)
}

func (c *ControlPlane) SendInput(ctx context.Context, request api.SendInputRequest) (*contract.RuntimeEvent, error) {
	return c.service.SendInput(ctx, request)
}

func (c *ControlPlane) Interrupt(ctx context.Context, sessionID string) (*contract.RuntimeEvent, error) {
	return c.service.Interrupt(ctx, sessionID)
}

func (c *ControlPlane) Respond(ctx context.Context, request api.RespondRequest) (*contract.RuntimeEvent, error) {
	return c.service.Respond(ctx, request)
}

func (c *ControlPlane) StopSession(ctx context.Context, sessionID string) (*contract.RuntimeEvent, error) {
	return c.service.StopSession(ctx, sessionID)
}

func (c *ControlPlane) ListSessions(ctx context.Context, runtime string) ([]contract.RuntimeSession, error) {
	return c.service.ListSessions(ctx, runtime)
}

func (c *ControlPlane) GetTrackedSession(ctx context.Context, sessionID string, providerSessionID string) (*contract.TrackedSession, error) {
	return c.service.GetTrackedSession(ctx, sessionID, providerSessionID)
}

func (c *ControlPlane) ListTrackedSessions(ctx context.Context, runtime string) ([]contract.TrackedSession, error) {
	return c.service.ListTrackedSessions(ctx, runtime)
}

func (c *ControlPlane) GetThread(ctx context.Context, threadID string, providerSessionID string) (*contract.TrackedThread, error) {
	return c.service.GetThread(ctx, threadID, providerSessionID)
}

func (c *ControlPlane) SetThreadMetadata(ctx context.Context, threadID string, metadata contract.ThreadMetadata) error {
	return c.service.SetThreadMetadata(ctx, threadID, metadata)
}
