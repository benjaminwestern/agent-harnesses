package controlplane

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/benjaminwestern/agentic-control/pkg/contract"
	api "github.com/benjaminwestern/agentic-control/pkg/controlplane"
)

type RPCServer struct {
	service *Service
}

func NewRPCServer(service *Service) *RPCServer {
	return &RPCServer{service: service}
}

func (s *RPCServer) ServeUnix(ctx context.Context, socketPath string) error {
	if err := os.MkdirAll(filepath.Dir(socketPath), 0o755); err != nil {
		return err
	}
	_ = os.Remove(socketPath)

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return err
	}
	defer func() {
		_ = listener.Close()
		_ = os.Remove(socketPath)
	}()

	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	for {
		connection, err := listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) || ctx.Err() != nil {
				return nil
			}
			return err
		}
		go s.handleConnection(ctx, connection)
	}
}

type rpcRequest struct {
	JSONRPC         string          `json:"jsonrpc,omitempty"`
	ID              json.RawMessage `json:"id,omitempty"`
	Method          string          `json:"method"`
	Params          json.RawMessage `json:"params,omitempty"`
	ExpectsResponse bool            `json:"-"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc,omitempty"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcNotification struct {
	JSONRPC string `json:"jsonrpc,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcParamError struct {
	err error
}

func (e rpcParamError) Error() string {
	if e.err == nil {
		return "invalid params"
	}
	return e.err.Error()
}

func (r rpcResponse) MarshalJSON() ([]byte, error) {
	type alias rpcResponse
	if strings.TrimSpace(r.JSONRPC) == "" {
		r.JSONRPC = "2.0"
	}
	return json.Marshal(alias(r))
}

func (n rpcNotification) MarshalJSON() ([]byte, error) {
	type alias rpcNotification
	if strings.TrimSpace(n.JSONRPC) == "" {
		n.JSONRPC = "2.0"
	}
	return json.Marshal(alias(n))
}

type startSessionParams struct {
	Runtime      string           `json:"runtime"`
	SessionID    string           `json:"session_id,omitempty"`
	CWD          string           `json:"cwd,omitempty"`
	Model        string           `json:"model,omitempty"`
	ModelOptions api.ModelOptions `json:"model_options,omitempty"`
	Prompt       string           `json:"prompt,omitempty"`
	Metadata     map[string]any   `json:"metadata,omitempty"`
}

type resumeSessionParams struct {
	Runtime           string           `json:"runtime"`
	SessionID         string           `json:"session_id,omitempty"`
	ProviderSessionID string           `json:"provider_session_id"`
	CWD               string           `json:"cwd,omitempty"`
	Model             string           `json:"model,omitempty"`
	ModelOptions      api.ModelOptions `json:"model_options,omitempty"`
	Metadata          map[string]any   `json:"metadata,omitempty"`
}

type sendInputParams struct {
	SessionID string         `json:"session_id"`
	Text      string         `json:"text"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type getSessionParams struct {
	SessionID         string `json:"session_id,omitempty"`
	ProviderSessionID string `json:"provider_session_id,omitempty"`
}

type interruptParams struct {
	SessionID string `json:"session_id"`
}

type respondParams struct {
	SessionID string                   `json:"session_id"`
	RequestID string                   `json:"request_id"`
	Action    contract.RespondAction   `json:"action,omitempty"`
	Text      string                   `json:"text,omitempty"`
	OptionID  string                   `json:"option_id,omitempty"`
	Answers   []contract.RequestAnswer `json:"answers,omitempty"`
	Metadata  map[string]any           `json:"metadata,omitempty"`
}

type listSessionsParams struct {
	Runtime string `json:"runtime,omitempty"`
}

type listThreadsParams struct {
	Runtime  string `json:"runtime,omitempty"`
	Archived *bool  `json:"archived,omitempty"`
	Limit    int    `json:"limit,omitempty"`
}

type getThreadParams struct {
	ThreadID          string `json:"thread_id,omitempty"`
	ProviderSessionID string `json:"provider_session_id,omitempty"`
}

type archiveThreadParams struct {
	ThreadID string `json:"thread_id"`
	Archived bool   `json:"archived"`
}

type setThreadNameParams struct {
	ThreadID string `json:"thread_id"`
	Name     string `json:"name"`
}

type setThreadMetadataParams struct {
	ThreadID string                  `json:"thread_id"`
	Metadata contract.ThreadMetadata `json:"metadata,omitempty"`
}

type forkThreadParams struct {
	ThreadID string                  `json:"thread_id"`
	Name     string                  `json:"name,omitempty"`
	Metadata contract.ThreadMetadata `json:"metadata,omitempty"`
}

type rollbackThreadParams struct {
	ThreadID string `json:"thread_id"`
	Turns    int    `json:"turns,omitempty"`
}

type listThreadEventsParams struct {
	ThreadID string `json:"thread_id"`
	AfterID  int64  `json:"after_id,omitempty"`
	Limit    int    `json:"limit,omitempty"`
}

type stopSessionParams struct {
	SessionID string `json:"session_id"`
}

type attentionListParams struct {
	Status    contract.AttentionStatus `json:"status,omitempty"`
	Action    contract.AttentionAction `json:"action,omitempty"`
	SessionID string                   `json:"session_id,omitempty"`
	Limit     int                      `json:"limit,omitempty"`
}

type attentionUpdateParams struct {
	ID          string                   `json:"id,omitempty"`
	AttentionID string                   `json:"attention_id,omitempty"`
	Status      contract.AttentionStatus `json:"status,omitempty"`
	Metadata    map[string]any           `json:"metadata,omitempty"`
	Result      map[string]any           `json:"result,omitempty"`
	Error       string                   `json:"error,omitempty"`
}

type interactionCallParams struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

type interactionSubscribeParams struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

type interactionUnsubscribeParams struct {
	SubscriptionID string `json:"subscription_id"`
	Method         string `json:"method,omitempty"`
}

func (s *RPCServer) handleConnection(ctx context.Context, connection net.Conn) {
	defer func() {
		_ = connection.Close()
	}()

	writer := &lockedWriter{writer: connection}
	events, unsubscribe := s.service.SubscribeEvents(256)
	defer unsubscribe()

	var subscribed bool
	var subscriptionMu sync.RWMutex

	go func() {
		for event := range events {
			subscriptionMu.RLock()
			enabled := subscribed
			subscriptionMu.RUnlock()
			if !enabled {
				continue
			}
			_ = writer.writeJSON(rpcNotification{
				Method: "event",
				Params: event,
			})
		}
	}()

	scanner := bufio.NewScanner(connection)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		request, code, err := parseRPCRequest([]byte(line))
		if err != nil {
			_ = writer.writeJSON(errorResponseWithCode(nullRPCID(), code, err.Error()))
			continue
		}

		switch request.Method {
		case "system.ping":
			_ = writer.writeJSON(rpcResponse{
				ID:     request.ID,
				Result: map[string]any{"ok": true},
			})
		case "system.describe":
			_ = writer.writeJSON(rpcResponse{
				ID:     request.ID,
				Result: s.service.Describe(),
			})
		case "models.list":
			result, err := s.service.ModelRegistry(ctx)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			_ = writer.writeJSON(rpcResponse{ID: request.ID, Result: result})
		case "thread.list":
			var params listThreadsParams
			if err := unmarshalParams(request.Params, &params); err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			result, err := s.service.ListThreads(ctx, params.Runtime, params.Archived)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			_ = writer.writeJSON(rpcResponse{ID: request.ID, Result: result})
		case "thread.get":
			var params getThreadParams
			if err := unmarshalParams(request.Params, &params); err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			result, err := s.service.GetThread(ctx, params.ThreadID, params.ProviderSessionID)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			_ = writer.writeJSON(rpcResponse{ID: request.ID, Result: result})
		case "thread.archive":
			var params archiveThreadParams
			if err := unmarshalParams(request.Params, &params); err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			if err := s.service.SetThreadArchived(ctx, params.ThreadID, params.Archived); err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			_ = writer.writeJSON(rpcResponse{ID: request.ID, Result: map[string]any{"ok": true}})
		case "thread.set_name":
			var params setThreadNameParams
			if err := unmarshalParams(request.Params, &params); err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			if err := s.service.SetThreadName(ctx, params.ThreadID, params.Name); err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			_ = writer.writeJSON(rpcResponse{ID: request.ID, Result: map[string]any{"ok": true}})
		case "thread.set_metadata":
			var params setThreadMetadataParams
			if err := unmarshalParams(request.Params, &params); err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			if err := s.service.SetThreadMetadata(ctx, params.ThreadID, params.Metadata); err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			_ = writer.writeJSON(rpcResponse{ID: request.ID, Result: map[string]any{"ok": true}})
		case "thread.fork":
			var params forkThreadParams
			if err := unmarshalParams(request.Params, &params); err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			result, err := s.service.ForkThread(ctx, params.ThreadID, params.Name, params.Metadata)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			_ = writer.writeJSON(rpcResponse{ID: request.ID, Result: result})
		case "thread.rollback":
			var params rollbackThreadParams
			if err := unmarshalParams(request.Params, &params); err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			result, err := s.service.RollbackThread(ctx, params.ThreadID, params.Turns)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			_ = writer.writeJSON(rpcResponse{ID: request.ID, Result: result})
		case "thread.events":
			var params listThreadEventsParams
			if err := unmarshalParams(request.Params, &params); err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			result, err := s.service.ListThreadEvents(ctx, params.ThreadID, params.AfterID, params.Limit)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			_ = writer.writeJSON(rpcResponse{ID: request.ID, Result: result})
		case "thread.read":
			var params getThreadParams
			if err := unmarshalParams(request.Params, &params); err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			result, err := s.service.ReadThread(ctx, params.ThreadID)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			_ = writer.writeJSON(rpcResponse{ID: request.ID, Result: result})
		case "events.subscribe":
			subscriptionMu.Lock()
			subscribed = true
			subscriptionMu.Unlock()
			_ = writer.writeJSON(rpcResponse{
				ID:     request.ID,
				Result: map[string]any{"ok": true},
			})
		case "events.unsubscribe":
			subscriptionMu.Lock()
			subscribed = false
			subscriptionMu.Unlock()
			_ = writer.writeJSON(rpcResponse{
				ID:     request.ID,
				Result: map[string]any{"ok": true},
			})
		case "session.start":
			var params startSessionParams
			if err := unmarshalParams(request.Params, &params); err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			result, err := s.service.StartSession(ctx, params.Runtime, api.StartSessionRequest{
				SessionID:    params.SessionID,
				CWD:          params.CWD,
				Model:        params.Model,
				ModelOptions: params.ModelOptions,
				Prompt:       params.Prompt,
				Metadata:     params.Metadata,
			})
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			_ = writer.writeJSON(rpcResponse{ID: request.ID, Result: result})
		case "session.resume":
			var params resumeSessionParams
			if err := unmarshalParams(request.Params, &params); err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			result, err := s.service.ResumeSession(ctx, params.Runtime, api.ResumeSessionRequest{
				SessionID:         params.SessionID,
				ProviderSessionID: params.ProviderSessionID,
				CWD:               params.CWD,
				Model:             params.Model,
				ModelOptions:      params.ModelOptions,
				Metadata:          params.Metadata,
			})
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			_ = writer.writeJSON(rpcResponse{ID: request.ID, Result: result})
		case "session.send":
			var params sendInputParams
			if err := unmarshalParams(request.Params, &params); err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			result, err := s.service.SendInput(ctx, api.SendInputRequest{
				SessionID: params.SessionID,
				Text:      params.Text,
				Metadata:  params.Metadata,
			})
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			_ = writer.writeJSON(rpcResponse{ID: request.ID, Result: result})
		case "session.get":
			var params getSessionParams
			if err := unmarshalParams(request.Params, &params); err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			result, err := s.service.GetTrackedSession(ctx, params.SessionID, params.ProviderSessionID)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			_ = writer.writeJSON(rpcResponse{ID: request.ID, Result: result})
		case "session.history":
			var params listSessionsParams
			if err := unmarshalParams(request.Params, &params); err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			result, err := s.service.ListTrackedSessions(ctx, params.Runtime)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			_ = writer.writeJSON(rpcResponse{ID: request.ID, Result: result})
		case "session.interrupt":
			var params interruptParams
			if err := unmarshalParams(request.Params, &params); err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			result, err := s.service.Interrupt(ctx, params.SessionID)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			_ = writer.writeJSON(rpcResponse{ID: request.ID, Result: result})
		case "session.respond":
			var params respondParams
			if err := unmarshalParams(request.Params, &params); err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			result, err := s.service.Respond(ctx, api.RespondRequest{
				SessionID: params.SessionID,
				RequestID: params.RequestID,
				Action:    params.Action,
				Text:      params.Text,
				OptionID:  params.OptionID,
				Answers:   params.Answers,
				Metadata:  params.Metadata,
			})
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			_ = writer.writeJSON(rpcResponse{ID: request.ID, Result: result})
		case "session.stop":
			var params stopSessionParams
			if err := unmarshalParams(request.Params, &params); err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			result, err := s.service.StopSession(ctx, params.SessionID)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			_ = writer.writeJSON(rpcResponse{ID: request.ID, Result: result})
		case "session.list":
			var params listSessionsParams
			if err := unmarshalParams(request.Params, &params); err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			result, err := s.service.ListSessions(ctx, params.Runtime)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			_ = writer.writeJSON(rpcResponse{ID: request.ID, Result: result})
		case "interaction.call":
			var params interactionCallParams
			if err := unmarshalParams(request.Params, &params); err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			nativeParams, err := rawParamValue(params.Params)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			result, err := s.service.InteractionCall(ctx, params.Method, nativeParams)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			_ = writer.writeJSON(rpcResponse{ID: request.ID, Result: result})
		case "interaction.subscribe":
			var params interactionSubscribeParams
			if err := unmarshalParams(request.Params, &params); err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			nativeParams, err := rawParamValue(params.Params)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			result, err := s.service.SubscribeInteraction(ctx, params.Method, nativeParams)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			_ = writer.writeJSON(rpcResponse{ID: request.ID, Result: result})
		case "interaction.unsubscribe":
			var params interactionUnsubscribeParams
			if err := unmarshalParams(request.Params, &params); err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			result, err := s.service.UnsubscribeInteraction(ctx, params.SubscriptionID, params.Method)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			_ = writer.writeJSON(rpcResponse{ID: request.ID, Result: result})
		case "speech.tts.enqueue":
			params, err := paramsMap(request.Params)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			result, err := s.service.EnqueueTTS(ctx, params)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			_ = writer.writeJSON(rpcResponse{ID: request.ID, Result: result})
		case "speech.tts.cancel":
			params, err := paramsMap(request.Params)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			result, err := s.service.CancelTTS(ctx, params)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			_ = writer.writeJSON(rpcResponse{ID: request.ID, Result: result})
		case "speech.tts.status":
			result, err := s.service.CallInteraction(ctx, "tts.status", nil)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			_ = writer.writeJSON(rpcResponse{ID: request.ID, Result: result})
		case "speech.tts.voices.list":
			result, err := s.service.CallInteraction(ctx, "tts.voices.list", nil)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			_ = writer.writeJSON(rpcResponse{ID: request.ID, Result: result})
		case "speech.tts.config.get":
			result, err := s.service.CallInteraction(ctx, "tts.config.get", nil)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			_ = writer.writeJSON(rpcResponse{ID: request.ID, Result: result})
		case "speech.tts.config.set":
			params, err := paramsMap(request.Params)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			result, err := s.service.CallInteraction(ctx, "tts.config.set", params)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			_ = writer.writeJSON(rpcResponse{ID: request.ID, Result: result})
		case "speech.stt.start":
			params, err := paramsMap(request.Params)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			result, err := s.service.STTCommand(ctx, "stt.start", params)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			_ = writer.writeJSON(rpcResponse{ID: request.ID, Result: result})
		case "speech.stt.stop":
			params, err := paramsMap(request.Params)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			result, err := s.service.STTCommand(ctx, "stt.stop", params)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			_ = writer.writeJSON(rpcResponse{ID: request.ID, Result: result})
		case "speech.stt.status":
			result, err := s.service.CallInteraction(ctx, "stt.status", nil)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			_ = writer.writeJSON(rpcResponse{ID: request.ID, Result: result})
		case "speech.stt.submit":
			params, err := paramsMap(request.Params)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			result, err := s.service.SubmitSTT(ctx, params)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			_ = writer.writeJSON(rpcResponse{ID: request.ID, Result: result})
		case "speech.stt.subscribe":
			params, err := paramsMap(request.Params)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			result, err := s.service.SubscribeSTT(ctx, params)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			_ = writer.writeJSON(rpcResponse{ID: request.ID, Result: result})
		case "speech.stt.unsubscribe":
			params, err := paramsMap(request.Params)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			result, err := s.service.UnsubscribeSTT(ctx, params)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			_ = writer.writeJSON(rpcResponse{ID: request.ID, Result: result})
		case "speech.stt.models.list":
			result, err := s.service.CallInteraction(ctx, "stt.models.list", nil)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			_ = writer.writeJSON(rpcResponse{ID: request.ID, Result: result})
		case "speech.stt.model.get":
			result, err := s.service.CallInteraction(ctx, "stt.model.get", nil)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			_ = writer.writeJSON(rpcResponse{ID: request.ID, Result: result})
		case "speech.stt.model.set":
			params, err := paramsMap(request.Params)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			result, err := s.service.CallInteraction(ctx, "stt.model.set", params)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			_ = writer.writeJSON(rpcResponse{ID: request.ID, Result: result})
		case "speech.stt.model.download":
			params, err := paramsMap(request.Params)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			result, err := s.service.CallInteraction(ctx, "stt.model.download", params)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			_ = writer.writeJSON(rpcResponse{ID: request.ID, Result: result})
		case "app.open":
			params, err := paramsMap(request.Params)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			result, err := s.service.OpenApp(ctx, "apps.open", params)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			_ = writer.writeJSON(rpcResponse{ID: request.ID, Result: result})
		case "app.activate":
			params, err := paramsMap(request.Params)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			result, err := s.service.OpenApp(ctx, "apps.activate", params)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			_ = writer.writeJSON(rpcResponse{ID: request.ID, Result: result})
		case "insert.targets.list":
			params, err := paramsMap(request.Params)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			result, err := s.service.ListInsertTargets(ctx, params)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			_ = writer.writeJSON(rpcResponse{ID: request.ID, Result: result})
		case "insert.enqueue":
			params, err := paramsMap(request.Params)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			result, err := s.service.EnqueueInsert(ctx, params)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			_ = writer.writeJSON(rpcResponse{ID: request.ID, Result: result})
		case "screen.observe":
			params, err := paramsMap(request.Params)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			result, err := s.service.ObserveScreen(ctx, params)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			_ = writer.writeJSON(rpcResponse{ID: request.ID, Result: result})
		case "screen.click":
			params, err := paramsMap(request.Params)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			result, err := s.service.ClickScreen(ctx, params)
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			_ = writer.writeJSON(rpcResponse{ID: request.ID, Result: result})
		case "attention.enqueue":
			var item contract.AttentionItem
			if err := unmarshalParams(request.Params, &item); err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			if item.Action == "" {
				item.Action = contract.AttentionActionAsk
			}
			result := s.service.EnqueueAttention(item)
			_ = writer.writeJSON(rpcResponse{ID: request.ID, Result: result})
		case "attention.list":
			var params attentionListParams
			if err := unmarshalParams(request.Params, &params); err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			result := s.service.ListAttention(AttentionListFilter{
				Status:    params.Status,
				Action:    params.Action,
				SessionID: params.SessionID,
				Limit:     params.Limit,
			})
			_ = writer.writeJSON(rpcResponse{ID: request.ID, Result: result})
		case "attention.update":
			var params attentionUpdateParams
			if err := unmarshalParams(request.Params, &params); err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			id := firstNonEmptyString(params.ID, params.AttentionID)
			result, err := s.service.UpdateAttention(id, AttentionUpdate{
				Status:   params.Status,
				Metadata: params.Metadata,
				Result:   params.Result,
				Error:    params.Error,
			})
			if err != nil {
				_ = writer.writeJSON(errorResponse(request.ID, err))
				continue
			}
			_ = writer.writeJSON(rpcResponse{ID: request.ID, Result: result})
		default:
			_ = writer.writeJSON(errorResponseWithCode(request.ID, -32601, fmt.Sprintf("unknown method: %s", request.Method)))
		}
	}
}

type lockedWriter struct {
	mu     sync.Mutex
	writer io.Writer
}

func (w *lockedWriter) writeJSON(value any) error {
	switch response := value.(type) {
	case rpcResponse:
		if len(response.ID) == 0 {
			return nil
		}
	case *rpcResponse:
		if response != nil && len(response.ID) == 0 {
			return nil
		}
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	_, err = w.writer.Write(encoded)
	return err
}

func unmarshalParams(raw json.RawMessage, target any) error {
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return rpcParamError{err: err}
	}
	return nil
}

func paramsMap(raw json.RawMessage) (map[string]any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var params map[string]any
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, rpcParamError{err: err}
	}
	if params == nil {
		params = map[string]any{}
	}
	return params, nil
}

func rawParamValue(raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, rpcParamError{err: err}
	}
	return value, nil
}

func parseRPCRequest(raw []byte) (rpcRequest, int, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return rpcRequest{}, -32700, err
	}

	var request rpcRequest
	if versionRaw, ok := envelope["jsonrpc"]; ok && !isJSONNull(versionRaw) {
		if err := json.Unmarshal(versionRaw, &request.JSONRPC); err != nil {
			return rpcRequest{}, -32600, errors.New("json-rpc version must be a string")
		}
		if request.JSONRPC != "" && request.JSONRPC != "2.0" {
			return rpcRequest{}, -32600, fmt.Errorf("unsupported json-rpc version: %s", request.JSONRPC)
		}
	}

	methodRaw, ok := envelope["method"]
	if !ok {
		return rpcRequest{}, -32600, errors.New("json-rpc method is required")
	}
	if err := json.Unmarshal(methodRaw, &request.Method); err != nil || strings.TrimSpace(request.Method) == "" {
		return rpcRequest{}, -32600, errors.New("json-rpc method is required")
	}

	if paramsRaw, ok := envelope["params"]; ok && !isJSONNull(paramsRaw) {
		request.Params = cloneRawMessage(paramsRaw)
	}

	if idRaw, ok := envelope["id"]; ok {
		if !isValidRPCID(idRaw) {
			return rpcRequest{}, -32600, errors.New("json-rpc id must be a string, number, or null")
		}
		request.ID = cloneRawMessage(idRaw)
		request.ExpectsResponse = true
	}

	return request, 0, nil
}

func isJSONNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func isValidRPCID(raw json.RawMessage) bool {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return false
	}
	switch value.(type) {
	case nil, string, float64:
		return true
	default:
		return false
	}
}

func cloneRawMessage(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}

func nullRPCID() json.RawMessage {
	return json.RawMessage("null")
}

func errorResponse(id json.RawMessage, err error) rpcResponse {
	code := -32000
	var paramErr rpcParamError
	if errors.As(err, &paramErr) {
		code = -32602
	}
	return errorResponseWithCode(id, code, err.Error())
}

func errorResponseWithCode(id json.RawMessage, code int, message string) rpcResponse {
	return rpcResponse{
		ID: cloneRawMessage(id),
		Error: &rpcError{
			Code:    code,
			Message: message,
		},
	}
}
