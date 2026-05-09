package apphost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/benjaminwestern/agentic-control/internal/config"
	internalcp "github.com/benjaminwestern/agentic-control/internal/controlplane"
	"github.com/benjaminwestern/agentic-control/internal/court"
	"github.com/benjaminwestern/agentic-control/pkg/contract"
	api "github.com/benjaminwestern/agentic-control/pkg/controlplane"
	"github.com/benjaminwestern/agentic-control/pkg/providers/claude"
	"github.com/benjaminwestern/agentic-control/pkg/providers/codex"
	"github.com/benjaminwestern/agentic-control/pkg/providers/gemini"
	"github.com/benjaminwestern/agentic-control/pkg/providers/openaicompatible"
	"github.com/benjaminwestern/agentic-control/pkg/providers/opencode"
	"github.com/benjaminwestern/agentic-control/pkg/providers/pi"
)

const defaultEventBuffer = 512
const defaultNotificationAudioEvent = "input.required"

type Options struct {
	Workspace     string
	Backend       string
	WorkerCommand string
	EventBuffer   int
	Court         court.EngineOptions
	ProjectStore  string
}

type Core struct {
	service  *internalcp.Service
	engine   *court.Engine
	voices   *VoiceManager
	projects *ProjectManager

	workspace string
	backend   string

	ctx       context.Context
	cancel    context.CancelFunc
	closeOnce sync.Once

	eventLimit int
	eventSeq   atomic.Int64

	eventsMu    sync.RWMutex
	events      []ObservedEvent
	subscribers map[chan ObservedEvent]struct{}
}

type ObservedEvent struct {
	Sequence int64                 `json:"sequence"`
	Event    contract.RuntimeEvent `json:"event"`
}

type EventFilter struct {
	SessionID string `json:"session_id,omitempty"`
	After     int64  `json:"after,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}

type SnapshotRequest struct {
	Workspace string `json:"workspace,omitempty"`
	Backend   string `json:"backend,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}

type Snapshot struct {
	GeneratedAt     time.Time                 `json:"generated_at"`
	Workspace       string                    `json:"workspace"`
	Backend         string                    `json:"backend"`
	System          contract.SystemDescriptor `json:"system"`
	Models          contract.ModelRegistry    `json:"models"`
	Sessions        []contract.RuntimeSession `json:"sessions"`
	TrackedSessions []contract.TrackedSession `json:"tracked_sessions"`
	Threads         []contract.TrackedThread  `json:"threads"`
	Court           CourtCatalog              `json:"court"`
	Voices          VoiceStatus               `json:"voices"`
	Runs            []court.Run               `json:"runs"`
	Attention       []contract.AttentionItem  `json:"attention"`
	RecentEvents    []ObservedEvent           `json:"recent_events"`
	Errors          []string                  `json:"errors,omitempty"`
}

type AgentsThreadsRequest struct {
	Workspace string `json:"workspace,omitempty"`
	Backend   string `json:"backend,omitempty"`
	Runtime   string `json:"runtime,omitempty"`
	Archived  *bool  `json:"archived,omitempty"`
}

type AgentsThreadsFilters struct {
	Runtime  string `json:"runtime,omitempty"`
	Archived *bool  `json:"archived,omitempty"`
}

type AgentsThreadsPage struct {
	GeneratedAt     time.Time                 `json:"generated_at"`
	Workspace       string                    `json:"workspace"`
	Backend         string                    `json:"backend"`
	Filters         AgentsThreadsFilters      `json:"filters"`
	Sessions        []contract.RuntimeSession `json:"sessions"`
	TrackedSessions []contract.TrackedSession `json:"tracked_sessions"`
	Threads         []contract.TrackedThread  `json:"threads"`
	Errors          []string                  `json:"errors,omitempty"`
}

type RuntimeStatusRequest struct {
	Workspace string `json:"workspace,omitempty"`
	Backend   string `json:"backend,omitempty"`
}

type RuntimeStatusPage struct {
	GeneratedAt time.Time                 `json:"generated_at"`
	Workspace   string                    `json:"workspace"`
	Backend     string                    `json:"backend"`
	System      contract.SystemDescriptor `json:"system"`
	Models      contract.ModelRegistry    `json:"models"`
	Errors      []string                  `json:"errors,omitempty"`
}

type AttentionRequest struct {
	Status    contract.AttentionStatus `json:"status,omitempty"`
	Action    contract.AttentionAction `json:"action,omitempty"`
	SessionID string                   `json:"session_id,omitempty"`
	Limit     int                      `json:"limit,omitempty"`
}

type AttentionFilters struct {
	Status    contract.AttentionStatus `json:"status,omitempty"`
	Action    contract.AttentionAction `json:"action,omitempty"`
	SessionID string                   `json:"session_id,omitempty"`
	Limit     int                      `json:"limit,omitempty"`
}

type AttentionPage struct {
	GeneratedAt time.Time                `json:"generated_at"`
	Filters     AttentionFilters         `json:"filters"`
	Total       int                      `json:"total"`
	Items       []contract.AttentionItem `json:"items"`
}

type CourtCatalog struct {
	List       court.CatalogListResult      `json:"list"`
	Selected   *court.CatalogGetResult      `json:"selected,omitempty"`
	Validation *court.CatalogValidateResult `json:"validation,omitempty"`
	Presets    []court.Preset               `json:"presets"`
	Juries     []court.Jury                 `json:"juries"`
	Roles      []court.Role                 `json:"roles"`
	Agents     []court.AgentConfig          `json:"agents"`
}

type StartSessionRequest struct {
	Runtime        string           `json:"runtime"`
	SessionID      string           `json:"session_id,omitempty"`
	CWD            string           `json:"cwd,omitempty"`
	Model          string           `json:"model,omitempty"`
	ModelOptions   api.ModelOptions `json:"model_options,omitempty"`
	Prompt         string           `json:"prompt,omitempty"`
	Metadata       map[string]any   `json:"metadata,omitempty"`
	ResponseSchema map[string]any   `json:"response_schema,omitempty"`
}

type ResumeSessionRequest struct {
	Runtime           string           `json:"runtime"`
	SessionID         string           `json:"session_id,omitempty"`
	ProviderSessionID string           `json:"provider_session_id"`
	CWD               string           `json:"cwd,omitempty"`
	Model             string           `json:"model,omitempty"`
	ModelOptions      api.ModelOptions `json:"model_options,omitempty"`
	Metadata          map[string]any   `json:"metadata,omitempty"`
	ResponseSchema    map[string]any   `json:"response_schema,omitempty"`
}

type SendSessionInputRequest struct {
	SessionID string                 `json:"session_id"`
	Text      string                 `json:"text"`
	Parts     []contract.ContentPart `json:"parts,omitempty"`
	Metadata  map[string]any         `json:"metadata,omitempty"`
}

type RespondSessionRequest struct {
	SessionID string                   `json:"session_id"`
	RequestID string                   `json:"request_id"`
	Action    contract.RespondAction   `json:"action,omitempty"`
	Text      string                   `json:"text,omitempty"`
	OptionID  string                   `json:"option_id,omitempty"`
	Answers   []contract.RequestAnswer `json:"answers,omitempty"`
	Metadata  map[string]any           `json:"metadata,omitempty"`
}

type SessionIDRequest struct {
	SessionID string `json:"session_id"`
}

type InteractionCallRequest struct {
	Method string         `json:"method"`
	Params map[string]any `json:"params,omitempty"`
}

type SpeechRequest struct {
	Params map[string]any `json:"params,omitempty"`
}

type NotificationRequest struct {
	Params map[string]any `json:"params,omitempty"`
}

type CourtCatalogRequest struct {
	Workspace string `json:"workspace,omitempty"`
	Backend   string `json:"backend,omitempty"`
	PresetID  string `json:"preset_id,omitempty"`
}

type StartCourtRunRequest struct {
	Task            string                   `json:"task"`
	Preset          string                   `json:"preset,omitempty"`
	Workspace       string                   `json:"workspace,omitempty"`
	Workflow        string                   `json:"workflow,omitempty"`
	DelegationScope string                   `json:"delegation_scope,omitempty"`
	Backend         string                   `json:"backend,omitempty"`
	Model           string                   `json:"model,omitempty"`
	ModelOptions    api.ModelOptions         `json:"model_options,omitempty"`
	Selection       *contract.ModelSelection `json:"selection,omitempty"`
}

type CourtMonitorRequest struct {
	RunID      string `json:"run_id"`
	EventLimit int    `json:"event_limit,omitempty"`
}

type CourtRuntimeResponseRequest struct {
	ID       int64                        `json:"id"`
	Response court.RuntimeRequestResponse `json:"response"`
}

type CourtWorkerControlRequest struct {
	WorkerID string `json:"worker_id"`
	Action   string `json:"action"`
}

func New(opts Options) (*Core, error) {
	if strings.TrimSpace(opts.Workspace) == "" {
		if cwd, err := os.Getwd(); err == nil {
			opts.Workspace = cwd
		}
	}
	if strings.TrimSpace(opts.Backend) == "" {
		opts.Backend = "opencode"
	}
	if opts.EventBuffer <= 0 {
		opts.EventBuffer = defaultEventBuffer
	}

	service := newControlPlaneService()
	engineOptions := court.EngineOptionsFromEnvironment()
	engineOptions.ControlPlane = controlPlaneAdapter{service: service}
	if opts.WorkerCommand != "" {
		engineOptions.WorkerCommand = opts.WorkerCommand
	}
	mergeCourtOptions(&engineOptions, opts.Court)

	engine, err := court.NewEngine(engineOptions)
	if err != nil {
		return nil, err
	}
	voiceManager, err := NewVoiceManager("")
	if err != nil {
		_ = engine.Close()
		return nil, err
	}
	projectManager, err := NewProjectManager(opts.ProjectStore, opts.Workspace)
	if err != nil {
		_ = engine.Close()
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	core := &Core{
		service:     service,
		engine:      engine,
		voices:      voiceManager,
		projects:    projectManager,
		workspace:   opts.Workspace,
		backend:     opts.Backend,
		ctx:         ctx,
		cancel:      cancel,
		eventLimit:  opts.EventBuffer,
		subscribers: make(map[chan ObservedEvent]struct{}),
	}
	core.startEventRecorder()
	return core, nil
}

func (c *Core) Close() error {
	var err error
	c.closeOnce.Do(func() {
		c.cancel()
		c.eventsMu.Lock()
		for subscriber := range c.subscribers {
			close(subscriber)
			delete(c.subscribers, subscriber)
		}
		c.eventsMu.Unlock()
		if c.engine != nil {
			err = c.engine.Close()
		}
		if c.service != nil {
			err = errors.Join(err, c.service.Close())
		}
	})
	return err
}

func (c *Core) Snapshot(ctx context.Context, req SnapshotRequest) (Snapshot, error) {
	workspace := firstNonEmpty(req.Workspace, c.workspace)
	backend := firstNonEmpty(req.Backend, c.backend)
	limit := req.Limit
	if limit <= 0 {
		limit = 80
	}

	snapshot := Snapshot{
		GeneratedAt:  time.Now(),
		Workspace:    workspace,
		Backend:      backend,
		System:       c.service.Describe(),
		RecentEvents: c.RecentEvents(0, limit),
		Attention:    c.service.ListAttention(internalcp.AttentionListFilter{Limit: 50}),
	}

	models, err := c.service.ModelRegistry(ctx)
	if err != nil {
		snapshot.Errors = append(snapshot.Errors, err.Error())
	} else {
		snapshot.Models = models
	}
	sessions, err := c.service.ListSessions(ctx, "")
	if err != nil {
		snapshot.Errors = append(snapshot.Errors, err.Error())
	} else {
		snapshot.Sessions = sessions
		c.voices.SyncSessions(workspace, sessions)
	}
	tracked, err := c.service.ListTrackedSessions(ctx, "")
	if err != nil {
		snapshot.Errors = append(snapshot.Errors, err.Error())
	} else {
		snapshot.TrackedSessions = tracked
	}
	threads, err := c.service.ListThreads(ctx, "", nil)
	if err != nil {
		snapshot.Errors = append(snapshot.Errors, err.Error())
	} else {
		snapshot.Threads = threads
	}
	catalog, err := c.CourtCatalog(ctx, CourtCatalogRequest{Workspace: workspace, Backend: backend})
	if err != nil {
		snapshot.Errors = append(snapshot.Errors, err.Error())
	} else {
		snapshot.Court = catalog
	}
	runs, err := c.engine.ListRuns(ctx)
	if err != nil {
		snapshot.Errors = append(snapshot.Errors, err.Error())
	} else {
		snapshot.Runs = runs
	}
	if voices, err := c.VoiceStatus(ctx); err == nil {
		snapshot.Voices = voices
	} else {
		snapshot.Voices = c.voices.Status(nil)
		snapshot.Errors = append(snapshot.Errors, err.Error())
	}
	return snapshot, nil
}

func (c *Core) AgentsThreads(ctx context.Context, req AgentsThreadsRequest) (AgentsThreadsPage, error) {
	workspace := firstNonEmpty(req.Workspace, c.workspace)
	backend := firstNonEmpty(req.Backend, c.backend)
	runtime := strings.TrimSpace(req.Runtime)
	page := AgentsThreadsPage{
		GeneratedAt: time.Now(),
		Workspace:   workspace,
		Backend:     backend,
		Filters: AgentsThreadsFilters{
			Runtime:  runtime,
			Archived: req.Archived,
		},
	}

	sessions, err := c.service.ListSessions(ctx, runtime)
	if err != nil {
		page.Errors = append(page.Errors, err.Error())
	} else {
		page.Sessions = sessions
		c.voices.SyncSessions(workspace, sessions)
	}
	tracked, err := c.service.ListTrackedSessions(ctx, runtime)
	if err != nil {
		page.Errors = append(page.Errors, err.Error())
	} else {
		page.TrackedSessions = tracked
	}
	threads, err := c.service.ListThreads(ctx, runtime, req.Archived)
	if err != nil {
		page.Errors = append(page.Errors, err.Error())
	} else {
		page.Threads = threads
	}
	return page, nil
}

func (c *Core) RuntimeStatus(ctx context.Context, req RuntimeStatusRequest) (RuntimeStatusPage, error) {
	workspace := firstNonEmpty(req.Workspace, c.workspace)
	backend := firstNonEmpty(req.Backend, c.backend)
	page := RuntimeStatusPage{
		GeneratedAt: time.Now(),
		Workspace:   workspace,
		Backend:     backend,
		System:      c.service.Describe(),
	}

	models, err := c.service.ModelRegistry(ctx)
	if err != nil {
		page.Errors = append(page.Errors, err.Error())
	} else {
		page.Models = models
	}
	return page, nil
}

func (c *Core) Attention(_ context.Context, req AttentionRequest) (AttentionPage, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 50
	}
	items := c.service.ListAttention(internalcp.AttentionListFilter{
		Status:    req.Status,
		Action:    req.Action,
		SessionID: strings.TrimSpace(req.SessionID),
	})
	total := len(items)
	if limit < len(items) {
		items = slices.Clone(items[:limit])
	}
	return AttentionPage{
		GeneratedAt: time.Now(),
		Filters: AttentionFilters{
			Status:    req.Status,
			Action:    req.Action,
			SessionID: strings.TrimSpace(req.SessionID),
			Limit:     limit,
		},
		Total: total,
		Items: items,
	}, nil
}

func (c *Core) StartSession(ctx context.Context, req StartSessionRequest) (*contract.RuntimeSession, error) {
	if strings.TrimSpace(req.Runtime) == "" {
		return nil, errors.New("runtime is required")
	}
	if strings.TrimSpace(req.CWD) == "" {
		req.CWD = c.workspace
	}
	session, err := c.service.StartSession(ctx, req.Runtime, api.StartSessionRequest{
		SessionID:      req.SessionID,
		CWD:            req.CWD,
		Model:          req.Model,
		ModelOptions:   req.ModelOptions,
		Prompt:         req.Prompt,
		Metadata:       req.Metadata,
		ResponseSchema: req.ResponseSchema,
	})
	if err != nil {
		return nil, err
	}
	_, _ = c.ClaimVoice(ctx, VoiceClaimRequest{
		Project:   firstNonEmpty(stringFromAny(req.Metadata["project"]), stringFromAny(req.Metadata["workspace"]), req.CWD, c.workspace),
		AgentID:   firstNonEmpty(agentIDFromMetadata(req.Metadata), session.SessionID),
		SessionID: session.SessionID,
		Runtime:   session.Runtime,
		Metadata:  req.Metadata,
	})
	return session, nil
}

func (c *Core) ResumeSession(ctx context.Context, req ResumeSessionRequest) (*contract.RuntimeSession, error) {
	if strings.TrimSpace(req.Runtime) == "" {
		return nil, errors.New("runtime is required")
	}
	if strings.TrimSpace(req.CWD) == "" {
		req.CWD = c.workspace
	}
	return c.service.ResumeSession(ctx, req.Runtime, api.ResumeSessionRequest{
		SessionID:         req.SessionID,
		ProviderSessionID: req.ProviderSessionID,
		CWD:               req.CWD,
		Model:             req.Model,
		ModelOptions:      req.ModelOptions,
		Metadata:          req.Metadata,
		ResponseSchema:    req.ResponseSchema,
	})
}

func (c *Core) SendSessionInput(ctx context.Context, req SendSessionInputRequest) (*contract.RuntimeEvent, error) {
	if strings.TrimSpace(req.SessionID) == "" {
		return nil, errors.New("session_id is required")
	}
	if strings.TrimSpace(req.Text) == "" && len(req.Parts) == 0 {
		return nil, errors.New("text or parts are required")
	}
	return c.service.SendInput(ctx, api.SendInputRequest{
		SessionID: req.SessionID,
		Text:      req.Text,
		Parts:     req.Parts,
		Metadata:  req.Metadata,
	})
}

func (c *Core) RespondSession(ctx context.Context, req RespondSessionRequest) (*contract.RuntimeEvent, error) {
	return c.service.Respond(ctx, api.RespondRequest{
		SessionID: req.SessionID,
		RequestID: req.RequestID,
		Action:    req.Action,
		Text:      req.Text,
		OptionID:  req.OptionID,
		Answers:   req.Answers,
		Metadata:  req.Metadata,
	})
}

func (c *Core) InterruptSession(ctx context.Context, req SessionIDRequest) (*contract.RuntimeEvent, error) {
	if strings.TrimSpace(req.SessionID) == "" {
		return nil, errors.New("session_id is required")
	}
	return c.service.Interrupt(ctx, req.SessionID)
}

func (c *Core) StopSession(ctx context.Context, req SessionIDRequest) (*contract.RuntimeEvent, error) {
	if strings.TrimSpace(req.SessionID) == "" {
		return nil, errors.New("session_id is required")
	}
	return c.service.StopSession(ctx, req.SessionID)
}

func (c *Core) InteractionCall(ctx context.Context, req InteractionCallRequest) (any, error) {
	if strings.TrimSpace(req.Method) == "" {
		return nil, errors.New("method is required")
	}
	params := cloneParams(req.Params)
	if strings.TrimSpace(req.Method) == "notification.audio.play" {
		params = normalizeNotificationAudioPlayParams(req.Params)
	}
	raw, err := c.service.InteractionCall(ctx, req.Method, params)
	if err != nil {
		return nil, err
	}
	return decodeJSONValue(raw), nil
}

func (c *Core) SpeechSubmit(ctx context.Context, req SpeechRequest) (map[string]any, error) {
	return c.service.SubmitSTT(ctx, cloneParams(req.Params))
}

func (c *Core) SpeechSubscribe(ctx context.Context, req SpeechRequest) (map[string]any, error) {
	params := cloneParams(req.Params)
	if route := mapFromAny(params["route"]); route != nil {
		if sessionID := firstNonEmpty(stringFromAny(route["target_session_id"]), stringFromAny(route["targetSessionID"])); sessionID != "" {
			binding, err := c.ClaimVoice(ctx, VoiceClaimRequest{
				Project:   firstNonEmpty(stringFromAny(params["project"]), stringFromAny(params["workspace"]), c.workspace),
				AgentID:   firstNonEmpty(stringFromAny(params["agent_id"]), stringFromAny(params["agent"])),
				SessionID: sessionID,
				Runtime:   stringFromAny(params["runtime"]),
				Metadata:  mapFromAny(params["metadata"]),
			})
			if err != nil && speechRouteWantsResponses(params, route) {
				return nil, err
			}
			if err == nil {
				responseTTS := mapFromAny(params["response_tts"])
				if responseTTS == nil {
					responseTTS = map[string]any{}
				}
				responseTTS["voice"] = binding.Voice
				params["response_tts"] = responseTTS
				routeTTS := mapFromAny(route["tts"])
				if routeTTS == nil {
					routeTTS = map[string]any{}
				}
				routeTTS["voice"] = binding.Voice
				route["tts"] = routeTTS
			}
		}
	}
	return c.service.SubscribeSTT(ctx, params)
}

func (c *Core) SpeechUnsubscribe(ctx context.Context, req SpeechRequest) (map[string]any, error) {
	return c.service.UnsubscribeSTT(ctx, cloneParams(req.Params))
}

func (c *Core) TTS(ctx context.Context, req SpeechRequest) (map[string]any, error) {
	params, _, err := c.prepareVoiceParams(ctx, req.Params)
	if err != nil {
		return nil, err
	}
	return c.service.EnqueueTTS(ctx, params)
}

func (c *Core) NotificationAudioCatalog(ctx context.Context) (any, error) {
	raw, err := c.service.InteractionCall(ctx, "notification.audio.catalog", nil)
	if err != nil {
		return nil, err
	}
	return decodeJSONValue(raw), nil
}

func (c *Core) NotificationAudioStatus(ctx context.Context) (any, error) {
	raw, err := c.service.InteractionCall(ctx, "notification.audio.status", nil)
	if err != nil {
		return nil, err
	}
	return decodeJSONValue(raw), nil
}

func (c *Core) NotificationAudioPlay(ctx context.Context, req NotificationRequest) (any, error) {
	params := normalizeNotificationAudioPlayParams(req.Params)
	item := c.service.EnqueueAttention(contract.AttentionItem{
		Action:    contract.AttentionActionNotify,
		Status:    contract.AttentionStatusActive,
		Source:    firstNonEmpty(stringFromAny(params["source"]), "agentic-control-ui"),
		Runtime:   firstNonEmpty(stringFromAny(params["runtime"]), stringFromAny(params["source"]), "agentic-control-ui"),
		SessionID: stringFromAny(params["session_id"]),
		TurnID:    stringFromAny(params["turn_id"]),
		Priority:  100,
		Text:      firstNonEmpty(stringFromAny(params["text"]), "Agent requested visual attention"),
		Metadata:  mapFromAny(params["metadata"]),
	})
	raw, err := c.service.InteractionCall(ctx, "notification.audio.play", params)
	if err != nil {
		_, _ = c.service.UpdateAttention(item.ID, internalcp.AttentionUpdate{Status: contract.AttentionStatusFailed, Error: err.Error()})
		return nil, err
	}
	result := decodeJSONValue(raw)
	if resultMap, ok := result.(map[string]any); ok {
		_, _ = c.service.UpdateAttention(item.ID, internalcp.AttentionUpdate{Status: contract.AttentionStatusCompleted, Result: resultMap})
	}
	return result, nil
}

func (c *Core) NotificationAudioStop(ctx context.Context, req NotificationRequest) (any, error) {
	raw, err := c.service.InteractionCall(ctx, "notification.audio.stop", cloneParams(req.Params))
	if err != nil {
		return nil, err
	}
	return decodeJSONValue(raw), nil
}

func (c *Core) CourtCatalog(ctx context.Context, req CourtCatalogRequest) (CourtCatalog, error) {
	workspace := firstNonEmpty(req.Workspace, c.workspace)
	backend := firstNonEmpty(req.Backend, c.backend)
	list, err := c.engine.CatalogList(workspace, backend)
	if err != nil {
		return CourtCatalog{}, err
	}
	result := CourtCatalog{List: list}
	presetID := strings.TrimSpace(req.PresetID)
	if presetID == "" && len(list.Workflows) > 0 {
		presetID = list.Workflows[0].ID
	}
	if presetID != "" {
		selected, err := c.engine.CatalogGet(workspace, presetID, backend)
		if err == nil {
			result.Selected = &selected
		}
		validation, err := c.engine.CatalogValidate(workspace, presetID, backend)
		if err == nil {
			result.Validation = &validation
		}
	}
	presets, err := c.engine.ListAvailablePresets(workspace)
	if err != nil {
		return CourtCatalog{}, err
	}
	result.Presets = presets
	if juries, err := court.ListJuriesFromRoots(list.Roots); err == nil {
		result.Juries = juries
	}
	if roles, err := court.ListRolesFromRoots(list.Roots); err == nil {
		result.Roles = roles
	}
	if agents, err := court.ListAgentConfigsFromRoots(list.Roots); err == nil {
		result.Agents = agents
	}
	return result, nil
}

func (c *Core) StartCourtRun(ctx context.Context, req StartCourtRunRequest) (court.Run, error) {
	return c.engine.StartRunWithOptions(ctx, court.StartRunOptions{
		Task:            req.Task,
		Preset:          req.Preset,
		Workflow:        req.Workflow,
		DelegationScope: req.DelegationScope,
		Backend:         firstNonEmpty(req.Backend, c.backend),
		Workspace:       firstNonEmpty(req.Workspace, c.workspace),
		Model:           req.Model,
		ModelOptions:    req.ModelOptions,
		Selection:       req.Selection,
	})
}

func (c *Core) ListCourtRuns(ctx context.Context) ([]court.Run, error) {
	return c.engine.ListRuns(ctx)
}

func (c *Core) CourtMonitor(ctx context.Context, req CourtMonitorRequest) (court.MonitorSnapshot, error) {
	if req.EventLimit <= 0 {
		req.EventLimit = 40
	}
	return c.engine.MonitorSnapshot(ctx, req.RunID, req.EventLimit)
}

func (c *Core) CourtTrace(ctx context.Context, runID string) (court.RunTrace, error) {
	if strings.TrimSpace(runID) == "" {
		return court.RunTrace{}, errors.New("run_id is required")
	}
	return c.engine.TraceRun(ctx, runID)
}

func (c *Core) CourtRespondRuntimeRequest(ctx context.Context, req CourtRuntimeResponseRequest) (court.RuntimeRequestResponseResult, error) {
	if req.ID == 0 {
		return court.RuntimeRequestResponseResult{}, errors.New("id is required")
	}
	return c.engine.RespondToRuntimeRequest(ctx, req.ID, req.Response)
}

func (c *Core) CourtControlWorker(ctx context.Context, req CourtWorkerControlRequest) (court.WorkerControlResult, error) {
	switch strings.ToLower(strings.TrimSpace(req.Action)) {
	case "cancel":
		return c.engine.CancelWorker(ctx, req.WorkerID)
	case "interrupt":
		return c.engine.InterruptWorker(ctx, req.WorkerID)
	case "resume":
		return c.engine.ResumeWorker(ctx, req.WorkerID)
	case "retry":
		return c.engine.RetryWorker(ctx, req.WorkerID)
	default:
		return court.WorkerControlResult{}, fmt.Errorf("unsupported worker action: %s", req.Action)
	}
}

func (c *Core) RecentEvents(after int64, limit int) []ObservedEvent {
	return c.RecentEventsFiltered(EventFilter{After: after, Limit: limit})
}

func (c *Core) RecentEventsFiltered(filter EventFilter) []ObservedEvent {
	filter = normalizeEventFilter(filter)
	c.eventsMu.RLock()
	defer c.eventsMu.RUnlock()
	out := make([]ObservedEvent, 0, min(filter.Limit, len(c.events)))
	for _, event := range c.events {
		if !eventMatchesFilter(event, filter) {
			continue
		}
		out = append(out, event)
	}
	if len(out) > filter.Limit {
		out = out[len(out)-filter.Limit:]
	}
	return out
}

func normalizeEventFilter(filter EventFilter) EventFilter {
	filter.SessionID = strings.TrimSpace(filter.SessionID)
	if filter.Limit <= 0 {
		filter.Limit = 100
	}
	return filter
}

func eventMatchesFilter(event ObservedEvent, filter EventFilter) bool {
	if event.Sequence <= filter.After {
		return false
	}
	if filter.SessionID != "" && event.Event.SessionID != filter.SessionID {
		return false
	}
	return true
}

func (c *Core) SubscribeEvents(buffer int) (<-chan ObservedEvent, func()) {
	if buffer <= 0 {
		buffer = 64
	}
	ch := make(chan ObservedEvent, buffer)
	c.eventsMu.Lock()
	c.subscribers[ch] = struct{}{}
	c.eventsMu.Unlock()
	unsubscribe := func() {
		c.eventsMu.Lock()
		if _, ok := c.subscribers[ch]; ok {
			close(ch)
			delete(c.subscribers, ch)
		}
		c.eventsMu.Unlock()
	}
	return ch, unsubscribe
}

func (c *Core) startEventRecorder() {
	events, unsubscribe := c.service.SubscribeEvents(c.eventLimit)
	go func() {
		defer unsubscribe()
		for {
			select {
			case <-c.ctx.Done():
				return
			case event, ok := <-events:
				if !ok {
					return
				}
				c.recordEvent(event)
			}
		}
	}()
}

func (c *Core) recordEvent(event contract.RuntimeEvent) {
	observed := ObservedEvent{
		Sequence: c.eventSeq.Add(1),
		Event:    event,
	}
	c.eventsMu.Lock()
	c.events = append(c.events, observed)
	if len(c.events) > c.eventLimit {
		c.events = slices.Clone(c.events[len(c.events)-c.eventLimit:])
	}
	for subscriber := range c.subscribers {
		select {
		case subscriber <- observed:
		default:
		}
	}
	c.eventsMu.Unlock()
}

func newControlPlaneService() *internalcp.Service {
	var service *internalcp.Service
	emit := func(event contract.RuntimeEvent) { service.PublishEvent(event) }
	cfg := config.Load()
	service = internalcp.NewService(
		codex.NewProvider(emit, cfg.Runtimes["codex"]),
		claude.NewProvider(emit, cfg.Runtimes["claude"]),
		gemini.NewProvider(emit, cfg.Runtimes["gemini"]),
		opencode.NewProvider(emit, cfg.Runtimes["opencode"]),
		pi.NewProvider(emit, cfg.Runtimes["pi"]),
		openaicompatible.NewProvider(emit, cfg.Runtimes["openai-compatible"]),
	)
	return service
}

func mergeCourtOptions(target *court.EngineOptions, override court.EngineOptions) {
	if override.ConfigDir != "" {
		target.ConfigDir = override.ConfigDir
	}
	if override.DataDir != "" {
		target.DataDir = override.DataDir
	}
	if override.RootDir != "" {
		target.RootDir = override.RootDir
	}
	if override.DBPath != "" {
		target.DBPath = override.DBPath
	}
	if override.WorkerCommand != "" {
		target.WorkerCommand = override.WorkerCommand
	}
	if override.ControlPlane != nil {
		target.ControlPlane = override.ControlPlane
	}
}

func decodeJSONValue(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return string(raw)
	}
	return value
}

func cloneParams(params map[string]any) map[string]any {
	if len(params) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(params))
	for key, value := range params {
		out[key] = value
	}
	return out
}

func normalizeNotificationAudioPlayParams(params map[string]any) map[string]any {
	out := cloneParams(params)
	if len(out) == 1 {
		if nested := mapFromAny(out["params"]); nested != nil {
			out = cloneParams(nested)
		}
	}
	if systemSound := stringFromAny(out["systemSound"]); systemSound != "" && stringFromAny(out["system_sound"]) == "" {
		out["system_sound"] = systemSound
		delete(out, "systemSound")
	}
	if strings.EqualFold(stringFromAny(out["event"]), "visual_attention") {
		out["event"] = defaultNotificationAudioEvent
	}
	if !hasNonEmptyParam(out, "path", "system_sound", "event") {
		out["event"] = defaultNotificationAudioEvent
	}
	if _, ok := out["interrupt"]; !ok {
		out["interrupt"] = true
	}
	return out
}

func hasNonEmptyParam(params map[string]any, keys ...string) bool {
	for _, key := range keys {
		value, ok := params[key]
		if !ok || value == nil {
			continue
		}
		if text, ok := value.(string); ok {
			if strings.TrimSpace(text) != "" {
				return true
			}
			continue
		}
		return true
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

type controlPlaneAdapter struct {
	service *internalcp.Service
}

func (a controlPlaneAdapter) Describe() contract.SystemDescriptor {
	return a.service.Describe()
}

func (a controlPlaneAdapter) SubscribeEvents(buffer int) (<-chan contract.RuntimeEvent, func()) {
	return a.service.SubscribeEvents(buffer)
}

func (a controlPlaneAdapter) StartSession(ctx context.Context, runtime string, request api.StartSessionRequest) (*contract.RuntimeSession, error) {
	return a.service.StartSession(ctx, runtime, request)
}

func (a controlPlaneAdapter) ResumeSession(ctx context.Context, runtime string, request api.ResumeSessionRequest) (*contract.RuntimeSession, error) {
	return a.service.ResumeSession(ctx, runtime, request)
}

func (a controlPlaneAdapter) SendInput(ctx context.Context, request api.SendInputRequest) (*contract.RuntimeEvent, error) {
	return a.service.SendInput(ctx, request)
}

func (a controlPlaneAdapter) Interrupt(ctx context.Context, sessionID string) (*contract.RuntimeEvent, error) {
	return a.service.Interrupt(ctx, sessionID)
}

func (a controlPlaneAdapter) Respond(ctx context.Context, request api.RespondRequest) (*contract.RuntimeEvent, error) {
	return a.service.Respond(ctx, request)
}

func (a controlPlaneAdapter) StopSession(ctx context.Context, sessionID string) (*contract.RuntimeEvent, error) {
	return a.service.StopSession(ctx, sessionID)
}

func (a controlPlaneAdapter) ListSessions(ctx context.Context, runtime string) ([]contract.RuntimeSession, error) {
	return a.service.ListSessions(ctx, runtime)
}

func (a controlPlaneAdapter) GetTrackedSession(ctx context.Context, sessionID string, providerSessionID string) (*contract.TrackedSession, error) {
	return a.service.GetTrackedSession(ctx, sessionID, providerSessionID)
}

func (a controlPlaneAdapter) GetThread(ctx context.Context, threadID string, providerSessionID string) (*contract.TrackedThread, error) {
	return a.service.GetThread(ctx, threadID, providerSessionID)
}

func (a controlPlaneAdapter) SetThreadMetadata(ctx context.Context, threadID string, metadata contract.ThreadMetadata) error {
	return a.service.SetThreadMetadata(ctx, threadID, metadata)
}
