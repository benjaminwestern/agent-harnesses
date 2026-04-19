package pi

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/benjaminwestern/agentic-control/internal/controlplane/modelcatalog"
	"github.com/benjaminwestern/agentic-control/internal/controlplane/providerprobe"
	"github.com/benjaminwestern/agentic-control/pkg/contract"
	api "github.com/benjaminwestern/agentic-control/pkg/controlplane"
)

const runtimeName = "pi"

type eventSink func(contract.RuntimeEvent)

type Provider struct {
	mu       sync.RWMutex
	sessions map[string]*session
	emit     eventSink
	probe    *providerprobe.Cache
}

type session struct {
	mu                 sync.RWMutex
	appSessionID       string
	providerSessionID  string
	cmd                *exec.Cmd
	stdin              io.WriteCloser
	status             contract.SessionStatus
	cwd                string
	model              string
	title              string
	createdAtMS        int64
	updatedAtMS        int64
	lastError          string
	activeTurnID       string
	interruptRequested bool
	stopping           bool
	nextRequestID      int64
	pendingCalls       map[string]chan rpcResponse
	writeMu            sync.Mutex
	provider           *Provider
}

type rpcResponse struct {
	Type    string          `json:"type"`
	ID      string          `json:"id,omitempty"`
	Command string          `json:"command,omitempty"`
	Success bool            `json:"success,omitempty"`
	Error   string          `json:"error,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type stateResponse struct {
	Model                 *piModel `json:"model,omitempty"`
	ThinkingLevel         string   `json:"thinkingLevel,omitempty"`
	IsStreaming           bool     `json:"isStreaming,omitempty"`
	IsCompacting          bool     `json:"isCompacting,omitempty"`
	SteeringMode          string   `json:"steeringMode,omitempty"`
	FollowUpMode          string   `json:"followUpMode,omitempty"`
	SessionFile           string   `json:"sessionFile,omitempty"`
	SessionID             string   `json:"sessionId,omitempty"`
	SessionName           string   `json:"sessionName,omitempty"`
	AutoCompactionEnabled bool     `json:"autoCompactionEnabled,omitempty"`
	MessageCount          int      `json:"messageCount,omitempty"`
	PendingMessageCount   int      `json:"pendingMessageCount,omitempty"`
}

type piModel struct {
	Provider string `json:"provider,omitempty"`
	ID       string `json:"id,omitempty"`
}

type switchSessionResult struct {
	Cancelled bool `json:"cancelled"`
}

type eventEnvelope struct {
	Type string `json:"type"`
}

type toolExecutionStartEvent struct {
	Type       string         `json:"type"`
	ToolCallID string         `json:"toolCallId"`
	ToolName   string         `json:"toolName"`
	Args       map[string]any `json:"args"`
}

type toolExecutionUpdateEvent struct {
	Type          string         `json:"type"`
	ToolCallID    string         `json:"toolCallId"`
	ToolName      string         `json:"toolName"`
	Args          map[string]any `json:"args"`
	PartialResult map[string]any `json:"partialResult"`
}

type toolExecutionEndEvent struct {
	Type       string         `json:"type"`
	ToolCallID string         `json:"toolCallId"`
	ToolName   string         `json:"toolName"`
	Args       map[string]any `json:"args"`
	Result     map[string]any `json:"result"`
	IsError    bool           `json:"isError"`
}

type messageUpdateEvent struct {
	Type                  string         `json:"type"`
	Message               map[string]any `json:"message"`
	AssistantMessageEvent map[string]any `json:"assistantMessageEvent"`
}

type agentEndEvent struct {
	Type     string           `json:"type"`
	Messages []map[string]any `json:"messages"`
}

type extensionErrorEvent struct {
	Type          string `json:"type"`
	ExtensionPath string `json:"extensionPath"`
	Event         string `json:"event"`
	Error         string `json:"error"`
}

func NewProvider(emit func(contract.RuntimeEvent)) *Provider {
	return &Provider{
		sessions: make(map[string]*session),
		emit:     emit,
		probe: providerprobe.New(piBinaryPath, "--version").
			WithModels("runtime_default", modelcatalog.Pi()),
	}
}

func (p *Provider) Runtime() string {
	return runtimeName
}

func (p *Provider) Describe() contract.RuntimeDescriptor {
	descriptor := contract.NewRuntimeDescriptor(
		runtimeName,
		contract.OwnershipControlled,
		contract.TransportRPC,
		contract.RuntimeCapabilities{
			StartSession:             true,
			ResumeSession:            true,
			SendInput:                true,
			Interrupt:                true,
			Respond:                  false,
			StopSession:              true,
			ListSessions:             true,
			StreamEvents:             true,
			ApprovalRequests:         false,
			UserInputRequests:        false,
			ImmediateProviderSession: true,
			ResumeByProviderID:       true,
			AdoptExternalSessions:    true,
		},
	)
	descriptor.Probe = p.probe.Snapshot(context.Background())
	return descriptor
}

func (p *Provider) StartSession(ctx context.Context, request api.StartSessionRequest) (*contract.RuntimeSession, error) {
	resolvedCWD, err := resolveCWD(request.CWD)
	if err != nil {
		return nil, err
	}

	sess, err := p.spawnProcess(ctx, request.SessionID, resolvedCWD, request.Model)
	if err != nil {
		return nil, err
	}
	if err := sess.ensureSession(ctx); err != nil {
		sess.close()
		return nil, err
	}

	p.mu.Lock()
	p.sessions[request.SessionID] = sess
	p.mu.Unlock()

	p.emit(p.newEvent(sess, "session.started", "get_state", "", "Started pi RPC session", map[string]any{"status": string(contract.SessionIdle)}))

	if request.Prompt != "" {
		if _, err := p.SendInput(ctx, api.SendInputRequest{SessionID: request.SessionID, Text: request.Prompt, Metadata: request.Metadata}); err != nil {
			return nil, err
		}
	}

	return sess.snapshot(), nil
}

func (p *Provider) ResumeSession(ctx context.Context, request api.ResumeSessionRequest) (*contract.RuntimeSession, error) {
	resolvedCWD, err := resolveCWD(request.CWD)
	if err != nil {
		return nil, err
	}

	sess, err := p.spawnProcess(ctx, request.SessionID, resolvedCWD, request.Model)
	if err != nil {
		return nil, err
	}

	var switched switchSessionResult
	if err := sess.call(ctx, "switch_session", map[string]any{"sessionPath": request.ProviderSessionID}, &switched); err != nil {
		sess.close()
		return nil, err
	}
	if switched.Cancelled {
		sess.close()
		return nil, errors.New("pi switch_session was cancelled")
	}
	if err := sess.refreshState(ctx); err != nil {
		sess.close()
		return nil, err
	}
	if strings.TrimSpace(sess.providerSessionIDSnapshot()) == "" {
		sess.setProviderSessionID(request.ProviderSessionID)
	}

	p.mu.Lock()
	p.sessions[request.SessionID] = sess
	p.mu.Unlock()

	p.emit(p.newEvent(sess, "session.started", "switch_session", "", "Resumed pi session", map[string]any{"status": string(contract.SessionIdle)}))
	return sess.snapshot(), nil
}

func (p *Provider) SendInput(ctx context.Context, request api.SendInputRequest) (*contract.RuntimeEvent, error) {
	sess, err := p.getSession(request.SessionID)
	if err != nil {
		return nil, err
	}
	if status := sess.statusSnapshot(); status == contract.SessionRunning {
		return nil, errors.New("pi session already has an active turn")
	}

	if err := sess.call(ctx, "prompt", map[string]any{"message": request.Text}, nil); err != nil {
		return nil, err
	}

	turnID := newIdentifier("turn")
	sess.setActiveTurnID(turnID)
	sess.setInterruptRequested(false)
	sess.setLastError("")
	sess.setStatus(contract.SessionRunning)

	event := p.newEvent(sess, "turn.started", "prompt", turnID, fmt.Sprintf("Started pi turn: %s", truncate(request.Text, 120)), map[string]any{"status": string(contract.SessionRunning)})
	p.emit(event)
	return &event, nil
}

func (p *Provider) Interrupt(ctx context.Context, sessionID string) (*contract.RuntimeEvent, error) {
	sess, err := p.getSession(sessionID)
	if err != nil {
		return nil, err
	}
	if sess.statusSnapshot() != contract.SessionRunning {
		return nil, errors.New("pi session has no active turn")
	}

	if err := sess.call(ctx, "abort", nil, nil); err != nil {
		return nil, err
	}
	sess.setInterruptRequested(true)

	event := p.newEvent(sess, "turn.interrupt_requested", "abort", sess.activeTurnIDSnapshot(), "Requested pi turn interruption", map[string]any{"status": string(contract.SessionRunning), "interrupt_requested": true})
	p.emit(event)
	return &event, nil
}

func (p *Provider) Respond(ctx context.Context, request api.RespondRequest) (*contract.RuntimeEvent, error) {
	return nil, errors.New("pi controller does not support request responses")
}

func (p *Provider) StopSession(ctx context.Context, sessionID string) (*contract.RuntimeEvent, error) {
	sess, err := p.getSession(sessionID)
	if err != nil {
		return nil, err
	}
	sess.close()
	p.deleteSession(sessionID)
	sess.clearActiveTurnID()
	sess.setInterruptRequested(false)
	sess.setStatus(contract.SessionStopped)

	event := p.newEvent(sess, "session.stopped", "process/stop", "", "Stopped pi RPC session", map[string]any{"status": string(contract.SessionStopped)})
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

func (p *Provider) spawnProcess(ctx context.Context, appSessionID, cwd, model string) (*session, error) {
	args := []string{"--mode", "rpc", "--no-extensions"}
	if strings.TrimSpace(model) != "" {
		args = append(args, "--model", model)
	}
	command := exec.CommandContext(ctx, piBinaryPath(), args...)
	if cwd != "" {
		command.Dir = cwd
	}

	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := command.Start(); err != nil {
		return nil, err
	}

	now := time.Now().UnixMilli()
	sess := &session{
		appSessionID: appSessionID,
		cmd:          command,
		stdin:        stdin,
		status:       contract.SessionStarting,
		cwd:          cwd,
		model:        model,
		createdAtMS:  now,
		updatedAtMS:  now,
		pendingCalls: make(map[string]chan rpcResponse),
		provider:     p,
	}

	go sess.readLoop(stdout)
	go sess.stderrLoop(stderr)
	go sess.waitLoop()
	return sess, nil
}

func (s *session) ensureSession(ctx context.Context) error {
	if err := s.refreshState(ctx); err != nil {
		return err
	}
	if strings.TrimSpace(s.providerSessionIDSnapshot()) != "" {
		return nil
	}
	var result switchSessionResult
	if err := s.call(ctx, "new_session", nil, &result); err != nil {
		return err
	}
	if result.Cancelled {
		return errors.New("pi new_session was cancelled")
	}
	return s.refreshState(ctx)
}

func (s *session) refreshState(ctx context.Context) error {
	var state stateResponse
	if err := s.call(ctx, "get_state", nil, &state); err != nil {
		return err
	}
	if state.SessionFile != "" {
		s.setProviderSessionID(state.SessionFile)
	}
	if state.SessionName != "" {
		s.setTitle(state.SessionName)
	}
	if model := modelString(state.Model); model != "" {
		s.setModel(model)
	}
	if state.IsStreaming {
		s.setStatus(contract.SessionRunning)
	} else {
		s.setStatus(contract.SessionIdle)
	}
	return nil
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

func (p *Provider) deleteSession(sessionID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.sessions, sessionID)
}

func (p *Provider) newEvent(sess *session, eventType, nativeEventName, turnID, summary string, payload map[string]any) contract.RuntimeEvent {
	return contract.NewRuntimeEvent(*sess.snapshot(), eventType, nativeEventName, turnID, summary, payload)
}

func (s *session) readLoop(reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var envelope eventEnvelope
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			s.provider.emit(s.provider.newEvent(s, "runtime.decode_error", "decode", s.activeTurnIDSnapshot(), fmt.Sprintf("Failed to decode pi RPC message: %v", err), map[string]any{"line": line}))
			continue
		}
		if envelope.Type == "response" {
			var response rpcResponse
			if err := json.Unmarshal([]byte(line), &response); err != nil {
				continue
			}
			s.resolvePendingCall(response)
			continue
		}
		s.handleEvent(envelope.Type, []byte(line))
	}
}

func (s *session) stderrLoop(reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		s.provider.emit(s.provider.newEvent(s, "runtime.stderr", "stderr", s.activeTurnIDSnapshot(), truncate(line, 200), nil))
	}
}

func (s *session) waitLoop() {
	err := s.cmd.Wait()
	stopping := s.isStopping()
	status := contract.SessionStopped
	payload := map[string]any{"status": string(status)}
	summary := "pi RPC session stopped"
	eventType := "session.stopped"
	if err != nil && !stopping {
		status = contract.SessionErrored
		payload["status"] = string(status)
		payload["last_error"] = err.Error()
		summary = fmt.Sprintf("pi RPC session exited: %s", err)
		eventType = "session.errored"
	}
	s.setStatus(status)
	if err != nil && !stopping {
		s.setLastError(err.Error())
	}
	s.failPendingCalls(err)
	s.provider.deleteSession(s.appSessionID)
	s.provider.emit(s.provider.newEvent(s, eventType, "process/exit", s.activeTurnIDSnapshot(), summary, payload))
}

func (s *session) handleEvent(kind string, payload []byte) {
	switch kind {
	case "agent_start":
		s.setStatus(contract.SessionRunning)
	case "message_update":
		var event messageUpdateEvent
		if json.Unmarshal(payload, &event) == nil {
			s.handleMessageUpdate(event)
		}
	case "tool_execution_start":
		var event toolExecutionStartEvent
		if json.Unmarshal(payload, &event) == nil {
			s.handleToolExecutionStart(event)
		}
	case "tool_execution_update":
		var event toolExecutionUpdateEvent
		if json.Unmarshal(payload, &event) == nil {
			s.handleToolExecutionUpdate(event)
		}
	case "tool_execution_end":
		var event toolExecutionEndEvent
		if json.Unmarshal(payload, &event) == nil {
			s.handleToolExecutionEnd(event)
		}
	case "agent_end":
		var event agentEndEvent
		if json.Unmarshal(payload, &event) == nil {
			s.handleAgentEnd(event)
		}
	case "extension_error":
		var event extensionErrorEvent
		if json.Unmarshal(payload, &event) == nil {
			s.provider.emit(s.provider.newEvent(s, "runtime.event", kind, s.activeTurnIDSnapshot(), fmt.Sprintf("pi extension error in %s: %s", event.Event, event.Error), map[string]any{"extension_path": event.ExtensionPath, "event": event.Event, "error": event.Error}))
		}
	default:
		s.provider.emit(s.provider.newEvent(s, "runtime.event", kind, s.activeTurnIDSnapshot(), fmt.Sprintf("pi event: %s", kind), nil))
	}
}

func (s *session) handleMessageUpdate(event messageUpdateEvent) {
	kind, _ := event.AssistantMessageEvent["type"].(string)
	switch kind {
	case "text_delta":
		delta := valueAsString(event.AssistantMessageEvent["delta"])
		if delta == "" {
			return
		}
		s.provider.emit(s.provider.newEvent(s, "assistant.message.delta", "message_update", s.activeTurnIDSnapshot(), truncate(delta, 160), map[string]any{"delta": delta}))
	case "thinking_delta":
		delta := valueAsString(event.AssistantMessageEvent["delta"])
		if delta == "" {
			return
		}
		s.provider.emit(s.provider.newEvent(s, "assistant.thought.delta", "message_update", s.activeTurnIDSnapshot(), truncate(delta, 160), map[string]any{"delta": delta}))
	case "error":
		reason := valueAsString(event.AssistantMessageEvent["reason"])
		if reason == "" {
			reason = "pi assistant stream error"
		}
		s.setLastError(reason)
		s.provider.emit(s.provider.newEvent(s, "runtime.event", "message_update", s.activeTurnIDSnapshot(), fmt.Sprintf("pi assistant stream error: %s", reason), map[string]any{"reason": reason}))
	}
}

func (s *session) handleToolExecutionStart(event toolExecutionStartEvent) {
	s.provider.emit(s.provider.newEvent(s, "tool.started", "tool_execution_start", s.activeTurnIDSnapshot(), fmt.Sprintf("pi tool started: %s", coalesce(event.ToolName, "tool")), map[string]any{"tool_call_id": event.ToolCallID, "tool_name": event.ToolName, "args": event.Args, "command": commandFromArgs(event.Args)}))
}

func (s *session) handleToolExecutionUpdate(event toolExecutionUpdateEvent) {
	s.provider.emit(s.provider.newEvent(s, "tool.updated", "tool_execution_update", s.activeTurnIDSnapshot(), fmt.Sprintf("pi tool update: %s", coalesce(event.ToolName, "tool")), map[string]any{"tool_call_id": event.ToolCallID, "tool_name": event.ToolName, "args": event.Args, "partial_result": event.PartialResult}))
}

func (s *session) handleToolExecutionEnd(event toolExecutionEndEvent) {
	eventType := "tool.finished"
	summary := fmt.Sprintf("pi tool finished: %s", coalesce(event.ToolName, "tool"))
	if event.IsError {
		eventType = "tool.failed"
		summary = fmt.Sprintf("pi tool failed: %s", coalesce(event.ToolName, "tool"))
	}
	s.provider.emit(s.provider.newEvent(s, eventType, "tool_execution_end", s.activeTurnIDSnapshot(), summary, map[string]any{"tool_call_id": event.ToolCallID, "tool_name": event.ToolName, "args": event.Args, "command": commandFromArgs(event.Args), "result": event.Result, "is_error": event.IsError}))
}

func (s *session) handleAgentEnd(event agentEndEvent) {
	turnID := s.activeTurnIDSnapshot()
	assistant := lastAssistantMessage(event.Messages)
	stopReason := valueAsString(assistant["stopReason"])
	assistantText := assistantText(assistant)
	errorMessage := valueAsString(assistant["errorMessage"])
	interrupted := s.consumeInterruptRequested() || stopReason == "aborted"

	s.clearActiveTurnID()
	s.setStatus(contract.SessionIdle)
	if interrupted {
		s.setLastError("")
		s.provider.emit(s.provider.newEvent(s, "turn.interrupted", "agent_end", turnID, coalesce(summaryFromText("pi turn interrupted", assistantText), "pi turn interrupted"), map[string]any{"status": string(contract.SessionIdle), "stop_reason": stopReason}))
		return
	}
	if stopReason == "error" {
		s.setLastError(errorMessage)
		s.provider.emit(s.provider.newEvent(s, "turn.errored", "agent_end", turnID, coalesce(summaryFromText("pi turn failed", errorMessage), "pi turn failed"), map[string]any{"status": string(contract.SessionIdle), "stop_reason": stopReason, "last_error": errorMessage}))
		return
	}
	s.setLastError("")
	s.provider.emit(s.provider.newEvent(s, "turn.completed", "agent_end", turnID, coalesce(summaryFromText("pi turn completed", assistantText), "pi turn completed"), map[string]any{"status": string(contract.SessionIdle), "stop_reason": stopReason}))
}

func (s *session) failPendingCalls(err error) {
	s.mu.Lock()
	pending := make([]chan rpcResponse, 0, len(s.pendingCalls))
	for id, channel := range s.pendingCalls {
		_ = id
		pending = append(pending, channel)
	}
	s.pendingCalls = make(map[string]chan rpcResponse)
	s.mu.Unlock()

	message := "pi process exited"
	if err != nil {
		message = err.Error()
	}
	for _, channel := range pending {
		channel <- rpcResponse{Type: "response", Success: false, Error: message}
	}
}

func (s *session) resolvePendingCall(response rpcResponse) {
	s.mu.Lock()
	channel, ok := s.pendingCalls[response.ID]
	if ok {
		delete(s.pendingCalls, response.ID)
	}
	s.mu.Unlock()
	if ok {
		channel <- response
	}
}

func (s *session) call(ctx context.Context, command string, params map[string]any, target any) error {
	responseChannel, err := s.beginCall(command, params)
	if err != nil {
		return err
	}
	return awaitCall(ctx, responseChannel, target)
}

func (s *session) beginCall(command string, params map[string]any) (<-chan rpcResponse, error) {
	requestID := fmt.Sprintf("%d", s.nextID())
	responseChannel := make(chan rpcResponse, 1)

	s.mu.Lock()
	s.pendingCalls[requestID] = responseChannel
	s.mu.Unlock()

	payload := map[string]any{"type": command, "id": requestID}
	for key, value := range params {
		payload[key] = value
	}
	if err := s.writeJSON(payload); err != nil {
		s.mu.Lock()
		delete(s.pendingCalls, requestID)
		s.mu.Unlock()
		return nil, err
	}
	return responseChannel, nil
}

func (s *session) writeJSON(value any) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	_, err = s.stdin.Write(encoded)
	return err
}

func (s *session) close() {
	s.mu.Lock()
	s.stopping = true
	s.mu.Unlock()
	if s.cmd.Process != nil {
		_ = s.cmd.Process.Signal(syscall.SIGTERM)
	}
	_ = s.stdin.Close()
}

func (s *session) nextID() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextRequestID++
	return s.nextRequestID
}

func (s *session) snapshot() *contract.RuntimeSession {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return &contract.RuntimeSession{
		SchemaVersion:     contract.ControlPlaneSchemaVersion,
		SessionID:         s.appSessionID,
		Runtime:           runtimeName,
		Ownership:         contract.OwnershipControlled,
		Transport:         contract.TransportRPC,
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

func (s *session) isStopping() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stopping
}

func (s *session) setProviderSessionID(value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.providerSessionID = value
	s.updatedAtMS = time.Now().UnixMilli()
}

func (s *session) setStatus(status contract.SessionStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = status
	s.updatedAtMS = time.Now().UnixMilli()
}

func (s *session) setModel(value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.model = value
	s.updatedAtMS = time.Now().UnixMilli()
}

func (s *session) setTitle(value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.title = value
	s.updatedAtMS = time.Now().UnixMilli()
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
	s.updatedAtMS = time.Now().UnixMilli()
}

func (s *session) clearActiveTurnID() {
	s.setActiveTurnID("")
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

func awaitCall(ctx context.Context, responseChannel <-chan rpcResponse, target any) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case response := <-responseChannel:
		if !response.Success {
			if response.Error == "" {
				return fmt.Errorf("pi command %s failed", response.Command)
			}
			return errors.New(response.Error)
		}
		if target == nil || len(response.Data) == 0 || string(response.Data) == "null" {
			return nil
		}
		return json.Unmarshal(response.Data, target)
	}
}

func modelString(model *piModel) string {
	if model == nil {
		return ""
	}
	if strings.TrimSpace(model.Provider) != "" && strings.TrimSpace(model.ID) != "" {
		return model.Provider + "/" + model.ID
	}
	return model.ID
}

func commandFromArgs(args map[string]any) string {
	for _, key := range []string{"command", "path", "url"} {
		if value := valueAsString(args[key]); value != "" {
			return value
		}
	}
	return ""
}

func lastAssistantMessage(messages []map[string]any) map[string]any {
	for index := len(messages) - 1; index >= 0; index-- {
		if valueAsString(messages[index]["role"]) == "assistant" {
			return messages[index]
		}
	}
	return nil
}

func assistantText(message map[string]any) string {
	content, ok := message["content"].([]any)
	if !ok {
		return ""
	}
	parts := make([]string, 0, len(content))
	for _, item := range content {
		block, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if valueAsString(block["type"]) != "text" {
			continue
		}
		if text := valueAsString(block["text"]); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, ""))
}

func summaryFromText(prefix, text string) string {
	if strings.TrimSpace(text) == "" {
		return prefix
	}
	return fmt.Sprintf("%s: %s", prefix, truncate(text, 160))
}

func valueAsString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return ""
	}
}

func coalesce(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func truncate(value string, maxLen int) string {
	if len(value) <= maxLen {
		return value
	}
	return value[:maxLen]
}

func resolveCWD(value string) (string, error) {
	if strings.TrimSpace(value) != "" {
		return value, nil
	}
	return os.Getwd()
}

func piBinaryPath() string {
	if value := os.Getenv("AGENTIC_CONTROL_PI_BINARY"); value != "" {
		return value
	}
	return "pi"
}

func newIdentifier(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}
