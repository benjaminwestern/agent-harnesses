package controlplane

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/benjaminwestern/agentic-control/pkg/contract"
	api "github.com/benjaminwestern/agentic-control/pkg/controlplane"
	interactionrpc "github.com/benjaminwestern/agentic-control/pkg/interaction"
	"github.com/benjaminwestern/agentic-control/pkg/toolrepair"
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
	workspace        *WorkspaceStore
	sttMu            sync.Mutex
	sttSubscription  *interactionrpc.Subscription
	sttRoute         speechRouteConfig
	speechResponseMu sync.Mutex
	speechResponses  map[string]*speechResponseTurn
	turnTextMu       sync.Mutex
	turnTextBuffers  map[string]string
	textGenRouter    *api.TextGenerationRouter
	embeddingRouter  *api.EmbeddingRouter
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
		turnTextBuffers: make(map[string]string),
	}
	if store, err := NewThreadStoreFromEnv(); err == nil {
		service.threads = store
	}
	if store, err := NewWorkspaceStoreFromEnv(); err == nil {
		service.workspace = store
	}
	if logger, err := NewEventLoggerFromEnv(); err == nil {
		service.eventLogger = logger
	}

	textGenProviders := make(map[string]api.TextGenerationProvider)
	embeddingProviders := make(map[string]api.EmbeddingProvider)
	for _, provider := range providers {
		service.providers[provider.Runtime()] = provider
		if textGen, ok := provider.(api.TextGenerationProvider); ok {
			textGenProviders[provider.Runtime()] = textGen
		}
		if embeddings, ok := provider.(api.EmbeddingProvider); ok {
			embeddingProviders[provider.Runtime()] = embeddings
		}
	}
	service.textGenRouter = api.NewTextGenerationRouter("codex", textGenProviders)
	service.embeddingRouter = api.NewEmbeddingRouter("openai-compatible", embeddingProviders)

	return service
}

func (s *Service) TextGen() *api.TextGenerationRouter {
	return s.textGenRouter
}

func (s *Service) Embeddings() *api.EmbeddingRouter {
	return s.embeddingRouter
}

func (s *Service) Workspace() *WorkspaceStore {
	return s.workspace
}

func (s *Service) Close() error {
	s.mu.RLock()
	providers := make([]api.Provider, 0, len(s.providers))
	for _, provider := range s.providers {
		providers = append(providers, provider)
	}
	threads := s.threads
	workspace := s.workspace
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
	if workspace != nil {
		if err := workspace.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close workspace store: %w", err))
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

	event = s.repairToolCalls(event)

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

func (s *Service) repairToolCalls(event contract.RuntimeEvent) contract.RuntimeEvent {
	key, ok := toolRepairBufferKey(event)
	if !ok {
		return event
	}

	switch event.EventType {
	case contract.EventAssistantMessageDelta:
		if delta := contract.EventDeltaText(event); delta != "" {
			s.turnTextMu.Lock()
			s.turnTextBuffers[key] += delta
			s.turnTextMu.Unlock()
		}
	case contract.EventTurnCompleted:
		s.turnTextMu.Lock()
		text := s.turnTextBuffers[key]
		delete(s.turnTextBuffers, key)
		s.turnTextMu.Unlock()

		if text == "" {
			return event
		}
		tools := contractToolCallsFromRepair(text)
		if len(tools) == 0 {
			return event
		}
		if event.Payload == nil {
			event.Payload = map[string]any{}
		}
		event.Payload[contract.PayloadExtractedTools] = tools
	case contract.EventTurnErrored, contract.EventTurnInterrupted:
		s.turnTextMu.Lock()
		delete(s.turnTextBuffers, key)
		s.turnTextMu.Unlock()
	case contract.EventSessionErrored, contract.EventSessionStopped:
		s.clearToolRepairBuffersForSession(event)
	}
	return event
}

func toolRepairBufferKey(event contract.RuntimeEvent) (string, bool) {
	sessionID := strings.TrimSpace(event.SessionID)
	providerSessionID := strings.TrimSpace(event.ProviderSessionID)
	turnID := strings.TrimSpace(event.TurnID)
	if sessionID == "" && providerSessionID == "" && turnID == "" {
		return "", false
	}
	if turnID == "" {
		turnID = "_active"
	}
	return strings.Join([]string{strings.TrimSpace(event.Runtime), sessionID, providerSessionID, turnID}, "\x00"), true
}

func (s *Service) clearToolRepairBuffersForSession(event contract.RuntimeEvent) {
	sessionID := strings.TrimSpace(event.SessionID)
	providerSessionID := strings.TrimSpace(event.ProviderSessionID)
	if sessionID == "" && providerSessionID == "" {
		return
	}
	s.turnTextMu.Lock()
	defer s.turnTextMu.Unlock()
	for key := range s.turnTextBuffers {
		parts := strings.Split(key, "\x00")
		if len(parts) != 4 {
			continue
		}
		if (sessionID != "" && parts[1] == sessionID) || (providerSessionID != "" && parts[2] == providerSessionID) {
			delete(s.turnTextBuffers, key)
		}
	}
}

func contractToolCallsFromRepair(text string) []contract.ToolCall {
	extracted := toolrepair.ExtractToolCalls(text)
	tools := make([]contract.ToolCall, 0, len(extracted))
	for _, item := range extracted {
		if strings.TrimSpace(item.Name) == "" {
			continue
		}
		tools = append(tools, contract.ToolCall{
			ID:   item.ID,
			Type: "function",
			Function: contract.FunctionCall{
				Name:      strings.TrimSpace(item.Name),
				Arguments: item.Arguments,
			},
		})
	}
	return tools
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
			"memory.set",
			"memory.get",
			"memory.delete",
			"memory.list",
			"documents.write",
			"documents.get",
			"documents.delete",
			"documents.list",
			"tasks.create",
			"tasks.get",
			"tasks.update",
			"tasks.delete",
			"tasks.list",
			"tasks.comments.create",
			"tasks.comments.list",
			"wakeups.set",
			"wakeups.get",
			"wakeups.cancel",
			"wakeups.pause",
			"wakeups.resume",
			"wakeups.reset",
			"wakeups.list_pending",
			"leases.acquire",
			"leases.release",
			"leases.reset",
			"leases.get",
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
	request.Normalize()

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
	request.Normalize()

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
	if err := contract.ValidateContentParts(request.Parts); err != nil {
		return nil, err
	}
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
	activeSyntheticTurns := map[string]string{}
	syntheticTurnSeq := 0

	for _, event := range events {
		turnID, syntheticScope := resolveThreadTurnID(event, activeSyntheticTurns, &syntheticTurnSeq)
		if turnID == "" {
			continue
		}
		turn, ok := turns[turnID]
		if !ok {
			turn = &contract.ThreadTurn{TurnID: turnID, Status: contract.ThreadTurnRunning, StartedAtMS: event.RecordedAtMS}
			turns[turnID] = turn
			order = append(order, turnID)
		}
		turn.EventCount++
		if event.RequestID != "" && !containsThreadString(turn.RequestIDs, event.RequestID) {
			turn.RequestIDs = append(turn.RequestIDs, event.RequestID)
		}
		if event.EventType == contract.EventTurnStarted {
			if msg, ok := userMessageFromTurnStarted(event); ok && !hasRoleMessage(turn.Messages, contract.MessageRoleUser) {
				turn.Messages = append(turn.Messages, msg)
			}
		}
		if text := threadDeltaText(event.Event); text != "" {
			if event.EventType == contract.EventAssistantThoughtDelta {
				turn.ReasoningText += text
				appendAssistantPart(turn, contract.ContentPartTypeReasoning, text)
			} else {
				turn.AssistantText += text
				appendAssistantPart(turn, contract.ContentPartTypeText, text)
			}
		}
		if final := contract.EventFinalText(event.Event); final != "" {
			turn.AssistantText = final
			if !hasAssistantParts(turn) {
				appendAssistantPart(turn, contract.ContentPartTypeText, final)
			}
		}

		if event.EventType == contract.EventTurnCompleted && event.Event.Payload != nil {
			if tools := threadToolCallsFromPayload(event.Event.Payload[contract.PayloadExtractedTools]); len(tools) > 0 {
				appendAssistantToolCalls(turn, tools)
			}
		}
		if msg, ok := toolMessageFromThreadEvent(event); ok {
			turn.Messages = append(turn.Messages, msg)
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
		if syntheticScope != "" && (contract.IsTerminalTurnEvent(event.Event) || event.EventType == contract.EventTurnInterrupted) {
			delete(activeSyntheticTurns, syntheticScope)
		}
	}

	out := make([]contract.ThreadTurn, 0, len(order))
	for _, turnID := range order {
		t := turns[turnID]

		if !hasRoleMessage(t.Messages, contract.MessageRoleUser) && t.Summary != "" {
			prompt := strings.TrimPrefix(t.Summary, "Started turn: ")
			prependMessage(t, contract.Message{
				Role: contract.MessageRoleUser,
				Parts: []contract.ContentPart{
					{Type: contract.ContentPartTypeText, Text: prompt},
				},
			})
		}

		if !hasRoleMessage(t.Messages, contract.MessageRoleAssistant) {
			if t.ReasoningText != "" {
				appendAssistantPart(t, contract.ContentPartTypeReasoning, t.ReasoningText)
			}
			if t.AssistantText != "" {
				appendAssistantPart(t, contract.ContentPartTypeText, t.AssistantText)
			}
		}

		out = append(out, *t)
	}
	return out
}

func resolveThreadTurnID(event contract.ThreadEvent, active map[string]string, seq *int) (string, string) {
	if turnID := strings.TrimSpace(event.TurnID); turnID != "" {
		return turnID, ""
	}
	scope := firstThreadString(event.ThreadID, event.Event.SessionID, event.Event.ProviderSessionID)
	if scope == "" {
		return "", ""
	}
	if event.EventType == contract.EventTurnStarted || active[scope] == "" {
		*seq = *seq + 1
		active[scope] = fmt.Sprintf("%s:synthetic-turn-%d", scope, *seq)
	}
	return active[scope], scope
}

func threadDeltaText(event contract.RuntimeEvent) string {
	switch event.EventType {
	case contract.EventAssistantMessageDelta:
		return contract.EventDeltaText(event)
	case contract.EventAssistantThoughtDelta:
		return contract.PayloadString(event.Payload, "delta")
	default:
		return ""
	}
}

func userMessageFromTurnStarted(event contract.ThreadEvent) (contract.Message, bool) {
	parts := make([]contract.ContentPart, 0)
	if text := contract.EventPayloadString(event.Event, contract.PayloadInputText); text != "" {
		parts = append(parts, contract.ContentPart{Type: contract.ContentPartTypeText, Text: text})
	}
	parts = append(parts, contentPartsFromPayload(event.Event.Payload[contract.PayloadInputParts])...)
	if len(parts) == 0 {
		prompt := strings.TrimSpace(strings.TrimPrefix(event.Summary, "Started turn: "))
		if prompt == "" {
			return contract.Message{}, false
		}
		parts = append(parts, contract.ContentPart{Type: contract.ContentPartTypeText, Text: prompt})
	}
	return contract.Message{Role: contract.MessageRoleUser, Parts: parts}, true
}

func contentPartsFromPayload(raw any) []contract.ContentPart {
	switch value := raw.(type) {
	case nil:
		return nil
	case []contract.ContentPart:
		return append([]contract.ContentPart(nil), value...)
	case []any:
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil
		}
		var parts []contract.ContentPart
		if err := json.Unmarshal(encoded, &parts); err != nil {
			return nil
		}
		return parts
	default:
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil
		}
		var parts []contract.ContentPart
		if err := json.Unmarshal(encoded, &parts); err != nil {
			return nil
		}
		return parts
	}
}

func appendAssistantPart(turn *contract.ThreadTurn, partType string, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	if len(turn.Messages) == 0 || turn.Messages[len(turn.Messages)-1].Role != contract.MessageRoleAssistant {
		turn.Messages = append(turn.Messages, contract.Message{Role: contract.MessageRoleAssistant})
	}
	msg := &turn.Messages[len(turn.Messages)-1]
	if len(msg.Parts) > 0 && msg.Parts[len(msg.Parts)-1].Type == partType {
		if msg.Parts[len(msg.Parts)-1].Text == "" {
			msg.Parts[len(msg.Parts)-1].Text = text
		} else {
			msg.Parts[len(msg.Parts)-1].Text += text
		}
		return
	}
	msg.Parts = append(msg.Parts, contract.ContentPart{Type: partType, Text: text})
}

func appendAssistantToolCalls(turn *contract.ThreadTurn, tools []contract.ToolCall) {
	if len(tools) == 0 {
		return
	}
	idx := lastRoleMessageIndex(turn.Messages, contract.MessageRoleAssistant)
	if idx < 0 {
		turn.Messages = append(turn.Messages, contract.Message{Role: contract.MessageRoleAssistant})
		idx = len(turn.Messages) - 1
	}
	turn.Messages[idx].ToolCalls = append(turn.Messages[idx].ToolCalls, tools...)
}

func toolMessageFromThreadEvent(event contract.ThreadEvent) (contract.Message, bool) {
	switch event.EventType {
	case contract.EventToolCompleted, contract.EventToolErrored:
	default:
		return contract.Message{}, false
	}
	text := firstThreadString(
		contract.EventPayloadString(event.Event, "result"),
		contract.EventPayloadString(event.Event, "output"),
		contract.EventPayloadString(event.Event, "stdout"),
		contract.EventPayloadString(event.Event, "stderr"),
		contract.EventPayloadString(event.Event, "error"),
		event.Summary,
	)
	if text == "" {
		return contract.Message{}, false
	}
	return contract.Message{
		Role:       contract.MessageRoleTool,
		ToolCallID: firstThreadString(event.RequestID, event.Event.RequestID, contract.EventPayloadString(event.Event, "tool_call_id"), contract.EventPayloadString(event.Event, "id")),
		Name:       firstThreadString(contract.EventPayloadString(event.Event, "name"), contract.EventPayloadString(event.Event, "tool_name")),
		Parts:      []contract.ContentPart{{Type: contract.ContentPartTypeText, Text: text}},
	}, true
}

func threadToolCallsFromPayload(raw any) []contract.ToolCall {
	switch value := raw.(type) {
	case nil:
		return nil
	case []contract.ToolCall:
		return append([]contract.ToolCall(nil), value...)
	case []toolrepair.ToolCall:
		tools := make([]contract.ToolCall, 0, len(value))
		for _, item := range value {
			tools = append(tools, contract.ToolCall{
				ID:   item.ID,
				Type: "function",
				Function: contract.FunctionCall{
					Name:      item.Name,
					Arguments: item.Arguments,
				},
			})
		}
		return tools
	case []any:
		tools := make([]contract.ToolCall, 0, len(value))
		for _, item := range value {
			if tool, ok := threadToolCallFromAny(item); ok {
				tools = append(tools, tool)
			}
		}
		return tools
	default:
		var tools []contract.ToolCall
		encoded, err := json.Marshal(value)
		if err == nil && json.Unmarshal(encoded, &tools) == nil {
			return tools
		}
		if tool, ok := threadToolCallFromAny(value); ok {
			return []contract.ToolCall{tool}
		}
		return nil
	}
}

func threadToolCallFromAny(raw any) (contract.ToolCall, bool) {
	encoded, err := json.Marshal(raw)
	if err != nil {
		return contract.ToolCall{}, false
	}
	var tool contract.ToolCall
	if err := json.Unmarshal(encoded, &tool); err == nil && tool.Function.Name != "" {
		if tool.Type == "" {
			tool.Type = "function"
		}
		return tool, true
	}
	var legacy struct {
		ID        string         `json:"ID"`
		Name      string         `json:"Name"`
		Arguments map[string]any `json:"Arguments"`
	}
	if err := json.Unmarshal(encoded, &legacy); err == nil && legacy.Name != "" {
		return contract.ToolCall{
			ID:   legacy.ID,
			Type: "function",
			Function: contract.FunctionCall{
				Name:      legacy.Name,
				Arguments: legacy.Arguments,
			},
		}, true
	}
	return contract.ToolCall{}, false
}

func hasRoleMessage(messages []contract.Message, role contract.MessageRole) bool {
	return lastRoleMessageIndex(messages, role) >= 0
}

func lastRoleMessageIndex(messages []contract.Message, role contract.MessageRole) int {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == role {
			return i
		}
	}
	return -1
}

func hasAssistantParts(turn *contract.ThreadTurn) bool {
	for _, message := range turn.Messages {
		if message.Role == contract.MessageRoleAssistant && len(message.Parts) > 0 {
			return true
		}
	}
	return false
}

func prependMessage(turn *contract.ThreadTurn, message contract.Message) {
	turn.Messages = append([]contract.Message{message}, turn.Messages...)
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
