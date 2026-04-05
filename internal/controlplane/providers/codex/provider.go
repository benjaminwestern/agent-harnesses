package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/benjaminwestern/agentic-control/pkg/contract"
	api "github.com/benjaminwestern/agentic-control/pkg/controlplane"
)

const runtimeName = "codex"

type eventSink func(contract.RuntimeEvent)

type Provider struct {
	mu       sync.RWMutex
	sessions map[string]*session
	emit     eventSink
}

type session struct {
	mu               sync.RWMutex
	appSessionID     string
	cmd              *exec.Cmd
	stdin            io.WriteCloser
	providerThreadID string
	activeTurnID     string
	status           contract.SessionStatus
	cwd              string
	model            string
	createdAtMS      int64
	updatedAtMS      int64
	lastError        string
	stopping         bool
	nextRequestID    int64
	pendingCalls     map[string]chan rpcMessage
	pendingRequests  map[string]pendingRequest
	writeMu          sync.Mutex
	provider         *Provider
}

type pendingRequest struct {
	NativeID json.RawMessage
	Method   string
	Request  contract.PendingRequest
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
		contract.TransportAppServer,
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

	params := map[string]any{}
	if request.CWD != "" {
		params["cwd"] = request.CWD
	}
	if request.Model != "" {
		params["model"] = request.Model
	}

	var response struct {
		Thread struct {
			ID  string `json:"id"`
			CWD string `json:"cwd"`
		} `json:"thread"`
		Model string `json:"model"`
	}
	if err := sess.call(ctx, "thread/start", params, &response); err != nil {
		sess.close()
		return nil, err
	}
	sess.setThreadID(response.Thread.ID)
	if response.Thread.CWD != "" {
		sess.setCWD(response.Thread.CWD)
	}
	if response.Model != "" {
		sess.setModel(response.Model)
	}
	sess.setStatus(contract.SessionIdle)

	p.mu.Lock()
	p.sessions[request.SessionID] = sess
	p.mu.Unlock()

	p.emit(p.newEvent(sess, "session.started", "thread/start", "",
		"Started Codex controller session",
		map[string]any{"status": string(contract.SessionIdle)},
	))
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

	params := map[string]any{
		"threadId": request.ProviderSessionID,
	}
	if request.CWD != "" {
		params["cwd"] = request.CWD
	}
	if request.Model != "" {
		params["model"] = request.Model
	}

	var response struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if err := sess.call(ctx, "thread/resume", params, &response); err != nil {
		sess.close()
		return nil, err
	}
	sess.setThreadID(response.Thread.ID)
	if request.ProviderSessionID != "" {
		sess.setThreadID(request.ProviderSessionID)
	}
	sess.setStatus(contract.SessionIdle)

	p.mu.Lock()
	p.sessions[request.SessionID] = sess
	p.mu.Unlock()

	p.emit(p.newEvent(sess, "session.started", "thread/resume", "",
		"Resumed Codex controller session",
		map[string]any{"status": string(contract.SessionIdle)},
	))

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
	threadID := sess.threadID()
	if threadID == "" {
		return nil, errors.New("codex session is not initialized")
	}

	params := map[string]any{
		"threadId": threadID,
		"input": []map[string]any{
			{
				"type":          "text",
				"text":          request.Text,
				"text_elements": []any{},
			},
		},
	}

	var response struct {
		Turn struct {
			ID string `json:"id"`
		} `json:"turn"`
	}
	if err := sess.call(ctx, "turn/start", params, &response); err != nil {
		return nil, err
	}
	sess.setTurnID(response.Turn.ID)
	sess.setStatus(contract.SessionRunning)

	event := sess.provider.newEvent(sess, "turn.started", "turn/start", response.Turn.ID,
		fmt.Sprintf("Started Codex turn: %s", truncate(request.Text, 120)),
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

	threadID := sess.threadID()
	turnID := sess.turnID()
	if threadID == "" || turnID == "" {
		return nil, errors.New("codex session has no active turn")
	}

	if err := sess.call(ctx, "turn/interrupt", map[string]any{
		"threadId": threadID,
		"turnId":   turnID,
	}, nil); err != nil {
		return nil, err
	}
	sess.setStatus(contract.SessionInterrupted)

	event := p.newEvent(sess, "turn.interrupted", "turn/interrupt", turnID,
		"Interrupted Codex turn",
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

	result, err := buildResponseResult(pending, request)
	if err != nil {
		return nil, err
	}
	if err := sess.writeResponse(pending.NativeID, result); err != nil {
		return nil, err
	}
	sess.removePendingRequest(request.RequestID)

	event := p.newEvent(sess, "request.responded", pending.Method, pending.Request.TurnID,
		fmt.Sprintf("Responded to Codex request: %s", pending.Method),
		nil,
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
	sess.close()
	p.deleteSession(sessionID)

	event := p.newEvent(sess, "session.stopped", "process/stop", "",
		"Stopped Codex controller session",
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
	command := exec.CommandContext(ctx, codexBinaryPath(), "app-server")
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

	if err := sess.call(ctx, "initialize", map[string]any{
		"clientInfo": map[string]any{
			"name":    "agentic-control",
			"version": "0.1.0",
		},
		"capabilities": map[string]any{
			"experimentalApi": true,
		},
	}, nil); err != nil {
		sess.close()
		return nil, err
	}
	if err := sess.notify("initialized", nil); err != nil {
		sess.close()
		return nil, err
	}

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
				fmt.Sprintf("Failed to decode Codex message: %v", err), map[string]any{
					"status":     string(contract.SessionErrored),
					"last_error": err.Error(),
				}))
			continue
		}

		if len(message.Result) > 0 || message.Error != nil {
			s.resolvePendingCall(message)
			continue
		}

		if message.Method == "" {
			continue
		}
		if len(message.ID) > 0 {
			s.handleServerRequest(message)
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
			truncate(line, 200),
			nil,
		))
	}
}

func (s *session) waitLoop() {
	err := s.cmd.Wait()
	stopping := s.isStopping()
	status := contract.SessionStopped
	payload := map[string]any{
		"status": string(status),
	}
	summary := "Codex controller session stopped"
	if err != nil && !stopping {
		status = contract.SessionErrored
		payload["status"] = string(status)
		payload["last_error"] = err.Error()
		summary = fmt.Sprintf("Codex controller session exited: %s", err)
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
	case "thread/started":
		var params struct {
			Thread struct {
				ID string `json:"id"`
			} `json:"thread"`
		}
		if json.Unmarshal(message.Params, &params) == nil && params.Thread.ID != "" {
			s.setThreadID(params.Thread.ID)
		}
	case "thread/status/changed":
		var params struct {
			ThreadID string         `json:"threadId"`
			Status   map[string]any `json:"status"`
		}
		if json.Unmarshal(message.Params, &params) == nil {
			status := statusFromCodex(params.Status)
			s.setStatus(status)
			s.provider.emit(s.provider.newEvent(s, "session.status_changed", message.Method, "",
				fmt.Sprintf("Codex session status changed: %s", status),
				map[string]any{"status": string(status)},
			))
		}
	case "turn/started":
		var params struct {
			Turn struct {
				ID string `json:"id"`
			} `json:"turn"`
		}
		if json.Unmarshal(message.Params, &params) == nil {
			s.setTurnID(params.Turn.ID)
			s.setStatus(contract.SessionRunning)
			s.provider.emit(s.provider.newEvent(s, "turn.started", message.Method, params.Turn.ID,
				"Codex turn started",
				map[string]any{"status": string(contract.SessionRunning)},
			))
		}
	case "turn/completed":
		var params struct {
			Turn struct {
				ID string `json:"id"`
			} `json:"turn"`
		}
		if json.Unmarshal(message.Params, &params) == nil {
			s.clearTurnID()
			s.setStatus(contract.SessionIdle)
			s.provider.emit(s.provider.newEvent(s, "turn.completed", message.Method, params.Turn.ID,
				"Codex turn completed",
				map[string]any{"status": string(contract.SessionIdle)},
			))
		}
	case "item/agentMessage/delta":
		var params struct {
			Delta  string `json:"delta"`
			TurnID string `json:"turnId"`
		}
		if json.Unmarshal(message.Params, &params) == nil {
			s.provider.emit(s.provider.newEvent(s, "assistant.message.delta", message.Method, params.TurnID,
				truncate(params.Delta, 160),
				map[string]any{"delta": params.Delta},
			))
		}
	case "serverRequest/resolved":
		s.provider.emit(s.provider.newEvent(s, "request.resolved", message.Method, "",
			"Codex server request resolved",
			nil,
		))
	default:
		s.provider.emit(s.provider.newEvent(s, "runtime.event", message.Method, "",
			fmt.Sprintf("Codex notification: %s", message.Method),
			nil,
		))
	}
}

func (s *session) handleServerRequest(message rpcMessage) {
	appRequestID := fmt.Sprintf("req-%s", canonicalID(message.ID))
	var turnID string
	extensions := map[string]any{}
	summary := fmt.Sprintf("Codex request: %s", message.Method)
	kind := contract.RequestGeneric
	status := contract.SessionWaitingApproval
	var tool *contract.RequestToolContext
	var questions []contract.RequestQuestion

	switch message.Method {
	case "item/commandExecution/requestApproval":
		kind = contract.RequestApprovalCommand
		var params struct {
			Command *string `json:"command"`
			TurnID  string  `json:"turnId"`
			Reason  *string `json:"reason"`
		}
		if json.Unmarshal(message.Params, &params) == nil {
			turnID = params.TurnID
			if params.Command != nil {
				summary = fmt.Sprintf("Codex requested command approval: %s", truncate(*params.Command, 160))
				tool = &contract.RequestToolContext{
					Name:    "command",
					Title:   truncate(*params.Command, 160),
					Command: *params.Command,
				}
				extensions["command"] = *params.Command
			}
			if params.Reason != nil {
				extensions["reason"] = *params.Reason
			}
		}
	case "item/fileChange/requestApproval":
		kind = contract.RequestApprovalFileChange
		var params struct {
			TurnID string  `json:"turnId"`
			Reason *string `json:"reason"`
		}
		if json.Unmarshal(message.Params, &params) == nil {
			turnID = params.TurnID
			if params.Reason != nil {
				summary = fmt.Sprintf("Codex requested file approval: %s", truncate(*params.Reason, 160))
				tool = &contract.RequestToolContext{
					Name:  "file_change",
					Title: truncate(*params.Reason, 160),
				}
				extensions["reason"] = *params.Reason
			}
		}
	case "item/permissions/requestApproval":
		kind = contract.RequestApprovalPermissions
		var params struct {
			TurnID      string         `json:"turnId"`
			Reason      *string        `json:"reason"`
			Permissions map[string]any `json:"permissions"`
		}
		if json.Unmarshal(message.Params, &params) == nil {
			turnID = params.TurnID
			if params.Reason != nil {
				summary = fmt.Sprintf("Codex requested permissions: %s", truncate(*params.Reason, 160))
				extensions["reason"] = *params.Reason
			}
			if params.Permissions != nil {
				extensions["permissions"] = params.Permissions
			}
		}
	case "item/tool/requestUserInput":
		kind = contract.RequestUserInputTool
		status = contract.SessionWaitingUserInput
		var params struct {
			TurnID    string `json:"turnId"`
			Questions []struct {
				ID       string   `json:"id"`
				Question string   `json:"question"`
				Options  []string `json:"options"`
			} `json:"questions"`
		}
		if json.Unmarshal(message.Params, &params) == nil {
			turnID = params.TurnID
			for _, question := range params.Questions {
				questions = append(questions, contract.RequestQuestion{
					ID:      question.ID,
					Prompt:  question.Question,
					Options: requestOptionsFromStrings(question.Options),
				})
			}
			extensions["questions"] = params.Questions
			if len(params.Questions) > 0 {
				summary = fmt.Sprintf("Codex requested user input: %s", truncate(params.Questions[0].Question, 160))
			}
		}
	case "mcpServer/elicitation/request":
		kind = contract.RequestUserInputMCP
		status = contract.SessionWaitingUserInput
		summary = "Codex requested MCP elicitation input"
	}

	pending := contract.PendingRequest{
		SchemaVersion: contract.ControlPlaneSchemaVersion,
		RequestID:     appRequestID,
		SessionID:     s.appSessionID,
		Runtime:       runtimeName,
		Kind:          kind,
		NativeMethod:  message.Method,
		Status:        contract.RequestStatusOpen,
		Summary:       summary,
		TurnID:        turnID,
		CreatedAtMS:   time.Now().UnixMilli(),
		Tool:          tool,
		Questions:     questions,
		Extensions:    nilIfEmptyMap(extensions),
	}

	s.setStatus(status)
	s.storePendingRequest(pendingRequest{
		NativeID: message.ID,
		Method:   message.Method,
		Request:  pending,
	})

	event := s.provider.newEvent(s, "request.opened", message.Method, turnID, summary,
		map[string]any{"status": string(status)})
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
			return fmt.Errorf("codex rpc error %d: %s", response.Error.Code, response.Error.Message)
		}
		if target == nil || len(response.Result) == 0 {
			return nil
		}
		return json.Unmarshal(response.Result, target)
	}
}

func (s *session) notify(method string, params any) error {
	return s.writeJSON(map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	})
}

func (s *session) writeRequest(id string, method string, params any) error {
	return s.writeJSON(map[string]any{
		"jsonrpc": "2.0",
		"id":      json.Number(id),
		"method":  method,
		"params":  params,
	})
}

func (s *session) writeResponse(id json.RawMessage, result any) error {
	var parsed any
	if len(id) > 0 {
		if err := json.Unmarshal(id, &parsed); err != nil {
			parsed = canonicalID(id)
		}
	}
	return s.writeJSON(map[string]any{
		"jsonrpc": "2.0",
		"id":      parsed,
		"result":  result,
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
		Transport:         contract.TransportAppServer,
		Status:            s.status,
		ProviderSessionID: s.providerThreadID,
		ActiveTurnID:      s.activeTurnID,
		CWD:               s.cwd,
		Model:             s.model,
		CreatedAtMS:       s.createdAtMS,
		UpdatedAtMS:       s.updatedAtMS,
		LastActivityAtMS:  s.updatedAtMS,
		LastError:         s.lastError,
	}
}

func (s *session) threadID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.providerThreadID
}

func (s *session) turnID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.activeTurnID
}

func (s *session) setThreadID(value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.providerThreadID = value
	s.updatedAtMS = time.Now().UnixMilli()
}

func (s *session) setTurnID(value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activeTurnID = value
	s.updatedAtMS = time.Now().UnixMilli()
}

func (s *session) clearTurnID() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activeTurnID = ""
	s.updatedAtMS = time.Now().UnixMilli()
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

func buildResponseResult(pending pendingRequest, request api.RespondRequest) (map[string]any, error) {
	switch pending.Method {
	case "item/commandExecution/requestApproval":
		return map[string]any{"decision": normalizeApprovalAction(request.Action)}, nil
	case "item/fileChange/requestApproval":
		return map[string]any{"decision": normalizeApprovalAction(request.Action)}, nil
	case "item/permissions/requestApproval":
		permissions, ok := pending.Request.Extensions["permissions"]
		if !ok {
			return nil, errors.New("permissions response requires request extensions.permissions")
		}
		result := map[string]any{"permissions": permissions}
		if scope, ok := request.Metadata["scope"]; ok {
			result["scope"] = scope
		}
		return result, nil
	case "item/tool/requestUserInput":
		if len(request.Answers) > 0 {
			return map[string]any{"answers": requestAnswersPayload(request.Answers)}, nil
		}
		if len(pending.Request.Questions) == 0 || request.Text == "" {
			return nil, errors.New("user input response requires text or answers")
		}
		return map[string]any{
			"answers": map[string]any{
				pending.Request.Questions[0].ID: map[string]any{"answers": []string{request.Text}},
			},
		}, nil
	case "mcpServer/elicitation/request":
		result := map[string]any{
			"action": normalizeElicitationAction(request.Action),
		}
		if content, ok := request.Metadata["content"]; ok {
			result["content"] = content
		}
		return result, nil
	default:
		return nil, fmt.Errorf("unsupported Codex request method: %s", pending.Method)
	}
}

func normalizeApprovalAction(value contract.RespondAction) string {
	switch strings.ToLower(strings.TrimSpace(string(value))) {
	case "", "accept", "allow", "approve":
		return "accept"
	case "acceptforsession", "accept_for_session", "allow_for_session":
		return "acceptForSession"
	case "decline", "deny", "reject":
		return "decline"
	case "cancel", "interrupt":
		return "cancel"
	default:
		return string(value)
	}
}

func normalizeElicitationAction(value contract.RespondAction) string {
	switch strings.ToLower(strings.TrimSpace(string(value))) {
	case "", "accept", "allow", "approve":
		return "accept"
	case "decline", "deny", "reject":
		return "decline"
	case "cancel", "interrupt":
		return "cancel"
	default:
		return string(value)
	}
}

func requestOptionsFromStrings(options []string) []contract.RequestOption {
	if len(options) == 0 {
		return nil
	}
	result := make([]contract.RequestOption, 0, len(options))
	for _, option := range options {
		result = append(result, contract.RequestOption{
			ID:    option,
			Label: option,
		})
	}
	return result
}

func requestAnswersPayload(answers []contract.RequestAnswer) []map[string]any {
	result := make([]map[string]any, 0, len(answers))
	for _, answer := range answers {
		entry := map[string]any{}
		if answer.QuestionID != "" {
			entry["question_id"] = answer.QuestionID
		}
		if answer.OptionID != "" {
			entry["option_id"] = answer.OptionID
		}
		if answer.Text != "" {
			entry["text"] = answer.Text
		}
		result = append(result, entry)
	}
	return result
}

func nilIfEmptyMap(values map[string]any) map[string]any {
	if len(values) == 0 {
		return nil
	}
	return values
}

func statusFromCodex(status map[string]any) contract.SessionStatus {
	statusType, _ := status["type"].(string)
	switch statusType {
	case "idle":
		return contract.SessionIdle
	case "systemError":
		return contract.SessionErrored
	case "active":
		flags, _ := status["activeFlags"].([]any)
		for _, flag := range flags {
			flagString, _ := flag.(string)
			switch flagString {
			case "waitingOnApproval":
				return contract.SessionWaitingApproval
			case "waitingOnUserInput":
				return contract.SessionWaitingUserInput
			}
		}
		return contract.SessionRunning
	default:
		return contract.SessionRunning
	}
}

func canonicalID(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return asString
	}
	return strings.TrimSpace(string(raw))
}

func codexBinaryPath() string {
	if value := os.Getenv("AGENTIC_CONTROL_CODEX_BINARY"); value != "" {
		return value
	}
	return "codex"
}

func truncate(value string, maxLen int) string {
	if len(value) <= maxLen {
		return value
	}
	return value[:maxLen]
}
