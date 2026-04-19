package controlplane

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/benjaminwestern/agentic-control/pkg/contract"
)

type EventLogger struct {
	file    *os.File
	encoder *json.Encoder
	mu      sync.Mutex
}

func NewEventLoggerFromEnv() (*EventLogger, error) {
	path := os.Getenv("AGENTIC_CONTROL_EVENT_LOG")
	if path == "" {
		return nil, nil
	}
	return NewEventLogger(path)
}

func NewEventLogger(path string) (*EventLogger, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	return &EventLogger{
		file:    file,
		encoder: json.NewEncoder(file),
	}, nil
}

func (l *EventLogger) Write(event contract.RuntimeEvent) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.encoder.Encode(struct {
		Timestamp time.Time             `json:"timestamp"`
		Runtime   string                `json:"runtime"`
		SessionID string                `json:"session_id"`
		Event     contract.RuntimeEvent `json:"event"`
	}{
		Timestamp: time.Now().UTC(),
		Runtime:   event.Runtime,
		SessionID: event.SessionID,
		Event:     event,
	})
}
