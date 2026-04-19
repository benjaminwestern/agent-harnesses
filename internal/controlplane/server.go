package controlplane

import (
	"bufio"
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
	ID     string          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	ID     string    `json:"id,omitempty"`
	Result any       `json:"result,omitempty"`
	Error  *rpcError `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcNotification struct {
	Method string `json:"method"`
	Params any    `json:"params,omitempty"`
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

type stopSessionParams struct {
	SessionID string `json:"session_id"`
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

		var request rpcRequest
		if err := json.Unmarshal([]byte(line), &request); err != nil {
			_ = writer.writeJSON(rpcResponse{
				Error: &rpcError{Code: -32700, Message: err.Error()},
			})
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
		default:
			_ = writer.writeJSON(errorResponse(request.ID, fmt.Errorf("unknown method: %s", request.Method)))
		}
	}
}

type lockedWriter struct {
	mu     sync.Mutex
	writer io.Writer
}

func (w *lockedWriter) writeJSON(value any) error {
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
	return json.Unmarshal(raw, target)
}

func errorResponse(id string, err error) rpcResponse {
	return rpcResponse{
		ID: id,
		Error: &rpcError{
			Code:    -32000,
			Message: err.Error(),
		},
	}
}
