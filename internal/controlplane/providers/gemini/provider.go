package gemini

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

const (
	runtimeName     = "gemini"
	protocolVersion = 1
)

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
	title             string
	createdAtMS       int64
	updatedAtMS       int64
	lastError         string
	stopping          bool
	prompting         bool
	nextRequestID     int64
	pendingCalls      map[string]chan rpcMessage
	pendingRequests   map[string]pendingRequest
	writeMu           sync.Mutex
	provider          *Provider
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

type permissionOption struct {
	OptionID string `json:"optionId"`
	Kind     string `json:"kind"`
	Name     string `json:"name"`
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
		contract.TransportACP,
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
			UserInputRequests:        false,
			ImmediateProviderSession: true,
			ResumeByProviderID:       true,
		},
	)
}

func (p *Provider) StartSession(
	ctx context.Context,
	request api.StartSessionRequest,
) (*contract.RuntimeSession, error) {
	resolvedCWD, err := resolveCWD(request.CWD)
	if err != nil {
		return nil, err
	}

	sess, err := p.spawnProcess(ctx, request.SessionID, resolvedCWD, request.Model)
	if err != nil {
		return nil, err
	}

	var response struct {
		SessionID string `json:"sessionId"`
		Models    struct {
			CurrentModelID string `json:"currentModelId"`
		} `json:"models"`
	}
	if err := sess.call(ctx, "session/new", map[string]any{
		"cwd":        resolvedCWD,
		"mcpServers": []any{},
	}, &response); err != nil {
		sess.close()
		return nil, err
	}

	sess.setProviderSessionID(response.SessionID)
	sess.setStatus(contract.SessionIdle)
	if response.Models.CurrentModelID != "" {
		sess.setModel(response.Models.CurrentModelID)
	}

	if request.Model != "" {
		if err := sess.setRemoteModel(ctx, request.Model); err != nil {
			sess.close()
			return nil, err
		}
	}

	p.mu.Lock()
	p.sessions[request.SessionID] = sess
	p.mu.Unlock()

	p.emit(p.newEvent(sess, "session.started", "session/new", "",
		"Started Gemini ACP session",
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
	resolvedCWD, err := resolveCWD(request.CWD)
	if err != nil {
		return nil, err
	}

	sess, err := p.spawnProcess(ctx, request.SessionID, resolvedCWD, request.Model)
	if err != nil {
		return nil, err
	}

	var response struct {
		Models struct {
			CurrentModelID string `json:"currentModelId"`
		} `json:"models"`
	}
	if err := sess.call(ctx, "session/load", map[string]any{
		"sessionId":  request.ProviderSessionID,
		"cwd":        resolvedCWD,
		"mcpServers": []any{},
	}, &response); err != nil {
		sess.close()
		return nil, err
	}

	sess.setProviderSessionID(request.ProviderSessionID)
	sess.setStatus(contract.SessionIdle)
	if response.Models.CurrentModelID != "" {
		sess.setModel(response.Models.CurrentModelID)
	}

	if request.Model != "" {
		if err := sess.setRemoteModel(ctx, request.Model); err != nil {
			sess.close()
			return nil, err
		}
	}

	p.mu.Lock()
	p.sessions[request.SessionID] = sess
	p.mu.Unlock()

	p.emit(p.newEvent(sess, "session.started", "session/load", "",
		"Resumed Gemini ACP session",
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
	if sess.providerSessionIDSnapshot() == "" {
		return nil, errors.New("gemini session is not initialized")
	}
	if sess.promptingSnapshot() {
		return nil, errors.New("gemini session already has an active prompt")
	}

	responseChannel, err := sess.beginCall("session/prompt", map[string]any{
		"sessionId": sess.providerSessionIDSnapshot(),
		"prompt": []map[string]any{
			{
				"type": "text",
				"text": request.Text,
			},
		},
	})
	if err != nil {
		return nil, err
	}

	sess.setPrompting(true)
	sess.setStatus(contract.SessionRunning)

	event := p.newEvent(sess, "turn.started", "session/prompt", "",
		fmt.Sprintf("Started Gemini turn: %s", truncate(request.Text, 120)),
		map[string]any{"status": string(contract.SessionRunning)},
	)
	p.emit(event)

	go sess.finishPrompt(ctx, responseChannel)

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
	if !sess.promptingSnapshot() {
		return nil, errors.New("gemini session has no active prompt")
	}
	if err := sess.notify("session/cancel", map[string]any{
		"sessionId": sess.providerSessionIDSnapshot(),
	}); err != nil {
		return nil, err
	}

	event := p.newEvent(sess, "turn.interrupt_requested", "session/cancel", "",
		"Requested Gemini turn cancellation",
		nil,
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
	if pending.Method != "session/request_permission" {
		return nil, fmt.Errorf("unsupported Gemini request method: %s", pending.Method)
	}

	result, err := buildPermissionResponse(pending, request)
	if err != nil {
		return nil, err
	}
	if err := sess.writeResponse(pending.NativeID, result); err != nil {
		return nil, err
	}
	sess.removePendingRequest(request.RequestID)
	sess.setStatus(contract.SessionRunning)

	event := p.newEvent(sess, "request.responded", pending.Method, "",
		"Responded to Gemini permission request",
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
	sess.close()
	p.deleteSession(sessionID)

	event := p.newEvent(sess, "session.stopped", "process/stop", "",
		"Stopped Gemini ACP session",
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
	command := exec.CommandContext(ctx, geminiBinaryPath(), "--acp")
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
		"protocolVersion": protocolVersion,
		"clientCapabilities": map[string]any{
			"auth": map[string]any{
				"terminal": false,
			},
			"fs": map[string]any{
				"readTextFile":  false,
				"writeTextFile": false,
			},
			"terminal": false,
		},
		"clientInfo": map[string]any{
			"name":    "agentic-control",
			"version": "0.1.0",
		},
	}, nil); err != nil {
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

func (s *session) finishPrompt(ctx context.Context, responseChannel <-chan rpcMessage) {
	var response struct {
		StopReason string         `json:"stopReason"`
		Meta       map[string]any `json:"_meta"`
	}

	err := awaitCall(ctx, responseChannel, &response)

	s.setPrompting(false)

	if err != nil {
		s.setStatus(contract.SessionIdle)
		s.setLastError(err.Error())
		s.provider.emit(s.provider.newEvent(s, "turn.errored", "session/prompt", "",
			fmt.Sprintf("Gemini prompt failed: %s", err),
			map[string]any{
				"status":     string(contract.SessionIdle),
				"last_error": err.Error(),
			},
		))
		return
	}

	payload := map[string]any{
		"stop_reason": response.StopReason,
	}
	if response.Meta != nil {
		payload["meta"] = response.Meta
	}

	if response.StopReason == "cancelled" {
		s.setStatus(contract.SessionInterrupted)
		payload["status"] = string(contract.SessionInterrupted)
		s.provider.emit(s.provider.newEvent(s, "turn.interrupted", "session/prompt", "",
			"Gemini turn cancelled",
			payload,
		))
		return
	}

	s.setStatus(contract.SessionIdle)
	payload["status"] = string(contract.SessionIdle)
	s.provider.emit(s.provider.newEvent(s, "turn.completed", "session/prompt", "",
		fmt.Sprintf("Gemini turn completed: %s", response.StopReason),
		payload,
	))
}

func (s *session) setRemoteModel(ctx context.Context, model string) error {
	if err := s.call(ctx, "session/set_model", map[string]any{
		"sessionId": s.providerSessionIDSnapshot(),
		"modelId":   model,
	}, nil); err != nil {
		return err
	}
	s.setModel(model)
	return nil
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
				fmt.Sprintf("Failed to decode Gemini ACP message: %v", err),
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
	summary := "Gemini ACP session stopped"
	if err != nil && !stopping {
		status = contract.SessionErrored
		payload["status"] = string(status)
		payload["last_error"] = err.Error()
		summary = fmt.Sprintf("Gemini ACP session exited: %s", err)
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
	case "session/update":
		var params struct {
			SessionID string         `json:"sessionId"`
			Update    map[string]any `json:"update"`
		}
		if err := json.Unmarshal(message.Params, &params); err != nil {
			s.provider.emit(s.provider.newEvent(s, "runtime.decode_error", message.Method, "",
				fmt.Sprintf("Failed to decode Gemini notification: %v", err), nil))
			return
		}
		if params.SessionID != "" {
			s.setProviderSessionID(params.SessionID)
		}
		updateType, _ := params.Update["sessionUpdate"].(string)
		switch updateType {
		case "agent_message_chunk":
			s.setStatus(contract.SessionRunning)
			text := extractAcpText(params.Update["content"])
			s.provider.emit(s.provider.newEvent(s, "assistant.message.delta", message.Method, "",
				truncate(coalesce(text, "Gemini assistant chunk"), 160),
				map[string]any{
					"delta":  text,
					"status": string(contract.SessionRunning),
				},
			))
		case "agent_thought_chunk":
			text := extractAcpText(params.Update["content"])
			s.provider.emit(s.provider.newEvent(s, "assistant.thought.delta", message.Method, "",
				truncate(coalesce(text, "Gemini thought chunk"), 160),
				map[string]any{"delta": text},
			))
		case "user_message_chunk":
			text := extractAcpText(params.Update["content"])
			s.provider.emit(s.provider.newEvent(s, "turn.input.acknowledged", message.Method, "",
				truncate(coalesce(text, "Gemini input acknowledged"), 160),
				nil,
			))
		case "tool_call":
			title, _ := params.Update["title"].(string)
			status, _ := params.Update["status"].(string)
			s.provider.emit(s.provider.newEvent(s, "tool_call.opened", message.Method, "",
				fmt.Sprintf("Gemini tool call %s: %s", coalesce(status, "opened"), coalesce(title, "tool")),
				map[string]any{
					"tool_call_id": valueAsString(params.Update["toolCallId"]),
					"status":       status,
					"kind":         valueAsString(params.Update["kind"]),
				},
			))
		case "tool_call_update":
			title, _ := params.Update["title"].(string)
			status, _ := params.Update["status"].(string)
			s.provider.emit(s.provider.newEvent(s, "tool_call.updated", message.Method, "",
				fmt.Sprintf("Gemini tool call %s: %s", coalesce(status, "updated"), coalesce(title, "tool")),
				map[string]any{
					"tool_call_id": valueAsString(params.Update["toolCallId"]),
					"status":       status,
					"kind":         valueAsString(params.Update["kind"]),
				},
			))
		case "available_commands_update":
			s.provider.emit(s.provider.newEvent(s, "session.commands.updated", message.Method, "",
				"Gemini available commands updated",
				map[string]any{"available_commands": params.Update["availableCommands"]},
			))
		case "current_mode_update":
			s.provider.emit(s.provider.newEvent(s, "session.mode.updated", message.Method, "",
				fmt.Sprintf("Gemini mode changed: %s", valueAsString(params.Update["currentModeId"])),
				map[string]any{"current_mode_id": params.Update["currentModeId"]},
			))
		case "config_option_update":
			s.provider.emit(s.provider.newEvent(s, "session.config.updated", message.Method, "",
				"Gemini session config updated",
				map[string]any{"config_options": params.Update["configOptions"]},
			))
		case "session_info_update":
			if title, _ := params.Update["title"].(string); title != "" {
				s.setTitle(title)
			}
			s.provider.emit(s.provider.newEvent(s, "session.info.updated", message.Method, "",
				"Gemini session info updated",
				map[string]any{
					"title":      params.Update["title"],
					"updated_at": params.Update["updatedAt"],
				},
			))
		case "plan":
			s.provider.emit(s.provider.newEvent(s, "plan.updated", message.Method, "",
				"Gemini plan updated",
				map[string]any{"plan": params.Update["entries"]},
			))
		case "usage_update":
			s.provider.emit(s.provider.newEvent(s, "usage.updated", message.Method, "",
				"Gemini usage updated",
				map[string]any{
					"used": params.Update["used"],
					"size": params.Update["size"],
					"cost": params.Update["cost"],
				},
			))
		default:
			s.provider.emit(s.provider.newEvent(s, "runtime.event", message.Method, "",
				fmt.Sprintf("Gemini notification: %s", updateType),
				map[string]any{"update": params.Update},
			))
		}
	default:
		s.provider.emit(s.provider.newEvent(s, "runtime.event", message.Method, "",
			fmt.Sprintf("Gemini notification: %s", message.Method),
			nil,
		))
	}
}

func (s *session) handleServerRequest(message rpcMessage) {
	switch message.Method {
	case "session/request_permission":
		var params struct {
			SessionID string             `json:"sessionId"`
			Options   []permissionOption `json:"options"`
			ToolCall  map[string]any     `json:"toolCall"`
		}
		if err := json.Unmarshal(message.Params, &params); err != nil {
			_ = s.writeError(message.ID, -32602, err.Error(), nil)
			return
		}
		if params.SessionID != "" {
			s.setProviderSessionID(params.SessionID)
		}

		appRequestID := fmt.Sprintf("req-%s", canonicalID(message.ID))
		summary := "Gemini requested permission"
		if title, _ := params.ToolCall["title"].(string); title != "" {
			summary = fmt.Sprintf("Gemini requested permission: %s", truncate(title, 160))
		}

		extensions := map[string]any{
			"options":   params.Options,
			"tool_call": params.ToolCall,
		}
		request := contract.PendingRequest{
			SchemaVersion: contract.ControlPlaneSchemaVersion,
			RequestID:     appRequestID,
			SessionID:     s.appSessionID,
			Runtime:       runtimeName,
			Kind:          contract.RequestApprovalTool,
			NativeMethod:  message.Method,
			Status:        contract.RequestStatusOpen,
			Summary:       summary,
			CreatedAtMS:   time.Now().UnixMilli(),
			Tool: &contract.RequestToolContext{
				Title:       valueAsString(params.ToolCall["title"]),
				Kind:        valueAsString(params.ToolCall["kind"]),
				Description: valueAsString(params.ToolCall["status"]),
			},
			Options:    requestOptionsFromPermissionOptions(params.Options),
			Extensions: nilIfEmptyMap(extensions),
		}
		s.setStatus(contract.SessionWaitingApproval)
		s.storePendingRequest(pendingRequest{
			NativeID: message.ID,
			Method:   message.Method,
			Request:  request,
		})

		event := s.provider.newEvent(s, "request.opened", message.Method, "",
			summary,
			map[string]any{"status": string(contract.SessionWaitingApproval)},
		)
		event.RequestID = appRequestID
		event.Request = &request
		s.provider.emit(event)
	default:
		_ = s.writeError(message.ID, -32601, fmt.Sprintf("unsupported client request: %s", message.Method), map[string]any{"method": message.Method})
		s.provider.emit(s.provider.newEvent(s, "runtime.request.unhandled", message.Method, "",
			fmt.Sprintf("Unhandled Gemini client request: %s", message.Method),
			nil,
		))
	}
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
	responseChannel, err := s.beginCall(method, params)
	if err != nil {
		return err
	}
	return awaitCall(ctx, responseChannel, target)
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

func (s *session) writeError(id json.RawMessage, code int, message string, data any) error {
	var parsed any
	if len(id) > 0 {
		if err := json.Unmarshal(id, &parsed); err != nil {
			parsed = canonicalID(id)
		}
	}

	envelope := map[string]any{
		"code":    code,
		"message": message,
	}
	if data != nil {
		envelope["data"] = data
	}

	return s.writeJSON(map[string]any{
		"jsonrpc": "2.0",
		"id":      parsed,
		"error":   envelope,
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

func (s *session) beginCall(method string, params any) (<-chan rpcMessage, error) {
	requestID := strconv.FormatInt(s.nextID(), 10)
	responseChannel := make(chan rpcMessage, 1)

	s.mu.Lock()
	s.pendingCalls[requestID] = responseChannel
	s.mu.Unlock()

	if err := s.writeRequest(requestID, method, params); err != nil {
		s.mu.Lock()
		delete(s.pendingCalls, requestID)
		s.mu.Unlock()
		return nil, err
	}

	return responseChannel, nil
}

func (s *session) snapshot() *contract.RuntimeSession {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return &contract.RuntimeSession{
		SchemaVersion:     contract.ControlPlaneSchemaVersion,
		SessionID:         s.appSessionID,
		Runtime:           runtimeName,
		Ownership:         contract.OwnershipControlled,
		Transport:         contract.TransportACP,
		Status:            s.status,
		ProviderSessionID: s.providerSessionID,
		CWD:               s.cwd,
		Model:             s.model,
		Title:             s.title,
		CreatedAtMS:       s.createdAtMS,
		UpdatedAtMS:       s.updatedAtMS,
		LastActivityAtMS:  s.updatedAtMS,
		LastError:         s.lastError,
	}
}

func (s *session) setProviderSessionID(value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.providerSessionID = value
	s.updatedAtMS = time.Now().UnixMilli()
}

func (s *session) providerSessionIDSnapshot() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.providerSessionID
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

func (s *session) setPrompting(value bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prompting = value
	s.updatedAtMS = time.Now().UnixMilli()
}

func (s *session) promptingSnapshot() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.prompting
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

func buildPermissionResponse(pending pendingRequest, request api.RespondRequest) (map[string]any, error) {
	choice, err := selectPermissionOption(pending, request)
	if err != nil {
		return nil, err
	}
	if choice == "" {
		return map[string]any{
			"outcome": map[string]any{
				"outcome": "cancelled",
			},
		}, nil
	}
	return map[string]any{
		"outcome": map[string]any{
			"outcome":  "selected",
			"optionId": choice,
		},
	}, nil
}

func selectPermissionOption(pending pendingRequest, request api.RespondRequest) (string, error) {
	options, err := permissionOptionsFromRequest(pending.Request)
	if err != nil {
		return "", err
	}
	if len(options) == 0 {
		return "", errors.New("gemini permission response requires at least one option")
	}

	if value := strings.TrimSpace(request.OptionID); value != "" {
		for _, option := range options {
			if option.OptionID == value {
				return option.OptionID, nil
			}
		}
		return "", fmt.Errorf("unknown Gemini permission option: %s", value)
	}

	action := strings.ToLower(strings.TrimSpace(string(request.Action)))
	switch action {
	case "", "accept", "allow", "approve", "approve_once", "allow_once":
		if option, ok := firstOptionByKind(options, "allow_once"); ok {
			return option.OptionID, nil
		}
		if option, ok := firstOptionByKind(options, "allow_always"); ok {
			return option.OptionID, nil
		}
	case "approve_always", "allow_always":
		if option, ok := firstOptionByKind(options, "allow_always"); ok {
			return option.OptionID, nil
		}
	case "decline", "deny", "reject", "reject_once":
		if option, ok := firstOptionByKind(options, "reject_once"); ok {
			return option.OptionID, nil
		}
		if option, ok := firstOptionByKind(options, "reject_always"); ok {
			return option.OptionID, nil
		}
	case "reject_always", "deny_always":
		if option, ok := firstOptionByKind(options, "reject_always"); ok {
			return option.OptionID, nil
		}
	case "cancel", "interrupt":
		return "", nil
	default:
		for _, option := range options {
			if strings.EqualFold(option.OptionID, string(request.Action)) || strings.EqualFold(option.Name, string(request.Action)) {
				return option.OptionID, nil
			}
		}
	}

	return options[0].OptionID, nil
}

func requestOptionsFromPermissionOptions(options []permissionOption) []contract.RequestOption {
	if len(options) == 0 {
		return nil
	}
	result := make([]contract.RequestOption, 0, len(options))
	for index, option := range options {
		result = append(result, contract.RequestOption{
			ID:        option.OptionID,
			Label:     coalesce(option.Name, option.OptionID),
			Kind:      option.Kind,
			IsDefault: index == 0,
		})
	}
	return result
}

func permissionOptionsFromRequest(request contract.PendingRequest) ([]permissionOption, error) {
	if len(request.Options) > 0 {
		options := make([]permissionOption, 0, len(request.Options))
		for _, option := range request.Options {
			options = append(options, permissionOption{
				OptionID: option.ID,
				Kind:     option.Kind,
				Name:     option.Label,
			})
		}
		return options, nil
	}
	return permissionOptionsFromMetadata(request.Extensions["options"])
}

func nilIfEmptyMap(values map[string]any) map[string]any {
	if len(values) == 0 {
		return nil
	}
	return values
}

func permissionOptionsFromMetadata(raw any) ([]permissionOption, error) {
	if raw == nil {
		return nil, nil
	}

	switch values := raw.(type) {
	case []permissionOption:
		return values, nil
	case []any:
		options := make([]permissionOption, 0, len(values))
		for _, value := range values {
			asMap, ok := value.(map[string]any)
			if !ok {
				return nil, errors.New("invalid Gemini permission option payload")
			}
			options = append(options, permissionOption{
				OptionID: valueAsString(asMap["optionId"]),
				Kind:     valueAsString(asMap["kind"]),
				Name:     valueAsString(asMap["name"]),
			})
		}
		return options, nil
	default:
		encoded, err := json.Marshal(values)
		if err != nil {
			return nil, err
		}
		var options []permissionOption
		if err := json.Unmarshal(encoded, &options); err != nil {
			return nil, err
		}
		return options, nil
	}
}

func firstOptionByKind(options []permissionOption, kind string) (permissionOption, bool) {
	for _, option := range options {
		if option.Kind == kind {
			return option, true
		}
	}
	return permissionOption{}, false
}

func extractAcpText(raw any) string {
	asMap, ok := raw.(map[string]any)
	if !ok {
		return ""
	}
	return valueAsString(asMap["text"])
}

func valueAsString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	case json.Number:
		return typed.String()
	default:
		return ""
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

func geminiBinaryPath() string {
	if value := os.Getenv("AGENTIC_CONTROL_GEMINI_BINARY"); value != "" {
		return value
	}
	return "gemini"
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

func resolveCWD(value string) (string, error) {
	if strings.TrimSpace(value) != "" {
		return value, nil
	}
	return os.Getwd()
}

func awaitCall(ctx context.Context, responseChannel <-chan rpcMessage, target any) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case response := <-responseChannel:
		if response.Error != nil {
			return fmt.Errorf("gemini rpc error %d: %s", response.Error.Code, response.Error.Message)
		}
		if target == nil || len(response.Result) == 0 {
			return nil
		}
		return json.Unmarshal(response.Result, target)
	}
}
