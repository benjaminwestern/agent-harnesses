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
		"runtime":         runtime,
		"session_id":      request.SessionID,
		"cwd":             request.CWD,
		"model":           request.Model,
		"model_options":   request.ModelOptions,
		"prompt":          request.Prompt,
		"metadata":        request.Metadata,
		"response_schema": request.ResponseSchema,
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
		"response_schema":     request.ResponseSchema,
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
		"parts":      request.Parts,
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

// Memory
func (c *socketRPCClient) SetMemory(ctx context.Context, entry contract.MemoryEntry) error {
	var result map[string]any
	return c.call(ctx, "memory.set", entry, &result)
}
func (c *socketRPCClient) GetMemory(ctx context.Context, workspaceID, key string) (contract.MemoryEntry, error) {
	var result contract.MemoryEntry
	err := c.call(ctx, "memory.get", map[string]any{"workspace_id": workspaceID, "key": key}, &result)
	return result, err
}
func (c *socketRPCClient) DeleteMemory(ctx context.Context, workspaceID, key string) error {
	var result map[string]any
	return c.call(ctx, "memory.delete", map[string]any{"workspace_id": workspaceID, "key": key}, &result)
}
func (c *socketRPCClient) ListMemory(ctx context.Context, workspaceID string) ([]contract.MemoryEntry, error) {
	var result []contract.MemoryEntry
	err := c.call(ctx, "memory.list", map[string]any{"workspace_id": workspaceID}, &result)
	return result, err
}

// Documents
func (c *socketRPCClient) WriteDocument(ctx context.Context, doc contract.Document) (contract.Document, error) {
	var result contract.Document
	err := c.call(ctx, "documents.write", doc, &result)
	return result, err
}
func (c *socketRPCClient) GetDocument(ctx context.Context, workspaceID, id string) (contract.Document, error) {
	var result contract.Document
	err := c.call(ctx, "documents.get", map[string]any{"workspace_id": workspaceID, "key": id}, &result)
	return result, err
}
func (c *socketRPCClient) ListDocuments(ctx context.Context, workspaceID string) ([]contract.Document, error) {
	var result []contract.Document
	err := c.call(ctx, "documents.list", map[string]any{"workspace_id": workspaceID}, &result)
	return result, err
}
func (c *socketRPCClient) DeleteDocument(ctx context.Context, workspaceID, id string) error {
	var result map[string]any
	return c.call(ctx, "documents.delete", map[string]any{"workspace_id": workspaceID, "key": id}, &result)
}
func (c *socketRPCClient) AppendDocument(ctx context.Context, workspaceID, id, content string) error {
	var result map[string]any
	return c.call(ctx, "documents.append", map[string]any{"workspace_id": workspaceID, "key": id, "content": content}, &result)
}
func (c *socketRPCClient) AddDocumentMetadata(ctx context.Context, workspaceID, id string, metadata map[string]any) error {
	var result map[string]any
	return c.call(ctx, "documents.add_metadata", map[string]any{"workspace_id": workspaceID, "key": id, "metadata": metadata}, &result)
}
func (c *socketRPCClient) RenameDocument(ctx context.Context, workspaceID, id, name string) error {
	var result map[string]any
	return c.call(ctx, "documents.rename", map[string]any{"workspace_id": workspaceID, "key": id, "name": name}, &result)
}
func (c *socketRPCClient) ArchiveDocument(ctx context.Context, workspaceID, id string, archived bool) error {
	var result map[string]any
	return c.call(ctx, "documents.archive", map[string]any{"workspace_id": workspaceID, "key": id, "archived": archived}, &result)
}
func (c *socketRPCClient) ClearDocument(ctx context.Context, workspaceID, id string) error {
	var result map[string]any
	return c.call(ctx, "documents.clear", map[string]any{"workspace_id": workspaceID, "key": id}, &result)
}

// Tasks
func (c *socketRPCClient) CreateTask(ctx context.Context, task contract.Task) (contract.Task, error) {
	var result contract.Task
	err := c.call(ctx, "tasks.create", task, &result)
	return result, err
}
func (c *socketRPCClient) UpdateTask(ctx context.Context, task contract.Task) (contract.Task, error) {
	var result contract.Task
	err := c.call(ctx, "tasks.update", task, &result)
	return result, err
}
func (c *socketRPCClient) GetTask(ctx context.Context, workspaceID, id string) (contract.Task, error) {
	var result contract.Task
	err := c.call(ctx, "tasks.get", map[string]any{"workspace_id": workspaceID, "key": id}, &result)
	return result, err
}
func (c *socketRPCClient) ListTasks(ctx context.Context, workspaceID string) ([]contract.Task, error) {
	var result []contract.Task
	err := c.call(ctx, "tasks.list", map[string]any{"workspace_id": workspaceID}, &result)
	return result, err
}
func (c *socketRPCClient) DeleteTask(ctx context.Context, workspaceID, id string) error {
	var result map[string]any
	return c.call(ctx, "tasks.delete", map[string]any{"workspace_id": workspaceID, "key": id}, &result)
}
func (c *socketRPCClient) AddTaskMetadata(ctx context.Context, workspaceID, id string, metadata map[string]any) error {
	var result map[string]any
	return c.call(ctx, "tasks.add_metadata", map[string]any{"workspace_id": workspaceID, "key": id, "metadata": metadata}, &result)
}
func (c *socketRPCClient) AddTaskTag(ctx context.Context, workspaceID, id, tag string) error {
	var result map[string]any
	return c.call(ctx, "tasks.add_tag", map[string]any{"workspace_id": workspaceID, "key": id, "tag": tag}, &result)
}
func (c *socketRPCClient) RemoveTaskTag(ctx context.Context, workspaceID, id, tag string) error {
	var result map[string]any
	return c.call(ctx, "tasks.remove_tag", map[string]any{"workspace_id": workspaceID, "key": id, "tag": tag}, &result)
}
func (c *socketRPCClient) SetTaskBlockers(ctx context.Context, workspaceID, id string, blockerIDs []string) error {
	var result map[string]any
	return c.call(ctx, "tasks.set_blockers", map[string]any{"workspace_id": workspaceID, "key": id, "blocker_ids": blockerIDs}, &result)
}
func (c *socketRPCClient) AddTaskBlocker(ctx context.Context, workspaceID, id, blockerID string) error {
	var result map[string]any
	return c.call(ctx, "tasks.add_blocker", map[string]any{"workspace_id": workspaceID, "key": id, "blocker_id": blockerID}, &result)
}
func (c *socketRPCClient) RemoveTaskBlocker(ctx context.Context, workspaceID, id, blockerID string) error {
	var result map[string]any
	return c.call(ctx, "tasks.remove_blocker", map[string]any{"workspace_id": workspaceID, "key": id, "blocker_id": blockerID}, &result)
}
func (c *socketRPCClient) LockTask(ctx context.Context, workspaceID, id, actorID string) error {
	var result map[string]any
	return c.call(ctx, "tasks.lock", map[string]any{"workspace_id": workspaceID, "key": id, "actor_id": actorID}, &result)
}
func (c *socketRPCClient) UnlockTask(ctx context.Context, workspaceID, id, actorID string) error {
	var result map[string]any
	return c.call(ctx, "tasks.unlock", map[string]any{"workspace_id": workspaceID, "key": id, "actor_id": actorID}, &result)
}
func (c *socketRPCClient) CreateTaskComment(ctx context.Context, comment contract.TaskComment) (contract.TaskComment, error) {
	var result contract.TaskComment
	err := c.call(ctx, "tasks.comments.create", comment, &result)
	return result, err
}
func (c *socketRPCClient) UpdateTaskComment(ctx context.Context, id, body string) error {
	var result map[string]any
	return c.call(ctx, "tasks.comments.update", map[string]any{"id": id, "body": body}, &result)
}
func (c *socketRPCClient) DeleteTaskComment(ctx context.Context, id string) error {
	var result map[string]any
	return c.call(ctx, "tasks.comments.delete", map[string]any{"id": id}, &result)
}
func (c *socketRPCClient) ListTaskComments(ctx context.Context, taskID string) ([]contract.TaskComment, error) {
	var result []contract.TaskComment
	err := c.call(ctx, "tasks.comments.list", map[string]any{"task_id": taskID}, &result)
	return result, err
}

// Wakeups
func (c *socketRPCClient) SetWakeup(ctx context.Context, wakeup contract.Wakeup) error {
	var result map[string]any
	return c.call(ctx, "wakeups.set", wakeup, &result)
}
func (c *socketRPCClient) ListPendingWakeups(ctx context.Context, workspaceID string) ([]contract.Wakeup, error) {
	var result []contract.Wakeup
	err := c.call(ctx, "wakeups.list_pending", map[string]any{"workspace_id": workspaceID}, &result)
	return result, err
}
func (c *socketRPCClient) GetWakeup(ctx context.Context, workspaceID, id string) (contract.Wakeup, error) {
	var result contract.Wakeup
	err := c.call(ctx, "wakeups.get", map[string]any{"workspace_id": workspaceID, "key": id}, &result)
	return result, err
}
func (c *socketRPCClient) CancelWakeup(ctx context.Context, workspaceID, id string) error {
	var result map[string]any
	return c.call(ctx, "wakeups.cancel", map[string]any{"workspace_id": workspaceID, "key": id}, &result)
}
func (c *socketRPCClient) PauseWakeup(ctx context.Context, workspaceID, id string) error {
	var result map[string]any
	return c.call(ctx, "wakeups.pause", map[string]any{"workspace_id": workspaceID, "key": id}, &result)
}
func (c *socketRPCClient) ResumeWakeup(ctx context.Context, workspaceID, id string) error {
	var result map[string]any
	return c.call(ctx, "wakeups.resume", map[string]any{"workspace_id": workspaceID, "key": id}, &result)
}
func (c *socketRPCClient) ResetWakeup(ctx context.Context, workspaceID, id string, dueAtMS int64) error {
	var result map[string]any
	return c.call(ctx, "wakeups.reset", map[string]any{"workspace_id": workspaceID, "key": id, "due_at_ms": dueAtMS}, &result)
}

// Leases
func (c *socketRPCClient) AcquireLease(ctx context.Context, lease contract.Lease) (bool, error) {
	var result map[string]any
	err := c.call(ctx, "leases.acquire", lease, &result)
	if err != nil {
		return false, err
	}
	return result["acquired"].(bool), nil
}
func (c *socketRPCClient) ReleaseLease(ctx context.Context, workspaceID, lockKey, ownerID string) error {
	var result map[string]any
	return c.call(ctx, "leases.release", map[string]any{"workspace_id": workspaceID, "lock_key": lockKey, "owner_id": ownerID}, &result)
}
func (c *socketRPCClient) GetLease(ctx context.Context, workspaceID, lockKey string) (*contract.Lease, error) {
	var result contract.Lease
	err := c.call(ctx, "leases.get", map[string]any{"workspace_id": workspaceID, "key": lockKey}, &result)
	if err != nil {
		if stringsContains(err.Error(), "sql: no rows") {
			return nil, nil
		}
		return nil, err
	}
	if result.WorkspaceID == "" {
		return nil, nil
	}
	return &result, nil
}
func (c *socketRPCClient) ResetLease(ctx context.Context, workspaceID, lockKey string) error {
	var result map[string]any
	return c.call(ctx, "leases.reset", map[string]any{"workspace_id": workspaceID, "key": lockKey}, &result)
}

func stringsContains(s, substr string) bool {
	return len(s) >= len(substr) && s[0:len(substr)] == substr
}
