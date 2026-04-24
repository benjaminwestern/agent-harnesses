package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/benjaminwestern/agentic-control/pkg/contract"
	api "github.com/benjaminwestern/agentic-control/pkg/controlplane"
)

const defaultSocketPath = "agentic-control.sock"

type socketRPCClient struct {
	socketPath string
}

func newSocketRPCClient(socketPath string) *socketRPCClient {
	if socketPath == "" {
		socketPath = filepath.Join(os.TempDir(), defaultSocketPath)
	}
	return &socketRPCClient{socketPath: socketPath}
}

func (c *socketRPCClient) dial() (net.Conn, error) {
	connection, err := net.Dial("unix", c.socketPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOENT) || errors.Is(err, syscall.ECONNREFUSED) {
			return nil, fmt.Errorf("could not connect to agent_control at %q; start it first with `agent_control serve --socket-path %q`", c.socketPath, c.socketPath)
		}
		return nil, err
	}
	return connection, nil
}

func (c *socketRPCClient) call(ctx context.Context, method string, params any, out any) error {
	connection, err := c.dial()
	if err != nil {
		return err
	}
	defer func() { _ = connection.Close() }()
	requestID := fmt.Sprintf("cli-%d", time.Now().UnixNano())
	request := map[string]any{"jsonrpc": "2.0", "id": requestID, "method": method}
	if params != nil {
		request["params"] = params
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		return err
	}
	if _, err := connection.Write(append(encoded, '\n')); err != nil {
		return err
	}
	scanner := bufio.NewScanner(connection)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		var envelope struct {
			ID     string          `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &envelope); err != nil {
			return err
		}
		if envelope.ID != requestID {
			continue
		}
		if envelope.Error != nil {
			return errors.New(envelope.Error.Message)
		}
		if out != nil {
			return json.Unmarshal(envelope.Result, out)
		}
		return nil
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return fmt.Errorf("no response received for %s", method)
}

func (c *socketRPCClient) Describe() contract.SystemDescriptor {
	var result contract.SystemDescriptor
	_ = c.call(context.Background(), "system.describe", nil, &result)
	return result
}

func (c *socketRPCClient) Models(ctx context.Context) (contract.ModelRegistry, error) {
	var result contract.ModelRegistry
	err := c.call(ctx, "models.list", nil, &result)
	return result, err
}

func (c *socketRPCClient) Ping(ctx context.Context) error {
	return c.call(ctx, "system.ping", nil, nil)
}

func (c *socketRPCClient) WaitReady(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		interval = 200 * time.Millisecond
	}
	for {
		if err := c.Ping(ctx); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}

func (c *socketRPCClient) ListThreads(ctx context.Context, runtime string, archived *bool) ([]contract.TrackedThread, error) {
	var result []contract.TrackedThread
	err := c.call(ctx, "thread.list", map[string]any{"runtime": runtime, "archived": archived}, &result)
	return result, err
}

func (c *socketRPCClient) GetThread(ctx context.Context, threadID string, providerSessionID string) (*contract.TrackedThread, error) {
	var result contract.TrackedThread
	err := c.call(ctx, "thread.get", map[string]any{"thread_id": threadID, "provider_session_id": providerSessionID}, &result)
	if err != nil {
		if providerSessionID != "" {
			var retry contract.TrackedThread
			if retryErr := c.call(ctx, "thread.get", map[string]any{"provider_session_id": providerSessionID}, &retry); retryErr == nil {
				return &retry, nil
			}
		}
		return nil, err
	}
	return &result, nil
}

func (c *socketRPCClient) ArchiveThread(ctx context.Context, threadID string, archived bool) error {
	return c.call(ctx, "thread.archive", map[string]any{"thread_id": threadID, "archived": archived}, nil)
}

func (c *socketRPCClient) SetThreadName(ctx context.Context, threadID string, name string) error {
	return c.call(ctx, "thread.set_name", map[string]any{"thread_id": threadID, "name": name}, nil)
}

func (c *socketRPCClient) SetThreadMetadata(ctx context.Context, threadID string, metadata contract.ThreadMetadata) error {
	return c.call(ctx, "thread.set_metadata", map[string]any{"thread_id": threadID, "metadata": metadata}, nil)
}

func (c *socketRPCClient) ForkThread(ctx context.Context, threadID string, name string, metadata contract.ThreadMetadata) (*contract.TrackedThread, error) {
	var result contract.TrackedThread
	err := c.call(ctx, "thread.fork", map[string]any{"thread_id": threadID, "name": name, "metadata": metadata}, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *socketRPCClient) RollbackThread(ctx context.Context, threadID string, turns int) (*contract.TrackedThread, error) {
	var result contract.TrackedThread
	err := c.call(ctx, "thread.rollback", map[string]any{"thread_id": threadID, "turns": turns}, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *socketRPCClient) ThreadEvents(ctx context.Context, threadID string, afterID int64, limit int) ([]contract.ThreadEvent, error) {
	var result []contract.ThreadEvent
	err := c.call(ctx, "thread.events", map[string]any{"thread_id": threadID, "after_id": afterID, "limit": limit}, &result)
	return result, err
}

func (c *socketRPCClient) ReadThread(ctx context.Context, threadID string) (*contract.ThreadRead, error) {
	var result contract.ThreadRead
	err := c.call(ctx, "thread.read", map[string]any{"thread_id": threadID}, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *socketRPCClient) StartSession(ctx context.Context, runtime string, request api.StartSessionRequest) (*contract.RuntimeSession, error) {
	var result contract.RuntimeSession
	err := c.call(ctx, "session.start", map[string]any{
		"runtime":       runtime,
		"session_id":    request.SessionID,
		"cwd":           request.CWD,
		"model":         request.Model,
		"model_options": request.ModelOptions,
		"prompt":        request.Prompt,
		"metadata":      request.Metadata,
	}, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *socketRPCClient) ResumeSession(ctx context.Context, runtime string, request api.ResumeSessionRequest) (*contract.RuntimeSession, error) {
	var result contract.RuntimeSession
	err := c.call(ctx, "session.resume", map[string]any{
		"runtime":             runtime,
		"session_id":          request.SessionID,
		"provider_session_id": request.ProviderSessionID,
		"cwd":                 request.CWD,
		"model":               request.Model,
		"model_options":       request.ModelOptions,
		"metadata":            request.Metadata,
	}, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *socketRPCClient) SendInput(ctx context.Context, request api.SendInputRequest) (*contract.RuntimeEvent, error) {
	var result contract.RuntimeEvent
	err := c.call(ctx, "session.send", map[string]any{
		"session_id": request.SessionID,
		"text":       request.Text,
		"metadata":   request.Metadata,
	}, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *socketRPCClient) Interrupt(ctx context.Context, sessionID string) (*contract.RuntimeEvent, error) {
	var result contract.RuntimeEvent
	err := c.call(ctx, "session.interrupt", map[string]any{"session_id": sessionID}, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *socketRPCClient) Respond(ctx context.Context, request api.RespondRequest) (*contract.RuntimeEvent, error) {
	var result contract.RuntimeEvent
	err := c.call(ctx, "session.respond", map[string]any{
		"session_id": request.SessionID,
		"request_id": request.RequestID,
		"action":     request.Action,
		"text":       request.Text,
		"option_id":  request.OptionID,
		"answers":    request.Answers,
		"metadata":   request.Metadata,
	}, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *socketRPCClient) StopSession(ctx context.Context, sessionID string) (*contract.RuntimeEvent, error) {
	var result contract.RuntimeEvent
	err := c.call(ctx, "session.stop", map[string]any{"session_id": sessionID}, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *socketRPCClient) GetTrackedSession(ctx context.Context, sessionID string, providerSessionID string) (*contract.TrackedSession, error) {
	var result contract.TrackedSession
	err := c.call(ctx, "session.get", map[string]any{
		"session_id":          sessionID,
		"provider_session_id": providerSessionID,
	}, &result)
	if err == nil {
		return &result, nil
	}
	if sessionID != "" && providerSessionID == "" {
		var retry contract.TrackedSession
		if retryErr := c.call(ctx, "session.get", map[string]any{"provider_session_id": sessionID}, &retry); retryErr == nil {
			return &retry, nil
		}
	}
	return nil, err
}

func (c *socketRPCClient) ListTrackedSessions(ctx context.Context, runtime string) ([]contract.TrackedSession, error) {
	var result []contract.TrackedSession
	err := c.call(ctx, "session.history", map[string]any{"runtime": runtime}, &result)
	return result, err
}

func (c *socketRPCClient) ListSessions(ctx context.Context, runtime string) ([]contract.RuntimeSession, error) {
	var result []contract.RuntimeSession
	err := c.call(ctx, "session.list", map[string]any{"runtime": runtime}, &result)
	return result, err
}

func (c *socketRPCClient) SubscribeEvents(buffer int) (<-chan contract.RuntimeEvent, func()) {
	connection, err := c.dial()
	if err != nil {
		ch := make(chan contract.RuntimeEvent)
		close(ch)
		return ch, func() {}
	}
	ch := make(chan contract.RuntimeEvent, buffer)
	var once sync.Once
	stop := func() {
		once.Do(func() {
			_ = connection.Close()
			close(ch)
		})
	}
	go func() {
		requestID := fmt.Sprintf("events-%d", time.Now().UnixNano())
		encoded, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": requestID, "method": "events.subscribe"})
		if _, err := connection.Write(append(encoded, '\n')); err != nil {
			stop()
			return
		}
		scanner := bufio.NewScanner(connection)
		scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		for scanner.Scan() {
			var envelope struct {
				Method string          `json:"method"`
				Params json.RawMessage `json:"params"`
			}
			if err := json.Unmarshal(scanner.Bytes(), &envelope); err != nil {
				continue
			}
			if envelope.Method != "event" {
				continue
			}
			var event contract.RuntimeEvent
			if err := json.Unmarshal(envelope.Params, &event); err != nil {
				continue
			}
			ch <- event
		}
		stop()
	}()
	return ch, stop
}
