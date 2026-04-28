package providerprobe

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/benjaminwestern/agentic-control/pkg/contract"
)

const defaultTTL = 5 * time.Minute

type BinaryPathFunc func() string

type Cache struct {
	binaryPath  BinaryPathFunc
	args        []string
	authArgs    []string
	models      []contract.RuntimeModel
	modelSource string
	ttl         time.Duration

	mu      sync.Mutex
	result  contract.RuntimeProbe
	expires time.Time
}

func New(binaryPath BinaryPathFunc, args ...string) *Cache {
	return &Cache{
		binaryPath: binaryPath,
		args:       args,
		ttl:        defaultTTL,
	}
}

func (c *Cache) WithAuthCommand(args ...string) *Cache {
	c.authArgs = append([]string(nil), args...)
	return c
}

func (c *Cache) WithModels(source string, models []contract.RuntimeModel) *Cache {
	c.modelSource = strings.TrimSpace(source)
	c.models = append([]contract.RuntimeModel(nil), models...)
	return c
}

func (c *Cache) Snapshot(ctx context.Context) *contract.RuntimeProbe {
	if c == nil {
		return nil
	}

	now := time.Now()
	c.mu.Lock()
	if !c.expires.IsZero() && now.Before(c.expires) {
		result := c.result
		c.mu.Unlock()
		return &result
	}
	c.mu.Unlock()

	result := c.probe(ctx)

	c.mu.Lock()
	c.result = result
	c.expires = now.Add(c.ttl)
	c.mu.Unlock()

	return &result
}

func (c *Cache) probe(ctx context.Context) contract.RuntimeProbe {
	binaryPath := strings.TrimSpace(c.binaryPath())
	if binaryPath == "" {
		return contract.RuntimeProbe{
			Installed:   false,
			Status:      "missing",
			Auth:        contract.AuthProbe{Status: "unknown"},
			Models:      append([]contract.RuntimeModel(nil), c.models...),
			ModelSource: c.modelSource,
			Message:     "Runtime binary path is not configured",
			ProbedAtMS:  time.Now().UnixMilli(),
		}
	}

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	path, err := exec.LookPath(binaryPath)
	if err != nil {
		return contract.RuntimeProbe{
			Installed:   false,
			Status:      "missing",
			Auth:        contract.AuthProbe{Status: "unknown"},
			Models:      append([]contract.RuntimeModel(nil), c.models...),
			ModelSource: c.modelSource,
			Message:     "Runtime binary is not installed or not on PATH",
			ProbedAtMS:  time.Now().UnixMilli(),
		}
	}

	result := contract.RuntimeProbe{
		Installed:   true,
		Status:      "ready",
		BinaryPath:  path,
		Auth:        contract.AuthProbe{Status: "unknown"},
		Models:      append([]contract.RuntimeModel(nil), c.models...),
		ModelSource: c.modelSource,
		ProbedAtMS:  time.Now().UnixMilli(),
	}
	if len(c.args) == 0 {
		c.probeAuth(ctx, path, &result)
		result.Message = "Runtime binary found"
		return result
	}

	output, err := combinedOutput(ctx, path, c.args...)
	version := strings.TrimSpace(string(output))
	if version != "" {
		result.Version = firstLine(version)
	}
	if err != nil {
		result.Status = "warning"
		result.Message = strings.TrimSpace(err.Error())
		if ctx.Err() != nil {
			result.Message = ctx.Err().Error()
		}
		return result
	}
	c.probeAuth(ctx, path, &result)
	if result.Message == "" {
		result.Message = "Runtime binary found"
	}
	return result
}

func (c *Cache) probeAuth(parent context.Context, path string, result *contract.RuntimeProbe) {
	if len(c.authArgs) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()

	output, err := combinedOutput(ctx, path, c.authArgs...)
	auth := parseAuthProbe(string(output), err)
	auth.Method = strings.Join(c.authArgs, " ")
	result.Auth = auth
	if auth.Status == "authenticated" {
		return
	}
	if auth.Status == "unauthenticated" {
		result.Status = "error"
		if result.Message == "" {
			result.Message = auth.Message
		}
		return
	}
	if result.Status == "ready" {
		result.Status = "warning"
	}
	if result.Message == "" {
		result.Message = auth.Message
	}
}

func combinedOutput(ctx context.Context, path string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, path, args...)
	configureCommandGroup(command)
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			terminateCommandGroup(command)
		case <-done:
		}
	}()
	output, err := command.CombinedOutput()
	close(done)
	return output, err
}

func parseAuthProbe(output string, commandErr error) contract.AuthProbe {
	lowerOutput := strings.ToLower(output)
	if strings.Contains(lowerOutput, "not logged in") ||
		strings.Contains(lowerOutput, "login required") ||
		strings.Contains(lowerOutput, "authentication required") ||
		strings.Contains(lowerOutput, "run `codex login`") ||
		strings.Contains(lowerOutput, "run codex login") ||
		strings.Contains(lowerOutput, "run `claude login`") ||
		strings.Contains(lowerOutput, "run claude login") ||
		strings.Contains(lowerOutput, "run `gemini auth`") ||
		strings.Contains(lowerOutput, "run gemini auth") {
		return contract.AuthProbe{
			Status:  "unauthenticated",
			Message: firstNonEmptyLine(output, "Runtime is not authenticated."),
		}
	}
	if strings.Contains(lowerOutput, "unknown command") ||
		strings.Contains(lowerOutput, "unrecognized command") ||
		strings.Contains(lowerOutput, "unexpected argument") {
		return contract.AuthProbe{
			Status:  "unknown",
			Message: "Authentication status command is unavailable in this runtime version.",
		}
	}
	if authenticated, ok := extractAuthBoolean([]byte(strings.TrimSpace(output))); ok {
		if authenticated {
			return contract.AuthProbe{Status: "authenticated"}
		}
		return contract.AuthProbe{
			Status:  "unauthenticated",
			Message: "Runtime is not authenticated.",
		}
	}
	if commandErr == nil {
		return contract.AuthProbe{Status: "authenticated"}
	}
	return contract.AuthProbe{
		Status:  "unknown",
		Message: firstNonEmptyLine(output, commandErr.Error()),
	}
}

func extractAuthBoolean(data []byte) (bool, bool) {
	if len(data) == 0 || (data[0] != '{' && data[0] != '[') {
		return false, false
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return false, false
	}
	return walkAuthBoolean(value)
}

func walkAuthBoolean(value any) (bool, bool) {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			if value, ok := walkAuthBoolean(item); ok {
				return value, true
			}
		}
	case map[string]any:
		for _, key := range []string{"authenticated", "isAuthenticated", "loggedIn", "isLoggedIn"} {
			if value, ok := typed[key].(bool); ok {
				return value, true
			}
		}
		for _, key := range []string{"auth", "status", "session", "account"} {
			if value, ok := walkAuthBoolean(typed[key]); ok {
				return value, true
			}
		}
	}
	return false, false
}

func firstNonEmptyLine(output string, fallback string) string {
	output = strings.TrimSpace(output)
	if output == "" {
		return fallback
	}
	return firstLine(output)
}

func firstLine(value string) string {
	if index := strings.IndexAny(value, "\r\n"); index >= 0 {
		return strings.TrimSpace(value[:index])
	}
	return value
}
