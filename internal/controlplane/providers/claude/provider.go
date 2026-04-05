package claude

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/benjaminwestern/agentic-control/pkg/contract"
	api "github.com/benjaminwestern/agentic-control/pkg/controlplane"
)

const runtimeName = "claude"

type eventSink func(contract.RuntimeEvent)

type Provider struct {
	mu       sync.RWMutex
	sessions map[string]*session
	emit     eventSink
}

type session struct {
	mu                sync.RWMutex
	appSessionID      string
	providerSessionID string
	cmd               *exec.Cmd
	stdin             io.WriteCloser
	status            contract.SessionStatus
	cwd               string
	model             string
	createdAtMS       int64
	updatedAtMS       int64
	lastError         string
	stopping          bool
	nextRequestID     int64
	pendingCalls      map[string]chan rpcMessage
	pendingRequests   map[string]pendingRequest
	writeMu           sync.Mutex
	provider          *Provider
}

type pendingRequest struct {
	Request contract.PendingRequest
}

type rpcMessage struct {
	ID     json.RawMessage   `json:"id,omitempty"`
	Method string            `json:"method,omitempty"`
	Params json.RawMessage   `json:"params,omitempty"`
	Result json.RawMessage   `json:"result,omitempty"`
	Error  *rpcErrorEnvelope `json:"error,omitempty"`
}

type rpcErrorEnvelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type streamMessage struct {
	Type      string           `json:"type"`
	Subtype   string           `json:"subtype,omitempty"`
	SessionID string           `json:"session_id,omitempty"`
	Result    string           `json:"result,omitempty"`
	Message   map[string]any   `json:"message,omitempty"`
	Content   []map[string]any `json:"content,omitempty"`
	Event     map[string]any   `json:"event,omitempty"`
	Data      map[string]any   `json:"data,omitempty"`
	Raw       map[string]any   `json:"-"`
}

type bridgeSessionResult struct {
	SessionID string `json:"session_id"`
	Model     string `json:"model"`
	CWD       string `json:"cwd"`
}

type requestOpenedNotification struct {
	RequestID    string         `json:"request_id"`
	Kind         string         `json:"kind"`
	NativeMethod string         `json:"native_method"`
	Summary      string         `json:"summary"`
	CreatedAtMS  int64          `json:"created_at_ms"`
	Metadata     map[string]any `json:"metadata"`
}

type requestClosedNotification struct {
	RequestID string `json:"request_id"`
	Reason    string `json:"reason"`
}

type runtimeErrorNotification struct {
	Message string `json:"message"`
}

func NewProvider(emit func(contract.RuntimeEvent)) *Provider {
	return &Provider{
		sessions: make(map[string]*session),
		emit:     emit,
	}
}

func (p *Provider) Runtime() string {
	return runtimeName
}

func (p *Provider) Describe() contract.RuntimeDescriptor {
	return contract.NewRuntimeDescriptor(
		runtimeName,
		contract.OwnershipControlled,
		contract.TransportAgentSDK,
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
		},
	)
}

func (p *Provider) StartSession(
	ctx context.Context,
	request api.StartSessionRequest,
) (*contract.RuntimeSession, error) {
	sess, err := p.spawnProcess(ctx, request.SessionID, request.CWD, request.Model)
	if err != nil {
		return nil, err
	}

	result := bridgeSessionResult{}
	if err := sess.call(ctx, "session.start", map[string]any{
		"cwd":         request.CWD,
		"model":       request.Model,
		"claude_path": claudeCodeBinaryPath(),
	}, &result); err != nil {
		sess.close()
		return nil, err
	}

	sess.setProviderSessionID(result.SessionID)
	sess.setStatus(contract.SessionIdle)
	if result.Model != "" {
		sess.setModel(result.Model)
	}
	if result.CWD != "" {
		sess.setCWD(result.CWD)
	}

	p.mu.Lock()
	p.sessions[request.SessionID] = sess
	p.mu.Unlock()

	event := p.newEvent(sess, "session.started", "session.start", "",
		"Started Claude controller session",
		map[string]any{"status": string(contract.SessionIdle)},
	)
	p.emit(event)

	if request.Prompt != "" {
		if _, err := p.SendInput(ctx, api.SendInputRequest{
			SessionID: request.SessionID,
			Text:      request.Prompt,
			Metadata:  request.Metadata,
		}); err != nil {
			return nil, err
		}
	}

	return sess.snapshot(), nil
}

func (p *Provider) ResumeSession(
	ctx context.Context,
	request api.ResumeSessionRequest,
) (*contract.RuntimeSession, error) {
	sess, err := p.spawnProcess(ctx, request.SessionID, request.CWD, request.Model)
	if err != nil {
		return nil, err
	}

	result := bridgeSessionResult{}
	if err := sess.call(ctx, "session.resume", map[string]any{
		"cwd":               request.CWD,
		"model":             request.Model,
		"resume_session_id": request.ProviderSessionID,
		"claude_path":       claudeCodeBinaryPath(),
	}, &result); err != nil {
		sess.close()
		return nil, err
	}

	sess.setProviderSessionID(coalesce(result.SessionID, request.ProviderSessionID))
	sess.setStatus(contract.SessionIdle)
	if result.Model != "" {
		sess.setModel(result.Model)
	}
	if result.CWD != "" {
		sess.setCWD(result.CWD)
	}

	p.mu.Lock()
	p.sessions[request.SessionID] = sess
	p.mu.Unlock()

	event := p.newEvent(sess, "session.started", "session.resume", "",
		"Resumed Claude controller session",
		map[string]any{"status": string(contract.SessionIdle)},
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

	if err := sess.call(ctx, "session.send", map[string]any{
		"text": request.Text,
	}, nil); err != nil {
		return nil, err
	}

	sess.setStatus(contract.SessionRunning)
	event := p.newEvent(sess, "turn.started", "session.send", "",
		fmt.Sprintf("Queued Claude input: %s", truncate(request.Text, 120)),
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

	if err := sess.call(ctx, "session.interrupt", nil, nil); err != nil {
		return nil, err
	}

	sess.setStatus(contract.SessionInterrupted)
	event := p.newEvent(sess, "turn.interrupt_requested", "session.interrupt", "",
		"Requested Claude turn interruption",
		map[string]any{"status": string(contract.SessionInterrupted)},
	)
	p.emit(event)
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

	if err := sess.call(ctx, "session.respond", map[string]any{
		"request_id": request.RequestID,
		"action":     request.Action,
		"text":       request.Text,
		"option_id":  request.OptionID,
		"answers":    request.Answers,
		"metadata":   request.Metadata,
	}, nil); err != nil {
		return nil, err
	}

	sess.removePendingRequest(request.RequestID)
	sess.setStatus(contract.SessionRunning)

	event := p.newEvent(sess, "request.responded", pending.Request.NativeMethod, "",
		fmt.Sprintf("Responded to Claude request: %s", pending.Request.NativeMethod),
		map[string]any{"status": string(contract.SessionRunning)},
	)
	event.RequestID = request.RequestID
	respondedRequest := pending.Request
	respondedRequest.Status = contract.RequestStatusResponded
	event.Request = &respondedRequest
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
	_ = sess.call(ctx, "session.stop", nil, nil)
	sess.close()
	p.deleteSession(sessionID)

	event := p.newEvent(sess, "session.stopped", "process/stop", "",
		"Stopped Claude controller session",
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

func (p *Provider) spawnProcess(
	ctx context.Context,
	appSessionID string,
	cwd string,
	model string,
) (*session, error) {
	commandPath, args, err := claudeBridgeCommand()
	if err != nil {
		return nil, err
	}

	command := exec.CommandContext(ctx, commandPath, args...)
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
		appSessionID:    appSessionID,
		cmd:             command,
		stdin:           stdin,
		status:          contract.SessionStarting,
		cwd:             cwd,
		model:           model,
		createdAtMS:     now,
		updatedAtMS:     now,
		pendingCalls:    make(map[string]chan rpcMessage),
		pendingRequests: make(map[string]pendingRequest),
		provider:        p,
	}

	go sess.readLoop(stdout)
	go sess.stderrLoop(stderr)
	go sess.waitLoop()

	return sess, nil
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

func (s *session) readLoop(reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var message rpcMessage
		if err := json.Unmarshal([]byte(line), &message); err != nil {
			s.provider.emit(s.provider.newEvent(s, "runtime.decode_error", "decode", "",
				fmt.Sprintf("Failed to decode Claude bridge message: %v", err),
				map[string]any{
					"status":     string(contract.SessionErrored),
					"last_error": err.Error(),
				},
			))
			continue
		}

		if len(message.Result) > 0 || message.Error != nil {
			s.resolvePendingCall(message)
			continue
		}
		if message.Method == "" {
			continue
		}
		s.handleNotification(message)
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
		s.provider.emit(s.provider.newEvent(s, "runtime.stderr", "stderr", "",
			truncate(line, 200), nil))
	}
}

func (s *session) waitLoop() {
	err := s.cmd.Wait()
	stopping := s.isStopping()
	status := contract.SessionStopped
	payload := map[string]any{
		"status": string(status),
	}
	summary := "Claude controller session stopped"
	if err != nil && !stopping {
		status = contract.SessionErrored
		payload["status"] = string(status)
		payload["last_error"] = err.Error()
		summary = fmt.Sprintf("Claude controller session exited: %s", err)
	}
	s.setStatus(status)
	if err != nil && !stopping {
		s.setLastError(err.Error())
	}
	s.provider.deleteSession(s.appSessionID)
	s.provider.emit(s.provider.newEvent(s, map[bool]string{true: "session.errored", false: "session.stopped"}[err != nil && !stopping], "process/exit", "", summary, payload))
}

func (s *session) handleNotification(message rpcMessage) {
	switch message.Method {
	case "sdk.message":
		var raw map[string]any
		if err := json.Unmarshal(message.Params, &raw); err != nil {
			s.provider.emit(s.provider.newEvent(s, "runtime.decode_error", message.Method, "",
				fmt.Sprintf("Failed to decode Claude SDK message: %v", err), nil))
			return
		}

		payload, _ := json.Marshal(raw)
		stream := streamMessage{Raw: raw}
		_ = json.Unmarshal(payload, &stream)
		s.handleSDKMessage(stream)
	case "request.opened":
		var params requestOpenedNotification
		if err := json.Unmarshal(message.Params, &params); err != nil {
			s.provider.emit(s.provider.newEvent(s, "runtime.decode_error", message.Method, "",
				fmt.Sprintf("Failed to decode Claude request notification: %v", err), nil))
			return
		}
		s.handleRequestOpened(params)
	case "request.closed":
		var params requestClosedNotification
		if err := json.Unmarshal(message.Params, &params); err != nil {
			s.provider.emit(s.provider.newEvent(s, "runtime.decode_error", message.Method, "",
				fmt.Sprintf("Failed to decode Claude request closure: %v", err), nil))
			return
		}
		s.removePendingRequest(params.RequestID)
		s.provider.emit(s.provider.newEvent(s, "request.resolved", message.Method, "",
			fmt.Sprintf("Claude request closed: %s", coalesce(params.Reason, params.RequestID)),
			nil,
		))
	case "runtime.error":
		var params runtimeErrorNotification
		if err := json.Unmarshal(message.Params, &params); err != nil {
			s.provider.emit(s.provider.newEvent(s, "runtime.decode_error", message.Method, "",
				fmt.Sprintf("Failed to decode Claude runtime error: %v", err), nil))
			return
		}
		s.setLastError(params.Message)
		s.provider.emit(s.provider.newEvent(s, "runtime.error", message.Method, "",
			coalesce(params.Message, "Claude runtime error"),
			map[string]any{"last_error": params.Message},
		))
	default:
		s.provider.emit(s.provider.newEvent(s, "runtime.event", message.Method, "",
			fmt.Sprintf("Claude bridge event: %s", message.Method),
			nil,
		))
	}
}

func (s *session) handleSDKMessage(message streamMessage) {
	if message.SessionID != "" {
		s.setProviderSessionID(message.SessionID)
	}

	switch message.Type {
	case "system":
		switch message.Subtype {
		case "init":
			if model, _ := message.Raw["model"].(string); model != "" {
				s.setModel(model)
			}
			if cwd, _ := message.Raw["cwd"].(string); cwd != "" {
				s.setCWD(cwd)
			}
			s.setStatus(contract.SessionIdle)
			s.provider.emit(s.provider.newEvent(s, "session.initialized", "system.init", "",
				"Claude session initialized",
				map[string]any{"status": string(contract.SessionIdle)},
			))
		case "session_state_changed":
			state, _ := message.Raw["state"].(string)
			status := mapClaudeState(state)
			if status != "" {
				s.setStatus(status)
			}
			s.provider.emit(s.provider.newEvent(s, "session.state.changed", "system.session_state_changed", "",
				fmt.Sprintf("Claude session state changed: %s", state),
				map[string]any{
					"state":  state,
					"status": string(status),
				},
			))
		case "status":
			s.provider.emit(s.provider.newEvent(s, "session.status.updated", "system.status", "",
				"Claude session status updated",
				map[string]any{
					"status_name":     message.Raw["status"],
					"permission_mode": message.Raw["permissionMode"],
				},
			))
		default:
			s.provider.emit(s.provider.newEvent(s, "runtime.event", fmt.Sprintf("system.%s", message.Subtype), "",
				fmt.Sprintf("Claude system event: %s", message.Subtype),
				nil,
			))
		}
	case "assistant":
		s.setStatus(contract.SessionRunning)
		text := extractAssistantText(message.Raw)
		s.provider.emit(s.provider.newEvent(s, "assistant.message", "assistant", "",
			coalesce(text, "Claude assistant message"),
			map[string]any{"status": string(contract.SessionRunning)},
		))
	case "stream_event":
		if delta := extractStreamDelta(message.Raw); delta != "" {
			s.provider.emit(s.provider.newEvent(s, "assistant.message.delta", "stream_event", "",
				truncate(delta, 160),
				map[string]any{"delta": delta, "status": string(contract.SessionRunning)},
			))
		}
	case "result":
		s.handleResult(message)
	case "user":
		s.provider.emit(s.provider.newEvent(s, "turn.input.acknowledged", "user", "",
			"Claude input acknowledged",
			nil,
		))
	default:
		s.provider.emit(s.provider.newEvent(s, "runtime.event", message.Type, "",
			fmt.Sprintf("Claude event: %s", message.Type),
			nil,
		))
	}
}

func (s *session) handleResult(message streamMessage) {
	payload := map[string]any{
		"status": string(contract.SessionIdle),
	}
	if stopReason, ok := message.Raw["stop_reason"].(string); ok && stopReason != "" {
		payload["stop_reason"] = stopReason
	}
	if subtype, ok := message.Raw["subtype"].(string); ok && subtype != "" {
		payload["subtype"] = subtype
	}

	isError, _ := message.Raw["is_error"].(bool)
	if isError {
		s.setStatus(contract.SessionIdle)
		errorsList := extractStringSlice(message.Raw["errors"])
		lastError := strings.Join(errorsList, "; ")
		if lastError != "" {
			payload["last_error"] = lastError
			s.setLastError(lastError)
		}
		s.provider.emit(s.provider.newEvent(s, "turn.errored", "result", "",
			coalesce(lastError, "Claude turn errored"),
			payload,
		))
		return
	}

	s.setStatus(contract.SessionIdle)
	s.provider.emit(s.provider.newEvent(s, "turn.completed", "result", "",
		coalesce(message.Result, "Claude turn completed"),
		payload,
	))
}

func (s *session) handleRequestOpened(params requestOpenedNotification) {
	kind := contract.RequestKind(params.Kind)
	status := statusForRequestKind(kind)
	s.setStatus(status)

	appRequestID := params.RequestID
	if appRequestID == "" {
		appRequestID = fmt.Sprintf("req-%d", time.Now().UnixNano())
	}
	createdAt := params.CreatedAtMS
	if createdAt == 0 {
		createdAt = time.Now().UnixMilli()
	}

	pending := contract.PendingRequest{
		SchemaVersion: contract.ControlPlaneSchemaVersion,
		RequestID:     appRequestID,
		SessionID:     s.appSessionID,
		Runtime:       runtimeName,
		Kind:          kind,
		NativeMethod:  coalesce(params.NativeMethod, "canUseTool"),
		Status:        contract.RequestStatusOpen,
		Summary:       params.Summary,
		CreatedAtMS:   createdAt,
		Tool:          requestToolFromMetadata(params.Metadata),
		Questions:     requestQuestionsFromMetadata(params.Metadata),
		Extensions:    nilIfEmptyMap(params.Metadata),
	}
	s.storePendingRequest(pendingRequest{Request: pending})

	event := s.provider.newEvent(s, "request.opened", coalesce(params.NativeMethod, "canUseTool"), "",
		coalesce(params.Summary, "Claude requested input"),
		map[string]any{"status": string(status)},
	)
	event.RequestID = appRequestID
	event.Request = &pending
	s.provider.emit(event)
}

func (s *session) resolvePendingCall(message rpcMessage) {
	key := canonicalID(message.ID)
	s.mu.Lock()
	channel, ok := s.pendingCalls[key]
	if ok {
		delete(s.pendingCalls, key)
	}
	s.mu.Unlock()
	if ok {
		channel <- message
	}
}

func (s *session) call(ctx context.Context, method string, params any, target any) error {
	requestID := strconv.FormatInt(s.nextID(), 10)
	responseChannel := make(chan rpcMessage, 1)

	s.mu.Lock()
	s.pendingCalls[requestID] = responseChannel
	s.mu.Unlock()

	if err := s.writeRequest(requestID, method, params); err != nil {
		s.mu.Lock()
		delete(s.pendingCalls, requestID)
		s.mu.Unlock()
		return err
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case response := <-responseChannel:
		if response.Error != nil {
			return fmt.Errorf("claude bridge error %d: %s", response.Error.Code, response.Error.Message)
		}
		if target == nil || len(response.Result) == 0 {
			return nil
		}
		return json.Unmarshal(response.Result, target)
	}
}

func (s *session) writeRequest(id string, method string, params any) error {
	return s.writeJSON(map[string]any{
		"id":     json.Number(id),
		"method": method,
		"params": params,
	})
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
		Transport:         contract.TransportAgentSDK,
		Status:            s.status,
		ProviderSessionID: s.providerSessionID,
		CWD:               s.cwd,
		Model:             s.model,
		CreatedAtMS:       s.createdAtMS,
		UpdatedAtMS:       s.updatedAtMS,
		LastActivityAtMS:  s.updatedAtMS,
		LastError:         s.lastError,
	}
}

func (s *session) setStatus(status contract.SessionStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = status
	s.updatedAtMS = time.Now().UnixMilli()
}

func (s *session) setCWD(value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cwd = value
	s.updatedAtMS = time.Now().UnixMilli()
}

func (s *session) setModel(value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.model = value
	s.updatedAtMS = time.Now().UnixMilli()
}

func (s *session) setLastError(value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastError = value
	s.updatedAtMS = time.Now().UnixMilli()
}

func (s *session) setProviderSessionID(value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.providerSessionID = value
	s.updatedAtMS = time.Now().UnixMilli()
}

func (s *session) isStopping() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stopping
}

func (s *session) storePendingRequest(request pendingRequest) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingRequests[request.Request.RequestID] = request
}

func (s *session) pendingRequest(appRequestID string) (pendingRequest, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	request, ok := s.pendingRequests[appRequestID]
	return request, ok
}

func (s *session) removePendingRequest(appRequestID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.pendingRequests, appRequestID)
}

func claudeBridgeCommand() (string, []string, error) {
	if value := os.Getenv("AGENTIC_CONTROL_CLAUDE_BRIDGE"); strings.TrimSpace(value) != "" {
		return value, nil, nil
	}

	bridgePath, err := locateClaudeBridge()
	if err != nil {
		return "", nil, err
	}

	nodeBinary := os.Getenv("AGENTIC_CONTROL_NODE_BINARY")
	if strings.TrimSpace(nodeBinary) == "" {
		nodeBinary = "node"
	}
	return nodeBinary, []string{bridgePath}, nil
}

func locateClaudeBridge() (string, error) {
	candidates := make([]string, 0, 4)

	if executable, err := os.Executable(); err == nil {
		candidates = append(candidates,
			filepath.Clean(filepath.Join(filepath.Dir(executable), "..", "..", "internal", "controlplane", "providers", "claude", "sdkbridge", "bridge.mjs")),
			filepath.Clean(filepath.Join(filepath.Dir(executable), "internal", "controlplane", "providers", "claude", "sdkbridge", "bridge.mjs")),
		)
	}

	if workingDir, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(workingDir, "internal", "controlplane", "providers", "claude", "sdkbridge", "bridge.mjs"),
		)
	}

	if _, sourceFile, _, ok := runtime.Caller(0); ok {
		candidates = append(candidates,
			filepath.Join(filepath.Dir(sourceFile), "sdkbridge", "bridge.mjs"),
		)
	}

	for _, candidate := range candidates {
		if fileExists(candidate) {
			return candidate, nil
		}
	}

	return "", errors.New("could not locate the Claude SDK bridge; run `mise run build` from the repository root to install bridge dependencies")
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func claudeCodeBinaryPath() string {
	return os.Getenv("AGENTIC_CONTROL_CLAUDE_BINARY")
}

func canonicalID(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return asString
	}
	var asNumber json.Number
	if err := json.Unmarshal(raw, &asNumber); err == nil {
		return asNumber.String()
	}
	return string(raw)
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

func extractAssistantText(raw map[string]any) string {
	if message, ok := raw["message"].(map[string]any); ok {
		if content, ok := message["content"].([]any); ok {
			for _, item := range content {
				block, ok := item.(map[string]any)
				if !ok {
					continue
				}
				if block["type"] == "text" {
					if text, ok := block["text"].(string); ok {
						return truncate(text, 160)
					}
				}
			}
		}
	}
	return ""
}

func extractStreamDelta(raw map[string]any) string {
	event, ok := raw["event"].(map[string]any)
	if !ok {
		return ""
	}
	delta, ok := event["delta"].(map[string]any)
	if !ok {
		return ""
	}
	if text, ok := delta["text"].(string); ok {
		return text
	}
	return ""
}

func extractStringSlice(value any) []string {
	rawItems, ok := value.([]any)
	if !ok {
		return nil
	}
	items := make([]string, 0, len(rawItems))
	for _, rawItem := range rawItems {
		if item, ok := rawItem.(string); ok && strings.TrimSpace(item) != "" {
			items = append(items, item)
		}
	}
	return items
}

func mapClaudeState(state string) contract.SessionStatus {
	switch state {
	case "idle":
		return contract.SessionIdle
	case "running":
		return contract.SessionRunning
	case "requires_action":
		return contract.SessionWaitingApproval
	default:
		return ""
	}
}

func statusForRequestKind(kind contract.RequestKind) contract.SessionStatus {
	if strings.HasPrefix(string(kind), "user_input") {
		return contract.SessionWaitingUserInput
	}
	return contract.SessionWaitingApproval
}

func requestToolFromMetadata(metadata map[string]any) *contract.RequestToolContext {
	if len(metadata) == 0 {
		return nil
	}

	toolName := valueAsString(metadata["tool_name"])
	title := valueAsString(metadata["title"])
	description := valueAsString(metadata["description"])
	command := ""
	if input, ok := metadata["tool_input"].(map[string]any); ok {
		command = valueAsString(input["command"])
	}

	if toolName == "" && title == "" && description == "" && command == "" {
		return nil
	}
	return &contract.RequestToolContext{
		Name:        toolName,
		Title:       title,
		Command:     command,
		Description: description,
	}
}

func requestQuestionsFromMetadata(metadata map[string]any) []contract.RequestQuestion {
	rawQuestions, ok := metadata["questions"].([]any)
	if !ok || len(rawQuestions) == 0 {
		if typed, ok := metadata["questions"].([]map[string]any); ok {
			return requestQuestionsFromMaps(typed)
		}
		return nil
	}

	mapped := make([]map[string]any, 0, len(rawQuestions))
	for _, question := range rawQuestions {
		asMap, ok := question.(map[string]any)
		if !ok {
			continue
		}
		mapped = append(mapped, asMap)
	}
	return requestQuestionsFromMaps(mapped)
}

func requestQuestionsFromMaps(rawQuestions []map[string]any) []contract.RequestQuestion {
	if len(rawQuestions) == 0 {
		return nil
	}
	questions := make([]contract.RequestQuestion, 0, len(rawQuestions))
	for _, raw := range rawQuestions {
		options := make([]contract.RequestOption, 0)
		if rawOptions, ok := raw["options"].([]any); ok {
			for _, option := range rawOptions {
				switch value := option.(type) {
				case string:
					options = append(options, contract.RequestOption{ID: value, Label: value})
				case map[string]any:
					id := valueAsString(value["id"])
					label := valueAsString(value["label"])
					if label == "" {
						label = valueAsString(value["title"])
					}
					if id == "" {
						id = label
					}
					options = append(options, contract.RequestOption{
						ID:          id,
						Label:       label,
						Description: valueAsString(value["description"]),
					})
				}
			}
		}

		prompt := valueAsString(raw["question"])
		if prompt == "" {
			prompt = valueAsString(raw["prompt"])
		}
		questionID := valueAsString(raw["id"])
		if questionID == "" && prompt != "" {
			questionID = prompt
		}
		questions = append(questions, contract.RequestQuestion{
			ID:          questionID,
			Prompt:      prompt,
			Description: valueAsString(raw["description"]),
			Required:    valueAsBool(raw["required"]),
			Options:     options,
		})
	}
	if len(questions) == 0 {
		return nil
	}
	return questions
}

func valueAsBool(value any) bool {
	boolean, _ := value.(bool)
	return boolean
}

func valueAsString(value any) string {
	text, _ := value.(string)
	return text
}

func nilIfEmptyMap(values map[string]any) map[string]any {
	if len(values) == 0 {
		return nil
	}
	return values
}
