package opencode

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/benjaminwestern/agentic-control/internal/controlplane/providerprobe"
	"github.com/benjaminwestern/agentic-control/pkg/contract"
	api "github.com/benjaminwestern/agentic-control/pkg/controlplane"
)

const (
	runtimeName                 = "opencode"
	loopbackHost                = "127.0.0.1"
	toolCompletionDebounceDelay = 1500 * time.Millisecond
	inventoryProbeTTL           = 5 * time.Minute
)

type eventSink func(contract.RuntimeEvent)

type Provider struct {
	mu               sync.RWMutex
	resyncMu         sync.Mutex
	resyncRunning    bool
	sessions         map[string]*session
	providerSessions map[string]*session
	server           *serverProcess
	emit             eventSink
	probe            *providerprobe.Cache
	inventoryMu      sync.Mutex
	inventory        remoteProviderInventory
	inventoryExpires time.Time
}

type session struct {
	mu                   sync.RWMutex
	appSessionID         string
	providerSessionID    string
	status               contract.SessionStatus
	interruptRequested   bool
	cwd                  string
	model                string
	title                string
	createdAtMS          int64
	updatedAtMS          int64
	lastError            string
	activeTurnID         string
	lastAssistantSummary string
	messageTurns         map[string]string
	completedTurns       map[string]struct{}
	pendingRequests      map[string]pendingRequest
	toolActivity         bool
	turnGeneration       int64
	toolStates           map[string]string
	provider             *Provider
}

type pendingRequest struct {
	Request contract.PendingRequest
}

type serverProcess struct {
	ctx          context.Context
	cancel       context.CancelFunc
	cmd          *exec.Cmd
	baseURL      string
	client       *http.Client
	streamClient *http.Client
	provider     *Provider
}

type remoteEvent struct {
	Type       string          `json:"type"`
	Properties json.RawMessage `json:"properties"`
}

type remoteSessionEnvelope struct {
	Info remoteSession `json:"info"`
}

type remoteSession struct {
	ID        string          `json:"id"`
	ProjectID string          `json:"projectID"`
	Directory string          `json:"directory"`
	Title     string          `json:"title"`
	Version   string          `json:"version"`
	Time      remoteTimeRange `json:"time"`
}

type remoteTimeRange struct {
	Created int64 `json:"created"`
	Updated int64 `json:"updated"`
}

type remoteSessionStatusEnvelope struct {
	SessionID string              `json:"sessionID"`
	Status    remoteSessionStatus `json:"status"`
}

type remoteSessionStatus struct {
	Type    string `json:"type"`
	Attempt int    `json:"attempt,omitempty"`
	Message string `json:"message,omitempty"`
	Next    int64  `json:"next,omitempty"`
}

type remoteSessionIDEnvelope struct {
	SessionID string `json:"sessionID"`
}

type remoteSessionErrorEnvelope struct {
	SessionID string               `json:"sessionID,omitempty"`
	Error     *remoteProviderError `json:"error,omitempty"`
}

type remoteProviderError struct {
	Name string         `json:"name"`
	Data map[string]any `json:"data"`
}

type remoteProviderInventory struct {
	All       []remoteProviderCatalog `json:"all"`
	Connected []string                `json:"connected"`
}

type remoteProviderCatalog struct {
	ID     string                        `json:"id"`
	Name   string                        `json:"name"`
	Models map[string]remoteModelCatalog `json:"models"`
}

type remoteModelCatalog struct {
	ID           string         `json:"id"`
	ProviderID   string         `json:"providerID"`
	Name         string         `json:"name"`
	Variants     map[string]any `json:"variants"`
	Capabilities map[string]any `json:"capabilities"`
}

type remotePermission struct {
	ID         string         `json:"id"`
	RequestID  string         `json:"requestID"`
	Type       string         `json:"type"`
	Pattern    any            `json:"pattern"`
	Patterns   []any          `json:"patterns"`
	Always     bool           `json:"always"`
	SessionID  string         `json:"sessionID"`
	MessageID  string         `json:"messageID"`
	CallID     string         `json:"callID"`
	Title      string         `json:"title"`
	Metadata   map[string]any `json:"metadata"`
	Permission map[string]any `json:"permission"`
	Tool       *struct {
		MessageID string `json:"messageID"`
		CallID    string `json:"callID"`
	} `json:"tool,omitempty"`
	Time struct {
		Created int64 `json:"created"`
	} `json:"time"`
}

type remotePermissionReply struct {
	SessionID    string `json:"sessionID"`
	PermissionID string `json:"permissionID"`
	RequestID    string `json:"requestID"`
	Response     string `json:"response"`
	Reply        string `json:"reply"`
}

type remoteQuestionRequest struct {
	ID        string               `json:"id"`
	SessionID string               `json:"sessionID"`
	Questions []remoteQuestionInfo `json:"questions"`
	Tool      *remoteQuestionTool  `json:"tool,omitempty"`
}

type remoteQuestionInfo struct {
	Question string                 `json:"question"`
	Header   string                 `json:"header"`
	Options  []remoteQuestionOption `json:"options"`
	Multiple bool                   `json:"multiple,omitempty"`
	Custom   *bool                  `json:"custom,omitempty"`
}

type remoteQuestionOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

type remoteQuestionTool struct {
	MessageID string `json:"messageID"`
	CallID    string `json:"callID"`
}

type remoteQuestionReply struct {
	SessionID string     `json:"sessionID"`
	RequestID string     `json:"requestID"`
	Answers   [][]string `json:"answers"`
}

type remoteQuestionReject struct {
	SessionID string `json:"sessionID"`
	RequestID string `json:"requestID"`
}

type remoteMessageEnvelope struct {
	Info remoteMessage `json:"info"`
}

type remoteMessageRecord struct {
	Info  remoteMessage `json:"info"`
	Parts []remotePart  `json:"parts"`
}

type remoteMessage struct {
	ID         string  `json:"id"`
	SessionID  string  `json:"sessionID"`
	Role       string  `json:"role"`
	ParentID   string  `json:"parentID"`
	Mode       string  `json:"mode,omitempty"`
	Agent      string  `json:"agent,omitempty"`
	ModelID    string  `json:"modelID"`
	ProviderID string  `json:"providerID"`
	Cost       float64 `json:"cost,omitempty"`
	Tokens     struct {
		Total     int64 `json:"total"`
		Input     int64 `json:"input"`
		Output    int64 `json:"output"`
		Reasoning int64 `json:"reasoning"`
		Cache     struct {
			Read  int64 `json:"read"`
			Write int64 `json:"write"`
		} `json:"cache"`
	} `json:"tokens,omitempty"`
	Error *remoteProviderError `json:"error,omitempty"`
	Time  struct {
		Created   int64 `json:"created"`
		Completed int64 `json:"completed,omitempty"`
	} `json:"time"`
}

type remotePartEnvelope struct {
	Part  remotePart `json:"part"`
	Delta string     `json:"delta"`
}

type remotePartDeltaEnvelope struct {
	SessionID string `json:"sessionID"`
	MessageID string `json:"messageID"`
	PartID    string `json:"partID"`
	Field     string `json:"field"`
	Delta     string `json:"delta"`
}

type remotePart struct {
	ID        string           `json:"id"`
	SessionID string           `json:"sessionID"`
	MessageID string           `json:"messageID"`
	Type      string           `json:"type"`
	Text      string           `json:"text"`
	CallID    string           `json:"callID"`
	Tool      string           `json:"tool"`
	State     *remoteToolState `json:"state,omitempty"`
	Metadata  map[string]any   `json:"metadata,omitempty"`
}

type remoteToolState struct {
	Status   string         `json:"status"`
	Input    map[string]any `json:"input,omitempty"`
	Raw      string         `json:"raw,omitempty"`
	Output   string         `json:"output,omitempty"`
	Error    string         `json:"error,omitempty"`
	Title    string         `json:"title,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
	Time     struct {
		Start int64 `json:"start"`
		End   int64 `json:"end,omitempty"`
	} `json:"time"`
}

func NewProvider(emit func(contract.RuntimeEvent)) *Provider {
	return &Provider{
		sessions:         make(map[string]*session),
		providerSessions: make(map[string]*session),
		emit:             emit,
		probe:            providerprobe.New(opencodeBinaryPath, "--version"),
	}
}

func (p *Provider) Runtime() string {
	return runtimeName
}

func (p *Provider) Describe() contract.RuntimeDescriptor {
	descriptor := contract.NewRuntimeDescriptor(
		runtimeName,
		contract.OwnershipControlled,
		contract.TransportHTTPServer,
		contract.RuntimeCapabilities{
			StartSession:             true,
			ResumeSession:            true,
			SendInput:                true,
			Interrupt:                true,
			Respond:                  true,
			StopSession:              true,
			ListSessions:             true,
			StreamEvents:             true,
			ApprovalRequests:         true,
			UserInputRequests:        true,
			ImmediateProviderSession: true,
			ResumeByProviderID:       true,
			AdoptExternalSessions:    true,
		},
	)
	descriptor.Probe = p.probe.Snapshot(context.Background())
	p.enrichProbeWithInventory(context.Background(), descriptor.Probe)
	return descriptor
}

func (p *Provider) enrichProbeWithInventory(ctx context.Context, probe *contract.RuntimeProbe) {
	if probe == nil {
		return
	}

	p.mu.RLock()
	server := p.server
	p.mu.RUnlock()
	var inventory remoteProviderInventory
	if server == nil {
		var ok bool
		inventory, ok = p.cachedInventory()
		if !ok {
			var err error
			inventory, err = probeStandaloneOpenCodeInventory(ctx)
			if err != nil {
				if probe.ModelSource == "" {
					probe.ModelSource = "dynamic_error"
				}
				if probe.Status == "ready" {
					probe.Status = "warning"
				}
				probe.Message = appendProbeMessage(probe.Message, fmt.Sprintf("OpenCode model inventory unavailable: %s", err))
				return
			}
			p.storeInventory(inventory)
		}
	} else {
		ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()

		if err := server.doJSON(ctx, http.MethodGet, "/provider", "", nil, &inventory); err != nil {
			if probe.ModelSource == "" {
				probe.ModelSource = "dynamic_error"
			}
			if probe.Status == "ready" {
				probe.Status = "warning"
			}
			probe.Message = appendProbeMessage(probe.Message, fmt.Sprintf("OpenCode model inventory unavailable: %s", err))
			return
		}
		p.storeInventory(inventory)
	}

	models := runtimeModelsFromOpenCodeInventory(inventory)
	probe.Models = models
	probe.ModelSource = "opencode_provider_endpoint"
	if len(inventory.Connected) > 0 {
		probe.Auth = contract.AuthProbe{
			Status:  "authenticated",
			Type:    "provider",
			Label:   strings.Join(inventory.Connected, ", "),
			Method:  "GET /provider",
			Message: fmt.Sprintf("%d connected OpenCode provider(s)", len(inventory.Connected)),
		}
		if probe.Status != "missing" {
			probe.Status = "ready"
		}
		probe.Message = fmt.Sprintf("Found %d connected OpenCode provider(s) and %d model(s).", len(inventory.Connected), len(models))
		return
	}

	probe.Auth = contract.AuthProbe{
		Status:  "unauthenticated",
		Type:    "provider",
		Method:  "GET /provider",
		Message: "OpenCode has no connected providers.",
	}
	if probe.Status == "ready" {
		probe.Status = "warning"
	}
	probe.Message = "OpenCode has no connected providers."
}

func (p *Provider) cachedInventory() (remoteProviderInventory, bool) {
	p.inventoryMu.Lock()
	defer p.inventoryMu.Unlock()
	if time.Now().After(p.inventoryExpires) {
		return remoteProviderInventory{}, false
	}
	return p.inventory, true
}

func (p *Provider) storeInventory(inventory remoteProviderInventory) {
	p.inventoryMu.Lock()
	defer p.inventoryMu.Unlock()
	p.inventory = inventory
	p.inventoryExpires = time.Now().Add(inventoryProbeTTL)
}

func probeStandaloneOpenCodeInventory(ctx context.Context) (remoteProviderInventory, error) {
	var inventory remoteProviderInventory
	port, err := reservePort()
	if err != nil {
		return inventory, err
	}

	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	command := exec.CommandContext(
		ctx,
		opencodeBinaryPath(),
		"serve",
		"--hostname",
		loopbackHost,
		"--port",
		strconv.Itoa(port),
		"--pure",
	)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return inventory, err
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		return inventory, err
	}
	if err := command.Start(); err != nil {
		return inventory, err
	}
	go discardReader(stdout)
	go discardReader(stderr)

	server := &serverProcess{
		ctx:     ctx,
		cancel:  cancel,
		cmd:     command,
		baseURL: fmt.Sprintf("http://%s:%d", loopbackHost, port),
		client:  &http.Client{Timeout: 3 * time.Second},
	}
	defer func() {
		cancel()
		_ = command.Wait()
	}()

	if err := server.waitForHealth(ctx); err != nil {
		return inventory, err
	}
	if err := server.doJSON(ctx, http.MethodGet, "/provider", "", nil, &inventory); err != nil {
		return inventory, err
	}
	return inventory, nil
}

func (p *Provider) StartSession(
	ctx context.Context,
	request api.StartSessionRequest,
) (*contract.RuntimeSession, error) {
	resolvedCWD, err := resolveCWD(request.CWD)
	if err != nil {
		return nil, err
	}
	if err := p.ensureServer(ctx, resolvedCWD); err != nil {
		return nil, err
	}

	body := map[string]any{}
	if title := metadataString(request.Metadata, "title"); title != "" {
		body["title"] = title
	}
	if parentID := metadataString(request.Metadata, "parent_session_id"); parentID != "" {
		body["parentID"] = parentID
	}

	var remote remoteSession
	if err := p.doJSON(ctx, http.MethodPost, "/session", resolvedCWD, body, &remote); err != nil {
		return nil, err
	}

	sess := newSession(p, request.SessionID, remote.ID, coalesce(remote.Directory, resolvedCWD), remote.Title, request.Model, remote.Time)
	p.storeSession(sess)

	event := p.newEvent(sess, "session.started", "session.create", "",
		"Started OpenCode server session",
		map[string]any{"status": string(contract.SessionIdle)},
	)
	p.emit(event)

	if request.Prompt != "" {
		if _, err := p.SendInput(ctx, api.SendInputRequest{
			SessionID: request.SessionID,
			Text:      request.Prompt,
			Metadata:  request.Metadata,
		}); err != nil {
			rollbackErr := p.doJSON(
				ctx,
				http.MethodDelete,
				fmt.Sprintf("/session/%s", url.PathEscape(remote.ID)),
				resolvedCWD,
				nil,
				nil,
			)
			p.deleteSession(request.SessionID)
			p.shutdownServerIfIdle()
			sess.clearPendingRequests()
			sess.clearActiveTurnID()
			sess.setInterruptRequested(false)
			sess.setLastError(err.Error())
			sess.setStatus(contract.SessionErrored)
			p.emit(p.newEvent(sess, "session.errored", "session.create", "",
				fmt.Sprintf("OpenCode session start failed: %s", err),
				map[string]any{
					"status":     string(contract.SessionErrored),
					"last_error": err.Error(),
				},
			))
			if rollbackErr != nil {
				return nil, fmt.Errorf("%w (rollback failed: %v)", err, rollbackErr)
			}
			return nil, err
		}
	}

	return sess.snapshot(), nil
}

func (p *Provider) ResumeSession(
	ctx context.Context,
	request api.ResumeSessionRequest,
) (*contract.RuntimeSession, error) {
	resolvedCWD, err := resolveCWD(request.CWD)
	if err != nil {
		return nil, err
	}
	if err := p.ensureServer(ctx, resolvedCWD); err != nil {
		return nil, err
	}
	if _, ok := p.getSessionByProviderID(request.ProviderSessionID); ok {
		return nil, errors.New("opencode session is already adopted")
	}

	var remote remoteSession
	path := fmt.Sprintf("/session/%s", url.PathEscape(request.ProviderSessionID))
	if err := p.doJSON(ctx, http.MethodGet, path, resolvedCWD, nil, &remote); err != nil {
		return nil, err
	}

	sess := newSession(
		p,
		request.SessionID,
		request.ProviderSessionID,
		coalesce(remote.Directory, resolvedCWD),
		remote.Title,
		request.Model,
		remote.Time,
	)
	p.storeSession(sess)
	status, err := p.syncSessionStatusFromRemote(ctx, sess)
	if err != nil {
		p.deleteSession(request.SessionID)
		p.shutdownServerIfIdle()
		return nil, fmt.Errorf(
			"cannot verify OpenCode session %q status for adoption: %w",
			request.ProviderSessionID,
			err,
		)
	}
	if status != contract.SessionIdle {
		p.deleteSession(request.SessionID)
		p.shutdownServerIfIdle()
		return nil, fmt.Errorf(
			"cannot adopt active OpenCode session %q with status %q",
			request.ProviderSessionID,
			status,
		)
	}

	event := p.newEvent(sess, "session.started", "session.resume", "",
		"Resumed OpenCode server session",
		map[string]any{"status": string(sess.statusSnapshot())},
	)
	p.emit(event)
	return sess.snapshot(), nil
}

func (p *Provider) SendInput(
	ctx context.Context,
	request api.SendInputRequest,
) (*contract.RuntimeEvent, error) {
	sess, err := p.getSession(request.SessionID)
	if err != nil {
		return nil, err
	}
	status := sess.statusSnapshot()
	if sess.activeTurnIDSnapshot() != "" && (status == contract.SessionRunning || status == contract.SessionWaitingApproval || status == contract.SessionWaitingUserInput) {
		return nil, errors.New("opencode session already has an active turn")
	}
	if sess.pendingRequestCount() > 0 {
		return nil, errors.New("opencode session has pending requests")
	}

	messageID := newIdentifier("msg")
	body := map[string]any{
		"messageID": messageID,
		"parts": []map[string]any{
			{
				"type": "text",
				"text": request.Text,
			},
		},
	}
	if model := requestModel(sess, request.Metadata); model != nil {
		body["model"] = model
	}
	if agent := opencodeAgentName(metadataString(request.Metadata, "agent")); agent != "" {
		body["agent"] = agent
	}
	if system := metadataString(request.Metadata, "system"); system != "" {
		body["system"] = system
	}
	if tools := metadataBoolMap(request.Metadata, "tools"); len(tools) > 0 {
		body["tools"] = tools
	}

	path := fmt.Sprintf("/session/%s/prompt_async", url.PathEscape(sess.providerSessionIDSnapshot()))
	if err := p.doJSON(ctx, http.MethodPost, path, sess.cwdSnapshot(), body, nil); err != nil {
		return nil, err
	}

	sess.setActiveTurnID(messageID)
	sess.storeMessageTurn(messageID, messageID)
	sess.setInterruptRequested(false)
	sess.setStatus(contract.SessionRunning)
	sess.setLastError("")
	sess.setAssistantSummary("")
	sess.setToolActivity(false)

	event := p.newEvent(sess, "turn.started", "session.prompt_async", messageID,
		fmt.Sprintf("Started OpenCode turn: %s", truncate(request.Text, 120)),
		map[string]any{"status": string(contract.SessionRunning)},
	)
	p.emit(event)
	return &event, nil
}

func (p *Provider) Interrupt(
	ctx context.Context,
	sessionID string,
) (*contract.RuntimeEvent, error) {
	sess, err := p.getSession(sessionID)
	if err != nil {
		return nil, err
	}
	status := sess.statusSnapshot()
	if status != contract.SessionRunning && status != contract.SessionWaitingApproval && status != contract.SessionWaitingUserInput {
		return nil, errors.New("opencode session has no active turn")
	}

	turnID := sess.activeTurnIDSnapshot()
	eventSession := *sess.snapshot()
	eventSession.Status = normalizeInterruptStatus(status)
	eventSession.ActiveTurnID = turnID
	sess.setInterruptRequested(true)
	path := fmt.Sprintf("/session/%s/abort", url.PathEscape(sess.providerSessionIDSnapshot()))
	if err := p.doJSON(ctx, http.MethodPost, path, sess.cwdSnapshot(), nil, nil); err != nil {
		sess.setInterruptRequested(false)
		return nil, err
	}

	status = normalizeInterruptStatus(status)
	event := contract.NewRuntimeEvent(eventSession, "turn.interrupt_requested", "session.abort", turnID,
		"Requested OpenCode turn interruption",
		map[string]any{
			"status":              string(status),
			"interrupt_requested": true,
		},
	)
	p.emit(event)
	p.resolvePendingRequests(sess, "session.abort", "interrupt_requested")
	return &event, nil
}

func (p *Provider) Respond(
	ctx context.Context,
	request api.RespondRequest,
) (*contract.RuntimeEvent, error) {
	sess, err := p.getSession(request.SessionID)
	if err != nil {
		return nil, err
	}

	pending, ok := sess.pendingRequest(request.RequestID)
	if !ok {
		return nil, errors.New("unknown pending request")
	}

	payload, path, responseSummary, err := responsePayload(sess, pending, request)
	if err != nil {
		return nil, err
	}
	if err := p.doJSON(ctx, http.MethodPost, path, sess.cwdSnapshot(), payload, nil); err != nil {
		if !isPermissionRequestKind(pending.Request.Kind) {
			return nil, err
		}
		fallbackPayload, fallbackPath, fallbackErr := legacyPermissionResponsePayload(sess, request, responseSummary)
		if fallbackErr != nil {
			return nil, err
		}
		if fallbackPostErr := p.doJSON(ctx, http.MethodPost, fallbackPath, sess.cwdSnapshot(), fallbackPayload, nil); fallbackPostErr != nil {
			return nil, err
		}
	}

	sess.removePendingRequest(request.RequestID)
	status := nextPendingStatus(sess)
	sess.setStatus(status)

	event := p.newEvent(sess, "request.responded", pending.Request.NativeMethod, pending.Request.TurnID,
		fmt.Sprintf("Responded to OpenCode request: %s", pending.Request.Summary),
		map[string]any{
			"status":   string(status),
			"response": responseSummary,
		},
	)
	event.RequestID = request.RequestID
	responded := pending.Request
	responded.Status = contract.RequestStatusResponded
	event.Request = &responded
	p.emit(event)
	return &event, nil
}

func (p *Provider) StopSession(
	ctx context.Context,
	sessionID string,
) (*contract.RuntimeEvent, error) {
	sess, err := p.getSession(sessionID)
	if err != nil {
		return nil, err
	}

	status := sess.statusSnapshot()
	if status == contract.SessionRunning || status == contract.SessionWaitingApproval || status == contract.SessionWaitingUserInput {
		path := fmt.Sprintf("/session/%s/abort", url.PathEscape(sess.providerSessionIDSnapshot()))
		_ = p.doJSON(ctx, http.MethodPost, path, sess.cwdSnapshot(), nil, nil)
	}

	p.deleteSession(sessionID)
	p.shutdownServerIfIdle()

	sess.clearActiveTurnID()
	sess.setInterruptRequested(false)
	sess.setStatus(contract.SessionStopped)
	p.resolvePendingRequests(sess, "session.stop", "stopped")
	event := p.newEvent(sess, "session.stopped", "session.stop", "",
		"Stopped OpenCode controller session",
		map[string]any{"status": string(contract.SessionStopped)},
	)
	p.emit(event)
	return &event, nil
}

func (p *Provider) ListSessions(ctx context.Context) ([]contract.RuntimeSession, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	sessions := make([]contract.RuntimeSession, 0, len(p.sessions))
	for _, sess := range p.sessions {
		sessions = append(sessions, *sess.snapshot())
	}
	return sessions, nil
}

func (p *Provider) ensureServer(ctx context.Context, cwd string) error {
	p.mu.RLock()
	if p.server != nil {
		p.mu.RUnlock()
		return nil
	}
	p.mu.RUnlock()

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.server != nil {
		return nil
	}

	server, err := startServer(ctx, cwd, p)
	if err != nil {
		return err
	}
	p.server = server
	return nil
}

func (p *Provider) doJSON(
	ctx context.Context,
	method string,
	path string,
	directory string,
	body any,
	target any,
) error {
	p.mu.RLock()
	server := p.server
	p.mu.RUnlock()
	if server == nil {
		return errors.New("opencode server is not running")
	}
	return server.doJSON(ctx, method, path, directory, body, target)
}

func (p *Provider) storeSession(sess *session) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sessions[sess.appSessionID] = sess
	p.providerSessions[sess.providerSessionID] = sess
}

func (p *Provider) getSession(sessionID string) (*session, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	sess, ok := p.sessions[sessionID]
	if !ok {
		return nil, errors.New("unknown session")
	}
	return sess, nil
}

func (p *Provider) getSessionByProviderID(providerSessionID string) (*session, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	sess, ok := p.providerSessions[providerSessionID]
	return sess, ok
}

func (p *Provider) deleteSession(sessionID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	sess, ok := p.sessions[sessionID]
	if !ok {
		return
	}
	delete(p.sessions, sessionID)
	delete(p.providerSessions, sess.providerSessionIDSnapshot())
}

func (p *Provider) deleteSessionByProviderID(providerSessionID string) *session {
	p.mu.Lock()
	defer p.mu.Unlock()
	sess, ok := p.providerSessions[providerSessionID]
	if !ok {
		return nil
	}
	delete(p.providerSessions, providerSessionID)
	delete(p.sessions, sess.appSessionID)
	return sess
}

func (p *Provider) shutdownServerIfIdle() {
	p.mu.Lock()
	if len(p.sessions) != 0 || p.server == nil {
		p.mu.Unlock()
		return
	}
	server := p.server
	p.server = nil
	p.mu.Unlock()
	server.stop()
}

func (p *Provider) serverExited(server *serverProcess, err error) {
	p.mu.Lock()
	if p.server == server {
		p.server = nil
	}
	sessions := make([]*session, 0, len(p.sessions))
	for _, sess := range p.sessions {
		sessions = append(sessions, sess)
	}
	p.sessions = make(map[string]*session)
	p.providerSessions = make(map[string]*session)
	p.mu.Unlock()

	if len(sessions) == 0 {
		return
	}

	status := contract.SessionStopped
	eventType := "session.stopped"
	summary := "OpenCode server stopped"
	payload := map[string]any{"status": string(contract.SessionStopped)}
	if err != nil && !errors.Is(err, context.Canceled) {
		status = contract.SessionErrored
		eventType = "session.errored"
		summary = fmt.Sprintf("OpenCode server exited: %s", err)
		payload["status"] = string(contract.SessionErrored)
		payload["last_error"] = err.Error()
	}

	for _, sess := range sessions {
		sess.clearActiveTurnID()
		sess.setInterruptRequested(false)
		sess.setStatus(status)
		if status == contract.SessionErrored {
			sess.setLastError(err.Error())
		}
		p.resolvePendingRequests(sess, "process/exit", eventType)
		p.emit(p.newEvent(sess, eventType, "process/exit", "", summary, payload))
	}
}

func (p *Provider) newEvent(
	sess *session,
	eventType string,
	nativeEventName string,
	turnID string,
	summary string,
	payload map[string]any,
) contract.RuntimeEvent {
	return contract.NewRuntimeEvent(*sess.snapshot(), eventType, nativeEventName, turnID, summary, payload)
}

func startServer(ctx context.Context, cwd string, provider *Provider) (*serverProcess, error) {
	port, err := reservePort()
	if err != nil {
		return nil, err
	}

	serverCtx, cancel := context.WithCancel(context.Background())
	command := exec.CommandContext(
		serverCtx,
		opencodeBinaryPath(),
		"serve",
		"--hostname",
		loopbackHost,
		"--port",
		strconv.Itoa(port),
		"--pure",
	)
	if cwd != "" {
		command.Dir = cwd
	}

	stdout, err := command.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		cancel()
		return nil, err
	}

	if err := command.Start(); err != nil {
		cancel()
		return nil, err
	}

	server := &serverProcess{
		ctx:          serverCtx,
		cancel:       cancel,
		cmd:          command,
		baseURL:      fmt.Sprintf("http://%s:%d", loopbackHost, port),
		client:       &http.Client{Timeout: 30 * time.Second},
		streamClient: &http.Client{},
		provider:     provider,
	}

	go discardReader(stdout)
	go discardReader(stderr)

	if err := server.waitForHealth(ctx); err != nil {
		server.stop()
		_ = command.Wait()
		return nil, err
	}

	go server.eventLoop()
	go server.waitLoop()
	return server, nil
}

func (s *serverProcess) stop() {
	s.cancel()
}

func (s *serverProcess) waitLoop() {
	err := s.cmd.Wait()
	s.cancel()
	s.provider.serverExited(s, err)
}

func (s *serverProcess) eventLoop() {
	backoff := 200 * time.Millisecond
	for {
		err := s.readEventsOnce()
		if s.ctx.Err() != nil {
			return
		}
		if err == nil {
			backoff = 200 * time.Millisecond
		} else if backoff < 2*time.Second {
			backoff *= 2
		}

		timer := time.NewTimer(backoff)
		select {
		case <-s.ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (s *serverProcess) waitForHealth(ctx context.Context) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.NewTimer(10 * time.Second)
	defer timeout.Stop()

	for {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url("/global/health", ""), nil)
		if err != nil {
			return err
		}
		response, err := s.client.Do(request)
		if err == nil {
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return nil
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.ctx.Done():
			return errors.New("opencode server stopped before becoming healthy")
		case <-timeout.C:
			return errors.New("timed out waiting for opencode server")
		case <-ticker.C:
		}
	}
}

func (s *serverProcess) doJSON(
	ctx context.Context,
	method string,
	path string,
	directory string,
	body any,
	target any,
) error {
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(encoded)
	}

	request, err := http.NewRequestWithContext(ctx, method, s.url(path, directory), payload)
	if err != nil {
		return err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := s.client.Do(request)
	if err != nil {
		return err
	}
	defer func() {
		_ = response.Body.Close()
	}()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		bodyBytes, _ := io.ReadAll(response.Body)
		message := strings.TrimSpace(string(bodyBytes))
		if message == "" {
			message = response.Status
		}
		return fmt.Errorf("opencode http %s %s failed: %s", method, path, message)
	}
	if target == nil || response.StatusCode == http.StatusNoContent {
		_, _ = io.Copy(io.Discard, response.Body)
		return nil
	}
	return json.NewDecoder(response.Body).Decode(target)
}

func (s *serverProcess) url(path string, directory string) string {
	parsed, _ := url.Parse(s.baseURL + path)
	if directory != "" {
		query := parsed.Query()
		query.Set("directory", directory)
		parsed.RawQuery = query.Encode()
	}
	return parsed.String()
}

func (s *serverProcess) readEventsOnce() error {
	request, err := http.NewRequestWithContext(s.ctx, http.MethodGet, s.url("/event", ""), nil)
	if err != nil {
		return err
	}

	response, err := s.streamClient.Do(request)
	if err != nil {
		return err
	}
	defer func() {
		_ = response.Body.Close()
	}()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = response.Status
		}
		return fmt.Errorf("opencode event stream failed: %s", message)
	}
	s.provider.resyncSessions(s.ctx)

	reader := bufio.NewReader(response.Body)
	dataLines := make([]string, 0, 4)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if s.ctx.Err() != nil || errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if len(dataLines) == 0 {
				continue
			}
			payload := strings.Join(dataLines, "\n")
			dataLines = dataLines[:0]

			var event remoteEvent
			if err := json.Unmarshal([]byte(payload), &event); err == nil {
				s.provider.handleEvent(event)
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
}

func (p *Provider) handleEvent(event remoteEvent) {
	switch event.Type {
	case "server.connected":
		return
	case "session.created":
		var props remoteSessionEnvelope
		if json.Unmarshal(event.Properties, &props) == nil {
			p.handleSessionCreated(props.Info)
		}
	case "session.updated":
		var props remoteSessionEnvelope
		if json.Unmarshal(event.Properties, &props) == nil {
			p.handleSessionUpdated(props.Info)
		}
	case "session.deleted":
		var props remoteSessionEnvelope
		if json.Unmarshal(event.Properties, &props) == nil {
			p.handleSessionDeleted(props.Info)
		}
	case "session.status":
		var props remoteSessionStatusEnvelope
		if json.Unmarshal(event.Properties, &props) == nil {
			p.handleSessionStatus(props)
		}
	case "session.idle":
		var props remoteSessionIDEnvelope
		if json.Unmarshal(event.Properties, &props) == nil {
			p.handleSessionIdle(props.SessionID)
		}
	case "session.error":
		var props remoteSessionErrorEnvelope
		if json.Unmarshal(event.Properties, &props) == nil {
			p.handleSessionError(props)
		}
	case "permission.updated":
		var props remotePermission
		if json.Unmarshal(event.Properties, &props) == nil {
			p.handlePermissionUpdated(props)
		}
	case "permission.replied":
		var props remotePermissionReply
		if json.Unmarshal(event.Properties, &props) == nil {
			p.handlePermissionReplied(props)
		}
	case "question.asked":
		var props remoteQuestionRequest
		if json.Unmarshal(event.Properties, &props) == nil {
			p.handleQuestionAsked(props)
		}
	case "question.replied":
		var props remoteQuestionReply
		if json.Unmarshal(event.Properties, &props) == nil {
			p.handleQuestionReplied(props)
		}
	case "question.rejected":
		var props remoteQuestionReject
		if json.Unmarshal(event.Properties, &props) == nil {
			p.handleQuestionRejected(props)
		}
	case "message.updated":
		var props remoteMessageEnvelope
		if json.Unmarshal(event.Properties, &props) == nil {
			p.handleMessageUpdated(props.Info)
		}
	case "message.part.updated":
		var props remotePartEnvelope
		if json.Unmarshal(event.Properties, &props) == nil {
			p.handleMessagePartUpdated(props)
		}
	case "message.part.delta":
		var props remotePartDeltaEnvelope
		if json.Unmarshal(event.Properties, &props) == nil {
			p.handleMessagePartDelta(props)
		}
	}
}

func (p *Provider) handleSessionCreated(remote remoteSession) {
	sess, ok := p.getSessionByProviderID(remote.ID)
	if !ok {
		return
	}
	if remote.Directory != "" {
		sess.setCWD(remote.Directory)
	}
	if remote.Title != "" {
		sess.setTitle(remote.Title)
	}
	if remote.Time.Created > 0 {
		sess.setCreatedAt(remote.Time.Created)
	}
	if remote.Time.Updated > 0 {
		sess.setUpdatedAt(remote.Time.Updated)
	}
}

func (p *Provider) handleSessionUpdated(remote remoteSession) {
	sess, ok := p.getSessionByProviderID(remote.ID)
	if !ok {
		return
	}
	if remote.Directory != "" {
		sess.setCWD(remote.Directory)
	}
	if remote.Title != "" {
		sess.setTitle(remote.Title)
	}
	if remote.Time.Updated > 0 {
		sess.setUpdatedAt(remote.Time.Updated)
	}
}

func (p *Provider) handleSessionDeleted(remote remoteSession) {
	sess := p.deleteSessionByProviderID(remote.ID)
	if sess == nil {
		return
	}
	sess.clearActiveTurnID()
	sess.setInterruptRequested(false)
	sess.setStatus(contract.SessionStopped)
	p.resolvePendingRequests(sess, "session.deleted", "deleted")
	p.emit(p.newEvent(sess, "session.stopped", "session.deleted", "",
		"OpenCode session ended",
		map[string]any{"status": string(contract.SessionStopped)},
	))
	p.shutdownServerIfIdle()
}

func (p *Provider) handleSessionStatus(update remoteSessionStatusEnvelope) {
	sess, ok := p.getSessionByProviderID(update.SessionID)
	if !ok {
		return
	}
	status := mapSessionStatus(update.Status, pendingSessionStatus(sess))
	if update.Status.Type == "idle" {
		sess.setStatus(contract.SessionIdle)
		return
	}
	sess.setStatus(status)
	p.emit(p.newEvent(sess, "session.status_changed", "session.status", sess.activeTurnIDSnapshot(),
		fmt.Sprintf("OpenCode session status: %s", update.Status.Type),
		map[string]any{
			"status":      string(status),
			"status_name": update.Status.Type,
			"attempt":     update.Status.Attempt,
			"message":     update.Status.Message,
			"next":        update.Status.Next,
		},
	))
}

func (p *Provider) handleSessionIdle(providerSessionID string) {
	sess, ok := p.getSessionByProviderID(providerSessionID)
	if !ok {
		return
	}
	interrupted := sess.consumeInterruptRequested()
	previous := sess.statusSnapshot()
	turnID := sess.activeTurnIDSnapshot()
	sess.clearActiveTurnID()
	sess.setLastError("")
	if turnID == "" {
		sess.setStatus(contract.SessionIdle)
		return
	}
	if interrupted {
		sess.setStatus(contract.SessionInterrupted)
		p.resolvePendingRequests(sess, "session.idle", "interrupted")
		p.emit(p.newEvent(sess, "turn.interrupted", "session.idle", turnID,
			"OpenCode turn interrupted",
			map[string]any{"status": string(contract.SessionInterrupted)},
		))
		sess.setStatus(contract.SessionIdle)
		return
	}
	sess.markTurnCompleted(turnID)
	sess.setStatus(contract.SessionIdle)
	p.resolvePendingRequests(sess, "session.idle", "completed")
	if previous == contract.SessionInterrupted || previous == contract.SessionErrored {
		return
	}
	summary := "OpenCode turn completed"
	if text := sess.assistantSummarySnapshot(); text != "" {
		summary = fmt.Sprintf("OpenCode turn completed: %s", text)
	}
	p.emit(p.newEvent(sess, "turn.completed", "session.idle", turnID, summary,
		map[string]any{"status": string(contract.SessionIdle)},
	))
}

func (p *Provider) handleSessionError(update remoteSessionErrorEnvelope) {
	if strings.TrimSpace(update.SessionID) == "" {
		return
	}
	sess, ok := p.getSessionByProviderID(update.SessionID)
	if !ok {
		return
	}

	message := providerErrorMessage(update.Error)
	interrupted := sess.consumeInterruptRequested()
	turnID := sess.activeTurnIDSnapshot()
	sess.clearActiveTurnID()

	status := contract.SessionErrored
	eventType := "turn.errored"
	summary := fmt.Sprintf("OpenCode turn failed: %s", message)
	if interrupted || (update.Error != nil && update.Error.Name == "MessageAbortedError") {
		status = contract.SessionInterrupted
		eventType = "turn.interrupted"
		summary = coalesce(message, "OpenCode turn interrupted")
	}

	sess.setStatus(status)
	p.resolvePendingRequests(sess, "session.error", eventType)
	if status == contract.SessionErrored {
		sess.setLastError(message)
	} else {
		sess.setLastError("")
	}
	p.emit(p.newEvent(sess, eventType, "session.error", turnID, summary,
		map[string]any{
			"status":     string(status),
			"last_error": message,
			"error":      update.Error,
		},
	))
}

func (p *Provider) handlePermissionUpdated(permission remotePermission) {
	sess, ok := p.getSessionByProviderID(permission.SessionID)
	if !ok {
		return
	}
	requestID := permission.requestID()
	if _, exists := sess.pendingRequest(requestID); exists {
		return
	}

	messageID := permission.messageID()
	callID := permission.callID()
	turnID := sess.canonicalTurnID(messageID)
	pending := contract.PendingRequest{
		SchemaVersion: contract.ControlPlaneSchemaVersion,
		RequestID:     requestID,
		SessionID:     sess.appSessionID,
		Runtime:       runtimeName,
		Kind:          requestKindFromPermission(permission.Type),
		NativeMethod:  "permission.updated",
		Status:        contract.RequestStatusOpen,
		Summary:       coalesce(permission.Title, fmt.Sprintf("%s permission", permission.Type)),
		TurnID:        turnID,
		CreatedAtMS:   permission.Time.Created,
		Tool:          requestToolFromPermission(permission),
		Options:       requestOptionsForPermission(),
		Extensions: map[string]any{
			"permission_type": permission.Type,
			"pattern":         permission.patterns(),
			"always":          permission.Always,
			"metadata":        permission.Metadata,
			"message_id":      messageID,
			"call_id":         callID,
		},
	}

	sess.storePendingRequest(pendingRequest{Request: pending})
	sess.setStatus(statusForRequestKind(pending.Kind))

	event := p.newEvent(sess, "request.opened", pending.NativeMethod, pending.TurnID,
		fmt.Sprintf("OpenCode requested approval: %s", pending.Summary),
		map[string]any{
			"status":          string(sess.statusSnapshot()),
			"permission_type": permission.Type,
			"pattern":         permission.patterns(),
			"metadata":        permission.Metadata,
		},
	)
	event.RequestID = pending.RequestID
	event.Request = &pending
	p.emit(event)
}

func (p *Provider) handlePermissionReplied(reply remotePermissionReply) {
	sess, ok := p.getSessionByProviderID(reply.SessionID)
	if !ok {
		return
	}
	requestID := coalesce(reply.RequestID, reply.PermissionID)
	response := coalesce(reply.Reply, reply.Response)
	pending, exists := sess.pendingRequest(requestID)
	if !exists {
		return
	}

	sess.removePendingRequest(requestID)
	status := nextPendingStatus(sess)
	sess.setStatus(status)

	event := p.newEvent(sess, "request.responded", pending.Request.NativeMethod, pending.Request.TurnID,
		fmt.Sprintf("OpenCode permission answered: %s", response),
		map[string]any{
			"status":   string(status),
			"response": response,
		},
	)
	event.RequestID = requestID
	responded := pending.Request
	responded.Status = contract.RequestStatusResponded
	event.Request = &responded
	p.emit(event)
}

func (p *Provider) handleQuestionAsked(question remoteQuestionRequest) {
	sess, ok := p.getSessionByProviderID(question.SessionID)
	if !ok {
		return
	}
	if _, exists := sess.pendingRequest(question.ID); exists {
		return
	}

	turnID := sess.activeTurnIDSnapshot()
	if question.Tool != nil {
		turnID = sess.canonicalTurnID(question.Tool.MessageID)
	}
	pending := contract.PendingRequest{
		SchemaVersion: contract.ControlPlaneSchemaVersion,
		RequestID:     question.ID,
		SessionID:     sess.appSessionID,
		Runtime:       runtimeName,
		Kind:          contract.RequestUserInputTool,
		NativeMethod:  "question.asked",
		Status:        contract.RequestStatusOpen,
		Summary:       questionSummary(question),
		TurnID:        turnID,
		CreatedAtMS:   time.Now().UnixMilli(),
		Tool:          requestToolFromQuestion(question),
		Questions:     requestQuestionsFromQuestion(question),
		Extensions: map[string]any{
			"questions": question.Questions,
			"tool":      question.Tool,
		},
	}

	sess.storePendingRequest(pendingRequest{Request: pending})
	sess.setStatus(contract.SessionWaitingUserInput)

	event := p.newEvent(sess, "request.opened", pending.NativeMethod, pending.TurnID,
		fmt.Sprintf("OpenCode requested user input: %s", pending.Summary),
		map[string]any{
			"status":    string(contract.SessionWaitingUserInput),
			"questions": question.Questions,
			"tool":      question.Tool,
		},
	)
	event.RequestID = pending.RequestID
	event.Request = &pending
	p.emit(event)
}

func (p *Provider) handleQuestionReplied(reply remoteQuestionReply) {
	sess, ok := p.getSessionByProviderID(reply.SessionID)
	if !ok {
		return
	}
	pending, exists := sess.pendingRequest(reply.RequestID)
	if !exists {
		return
	}

	sess.removePendingRequest(reply.RequestID)
	status := nextPendingStatus(sess)
	sess.setStatus(status)

	event := p.newEvent(sess, "request.responded", pending.Request.NativeMethod, pending.Request.TurnID,
		"OpenCode question answered",
		map[string]any{
			"status":   string(status),
			"response": reply.Answers,
		},
	)
	event.RequestID = reply.RequestID
	responded := pending.Request
	responded.Status = contract.RequestStatusResponded
	event.Request = &responded
	p.emit(event)
}

func (p *Provider) handleQuestionRejected(reject remoteQuestionReject) {
	sess, ok := p.getSessionByProviderID(reject.SessionID)
	if !ok {
		return
	}
	pending, exists := sess.pendingRequest(reject.RequestID)
	if !exists {
		return
	}

	sess.removePendingRequest(reject.RequestID)
	status := nextPendingStatus(sess)
	sess.setStatus(status)

	event := p.newEvent(sess, "request.responded", pending.Request.NativeMethod, pending.Request.TurnID,
		"OpenCode question rejected",
		map[string]any{
			"status":   string(status),
			"response": "reject",
		},
	)
	event.RequestID = reject.RequestID
	responded := pending.Request
	responded.Status = contract.RequestStatusResponded
	event.Request = &responded
	p.emit(event)
}

func (p *Provider) handleMessageUpdated(message remoteMessage) {
	sess, ok := p.getSessionByProviderID(message.SessionID)
	if !ok {
		return
	}
	if message.Role == "assistant" {
		if message.ProviderID != "" && message.ModelID != "" {
			sess.setModel(message.ProviderID + "/" + message.ModelID)
		}
		turnID := strings.TrimSpace(message.ParentID)
		if turnID == "" {
			turnID = sess.activeTurnIDSnapshot()
		}
		if turnID != "" && sess.turnCompleted(turnID) {
			return
		}
		if turnID != "" {
			sess.storeMessageTurn(message.ID, turnID)
		}
		if sess.activeTurnIDSnapshot() == "" && turnID != "" {
			sess.setActiveTurnID(turnID)
		}
		if usagePayload := openCodeUsagePayloadFromMessage(message); usagePayload != nil {
			p.emit(p.newEvent(sess, contract.EventThreadTokenUsageUpdated, "message.updated", turnID,
				"OpenCode token usage updated",
				usagePayload,
			))
		}
		if message.Time.Completed > 0 {
			p.completeTurnFromAssistantMessage(sess, turnID)
		}
	}
}

func (p *Provider) handleMessagePartDelta(update remotePartDeltaEnvelope) {
	if update.Field != "text" || strings.TrimSpace(update.Delta) == "" {
		return
	}
	sess, ok := p.getSessionByProviderID(update.SessionID)
	if !ok {
		return
	}
	if sess.isActivePromptMessage(update.MessageID) {
		return
	}
	turnID := sess.canonicalTurnID(update.MessageID)
	sess.appendAssistantSummary(update.Delta)
	p.emit(p.newEvent(sess, "assistant.message.delta", "message.part.delta", turnID,
		truncate(update.Delta, 160),
		map[string]any{
			"delta":      update.Delta,
			"message_id": update.MessageID,
			"part_id":    update.PartID,
		},
	))
}

func (p *Provider) handleMessagePartUpdated(update remotePartEnvelope) {
	sess, ok := p.getSessionByProviderID(update.Part.SessionID)
	if !ok {
		return
	}
	if sess.isActivePromptMessage(update.Part.MessageID) {
		return
	}
	turnID := sess.canonicalTurnID(update.Part.MessageID)

	switch update.Part.Type {
	case "text":
		text := strings.TrimSpace(coalesce(update.Delta, update.Part.Text))
		if text == "" {
			return
		}
		sess.setAssistantSummary(text)
		p.emit(p.newEvent(sess, "assistant.message.updated", "message.part.updated", turnID,
			truncate(text, 160),
			map[string]any{
				"text": text,
				"part": update.Part,
			},
		))
	case "tool":
		p.handleToolPartUpdated(sess, turnID, update.Part)
	}
}

func (p *Provider) completeTurnFromAssistantMessage(sess *session, turnID string) {
	if turnID == "" {
		turnID = sess.activeTurnIDSnapshot()
	}
	if turnID == "" || sess.statusSnapshot() == contract.SessionIdle {
		return
	}
	if sess.toolActivitySnapshot() {
		generation := sess.turnGenerationSnapshot()
		time.AfterFunc(toolCompletionDebounceDelay, func() {
			p.completeTurnFromAssistantMessageIfCurrent(sess, turnID, generation)
		})
		return
	}
	p.completeTurnFromAssistantMessageNow(sess, turnID)
}

func (p *Provider) completeTurnFromAssistantMessageIfCurrent(sess *session, turnID string, generation int64) {
	if sess.activeTurnIDSnapshot() != turnID || sess.turnGenerationSnapshot() != generation || sess.turnCompleted(turnID) {
		return
	}
	p.completeTurnFromAssistantMessageNow(sess, turnID)
}

func (p *Provider) completeTurnFromAssistantMessageNow(sess *session, turnID string) {
	sess.clearActiveTurnID()
	sess.markTurnCompleted(turnID)
	sess.setLastError("")
	sess.setStatus(contract.SessionIdle)
	sess.clearPendingRequests()
	summary := "OpenCode turn completed"
	payload := map[string]any{"status": string(contract.SessionIdle)}
	if text := sess.assistantSummarySnapshot(); text != "" {
		summary = fmt.Sprintf("OpenCode turn completed: %s", text)
		payload["final_text"] = text
	}
	p.emit(p.newEvent(sess, "turn.completed", "message.updated", turnID, summary,
		payload,
	))
}

func (p *Provider) handleToolPartUpdated(sess *session, turnID string, part remotePart) {
	if part.State == nil {
		return
	}
	sess.setToolActivity(true)
	previous, changed := sess.updateToolState(part.ID, part.State.Status)
	if !changed {
		return
	}

	payload := map[string]any{
		"status":    string(sess.statusSnapshot()),
		"tool_name": coalesce(part.Tool, valueAsString(part.Metadata["name"])),
		"call_id":   part.CallID,
		"input":     nilIfEmptyMap(part.State.Input),
		"metadata":  nilIfEmptyMap(part.State.Metadata),
		"previous":  previous,
	}
	if title := coalesce(part.State.Title, valueAsString(part.Metadata["title"])); title != "" {
		payload["title"] = title
	}
	if part.State.Output != "" {
		payload["output"] = part.State.Output
	}
	if part.State.Error != "" {
		payload["error"] = part.State.Error
	}

	summary := ""
	eventType := "runtime.event"
	switch part.State.Status {
	case "pending":
		eventType = "tool_call.opened"
		summary = fmt.Sprintf("OpenCode tool queued: %s", coalesce(part.State.Title, part.Tool, part.CallID))
	case "running":
		eventType = "tool.started"
		summary = fmt.Sprintf("OpenCode tool started: %s", coalesce(part.State.Title, part.Tool, part.CallID))
	case "completed":
		eventType = "tool.finished"
		summary = fmt.Sprintf("OpenCode tool finished: %s", coalesce(part.State.Title, part.Tool, part.CallID))
	case "error":
		eventType = "tool.failed"
		summary = fmt.Sprintf("OpenCode tool failed: %s", coalesce(part.State.Title, part.Tool, part.CallID))
	default:
		summary = fmt.Sprintf("OpenCode tool update: %s", coalesce(part.Tool, part.CallID))
	}

	p.emit(p.newEvent(sess, eventType, "message.part.updated", turnID, summary, payload))
}

func (p *Provider) resolvePendingRequests(
	sess *session,
	nativeEventName string,
	reason string,
) {
	requests := sess.drainPendingRequests()
	if len(requests) == 0 {
		return
	}
	for _, pending := range requests {
		resolved := pending.Request
		resolved.Status = contract.RequestStatusResolved
		event := p.newEvent(sess, "request.resolved", nativeEventName, pending.Request.TurnID,
			fmt.Sprintf("OpenCode request resolved: %s", pending.Request.Summary),
			map[string]any{
				"status": string(sess.statusSnapshot()),
				"reason": reason,
			},
		)
		event.RequestID = pending.Request.RequestID
		event.Request = &resolved
		p.emit(event)
	}
}

func (p *Provider) resyncSessions(ctx context.Context) {
	if err := p.applyResync(ctx); err != nil {
		p.scheduleResyncRetry(ctx)
	}
}

func (p *Provider) applyResync(ctx context.Context) error {
	sessions := p.sessionsSnapshot()
	if len(sessions) == 0 {
		return nil
	}

	statuses, err := p.fetchRemoteSessionStatuses(ctx)
	if err != nil {
		return err
	}

	for _, sess := range sessions {
		providerSessionID := sess.providerSessionIDSnapshot()
		status, ok := statuses[providerSessionID]
		if !ok {
			if p.remoteSessionExists(ctx, sess) {
				continue
			}
			p.handleMissingSessionAfterReconnect(sess)
			continue
		}
		p.reconcileSessionAfterReconnect(ctx, sess, status)
	}
	return nil
}

func (p *Provider) scheduleResyncRetry(ctx context.Context) {
	p.resyncMu.Lock()
	if p.resyncRunning {
		p.resyncMu.Unlock()
		return
	}
	p.resyncRunning = true
	p.resyncMu.Unlock()

	go func() {
		defer func() {
			p.resyncMu.Lock()
			p.resyncRunning = false
			p.resyncMu.Unlock()
		}()

		for _, delay := range []time.Duration{250 * time.Millisecond, 500 * time.Millisecond, time.Second} {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}

			if err := p.applyResync(ctx); err == nil {
				return
			}
		}
	}()
}

func (p *Provider) sessionsSnapshot() []*session {
	p.mu.RLock()
	defer p.mu.RUnlock()

	sessions := make([]*session, 0, len(p.sessions))
	for _, sess := range p.sessions {
		sessions = append(sessions, sess)
	}
	return sessions
}

func (p *Provider) fetchRemoteSessionStatuses(ctx context.Context) (map[string]remoteSessionStatus, error) {
	statuses := make(map[string]remoteSessionStatus)
	if err := p.doJSON(ctx, http.MethodGet, "/session/status", "", nil, &statuses); err != nil {
		return nil, err
	}
	return statuses, nil
}

func (p *Provider) remoteSessionExists(ctx context.Context, sess *session) bool {
	var remote remoteSession
	path := fmt.Sprintf("/session/%s", url.PathEscape(sess.providerSessionIDSnapshot()))
	return p.doJSON(ctx, http.MethodGet, path, sess.cwdSnapshot(), nil, &remote) == nil
}

func (p *Provider) syncSessionStatusFromRemote(ctx context.Context, sess *session) (contract.SessionStatus, error) {
	statuses, err := p.fetchRemoteSessionStatuses(ctx)
	if err != nil {
		return "", err
	}
	status, ok := statuses[sess.providerSessionIDSnapshot()]
	if !ok {
		return "", errors.New("remote session status unavailable")
	}
	mapped := mapSessionStatus(status, pendingSessionStatus(sess))
	sess.setStatus(mapped)
	return mapped, nil
}

func (p *Provider) handleMissingSessionAfterReconnect(sess *session) {
	providerSessionID := sess.providerSessionIDSnapshot()
	removed := p.deleteSessionByProviderID(providerSessionID)
	if removed == nil {
		return
	}
	removed.clearActiveTurnID()
	removed.setInterruptRequested(false)
	removed.setStatus(contract.SessionStopped)
	p.resolvePendingRequests(removed, "session.status.recovered", "stopped")
	p.emit(p.newEvent(removed, "session.stopped", "session.status.recovered", "",
		"Recovered OpenCode session stop after event stream reconnect",
		map[string]any{
			"status":    string(contract.SessionStopped),
			"recovered": true,
		},
	))
	p.shutdownServerIfIdle()
}

func (p *Provider) fetchRecoveredTurnRecord(
	ctx context.Context,
	sess *session,
	turnID string,
) (*remoteMessageRecord, error) {
	var records []remoteMessageRecord
	path := fmt.Sprintf(
		"/session/%s/message?limit=5",
		url.PathEscape(sess.providerSessionIDSnapshot()),
	)
	if err := p.doJSON(ctx, http.MethodGet, path, sess.cwdSnapshot(), nil, &records); err != nil {
		return nil, err
	}
	for _, record := range records {
		if record.Info.Role == "assistant" && record.Info.ParentID == turnID {
			return &record, nil
		}
	}
	return nil, nil
}

func (p *Provider) reconcileSessionAfterReconnect(
	ctx context.Context,
	sess *session,
	remoteStatus remoteSessionStatus,
) {
	turnID := sess.activeTurnIDSnapshot()
	if turnID == "" {
		sess.setStatus(mapSessionStatus(remoteStatus, pendingSessionStatus(sess)))
		return
	}

	if remoteStatus.Type != "idle" {
		status := mapSessionStatus(remoteStatus, pendingSessionStatus(sess))
		sess.setStatus(status)
		if remoteStatus.Type == "busy" && pendingSessionStatus(sess) == "" {
			p.emit(p.newEvent(sess, "runtime.event", "session.status.recovered", turnID,
				"OpenCode reconnect restored a busy turn without recoverable in-flight detail",
				map[string]any{
					"status":    string(status),
					"recovered": false,
				},
			))
		}
		return
	}

	record, err := p.fetchRecoveredTurnRecord(ctx, sess, turnID)
	if err != nil {
		interrupted := sess.consumeInterruptRequested()
		sess.clearActiveTurnID()
		sess.setLastError("")
		sess.setStatus(contract.SessionIdle)
		p.resolvePendingRequests(sess, "session.status.recovered", "recovery_incomplete")
		p.emit(p.newEvent(sess, "runtime.event", "session.status.recovered", turnID,
			"OpenCode reconnect could not recover the terminal turn result",
			map[string]any{
				"status":              string(contract.SessionIdle),
				"recovered":           false,
				"recovery_error":      err.Error(),
				"interrupt_requested": interrupted,
			},
		))
		return
	}
	if record != nil {
		if text := assistantSummaryFromParts(record.Parts); text != "" {
			sess.setAssistantSummary(text)
		}
	}

	interrupted := sess.consumeInterruptRequested()
	sess.clearActiveTurnID()
	if record == nil {
		sess.setLastError("")
		if interrupted {
			sess.setStatus(contract.SessionInterrupted)
			p.resolvePendingRequests(sess, "session.status.recovered", "interrupted")
			p.emit(p.newEvent(sess, "turn.interrupted", "session.status.recovered", turnID,
				"Recovered interrupted OpenCode turn after event stream reconnect",
				map[string]any{
					"status":    string(contract.SessionInterrupted),
					"recovered": true,
				},
			))
			sess.setStatus(contract.SessionIdle)
			return
		}
		sess.setStatus(contract.SessionIdle)
		p.resolvePendingRequests(sess, "session.status.recovered", "recovery_incomplete")
		p.emit(p.newEvent(sess, "runtime.event", "session.status.recovered", turnID,
			"OpenCode reconnect could not recover the terminal turn result",
			map[string]any{
				"status":    string(contract.SessionIdle),
				"recovered": false,
			},
		))
		return
	}
	if record != nil && record.Info.Error != nil {
		message := providerErrorMessage(record.Info.Error)
		status := contract.SessionErrored
		eventType := "turn.errored"
		summary := fmt.Sprintf(
			"Recovered failed OpenCode turn after event stream reconnect: %s",
			message,
		)
		if interrupted || record.Info.Error.Name == "MessageAbortedError" {
			status = contract.SessionInterrupted
			eventType = "turn.interrupted"
			summary = fmt.Sprintf(
				"Recovered interrupted OpenCode turn after event stream reconnect: %s",
				message,
			)
		}
		if status == contract.SessionErrored {
			sess.setLastError(message)
		} else {
			sess.setLastError("")
		}
		sess.setStatus(status)
		p.resolvePendingRequests(sess, "session.status.recovered", eventType)
		p.emit(p.newEvent(sess, eventType, "session.status.recovered", turnID, summary,
			map[string]any{
				"status":     string(status),
				"recovered":  true,
				"last_error": message,
				"error":      record.Info.Error,
			},
		))
		sess.setStatus(contract.SessionIdle)
		return
	}

	sess.setLastError("")
	if interrupted {
		sess.setStatus(contract.SessionInterrupted)
		p.resolvePendingRequests(sess, "session.status.recovered", "interrupted")
		p.emit(p.newEvent(sess, "turn.interrupted", "session.status.recovered", turnID,
			"Recovered interrupted OpenCode turn after event stream reconnect",
			map[string]any{
				"status":    string(contract.SessionInterrupted),
				"recovered": true,
			},
		))
		sess.setStatus(contract.SessionIdle)
		return
	}

	sess.markTurnCompleted(turnID)
	sess.setStatus(contract.SessionIdle)
	p.resolvePendingRequests(sess, "session.status.recovered", "completed")
	summary := "Recovered OpenCode turn after event stream reconnect"
	payload := map[string]any{
		"status":    string(contract.SessionIdle),
		"recovered": true,
	}
	if text := sess.assistantSummarySnapshot(); text != "" {
		summary = fmt.Sprintf("Recovered OpenCode turn after event stream reconnect: %s", text)
		payload["final_text"] = text
	}
	p.emit(p.newEvent(sess, "turn.completed", "session.status.recovered", turnID, summary,
		payload,
	))
}

func assistantSummaryFromParts(parts []remotePart) string {
	for _, part := range parts {
		if part.Type != "text" {
			continue
		}
		if text := strings.TrimSpace(coalesce(part.Text)); text != "" {
			return text
		}
	}
	return ""
}

func newSession(
	provider *Provider,
	appSessionID string,
	providerSessionID string,
	cwd string,
	title string,
	model string,
	times remoteTimeRange,
) *session {
	now := time.Now().UnixMilli()
	createdAt := times.Created
	if createdAt == 0 {
		createdAt = now
	}
	updatedAt := times.Updated
	if updatedAt == 0 {
		updatedAt = createdAt
	}
	return &session{
		appSessionID:      appSessionID,
		providerSessionID: providerSessionID,
		status:            contract.SessionIdle,
		cwd:               cwd,
		model:             model,
		title:             title,
		createdAtMS:       createdAt,
		updatedAtMS:       updatedAt,
		messageTurns:      make(map[string]string),
		completedTurns:    make(map[string]struct{}),
		pendingRequests:   make(map[string]pendingRequest),
		toolStates:        make(map[string]string),
		provider:          provider,
	}
}

func (s *session) snapshot() *contract.RuntimeSession {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return &contract.RuntimeSession{
		SchemaVersion:     contract.ControlPlaneSchemaVersion,
		SessionID:         s.appSessionID,
		Runtime:           runtimeName,
		Ownership:         contract.OwnershipControlled,
		Transport:         contract.TransportHTTPServer,
		Status:            s.status,
		ProviderSessionID: s.providerSessionID,
		ActiveTurnID:      s.activeTurnID,
		CWD:               s.cwd,
		Model:             s.model,
		Title:             s.title,
		CreatedAtMS:       s.createdAtMS,
		UpdatedAtMS:       s.updatedAtMS,
		LastActivityAtMS:  s.updatedAtMS,
		LastError:         s.lastError,
	}
}

func (s *session) providerSessionIDSnapshot() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.providerSessionID
}

func (s *session) cwdSnapshot() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cwd
}

func (s *session) statusSnapshot() contract.SessionStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

func (s *session) activeTurnIDSnapshot() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.activeTurnID
}

func (s *session) assistantSummarySnapshot() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastAssistantSummary
}

func (s *session) pendingRequestCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.pendingRequests)
}

func (s *session) setStatus(status contract.SessionStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = status
	s.updatedAtMS = time.Now().UnixMilli()
}

func (s *session) setInterruptRequested(value bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.interruptRequested = value
	s.updatedAtMS = time.Now().UnixMilli()
}

func (s *session) consumeInterruptRequested() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	value := s.interruptRequested
	s.interruptRequested = false
	if value {
		s.updatedAtMS = time.Now().UnixMilli()
	}
	return value
}

func (s *session) setCWD(value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cwd = value
	s.updatedAtMS = time.Now().UnixMilli()
}

func (s *session) setTitle(value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.title = value
	s.updatedAtMS = time.Now().UnixMilli()
}

func (s *session) setModel(value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.model = value
	s.updatedAtMS = time.Now().UnixMilli()
}

func (s *session) setCreatedAt(value int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.createdAtMS = value
	s.updatedAtMS = maxInt64(s.updatedAtMS, value)
}

func (s *session) setUpdatedAt(value int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updatedAtMS = value
}

func (s *session) setLastError(value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastError = value
	s.updatedAtMS = time.Now().UnixMilli()
}

func (s *session) setActiveTurnID(value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activeTurnID = value
	if strings.TrimSpace(value) != "" {
		s.turnGeneration++
	}
	s.updatedAtMS = time.Now().UnixMilli()
}

func (s *session) clearActiveTurnID() {
	s.setActiveTurnID("")
}

func (s *session) storeMessageTurn(messageID string, turnID string) {
	messageID = strings.TrimSpace(messageID)
	turnID = strings.TrimSpace(turnID)
	if messageID == "" || turnID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messageTurns[messageID] = turnID
	s.updatedAtMS = time.Now().UnixMilli()
}

func (s *session) canonicalTurnID(messageID string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if turnID := strings.TrimSpace(s.activeTurnID); turnID != "" {
		return turnID
	}
	if turnID := strings.TrimSpace(s.messageTurns[strings.TrimSpace(messageID)]); turnID != "" {
		return turnID
	}
	return strings.TrimSpace(messageID)
}

func (s *session) isActivePromptMessage(messageID string) bool {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return strings.TrimSpace(s.activeTurnID) == messageID
}

func (s *session) markTurnCompleted(turnID string) {
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.completedTurns[turnID] = struct{}{}
	s.updatedAtMS = time.Now().UnixMilli()
}

func (s *session) turnCompleted(turnID string) bool {
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.completedTurns[turnID]
	return ok
}

func (s *session) setAssistantSummary(value string) {
	trimmed := strings.TrimSpace(value)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastAssistantSummary = truncate(trimmed, 160)
	s.updatedAtMS = time.Now().UnixMilli()
}

func (s *session) setToolActivity(value bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.toolActivity = value
	s.updatedAtMS = time.Now().UnixMilli()
}

func (s *session) toolActivitySnapshot() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.toolActivity
}

func (s *session) turnGenerationSnapshot() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.turnGeneration
}

func (s *session) appendAssistantSummary(value string) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastAssistantSummary = truncate(s.lastAssistantSummary+trimmed, 160)
	s.updatedAtMS = time.Now().UnixMilli()
}

func (s *session) storePendingRequest(request pendingRequest) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingRequests[request.Request.RequestID] = request
	s.updatedAtMS = time.Now().UnixMilli()
}

func (s *session) pendingRequest(requestID string) (pendingRequest, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	request, ok := s.pendingRequests[requestID]
	return request, ok
}

func (s *session) removePendingRequest(requestID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.pendingRequests, requestID)
	s.updatedAtMS = time.Now().UnixMilli()
}

func (s *session) clearPendingRequests() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.pendingRequests) == 0 {
		return
	}
	s.pendingRequests = make(map[string]pendingRequest)
	s.updatedAtMS = time.Now().UnixMilli()
}

func (s *session) drainPendingRequests() []pendingRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.pendingRequests) == 0 {
		return nil
	}
	requests := make([]pendingRequest, 0, len(s.pendingRequests))
	for _, request := range s.pendingRequests {
		requests = append(requests, request)
	}
	s.pendingRequests = make(map[string]pendingRequest)
	s.updatedAtMS = time.Now().UnixMilli()
	return requests
}

func (s *session) updateToolState(partID string, state string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := s.toolStates[partID]
	if previous == state {
		return previous, false
	}
	s.toolStates[partID] = state
	s.updatedAtMS = time.Now().UnixMilli()
	return previous, true
}

func requestModel(sess *session, metadata map[string]any) map[string]string {
	model := metadataString(metadata, "model")
	if model == "" {
		model = sess.snapshot().Model
	}
	parts := strings.SplitN(strings.TrimSpace(model), "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil
	}
	return map[string]string{
		"providerID": parts[0],
		"modelID":    parts[1],
	}
}

func requestKindFromPermission(permissionType string) contract.RequestKind {
	switch strings.ToLower(strings.TrimSpace(permissionType)) {
	case "bash":
		return contract.RequestApprovalCommand
	case "edit":
		return contract.RequestApprovalFileChange
	case "external_directory", "doom_loop":
		return contract.RequestApprovalPermissions
	default:
		return contract.RequestApprovalTool
	}
}

func statusForRequestKind(kind contract.RequestKind) contract.SessionStatus {
	if strings.HasPrefix(string(kind), "user_input") {
		return contract.SessionWaitingUserInput
	}
	return contract.SessionWaitingApproval
}

func pendingSessionStatus(sess *session) contract.SessionStatus {
	sess.mu.RLock()
	defer sess.mu.RUnlock()
	hasApproval := false
	for _, pending := range sess.pendingRequests {
		if strings.HasPrefix(string(pending.Request.Kind), "user_input") {
			return contract.SessionWaitingUserInput
		}
		if strings.HasPrefix(string(pending.Request.Kind), "approval") {
			hasApproval = true
		}
	}
	if hasApproval {
		return contract.SessionWaitingApproval
	}
	return ""
}

func nextPendingStatus(sess *session) contract.SessionStatus {
	if status := pendingSessionStatus(sess); status != "" {
		return status
	}
	return contract.SessionRunning
}

func requestToolFromPermission(permission remotePermission) *contract.RequestToolContext {
	patterns := permission.patterns()
	command := ""
	if len(patterns) > 0 {
		command = patterns[0]
	}
	if value := valueAsString(permission.Metadata["command"]); value != "" {
		command = value
	}
	return &contract.RequestToolContext{
		Name:        permission.Type,
		Title:       permission.Title,
		Kind:        permission.Type,
		Command:     command,
		Description: valueAsString(permission.Metadata["description"]),
	}
}

func (permission remotePermission) requestID() string {
	return coalesce(permission.RequestID, permission.ID)
}

func (permission remotePermission) messageID() string {
	if permission.Tool != nil && permission.Tool.MessageID != "" {
		return permission.Tool.MessageID
	}
	return permission.MessageID
}

func (permission remotePermission) callID() string {
	if permission.Tool != nil && permission.Tool.CallID != "" {
		return permission.Tool.CallID
	}
	return permission.CallID
}

func (permission remotePermission) patterns() []string {
	if len(permission.Patterns) > 0 {
		result := make([]string, 0, len(permission.Patterns))
		for _, pattern := range permission.Patterns {
			if text := valueAsString(pattern); text != "" {
				result = append(result, text)
			}
		}
		return result
	}
	return patternSlice(permission.Pattern)
}

func requestOptionsForPermission() []contract.RequestOption {
	return []contract.RequestOption{
		{ID: "once", Label: "Allow once", Kind: "allow_once", IsDefault: true},
		{ID: "always", Label: "Always allow", Kind: "allow_always"},
		{ID: "reject", Label: "Reject", Kind: "reject_once"},
	}
}

func requestQuestionsFromQuestion(question remoteQuestionRequest) []contract.RequestQuestion {
	if len(question.Questions) == 0 {
		return nil
	}
	result := make([]contract.RequestQuestion, 0, len(question.Questions))
	for index, item := range question.Questions {
		questionID := item.Header
		if questionID == "" {
			questionID = fmt.Sprintf("question_%d", index+1)
		}
		options := make([]contract.RequestOption, 0, len(item.Options))
		for _, option := range item.Options {
			options = append(options, contract.RequestOption{
				ID:          option.Label,
				Label:       option.Label,
				Description: option.Description,
			})
		}
		result = append(result, contract.RequestQuestion{
			ID:          questionID,
			Prompt:      item.Question,
			Description: item.Header,
			Required:    true,
			Options:     options,
		})
	}
	return result
}

func requestToolFromQuestion(question remoteQuestionRequest) *contract.RequestToolContext {
	if question.Tool == nil {
		return nil
	}
	return &contract.RequestToolContext{
		Name:        "question",
		Title:       "Question request",
		Kind:        "question",
		Description: fmt.Sprintf("OpenCode requested input for call %s", question.Tool.CallID),
	}
}

func questionSummary(question remoteQuestionRequest) string {
	if len(question.Questions) == 0 {
		return "OpenCode requested user input"
	}
	return truncate(strings.TrimSpace(question.Questions[0].Question), 160)
}

func responsePayload(
	sess *session,
	pending pendingRequest,
	request api.RespondRequest,
) (map[string]any, string, any, error) {
	switch pending.Request.Kind {
	case contract.RequestApprovalTool, contract.RequestApprovalCommand, contract.RequestApprovalFileChange, contract.RequestApprovalPermissions:
		response, err := normalizePermissionResponse(request)
		if err != nil {
			return nil, "", nil, err
		}
		return map[string]any{"reply": response}, fmt.Sprintf(
			"/permission/%s/reply",
			url.PathEscape(request.RequestID),
		), response, nil
	case contract.RequestUserInputTool, contract.RequestUserInputMCP:
		action := normalizeRespondAction(request.Action)
		if action == contract.RespondActionCancel || action == contract.RespondActionDeny {
			return map[string]any{}, fmt.Sprintf("/question/%s/reject", url.PathEscape(request.RequestID)), "reject", nil
		}
		answers, err := questionAnswersPayload(pending.Request, request)
		if err != nil {
			return nil, "", nil, err
		}
		return map[string]any{"answers": answers}, fmt.Sprintf("/question/%s/reply", url.PathEscape(request.RequestID)), answers, nil
	default:
		return nil, "", nil, fmt.Errorf("unsupported OpenCode request kind: %s", pending.Request.Kind)
	}
}

func legacyPermissionResponsePayload(
	sess *session,
	request api.RespondRequest,
	response any,
) (map[string]any, string, error) {
	if _, ok := response.(string); !ok {
		return nil, "", errors.New("legacy OpenCode permission response requires string response")
	}
	return map[string]any{"response": response}, fmt.Sprintf(
		"/session/%s/permissions/%s",
		url.PathEscape(sess.providerSessionIDSnapshot()),
		url.PathEscape(request.RequestID),
	), nil
}

func isPermissionRequestKind(kind contract.RequestKind) bool {
	switch kind {
	case contract.RequestApprovalTool, contract.RequestApprovalCommand, contract.RequestApprovalFileChange, contract.RequestApprovalPermissions:
		return true
	default:
		return false
	}
}

func questionAnswersPayload(pending contract.PendingRequest, request api.RespondRequest) ([][]string, error) {
	questions := pending.Questions
	if len(questions) == 0 {
		return nil, errors.New("question response requires pending request questions")
	}
	answerMap := make(map[string][]string, len(questions))
	for _, question := range questions {
		answerMap[question.ID] = nil
	}
	appendAnswer := func(questionID string, value string) error {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil
		}
		if questionID == "" {
			if len(questions) != 1 {
				return errors.New("question response requires question_id when multiple questions are pending")
			}
			questionID = questions[0].ID
		}
		if _, ok := answerMap[questionID]; !ok {
			return fmt.Errorf("unknown OpenCode question id: %s", questionID)
		}
		answerMap[questionID] = append(answerMap[questionID], value)
		return nil
	}

	if len(request.Answers) > 0 {
		for _, answer := range request.Answers {
			value := answer.OptionID
			if value == "" {
				value = answer.Text
			}
			if err := appendAnswer(answer.QuestionID, value); err != nil {
				return nil, err
			}
		}
	} else {
		value := request.OptionID
		if value == "" {
			value = request.Text
		}
		if strings.TrimSpace(value) == "" {
			return nil, errors.New("question response requires answers, option_id, or text")
		}
		if err := appendAnswer("", value); err != nil {
			return nil, err
		}
	}

	result := make([][]string, 0, len(questions))
	for _, question := range questions {
		result = append(result, answerMap[question.ID])
	}
	return result, nil
}

func normalizeInterruptStatus(status contract.SessionStatus) contract.SessionStatus {
	switch status {
	case contract.SessionRunning, contract.SessionWaitingApproval, contract.SessionWaitingUserInput:
		return status
	default:
		return contract.SessionRunning
	}
}

func normalizeRespondAction(action contract.RespondAction) contract.RespondAction {
	switch strings.ToLower(strings.TrimSpace(string(action))) {
	case "":
		return ""
	case "allow", "approve", "accept", "allow_once", "approve_once":
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

func normalizePermissionResponse(request api.RespondRequest) (string, error) {
	if option := strings.ToLower(strings.TrimSpace(request.OptionID)); option != "" {
		switch option {
		case "once", "always", "reject":
			return option, nil
		default:
			return "", fmt.Errorf("unsupported OpenCode permission option: %s", request.OptionID)
		}
	}

	switch strings.ToLower(strings.TrimSpace(string(request.Action))) {
	case "", "allow", "approve", "accept", "allow_once", "approve_once":
		return "once", nil
	case "allow_always", "approve_always":
		return "always", nil
	case "deny", "reject", "decline", "cancel", "dismiss":
		return "reject", nil
	default:
		return "", fmt.Errorf("unsupported OpenCode respond action: %s", request.Action)
	}
}

func mapSessionStatus(status remoteSessionStatus, pendingStatus contract.SessionStatus) contract.SessionStatus {
	switch status.Type {
	case "idle":
		return contract.SessionIdle
	case "busy":
		if pendingStatus != "" {
			return pendingStatus
		}
		return contract.SessionRunning
	case "retry":
		return contract.SessionRunning
	default:
		return contract.SessionRunning
	}
}

func providerErrorMessage(err *remoteProviderError) string {
	if err == nil {
		return "OpenCode session error"
	}
	if value := valueAsString(err.Data["message"]); value != "" {
		return value
	}
	if strings.TrimSpace(err.Name) != "" {
		return err.Name
	}
	return "OpenCode session error"
}

func metadataString(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	return valueAsString(metadata[key])
}

func metadataBoolMap(metadata map[string]any, key string) map[string]bool {
	if metadata == nil {
		return nil
	}
	raw, ok := metadata[key]
	if !ok {
		return nil
	}
	switch typed := raw.(type) {
	case map[string]bool:
		return typed
	case map[string]any:
		result := make(map[string]bool, len(typed))
		for key, value := range typed {
			boolean, ok := value.(bool)
			if ok {
				result[key] = boolean
			}
		}
		return result
	default:
		return nil
	}
}

func patternSlice(raw any) []string {
	switch value := raw.(type) {
	case string:
		if strings.TrimSpace(value) == "" {
			return nil
		}
		return []string{value}
	case []string:
		return value
	case []any:
		result := make([]string, 0, len(value))
		for _, item := range value {
			if text := valueAsString(item); text != "" {
				result = append(result, text)
			}
		}
		return result
	default:
		return nil
	}
}

func nilIfEmptyMap(values map[string]any) map[string]any {
	if len(values) == 0 {
		return nil
	}
	return values
}

func discardReader(reader io.Reader) {
	_, _ = io.Copy(io.Discard, reader)
}

func reservePort() (int, error) {
	listener, err := net.Listen("tcp", net.JoinHostPort(loopbackHost, "0"))
	if err != nil {
		return 0, err
	}
	defer func() {
		_ = listener.Close()
	}()
	address, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, errors.New("failed to resolve TCP port")
	}
	return address.Port, nil
}

func opencodeBinaryPath() string {
	if value := os.Getenv("AGENTIC_CONTROL_OPENCODE_BINARY"); value != "" {
		return value
	}
	return "opencode"
}

func resolveCWD(value string) (string, error) {
	if strings.TrimSpace(value) != "" {
		return value, nil
	}
	return os.Getwd()
}

func valueAsString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	case fmt.Stringer:
		return typed.String()
	default:
		return ""
	}
}

func truncate(value string, maxLen int) string {
	if len(value) <= maxLen {
		return value
	}
	return value[:maxLen]
}

func coalesce(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func runtimeModelsFromOpenCodeInventory(inventory remoteProviderInventory) []contract.RuntimeModel {
	connected := make(map[string]struct{}, len(inventory.Connected))
	for _, providerID := range inventory.Connected {
		providerID = strings.TrimSpace(providerID)
		if providerID != "" {
			connected[providerID] = struct{}{}
		}
	}

	models := make([]contract.RuntimeModel, 0)
	for _, provider := range inventory.All {
		providerID := strings.TrimSpace(provider.ID)
		if len(connected) > 0 {
			if _, ok := connected[providerID]; !ok {
				continue
			}
		}
		for key, model := range provider.Models {
			modelID := coalesce(strings.TrimSpace(model.ID), strings.TrimSpace(key))
			if modelID == "" {
				continue
			}
			modelProviderID := coalesce(strings.TrimSpace(model.ProviderID), providerID)
			models = append(models, contract.RuntimeModel{
				ID:       qualifiedOpenCodeModelID(modelProviderID, modelID),
				Label:    openCodeModelLabel(provider, model, modelID),
				Provider: modelProviderID,
				Capabilities: contract.RuntimeModelCapabilities{
					VariantOptions: openCodeVariantOptions(modelProviderID, model.Variants),
				},
			})
		}
	}

	sort.Slice(models, func(i, j int) bool {
		if models[i].Provider == models[j].Provider {
			return models[i].ID < models[j].ID
		}
		return models[i].Provider < models[j].Provider
	})
	return models
}

func qualifiedOpenCodeModelID(providerID string, modelID string) string {
	if providerID == "" {
		return modelID
	}
	if strings.HasPrefix(modelID, providerID+"/") {
		return modelID
	}
	return providerID + "/" + modelID
}

func openCodeModelLabel(provider remoteProviderCatalog, model remoteModelCatalog, modelID string) string {
	modelName := coalesce(strings.TrimSpace(model.Name), modelID)
	providerName := strings.TrimSpace(provider.Name)
	if providerName == "" {
		return modelName
	}
	return providerName + " - " + modelName
}

func openCodeVariantOptions(providerID string, variants map[string]any) []contract.RuntimeModelOption {
	if len(variants) == 0 {
		return nil
	}

	values := make([]string, 0, len(variants))
	for variant := range variants {
		if variant = strings.TrimSpace(variant); variant != "" {
			values = append(values, variant)
		}
	}
	sort.Strings(values)

	defaultVariant := defaultOpenCodeVariant(providerID, values)
	options := make([]contract.RuntimeModelOption, 0, len(values))
	for _, value := range values {
		options = append(options, contract.RuntimeModelOption{
			Value:     value,
			Label:     value,
			IsDefault: value == defaultVariant,
		})
	}
	return options
}

func defaultOpenCodeVariant(providerID string, values []string) string {
	if len(values) == 0 {
		return ""
	}
	if len(values) == 1 {
		return values[0]
	}
	switch {
	case strings.Contains(providerID, "anthropic"):
		return firstMatchingVariant(values, "high", "medium", "default")
	case strings.Contains(providerID, "google"):
		return firstMatchingVariant(values, "high", "medium", "default")
	case strings.Contains(providerID, "openai"):
		return firstMatchingVariant(values, "medium", "high", "default")
	default:
		return firstMatchingVariant(values, "default", "medium", "high")
	}
}

func openCodeUsagePayloadFromMessage(message remoteMessage) map[string]any {
	if message.Tokens.Total == 0 && message.Tokens.Input == 0 && message.Tokens.Output == 0 && message.Tokens.Reasoning == 0 && message.Tokens.Cache.Read == 0 {
		return nil
	}
	return map[string]any{
		"usage": map[string]any{
			"total_tokens":     message.Tokens.Total,
			"input_tokens":     message.Tokens.Input,
			"output_tokens":    message.Tokens.Output,
			"reasoning_tokens": message.Tokens.Reasoning,
			"cached_tokens":    message.Tokens.Cache.Read,
		},
		"cost":        message.Cost,
		"provider_id": message.ProviderID,
		"model_id":    message.ModelID,
	}
}

func firstMatchingVariant(values []string, candidates ...string) string {
	for _, candidate := range candidates {
		for _, value := range values {
			if value == candidate {
				return value
			}
		}
	}
	return values[0]
}

func appendProbeMessage(current string, addition string) string {
	current = strings.TrimSpace(current)
	addition = strings.TrimSpace(addition)
	if current == "" {
		return addition
	}
	if addition == "" {
		return current
	}
	return current + " " + addition
}

func opencodeAgentName(value string) string {
	switch strings.TrimSpace(value) {
	case "default_readonly", "readonly", "read_only", "reader", "explorer":
		return "explore"
	case "default_reviewer", "reviewer", "review", "builder":
		return "build"
	case "review_reporter", "security_reporter", "reporter", "writer":
		return "build"
	default:
		return strings.TrimSpace(value)
	}
}

func maxInt64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}

func newIdentifier(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}
