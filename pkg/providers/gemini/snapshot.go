package gemini

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	geminiSessionFilePrefix = "session-"
	geminiChatDirName       = "chats"
	maxGeminiSnapshots      = 16
)

func (s *session) persistTurnSnapshot(turnID string, items []any) (storedTurn, error) {
	stored := storedTurn{
		TurnID: turnID,
		Items:  cloneUnknownItems(items),
	}

	providerSessionID := s.providerSessionIDSnapshot()
	if providerSessionID == "" {
		return stored, nil
	}

	sessionFilePath, err := s.findSessionFile()
	if err != nil {
		return stored, err
	}
	if sessionFilePath == "" {
		return stored, nil
	}

	snapshotSessionID := newGeminiSnapshotID()
	snapshotFilePath, err := cloneGeminiSessionFile(sessionFilePath, snapshotSessionID)
	if err != nil {
		return stored, err
	}

	stored.SnapshotSessionID = snapshotSessionID
	stored.SnapshotFilePath = snapshotFilePath

	s.mu.Lock()
	s.turns = append(s.turns, stored)
	stale := staleSnapshotPaths(s.turns, maxGeminiSnapshots)
	if len(stale) > 0 {
		s.turns = s.turns[len(s.turns)-maxGeminiSnapshots:]
	}
	s.mu.Unlock()

	for _, path := range stale {
		_ = os.Remove(path)
	}
	return stored, nil
}

func (s *session) findSessionFile() (string, error) {
	sessionID := s.providerSessionIDSnapshot()
	if sessionID == "" {
		return "", nil
	}

	s.mu.RLock()
	hintedPath := s.sessionFilePath
	s.mu.RUnlock()

	path, err := findGeminiSessionFileByID(sessionID, hintedPath)
	if err != nil || path == "" {
		return path, err
	}

	s.mu.Lock()
	s.sessionFilePath = path
	s.mu.Unlock()
	return path, nil
}

func findGeminiSessionFileByID(sessionID string, hintedPath string) (string, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "", nil
	}

	candidates := make([]string, 0, 16)
	if hintedPath != "" {
		candidates = append(candidates, hintedPath)
	}

	tmpDir, err := geminiTmpDir()
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}

	prefix := sessionID
	if len(prefix) > 8 {
		prefix = prefix[:8]
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		chatDir := filepath.Join(tmpDir, entry.Name(), geminiChatDirName)
		files, err := os.ReadDir(chatDir)
		if err != nil {
			continue
		}
		for _, file := range files {
			if file.IsDir() {
				continue
			}
			name := file.Name()
			if strings.HasPrefix(name, geminiSessionFilePrefix) &&
				strings.HasSuffix(name, ".json") &&
				strings.Contains(name, prefix) {
				candidates = append(candidates, filepath.Join(chatDir, name))
			}
		}
	}

	for _, candidate := range candidates {
		stored, err := readStoredGeminiSession(candidate)
		if err == nil && stored.SessionID == sessionID {
			return candidate, nil
		}
	}
	return "", nil
}

type storedGeminiSession struct {
	SessionID   string `json:"sessionId"`
	Messages    []any  `json:"messages"`
	StartTime   string `json:"startTime"`
	LastUpdated string `json:"lastUpdated"`
}

func readStoredGeminiSession(path string) (storedGeminiSession, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return storedGeminiSession{}, err
	}
	var session storedGeminiSession
	if err := json.Unmarshal(data, &session); err != nil {
		return storedGeminiSession{}, err
	}
	if session.SessionID == "" || session.Messages == nil || session.StartTime == "" || session.LastUpdated == "" {
		return storedGeminiSession{}, fmt.Errorf("invalid Gemini session file: %s", path)
	}
	return session, nil
}

func cloneGeminiSessionFile(sourcePath string, sessionID string) (string, error) {
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return "", err
	}
	var session map[string]any
	if err := json.Unmarshal(data, &session); err != nil {
		return "", err
	}
	if _, ok := session["messages"].([]any); !ok {
		return "", fmt.Errorf("invalid Gemini session file: %s", sourcePath)
	}
	session["sessionId"] = sessionID
	session["lastUpdated"] = time.Now().UTC().Format(time.RFC3339Nano)

	data, err = json.MarshalIndent(session, "", "  ")
	if err != nil {
		return "", err
	}
	data = append(data, '\n')

	destination := filepath.Join(filepath.Dir(sourcePath), geminiSessionFileName(sessionID))
	if err := os.WriteFile(destination, data, 0o600); err != nil {
		return "", err
	}
	return destination, nil
}

func geminiSessionFileName(sessionID string) string {
	timestamp := strings.NewReplacer(":", "-", ".", "-").Replace(time.Now().UTC().Format(time.RFC3339Nano))
	prefix := sessionID
	if len(prefix) > 8 {
		prefix = prefix[:8]
	}
	return fmt.Sprintf("%s%s-%s.json", geminiSessionFilePrefix, timestamp, prefix)
}

func geminiTmpDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".gemini", "tmp"), nil
}

func newGeminiSnapshotID() string {
	return fmt.Sprintf("snapshot-%d", time.Now().UnixNano())
}

func cloneUnknownItems(items []any) []any {
	if len(items) == 0 {
		return nil
	}
	out := make([]any, 0, len(items))
	for _, item := range items {
		if record, ok := item.(map[string]any); ok {
			next := make(map[string]any, len(record))
			for key, value := range record {
				next[key] = value
			}
			out = append(out, next)
			continue
		}
		out = append(out, item)
	}
	return out
}

func staleSnapshotPaths(turns []storedTurn, keep int) []string {
	if keep <= 0 || len(turns) <= keep {
		return nil
	}
	stale := make([]string, 0, len(turns)-keep)
	for _, turn := range turns[:len(turns)-keep] {
		if turn.SnapshotFilePath != "" {
			stale = append(stale, turn.SnapshotFilePath)
		}
	}
	return stale
}
