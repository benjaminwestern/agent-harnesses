package interaction

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"slices"
	"strings"
	"sync"
	"time"
)

const SocketEnv = "AGENTIC_INTERACTION_RPC_SOCKET"

type Client struct {
	socketPath string

	mu      sync.RWMutex
	status  Status
	methods map[string]struct{}
}

type Status struct {
	SchemaVersion string          `json:"schema_version,omitempty"`
	Service       string          `json:"service,omitempty"`
	Endpoint      string          `json:"endpoint,omitempty"`
	Transport     string          `json:"transport,omitempty"`
	Available     bool            `json:"available"`
	Methods       []string        `json:"methods,omitempty"`
	Capabilities  map[string]bool `json:"capabilities,omitempty"`
	Health        json.RawMessage `json:"health,omitempty"`
	LastError     string          `json:"last_error,omitempty"`
}

type Subscription struct {
	SubscriptionID string
	Result         json.RawMessage
	Cancel         func()
}

type interactionRPCRequest struct {
	JSONRPC string `json:"jsonrpc,omitempty"`
	ID      string `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type interactionRPCMessage struct {
	JSONRPC string          `json:"jsonrpc,omitempty"`
	ID      string          `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e RPCError) Error() string {
	if len(e.Data) == 0 {
		return fmt.Sprintf("agentic interaction RPC error %d: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("agentic interaction RPC error %d: %s (%s)", e.Code, e.Message, string(e.Data))
}

func NewClientFromEnv() *Client {
	socketPath := strings.TrimSpace(os.Getenv(SocketEnv))
	if socketPath == "" {
		socketPath = DefaultSocketPath()
	}
	client := NewClient(socketPath)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = client.Refresh(ctx)
	return client
}

func NewClient(socketPath string) *Client {
	return &Client{
		socketPath: socketPath,
		status: Status{
			Endpoint:  "unix://" + socketPath,
			Available: false,
		},
		methods: make(map[string]struct{}),
	}
}

func DefaultSocketPath() string {
	candidates := []string{
		"/tmp/agentic-interaction.sock",
		"/tmp/agentic-interaction-dev.sock",
		fmt.Sprintf("/tmp/agentic-interaction-%d.sock", os.Getuid()),
	}
	for _, candidate := range candidates {
		if socketExists(candidate) {
			return candidate
		}
	}
	return candidates[0]
}

func socketExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeSocket != 0
}

func (c *Client) SocketPath() string {
	if c == nil {
		return ""
	}
	return c.socketPath
}

func (c *Client) Describe() Status {
	if c == nil {
		return Status{Available: false, LastError: "agentic interaction client is not configured"}
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	status := c.status
	if len(c.status.Methods) > 0 {
		status.Methods = slices.Clone(c.status.Methods)
	}
	if len(c.status.Capabilities) > 0 {
		status.Capabilities = make(map[string]bool, len(c.status.Capabilities))
		for key, value := range c.status.Capabilities {
			status.Capabilities[key] = value
		}
	}
	if len(c.status.Health) > 0 {
		status.Health = slices.Clone(c.status.Health)
	}
	return status
}

func (c *Client) Refresh(ctx context.Context) error {
	if c == nil {
		return errors.New("agentic interaction client is not configured")
	}

	raw, err := c.callRaw(ctx, MethodSystemDescribe, nil)
	if err != nil {
		c.setUnavailable(err)
		return err
	}

	var describe struct {
		SchemaVersion string          `json:"schema_version"`
		Service       string          `json:"service"`
		Endpoint      string          `json:"endpoint"`
		Transport     string          `json:"transport"`
		Methods       []string        `json:"methods"`
		Capabilities  map[string]bool `json:"capabilities"`
	}
	if err := json.Unmarshal(raw, &describe); err != nil {
		c.setUnavailable(err)
		return err
	}

	methods := make(map[string]struct{}, len(describe.Methods))
	for _, method := range describe.Methods {
		method = strings.TrimSpace(method)
		if method != "" {
			methods[method] = struct{}{}
		}
	}
	slices.Sort(describe.Methods)

	status := Status{
		SchemaVersion: describe.SchemaVersion,
		Service:       describe.Service,
		Endpoint:      describe.Endpoint,
		Transport:     describe.Transport,
		Available:     true,
		Methods:       describe.Methods,
		Capabilities:  describe.Capabilities,
	}
	if status.Endpoint == "" {
		status.Endpoint = "unix://" + c.socketPath
	}
	if healthRaw, err := c.callRaw(ctx, MethodSystemHealth, nil); err == nil && len(healthRaw) > 0 {
		status.Health = slices.Clone(healthRaw)
	}

	c.mu.Lock()
	c.status = status
	c.methods = methods
	c.mu.Unlock()
	return nil
}

func (c *Client) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	if err := c.ensureMethod(ctx, method); err != nil {
		return nil, err
	}
	return c.callRaw(ctx, method, params)
}

func (c *Client) CallInto(ctx context.Context, method string, params any, target any) error {
	raw, err := c.Call(ctx, method, params)
	if err != nil {
		return err
	}
	if target == nil {
		return nil
	}
	return json.Unmarshal(raw, target)
}

func (c *Client) StartSubscription(
	ctx context.Context,
	method string,
	params any,
	onNotification func(method string, params json.RawMessage),
) (*Subscription, error) {
	if err := c.ensureMethod(ctx, method); err != nil {
		return nil, err
	}

	connection, err := c.dial(ctx)
	if err != nil {
		c.setUnavailable(err)
		return nil, err
	}

	requestID := newIdentifier("rpc")
	encoded, err := json.Marshal(interactionRPCRequest{
		JSONRPC: "2.0",
		ID:      requestID,
		Method:  method,
		Params:  params,
	})
	if err != nil {
		_ = connection.Close()
		return nil, err
	}
	if _, err := connection.Write(append(encoded, '\n')); err != nil {
		_ = connection.Close()
		return nil, err
	}

	var closeOnce sync.Once
	closed := make(chan struct{})
	cancel := func() {
		closeOnce.Do(func() {
			close(closed)
			_ = connection.Close()
		})
	}
	go func() {
		select {
		case <-ctx.Done():
			cancel()
		case <-closed:
		}
	}()

	initialResponse := make(chan interactionRPCMessage, 1)
	initialError := make(chan error, 1)
	go func() {
		defer cancel()
		scanner := bufio.NewScanner(connection)
		scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		sawInitialResponse := false
		for scanner.Scan() {
			var message interactionRPCMessage
			if err := json.Unmarshal(scanner.Bytes(), &message); err != nil {
				continue
			}
			if message.ID == requestID {
				if !sawInitialResponse {
					sawInitialResponse = true
					initialResponse <- message
				}
				continue
			}
			if message.Method != "" && onNotification != nil {
				onNotification(message.Method, message.Params)
			}
		}
		if err := scanner.Err(); err != nil {
			if !sawInitialResponse {
				initialError <- err
			}
			return
		}
		if !sawInitialResponse {
			initialError <- io.EOF
		}
	}()

	select {
	case response := <-initialResponse:
		if response.Error != nil {
			cancel()
			return nil, *response.Error
		}
		return &Subscription{
			SubscriptionID: subscriptionID(response.Result),
			Result:         response.Result,
			Cancel:         cancel,
		}, nil
	case err := <-initialError:
		cancel()
		if err == nil {
			err = io.EOF
		}
		return nil, err
	case <-ctx.Done():
		cancel()
		return nil, ctx.Err()
	}
}

func (c *Client) ensureMethod(ctx context.Context, method string) error {
	if c == nil {
		return errors.New("agentic interaction client is not configured")
	}
	if c.hasMethod(method) {
		return nil
	}
	if err := c.Refresh(ctx); err != nil {
		return fmt.Errorf("agentic interaction unavailable at %q: %w", c.socketPath, err)
	}
	if c.hasMethod(method) {
		return nil
	}
	return fmt.Errorf("agentic interaction method unavailable: %s", method)
}

func (c *Client) hasMethod(method string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.methods[method]
	return ok
}

func (c *Client) callRaw(ctx context.Context, method string, params any) (json.RawMessage, error) {
	connection, err := c.dial(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = connection.Close()
	}()

	requestID := newIdentifier("rpc")
	encoded, err := json.Marshal(interactionRPCRequest{
		JSONRPC: "2.0",
		ID:      requestID,
		Method:  method,
		Params:  params,
	})
	if err != nil {
		return nil, err
	}
	if _, err := connection.Write(append(encoded, '\n')); err != nil {
		return nil, err
	}

	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = connection.Close()
		case <-done:
		}
	}()

	scanner := bufio.NewScanner(connection)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var response interactionRPCMessage
		if err := json.Unmarshal(scanner.Bytes(), &response); err != nil {
			return nil, err
		}
		if response.ID != requestID {
			continue
		}
		if response.Error != nil {
			return nil, *response.Error
		}
		return response.Result, nil
	}
	if err := scanner.Err(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	return nil, io.EOF
}

func (c *Client) dial(ctx context.Context) (net.Conn, error) {
	var dialer net.Dialer
	return dialer.DialContext(ctx, "unix", c.socketPath)
}

func (c *Client) setUnavailable(err error) {
	status := Status{
		Endpoint:  "unix://" + c.socketPath,
		Available: false,
	}
	if err != nil {
		status.LastError = err.Error()
	}

	c.mu.Lock()
	c.status = status
	c.methods = make(map[string]struct{})
	c.mu.Unlock()
}

func subscriptionID(raw json.RawMessage) string {
	var response struct {
		SubscriptionID string `json:"subscription_id"`
	}
	if err := json.Unmarshal(raw, &response); err == nil && response.SubscriptionID != "" {
		return response.SubscriptionID
	}
	var camelResponse struct {
		SubscriptionID string `json:"subscriptionId"`
	}
	if err := json.Unmarshal(raw, &camelResponse); err == nil {
		return camelResponse.SubscriptionID
	}
	return ""
}

func newIdentifier(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}
