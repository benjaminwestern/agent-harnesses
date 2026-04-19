package controlplane

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/benjaminwestern/agentic-control/pkg/contract"
	api "github.com/benjaminwestern/agentic-control/pkg/controlplane"
)

type Service struct {
	mu             sync.RWMutex
	providers      map[string]api.Provider
	sessionRuntime map[string]string
	directory      *SessionDirectory
	events         *EventBus
	operationLocks map[string]*sync.Mutex
	locksMu        sync.Mutex
	eventLogger    *EventLogger
}

func NewService(providers ...api.Provider) *Service {
	service := &Service{
		providers:      make(map[string]api.Provider),
		sessionRuntime: make(map[string]string),
		directory:      NewSessionDirectory(),
		events:         NewEventBus(),
		operationLocks: make(map[string]*sync.Mutex),
	}
	if logger, err := NewEventLoggerFromEnv(); err == nil {
		service.eventLogger = logger
	}
	for _, provider := range providers {
		service.providers[provider.Runtime()] = provider
	}
	return service
}

func (s *Service) PublishEvent(event contract.RuntimeEvent) {
	if s.eventLogger != nil {
		_ = s.eventLogger.Write(event)
	}
	s.directory.UpdateFromEvent(event)
	if event.EventType == "session.stopped" || event.EventType == "session.errored" {
		s.mu.Lock()
		delete(s.sessionRuntime, event.SessionID)
		s.mu.Unlock()
	}
	s.events.Publish(event)
}

func (s *Service) SubscribeEvents(buffer int) (<-chan contract.RuntimeEvent, func()) {
	return s.events.Subscribe(buffer)
}

func (s *Service) Describe() contract.SystemDescriptor {
	s.mu.RLock()
	descriptors := make([]contract.RuntimeDescriptor, 0, len(s.providers))
	for _, provider := range s.providers {
		descriptors = append(descriptors, provider.Describe())
	}
	s.mu.RUnlock()

	slices.SortFunc(descriptors, func(left, right contract.RuntimeDescriptor) int {
		switch {
		case left.Runtime < right.Runtime:
			return -1
		case left.Runtime > right.Runtime:
			return 1
		default:
			return 0
		}
	})

	return contract.SystemDescriptor{
		SchemaVersion:       contract.ControlPlaneSchemaVersion,
		WireProtocolVersion: contract.WireProtocolVersion,
		Methods: []string{
			"system.ping",
			"system.describe",
			"events.subscribe",
			"events.unsubscribe",
			"session.start",
			"session.resume",
			"session.send",
			"session.interrupt",
			"session.respond",
			"session.stop",
			"session.list",
		},
		Runtimes: descriptors,
	}
}

func (s *Service) StartSession(
	ctx context.Context,
	runtime string,
	request api.StartSessionRequest,
) (*contract.RuntimeSession, error) {
	provider, err := s.providerByRuntime(runtime)
	if err != nil {
		return nil, err
	}
	if request.SessionID == "" {
		request.SessionID = newIdentifier("session")
	}

	var session *contract.RuntimeSession
	err = s.withSessionLock(request.SessionID, func() error {
		var err error
		session, err = provider.StartSession(ctx, request)
		return err
	})
	if err != nil {
		return nil, err
	}
	s.rememberSession(*session)
	return session, nil
}

func (s *Service) ResumeSession(
	ctx context.Context,
	runtime string,
	request api.ResumeSessionRequest,
) (*contract.RuntimeSession, error) {
	provider, err := s.providerByRuntime(runtime)
	if err != nil {
		return nil, err
	}
	if request.SessionID == "" {
		request.SessionID = newIdentifier("session")
	}

	var session *contract.RuntimeSession
	err = s.withSessionLock(request.SessionID, func() error {
		var err error
		session, err = provider.ResumeSession(ctx, request)
		return err
	})
	if err != nil {
		return nil, err
	}
	s.rememberSession(*session)
	return session, nil
}

func (s *Service) SendInput(
	ctx context.Context,
	request api.SendInputRequest,
) (*contract.RuntimeEvent, error) {
	provider, err := s.providerBySession(ctx, request.SessionID)
	if err != nil {
		return nil, err
	}
	var event *contract.RuntimeEvent
	err = s.withSessionLock(request.SessionID, func() error {
		var err error
		event, err = provider.SendInput(ctx, request)
		return err
	})
	return event, err
}

func (s *Service) Interrupt(
	ctx context.Context,
	sessionID string,
) (*contract.RuntimeEvent, error) {
	provider, err := s.providerBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	var event *contract.RuntimeEvent
	err = s.withSessionLock(sessionID, func() error {
		var err error
		event, err = provider.Interrupt(ctx, sessionID)
		return err
	})
	return event, err
}

func (s *Service) Respond(
	ctx context.Context,
	request api.RespondRequest,
) (*contract.RuntimeEvent, error) {
	request.Normalize()
	if err := request.Validate(); err != nil {
		return nil, err
	}
	provider, err := s.providerBySession(ctx, request.SessionID)
	if err != nil {
		return nil, err
	}
	var event *contract.RuntimeEvent
	err = s.withSessionLock(request.SessionID, func() error {
		var err error
		event, err = provider.Respond(ctx, request)
		return err
	})
	return event, err
}

func (s *Service) StopSession(
	ctx context.Context,
	sessionID string,
) (*contract.RuntimeEvent, error) {
	provider, err := s.providerBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	var event *contract.RuntimeEvent
	err = s.withSessionLock(sessionID, func() error {
		var err error
		event, err = provider.StopSession(ctx, sessionID)
		return err
	})
	return event, err
}

func (s *Service) ListSessions(
	ctx context.Context,
	runtime string,
) ([]contract.RuntimeSession, error) {
	if runtime != "" {
		provider, err := s.providerByRuntime(runtime)
		if err != nil {
			return nil, err
		}
		sessions, err := provider.ListSessions(ctx)
		if err != nil {
			return nil, err
		}
		s.refreshRuntime(runtime, sessions)
		return sessions, nil
	}

	s.mu.RLock()
	providers := make([]api.Provider, 0, len(s.providers))
	for _, provider := range s.providers {
		providers = append(providers, provider)
	}
	s.mu.RUnlock()

	var sessions []contract.RuntimeSession
	for _, provider := range providers {
		providerSessions, err := provider.ListSessions(ctx)
		if err != nil {
			return nil, err
		}
		s.refreshRuntime(provider.Runtime(), providerSessions)
		sessions = append(sessions, providerSessions...)
	}
	return sessions, nil
}

func (s *Service) providerByRuntime(runtime string) (api.Provider, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	provider, ok := s.providers[runtime]
	if !ok {
		return nil, fmt.Errorf("unsupported runtime: %s", runtime)
	}
	return provider, nil
}

func (s *Service) providerBySession(
	ctx context.Context,
	sessionID string,
) (api.Provider, error) {
	s.mu.RLock()
	runtime, ok := s.sessionRuntime[sessionID]
	s.mu.RUnlock()
	if ok {
		return s.providerByRuntime(runtime)
	}

	sessions, err := s.ListSessions(ctx, "")
	if err != nil {
		return nil, err
	}
	for _, session := range sessions {
		if session.SessionID == sessionID {
			return s.providerByRuntime(session.Runtime)
		}
	}
	return nil, errors.New("unknown session")
}

func (s *Service) rememberSession(session contract.RuntimeSession) {
	s.mu.Lock()
	s.sessionRuntime[session.SessionID] = session.Runtime
	s.mu.Unlock()
	s.directory.Upsert(session)
}

func (s *Service) refreshRuntime(runtime string, sessions []contract.RuntimeSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, session := range sessions {
		s.sessionRuntime[session.SessionID] = runtime
		s.directory.Upsert(session)
	}
}

func (s *Service) withSessionLock(sessionID string, fn func() error) error {
	if sessionID == "" {
		return fn()
	}

	s.locksMu.Lock()
	lock, ok := s.operationLocks[sessionID]
	if !ok {
		lock = &sync.Mutex{}
		s.operationLocks[sessionID] = lock
	}
	s.locksMu.Unlock()

	lock.Lock()
	defer lock.Unlock()
	return fn()
}

func newIdentifier(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}
