package controlplane

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/benjaminwestern/agentic-control/pkg/contract"
	api "github.com/benjaminwestern/agentic-control/pkg/controlplane"
	interactionrpc "github.com/benjaminwestern/agentic-control/pkg/interaction"
)

type Service struct {
	mu               sync.RWMutex
	providers        map[string]api.Provider
	sessionRuntime   map[string]string
	directory        *SessionDirectory
	ledger           *SessionLedger
	events           *EventBus
	interaction      *interactionrpc.Client
	attention        *AttentionQueue
	interactionSubs  map[string]nativeSubscription
	operationLocks   map[string]*sync.Mutex
	locksMu          sync.Mutex
	interactionSubMu sync.Mutex
	eventLogger      *EventLogger
	threads          *ThreadStore
	sttMu            sync.Mutex
	sttSubscription  *interactionrpc.Subscription
	sttRoute         speechRouteConfig
	speechResponseMu sync.Mutex
	speechResponses  map[string]*speechResponseTurn
}

type closeProvider interface {
	Close() error
}

func NewService(providers ...api.Provider) *Service {
	service := &Service{
		providers:       make(map[string]api.Provider),
		sessionRuntime:  make(map[string]string),
		directory:       NewSessionDirectory(),
		ledger:          NewSessionLedger(),
		events:          NewEventBus(),
		interaction:     interactionrpc.NewClientFromEnv(),
		attention:       NewAttentionQueue(),
		interactionSubs: make(map[string]nativeSubscription),
		operationLocks:  make(map[string]*sync.Mutex),
		speechResponses: make(map[string]*speechResponseTurn),
	}
	if store, err := NewThreadStoreFromEnv(); err == nil {
		service.threads = store
	}
	if logger, err := NewEventLoggerFromEnv(); err == nil {
		service.eventLogger = logger
	}
	for _, provider := range providers {
		service.providers[provider.Runtime()] = provider
	}
	return service
}

func (s *Service) Close() error {
	s.mu.RLock()
	providers := make([]api.Provider, 0, len(s.providers))
	for _, provider := range s.providers {
		providers = append(providers, provider)
	}
	threads := s.threads
	eventLogger := s.eventLogger
	s.mu.RUnlock()

	var errs []error
	for _, provider := range providers {
		closer, ok := provider.(closeProvider)
		if !ok {
			continue
		}
		if err := closer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close provider %s: %w", provider.Runtime(), err))
		}
	}
	if threads != nil {
		if err := threads.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close thread store: %w", err))
		}
	}
	if eventLogger != nil {
		if err := eventLogger.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close event logger: %w", err))
		}
	}
	return errors.Join(errs...)
}

func (s *Service) PublishEvent(event contract.RuntimeEvent) {
	if s.eventLogger != nil {
		_ = s.eventLogger.Write(event)
	}
	s.directory.UpdateFromEvent(event)
	s.ledger.UpdateFromEvent(event)
	if s.threads != nil {
		if tracked, ok := s.ledger.Get(event.SessionID, event.ProviderSessionID); ok {
			_ = s.threads.UpsertTrackedSession(context.Background(), tracked)
		}
		_ = s.threads.AddEvent(context.Background(), event)
	}
	if event.EventType == "session.stopped" || event.EventType == "session.errored" {
		s.mu.Lock()
		delete(s.sessionRuntime, event.SessionID)
		s.mu.Unlock()
	}
	s.events.Publish(event)
	s.handleSpeechResponseEvent(event)
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
			"models.list",
			"thread.list",
			"thread.get",
			"thread.archive",
			"thread.set_name",
			"thread.set_metadata",
			"thread.fork",
			"thread.rollback",
			"thread.events",
			"thread.read",
			"events.subscribe",
			"events.unsubscribe",
			"session.start",
			"session.resume",
			"session.send",
			"session.get",
			"session.history",
			"session.interrupt",
			"session.respond",
			"session.stop",
			"session.list",
			"interaction.call",
			"interaction.subscribe",
			"interaction.unsubscribe",
			"diagnostics.repair",
			"diagnostics.support_report",
			"permissions.open_microphone_settings",
			"permissions.open_input_monitoring_settings",
			"permissions.open_screen_recording_settings",
			"transcript.paste_last",
			"transforms.process",
			"transforms.apply",
			"speech.tts.enqueue",
			"speech.tts.cancel",
			"speech.tts.status",
			"speech.tts.voices.list",
			"speech.tts.voices.refresh",
			"speech.tts.config.get",
			"speech.tts.config.set",
			"notification.audio.catalog",
			"notification.audio.play",
			"notification.audio.stop",
			"notification.audio.status",
			"speech.stt.start",
			"speech.stt.stop",
			"speech.stt.status",
			"speech.stt.submit",
			"speech.stt.subscribe",
			"speech.stt.unsubscribe",
			"speech.stt.models.list",
			"speech.stt.model.get",
			"speech.stt.model.set",
			"speech.stt.model.download",
			"app.open",
			"app.activate",
			"insert.targets.list",
			"insert.enqueue",
			"accessibility.diagnostics",
			"accessibility.target_overlay.open",
			"accessibility.target.highlight",
			"accessibility.target_profiles.list",
			"accessibility.target_profiles.save",
			"accessibility.target_profiles.apply",
			"accessibility.target_profiles.delete",
			"observation.target.highlight",
			"screen.observe",
			"screen.click",
			"attention.enqueue",
			"attention.list",
			"attention.update",
		},
		Runtimes:    descriptors,
		Interaction: ptrTo(interactionStatus(s.interaction.Describe())),
	}
}

func interactionStatus(status interactionrpc.Status) contract.InteractionStatus {
	return contract.InteractionStatus{
		SchemaVersion: status.SchemaVersion,
		Service:       status.Service,
		Endpoint:      status.Endpoint,
		Transport:     status.Transport,
		Available:     status.Available,
		Methods:       status.Methods,
		Capabilities:  status.Capabilities,
		Health:        status.Health,
		LastError:     status.LastError,
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
	session.Metadata = mergeSessionMetadata(session.Metadata, request.Metadata)
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
	session.Metadata = mergeSessionMetadata(session.Metadata, request.Metadata)
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

func (s *Service) GetTrackedSession(
	ctx context.Context,
	sessionID string,
	providerSessionID string,
) (*contract.TrackedSession, error) {
	if s.threads != nil {
		if thread, err := s.threads.GetThread(ctx, sessionID, providerSessionID); err == nil {
			return &thread.TrackedSession, nil
		}
		if sessionID != "" && providerSessionID != "" {
			if thread, err := s.threads.GetThread(ctx, "", providerSessionID); err == nil {
				return &thread.TrackedSession, nil
			}
		}
	}
	if tracked, ok := s.ledger.Get(sessionID, providerSessionID); ok {
		return &tracked, nil
	}
	if _, err := s.ListSessions(ctx, ""); err != nil {
		return nil, err
	}
	if tracked, ok := s.ledger.Get(sessionID, providerSessionID); ok {
		return &tracked, nil
	}
	return nil, errors.New("unknown session")
}

func (s *Service) ListTrackedSessions(
	ctx context.Context,
	runtime string,
) ([]contract.TrackedSession, error) {
	if _, err := s.ListSessions(ctx, runtime); err != nil {
		return nil, err
	}
	if s.threads != nil {
		threads, err := s.threads.ListThreads(ctx, runtime, nil)
		if err == nil {
			tracked := make([]contract.TrackedSession, 0, len(threads))
			for _, thread := range threads {
				tracked = append(tracked, thread.TrackedSession)
			}
			return tracked, nil
		}
	}
	tracked := s.ledger.List()
	if runtime == "" {
		return tracked, nil
	}
	filtered := make([]contract.TrackedSession, 0, len(tracked))
	for _, session := range tracked {
		if session.Session.Runtime == runtime {
			filtered = append(filtered, session)
		}
	}
	return filtered, nil
}

func (s *Service) ListThreads(ctx context.Context, runtime string, archived *bool) ([]contract.TrackedThread, error) {
	if _, err := s.ListSessions(ctx, runtime); err != nil {
		return nil, err
	}
	if s.threads == nil {
		tracked, err := s.ListTrackedSessions(ctx, runtime)
		if err != nil {
			return nil, err
		}
		threads := make([]contract.TrackedThread, 0, len(tracked))
		for _, item := range tracked {
			threads = append(threads, contract.TrackedThread{ThreadID: item.Session.SessionID, TrackedSession: item})
		}
		return threads, nil
	}
	return s.threads.ListThreads(ctx, runtime, archived)
}

func (s *Service) GetThread(ctx context.Context, threadID string, providerSessionID string) (*contract.TrackedThread, error) {
	if s.threads == nil {
		tracked, err := s.GetTrackedSession(ctx, threadID, providerSessionID)
		if err != nil {
			return nil, err
		}
		return &contract.TrackedThread{ThreadID: tracked.Session.SessionID, TrackedSession: *tracked}, nil
	}
	thread, err := s.threads.GetThread(ctx, threadID, providerSessionID)
	if err != nil {
		if threadID != "" && providerSessionID != "" {
			if thread, retryErr := s.threads.GetThread(ctx, "", providerSessionID); retryErr == nil {
				return &thread, nil
			}
		}
		return nil, err
	}
	return &thread, nil
}

func (s *Service) SetThreadArchived(ctx context.Context, threadID string, archived bool) error {
	if s.threads == nil {
		return errors.New("thread store unavailable")
	}
	return s.threads.SetArchived(ctx, threadID, archived)
}

func (s *Service) ListThreadEvents(ctx context.Context, threadID string, afterID int64, limit int) ([]contract.ThreadEvent, error) {
	if s.threads == nil {
		return nil, errors.New("thread store unavailable")
	}
	return s.threads.ListThreadEvents(ctx, threadID, afterID, limit)
}

func (s *Service) SetThreadName(ctx context.Context, threadID string, name string) error {
	if s.threads == nil {
		return errors.New("thread store unavailable")
	}
	return s.threads.SetName(ctx, threadID, name)
}

func (s *Service) SetThreadMetadata(ctx context.Context, threadID string, metadata contract.ThreadMetadata) error {
	if s.threads == nil {
		return errors.New("thread store unavailable")
	}
	return s.threads.SetMetadata(ctx, threadID, metadata)
}

func (s *Service) ForkThread(ctx context.Context, threadID string, name string, metadata contract.ThreadMetadata) (*contract.TrackedThread, error) {
	thread, err := s.GetThread(ctx, threadID, "")
	if err != nil {
		return nil, err
	}
	if thread.TrackedSession.Session.ProviderSessionID == "" {
		return nil, errors.New("thread cannot be forked because no provider session id is available")
	}
	if s.threads == nil {
		return nil, errors.New("thread store unavailable")
	}
	child, err := s.threads.ForkThread(ctx, thread.ThreadID, newIdentifier("thread"), true)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(name) != "" {
		_ = s.threads.SetName(ctx, child.ThreadID, name)
		child.Name = strings.TrimSpace(name)
	}
	metadata.ForkedFromThreadID = thread.ThreadID
	metadata.ForkedFromProviderSessionID = thread.TrackedSession.Session.ProviderSessionID
	metadata.ForkMode = "logical"
	_ = s.threads.SetMetadata(ctx, child.ThreadID, metadata)
	child.Metadata = metadata
	return &child, nil
}

func (s *Service) RollbackThread(ctx context.Context, threadID string, turns int) (*contract.TrackedThread, error) {
	if turns <= 0 {
		turns = 1
	}
	if s.threads == nil {
		return nil, errors.New("thread store unavailable")
	}
	thread, err := s.GetThread(ctx, threadID, "")
	if err != nil {
		return nil, err
	}
	metadata := thread.Metadata
	metadata.RollbackMode = "logical"
	metadata.RollbackTurns = turns
	metadata.RollbackFromThreadID = thread.ThreadID
	metadata.RollbackFromProviderSessionID = thread.TrackedSession.Session.ProviderSessionID
	child, err := s.threads.ForkThread(ctx, thread.ThreadID, newIdentifier("thread"), false)
	if err != nil {
		return nil, err
	}
	name := thread.Name
	if strings.TrimSpace(name) == "" {
		name = thread.TrackedSession.Session.Title
	}
	if strings.TrimSpace(name) != "" {
		name = strings.TrimSpace(name) + fmt.Sprintf(" (rollback %d)", turns)
		_ = s.threads.SetName(ctx, child.ThreadID, name)
		child.Name = name
	}
	_ = s.threads.SetMetadata(ctx, child.ThreadID, metadata)
	child.Metadata = metadata
	return &child, nil
}

func (s *Service) ModelRegistry(ctx context.Context) (contract.ModelRegistry, error) {
	return api.BuildModelRegistry(s.Describe().Runtimes), nil
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
	s.ledger.Upsert(session)
	if s.threads != nil {
		if tracked, ok := s.ledger.Get(session.SessionID, session.ProviderSessionID); ok {
			_ = s.threads.UpsertTrackedSession(context.Background(), tracked)
		}
	}
}

func mergeSessionMetadata(current map[string]any, next map[string]any) map[string]any {
	if len(current) == 0 && len(next) == 0 {
		return nil
	}
	merged := make(map[string]any, len(current)+len(next))
	for key, value := range current {
		merged[key] = value
	}
	for key, value := range next {
		merged[key] = value
	}
	return merged
}

func (s *Service) ReadThread(ctx context.Context, threadID string) (*contract.ThreadRead, error) {
	thread, err := s.GetThread(ctx, threadID, "")
	if err != nil {
		return nil, err
	}
	events, err := s.ListThreadEvents(ctx, threadID, 0, 1000)
	if err != nil {
		return nil, err
	}
	return &contract.ThreadRead{
		Thread:     *thread,
		Turns:      deriveThreadTurns(events),
		EventCount: len(events),
	}, nil
}

func deriveThreadTurns(events []contract.ThreadEvent) []contract.ThreadTurn {
	turns := map[string]*contract.ThreadTurn{}
	order := make([]string, 0)
	for _, event := range events {
		if strings.TrimSpace(event.TurnID) == "" {
			continue
		}
		turn, ok := turns[event.TurnID]
		if !ok {
			turn = &contract.ThreadTurn{TurnID: event.TurnID, Status: contract.ThreadTurnRunning, StartedAtMS: event.RecordedAtMS}
			turns[event.TurnID] = turn
			order = append(order, event.TurnID)
		}
		turn.EventCount++
		if event.RequestID != "" && !containsThreadString(turn.RequestIDs, event.RequestID) {
			turn.RequestIDs = append(turn.RequestIDs, event.RequestID)
		}
		if text := contract.EventDeltaText(event.Event); text != "" {
			turn.AssistantText += text
		}
		if final := contract.EventFinalText(event.Event); final != "" {
			turn.AssistantText = final
		}
		switch event.EventType {
		case contract.EventTurnCompleted:
			turn.Status = contract.ThreadTurnCompleted
			turn.CompletedAtMS = event.RecordedAtMS
			turn.Summary = firstThreadString(event.Summary, turn.Summary)
		case contract.EventTurnErrored, contract.EventSessionErrored:
			turn.Status = contract.ThreadTurnErrored
			turn.CompletedAtMS = event.RecordedAtMS
			turn.Summary = firstThreadString(contract.EventErrorText(event.Event), event.Summary, turn.Summary)
		case contract.EventTurnInterrupted:
			turn.Status = contract.ThreadTurnInterrupted
			turn.CompletedAtMS = event.RecordedAtMS
			turn.Summary = firstThreadString(event.Summary, turn.Summary)
		}
	}
	out := make([]contract.ThreadTurn, 0, len(order))
	for _, turnID := range order {
		out = append(out, *turns[turnID])
	}
	return out
}

func containsThreadString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func firstThreadString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (s *Service) refreshRuntime(runtime string, sessions []contract.RuntimeSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, session := range sessions {
		s.sessionRuntime[session.SessionID] = runtime
		s.directory.Upsert(session)
		s.ledger.Upsert(session)
		if s.threads != nil {
			if tracked, ok := s.ledger.Get(session.SessionID, session.ProviderSessionID); ok {
				_ = s.threads.UpsertTrackedSession(context.Background(), tracked)
			}
		}
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
