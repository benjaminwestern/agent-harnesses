package opencode

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	serverOwnerDirEnv = "AGENTIC_CONTROL_OPENCODE_OWNER_DIR"
	serverOwnerKind   = "agentic-control-opencode-server"
)

type serverOwnerRecord struct {
	Kind        string `json:"kind"`
	OwnerPID    int    `json:"owner_pid"`
	ServerPID   int    `json:"server_pid"`
	BaseURL     string `json:"base_url"`
	CWD         string `json:"cwd,omitempty"`
	StartedAtMS int64  `json:"started_at_ms"`
}

func cleanupStaleOwnedServers() {
	dir := serverOwnerDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		record, ok := readServerOwnerRecord(path)
		if !ok || record.Kind != serverOwnerKind {
			continue
		}
		if record.OwnerPID > 0 && serverPIDAlive(record.OwnerPID) {
			continue
		}
		if record.ServerPID > 0 && serverPIDAlive(record.ServerPID) && serverCommandMatches(record.ServerPID, record.BaseURL) {
			terminateOwnedServerPID(record.ServerPID)
		}
		_ = os.Remove(path)
	}
}

func writeServerOwnerRecord(server *serverProcess, cwd string) string {
	if server == nil || server.cmd == nil || server.cmd.Process == nil {
		return ""
	}
	dir := serverOwnerDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return ""
	}
	record := serverOwnerRecord{
		Kind:        serverOwnerKind,
		OwnerPID:    os.Getpid(),
		ServerPID:   server.cmd.Process.Pid,
		BaseURL:     server.baseURL,
		CWD:         strings.TrimSpace(cwd),
		StartedAtMS: time.Now().UnixMilli(),
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return ""
	}
	path := filepath.Join(dir, fmt.Sprintf("server-%d-%d.json", record.OwnerPID, record.ServerPID))
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return ""
	}
	return path
}

func (s *serverProcess) removeOwnerRecord() {
	if s == nil || s.ownerPath == "" {
		return
	}
	_ = os.Remove(s.ownerPath)
}

func readServerOwnerRecord(path string) (serverOwnerRecord, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return serverOwnerRecord{}, false
	}
	var record serverOwnerRecord
	if err := json.Unmarshal(data, &record); err != nil {
		_ = os.Remove(path)
		return serverOwnerRecord{}, false
	}
	return record, true
}

func serverOwnerDir() string {
	if dir := strings.TrimSpace(os.Getenv(serverOwnerDirEnv)); dir != "" {
		return dir
	}
	return filepath.Join(os.TempDir(), "agentic-control", "opencode-servers")
}

func serverOwnerPort(baseURL string) string {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	_, port, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		return ""
	}
	return port
}
