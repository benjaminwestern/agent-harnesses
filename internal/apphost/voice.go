package apphost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/benjaminwestern/agentic-control/pkg/contract"
)

type VoiceManager struct {
	mu          sync.RWMutex
	path        string
	assignments map[string]VoiceAssignment
	live        map[string]VoiceBinding
}

type VoiceAssignment struct {
	Project     string `json:"project"`
	AgentID     string `json:"agent_id"`
	Voice       string `json:"voice"`
	UpdatedAtMS int64  `json:"updated_at_ms"`
}

type VoiceBinding struct {
	Project     string `json:"project"`
	AgentID     string `json:"agent_id"`
	SessionID   string `json:"session_id,omitempty"`
	Runtime     string `json:"runtime,omitempty"`
	Voice       string `json:"voice"`
	Pinned      bool   `json:"pinned,omitempty"`
	Source      string `json:"source,omitempty"`
	UpdatedAtMS int64  `json:"updated_at_ms"`
}

type VoiceStatus struct {
	Voices      []string          `json:"voices,omitempty"`
	Assignments []VoiceAssignment `json:"assignments,omitempty"`
	Live        []VoiceBinding    `json:"live,omitempty"`
}

type VoiceClaimRequest struct {
	Project    string         `json:"project,omitempty"`
	AgentID    string         `json:"agent_id,omitempty"`
	SessionID  string         `json:"session_id,omitempty"`
	Runtime    string         `json:"runtime,omitempty"`
	Voice      string         `json:"voice,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	Persistent *bool          `json:"persistent,omitempty"`
}

type VoiceAssignRequest struct {
	Project string `json:"project"`
	AgentID string `json:"agent_id"`
	Voice   string `json:"voice"`
}

type VoiceReleaseRequest struct {
	SessionID string `json:"session_id,omitempty"`
	Project   string `json:"project,omitempty"`
	AgentID   string `json:"agent_id,omitempty"`
	Voice     string `json:"voice,omitempty"`
}

type voiceStateFile struct {
	Assignments []VoiceAssignment `json:"assignments"`
}

func NewVoiceManager(path string) (*VoiceManager, error) {
	if strings.TrimSpace(path) == "" {
		configDir, err := os.UserConfigDir()
		if err != nil {
			return nil, err
		}
		path = filepath.Join(configDir, "agentic-control", "voice-assignments.json")
	}
	manager := &VoiceManager{
		path:        path,
		assignments: make(map[string]VoiceAssignment),
		live:        make(map[string]VoiceBinding),
	}
	if err := manager.load(); err != nil {
		return nil, err
	}
	return manager, nil
}

func (m *VoiceManager) Status(voices []string) VoiceStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	assignments := make([]VoiceAssignment, 0, len(m.assignments))
	for _, assignment := range m.assignments {
		assignments = append(assignments, assignment)
	}
	slices.SortFunc(assignments, func(left, right VoiceAssignment) int {
		if left.Project == right.Project {
			return strings.Compare(left.AgentID, right.AgentID)
		}
		return strings.Compare(left.Project, right.Project)
	})

	live := make([]VoiceBinding, 0, len(m.live))
	for _, binding := range m.live {
		live = append(live, binding)
	}
	slices.SortFunc(live, func(left, right VoiceBinding) int {
		return strings.Compare(left.AgentID+left.SessionID, right.AgentID+right.SessionID)
	})

	return VoiceStatus{
		Voices:      voices,
		Assignments: assignments,
		Live:        live,
	}
}

func (m *VoiceManager) Assign(req VoiceAssignRequest) (VoiceAssignment, error) {
	project := normalizeProject(req.Project)
	agentID := normalizeAgent(req.AgentID)
	voice := strings.TrimSpace(req.Voice)
	if project == "" {
		return VoiceAssignment{}, errors.New("project is required")
	}
	if agentID == "" {
		return VoiceAssignment{}, errors.New("agent_id is required")
	}
	if voice == "" {
		return VoiceAssignment{}, errors.New("voice is required")
	}
	assignment := VoiceAssignment{
		Project:     project,
		AgentID:     agentID,
		Voice:       voice,
		UpdatedAtMS: time.Now().UnixMilli(),
	}

	m.mu.Lock()
	m.assignments[voiceKey(project, agentID)] = assignment
	err := m.saveLocked()
	m.mu.Unlock()
	if err != nil {
		return VoiceAssignment{}, err
	}
	return assignment, nil
}

func (m *VoiceManager) Claim(req VoiceClaimRequest, voices []string) (VoiceBinding, error) {
	project := normalizeProject(req.Project)
	agentID := normalizeAgent(firstNonEmpty(req.AgentID, agentIDFromMetadata(req.Metadata), req.SessionID))
	if project == "" {
		return VoiceBinding{}, errors.New("project is required")
	}
	if agentID == "" {
		return VoiceBinding{}, errors.New("agent_id or session_id is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	pinned := false
	source := "auto"
	voice := strings.TrimSpace(req.Voice)
	if assignment, ok := m.assignments[voiceKey(project, agentID)]; ok {
		voice = assignment.Voice
		pinned = true
		source = "assignment"
	} else if voice != "" {
		source = "agent"
	}
	if voice == "" {
		voice = m.firstAvailableLocked(voices, project, agentID, req.SessionID)
	}
	if voice == "" {
		return VoiceBinding{}, errors.New("no available voice")
	}
	if conflict := m.voiceConflictLocked(voice, project, agentID, req.SessionID); conflict != nil {
		return VoiceBinding{}, fmt.Errorf("voice %q is already reserved by live agent %s in %s", voice, conflict.AgentID, conflict.Project)
	}

	binding := VoiceBinding{
		Project:     project,
		AgentID:     agentID,
		SessionID:   strings.TrimSpace(req.SessionID),
		Runtime:     strings.TrimSpace(req.Runtime),
		Voice:       voice,
		Pinned:      pinned,
		Source:      source,
		UpdatedAtMS: time.Now().UnixMilli(),
	}
	m.live[liveVoiceKey(binding)] = binding

	if req.Persistent != nil && *req.Persistent {
		m.assignments[voiceKey(project, agentID)] = VoiceAssignment{
			Project:     project,
			AgentID:     agentID,
			Voice:       voice,
			UpdatedAtMS: binding.UpdatedAtMS,
		}
		if err := m.saveLocked(); err != nil {
			return VoiceBinding{}, err
		}
	}
	return binding, nil
}

func (m *VoiceManager) Release(req VoiceReleaseRequest) {
	m.mu.Lock()
	defer m.mu.Unlock()
	sessionID := strings.TrimSpace(req.SessionID)
	project := normalizeProject(req.Project)
	agentID := normalizeAgent(req.AgentID)
	voice := strings.TrimSpace(req.Voice)
	for key, binding := range m.live {
		if sessionID != "" && binding.SessionID != sessionID {
			continue
		}
		if project != "" && binding.Project != project {
			continue
		}
		if agentID != "" && binding.AgentID != agentID {
			continue
		}
		if voice != "" && binding.Voice != voice {
			continue
		}
		delete(m.live, key)
	}
}

func (m *VoiceManager) SyncSessions(project string, sessions []contract.RuntimeSession) {
	liveSessions := map[string]struct{}{}
	for _, session := range sessions {
		if session.SessionID == "" || isTerminalSession(session.Status) {
			continue
		}
		liveSessions[session.SessionID] = struct{}{}
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	for key, binding := range m.live {
		if binding.SessionID == "" {
			continue
		}
		if _, ok := liveSessions[binding.SessionID]; !ok {
			delete(m.live, key)
		}
	}
	for _, session := range sessions {
		if session.SessionID == "" || isTerminalSession(session.Status) {
			continue
		}
		agentID := normalizeAgent(agentIDFromSession(session))
		if agentID == "" {
			continue
		}
		sessionProject := normalizeProject(firstNonEmpty(projectFromSession(session), project))
		assignment, ok := m.assignments[voiceKey(sessionProject, agentID)]
		if !ok {
			continue
		}
		if conflict := m.voiceConflictLocked(assignment.Voice, sessionProject, agentID, session.SessionID); conflict != nil {
			continue
		}
		binding := VoiceBinding{
			Project:     sessionProject,
			AgentID:     agentID,
			SessionID:   session.SessionID,
			Runtime:     session.Runtime,
			Voice:       assignment.Voice,
			Pinned:      true,
			Source:      "assignment",
			UpdatedAtMS: time.Now().UnixMilli(),
		}
		m.live[liveVoiceKey(binding)] = binding
	}
}

func (m *VoiceManager) firstAvailableLocked(voices []string, project string, agentID string, sessionID string) string {
	for _, voice := range voices {
		voice = strings.TrimSpace(voice)
		if voice == "" {
			continue
		}
		if m.voiceConflictLocked(voice, project, agentID, sessionID) == nil {
			return voice
		}
	}
	return ""
}

func (m *VoiceManager) voiceConflictLocked(voice string, project string, agentID string, sessionID string) *VoiceBinding {
	for _, binding := range m.live {
		if binding.Voice != voice {
			continue
		}
		if binding.Project == project && binding.AgentID == agentID {
			return nil
		}
		if sessionID != "" && binding.SessionID == sessionID {
			return nil
		}
		copy := binding
		return &copy
	}
	return nil
}

func (m *VoiceManager) load() error {
	data, err := os.ReadFile(m.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var state voiceStateFile
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}
	for _, assignment := range state.Assignments {
		assignment.Project = normalizeProject(assignment.Project)
		assignment.AgentID = normalizeAgent(assignment.AgentID)
		if assignment.Project == "" || assignment.AgentID == "" || assignment.Voice == "" {
			continue
		}
		m.assignments[voiceKey(assignment.Project, assignment.AgentID)] = assignment
	}
	return nil
}

func (m *VoiceManager) saveLocked() error {
	assignments := make([]VoiceAssignment, 0, len(m.assignments))
	for _, assignment := range m.assignments {
		assignments = append(assignments, assignment)
	}
	slices.SortFunc(assignments, func(left, right VoiceAssignment) int {
		if left.Project == right.Project {
			return strings.Compare(left.AgentID, right.AgentID)
		}
		return strings.Compare(left.Project, right.Project)
	})
	data, err := json.MarshalIndent(voiceStateFile{Assignments: assignments}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(m.path), 0o750); err != nil {
		return err
	}
	return os.WriteFile(m.path, append(data, '\n'), 0o600)
}

func (c *Core) VoiceStatus(ctx context.Context) (VoiceStatus, error) {
	voices, _ := c.ListVoices(ctx)
	return c.voices.Status(voices), nil
}

func (c *Core) ListVoices(ctx context.Context) ([]string, error) {
	raw, err := c.service.InteractionCall(ctx, "tts.voices.list", nil)
	if err != nil {
		return nil, err
	}
	return extractVoiceNames(decodeJSONValue(raw)), nil
}

func (c *Core) ClaimVoice(ctx context.Context, req VoiceClaimRequest) (VoiceBinding, error) {
	req.Project = firstNonEmpty(req.Project, c.workspace)
	if req.SessionID != "" && (req.AgentID == "" || req.Runtime == "") {
		if tracked, err := c.service.GetTrackedSession(ctx, req.SessionID, ""); err == nil {
			if req.AgentID == "" {
				req.AgentID = agentIDFromSession(tracked.Session)
			}
			if req.Runtime == "" {
				req.Runtime = tracked.Session.Runtime
			}
			if req.Project == "" {
				req.Project = firstNonEmpty(projectFromSession(tracked.Session), c.workspace)
			}
		}
	}
	voices, _ := c.ListVoices(ctx)
	return c.voices.Claim(req, voices)
}

func (c *Core) AssignVoice(req VoiceAssignRequest) (VoiceAssignment, error) {
	req.Project = firstNonEmpty(req.Project, c.workspace)
	return c.voices.Assign(req)
}

func (c *Core) ReleaseVoice(req VoiceReleaseRequest) map[string]bool {
	c.voices.Release(req)
	return map[string]bool{"released": true}
}

func (c *Core) prepareVoiceParams(ctx context.Context, params map[string]any) (map[string]any, VoiceBinding, error) {
	params = cloneParams(params)
	req := VoiceClaimRequest{
		Project:   firstNonEmpty(stringFromAny(params["project"]), stringFromAny(params["workspace"]), c.workspace),
		AgentID:   firstNonEmpty(stringFromAny(params["agent_id"]), stringFromAny(params["agent"])),
		SessionID: stringFromAny(params["session_id"]),
		Runtime:   firstNonEmpty(stringFromAny(params["runtime"]), stringFromAny(params["source"])),
		Voice:     stringFromAny(params["voice"]),
		Metadata:  mapFromAny(params["metadata"]),
	}
	binding, err := c.ClaimVoice(ctx, req)
	if err != nil {
		return nil, VoiceBinding{}, err
	}
	params["voice"] = binding.Voice
	metadata := mapFromAny(params["metadata"])
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["voice"] = binding.Voice
	metadata["voice_source"] = binding.Source
	metadata["agent_id"] = binding.AgentID
	params["metadata"] = metadata
	return params, binding, nil
}

func extractVoiceNames(value any) []string {
	seen := map[string]struct{}{}
	var out []string
	var visit func(any)
	visit = func(value any) {
		switch typed := value.(type) {
		case []any:
			for _, item := range typed {
				visit(item)
			}
		case map[string]any:
			for _, key := range []string{"code", "voice", "identifier"} {
				if text := stringFromAny(typed[key]); text != "" {
					if _, ok := seen[text]; !ok {
						seen[text] = struct{}{}
						out = append(out, text)
					}
					return
				}
			}
			hadChildren := false
			for _, key := range []string{"providers", "groups", "voices", "items", "results"} {
				if child, ok := typed[key]; ok {
					hadChildren = true
					visit(child)
				}
			}
			if hadChildren {
				return
			}
			for _, key := range []string{"id", "name", "label"} {
				if text := stringFromAny(typed[key]); text != "" {
					if _, ok := seen[text]; !ok {
						seen[text] = struct{}{}
						out = append(out, text)
					}
					break
				}
			}
		case string:
			text := strings.TrimSpace(typed)
			if text != "" {
				if _, ok := seen[text]; !ok {
					seen[text] = struct{}{}
					out = append(out, text)
				}
			}
		}
	}
	visit(value)
	slices.Sort(out)
	return out
}

func agentIDFromSession(session contract.RuntimeSession) string {
	metadata := session.Metadata
	return firstNonEmpty(
		stringFromAny(metadata["agent_id"]),
		stringFromAny(metadata["agent"]),
		stringFromAny(metadata["court_agent"]),
		stringFromAny(metadata["court_role_id"]),
		stringFromAny(mapFromAny(metadata["labels"])["court_agent"]),
		stringFromAny(mapFromAny(metadata["labels"])["court_role_id"]),
		session.Title,
		session.SessionID,
	)
}

func agentIDFromMetadata(metadata map[string]any) string {
	return firstNonEmpty(
		stringFromAny(metadata["agent_id"]),
		stringFromAny(metadata["agent"]),
		stringFromAny(metadata["court_agent"]),
		stringFromAny(metadata["court_role_id"]),
		stringFromAny(mapFromAny(metadata["labels"])["court_agent"]),
		stringFromAny(mapFromAny(metadata["labels"])["court_role_id"]),
	)
}

func projectFromSession(session contract.RuntimeSession) string {
	metadata := session.Metadata
	return firstNonEmpty(
		stringFromAny(metadata["project"]),
		stringFromAny(metadata["workspace"]),
		stringFromAny(mapFromAny(metadata["labels"])["project"]),
		stringFromAny(mapFromAny(metadata["labels"])["workspace"]),
		session.CWD,
	)
}

func voiceKey(project string, agentID string) string {
	return normalizeProject(project) + "\x00" + normalizeAgent(agentID)
}

func liveVoiceKey(binding VoiceBinding) string {
	if binding.SessionID != "" {
		return "session:" + binding.SessionID
	}
	return "agent:" + voiceKey(binding.Project, binding.AgentID)
}

func normalizeProject(project string) string {
	project = strings.TrimSpace(project)
	if project == "" {
		return ""
	}
	if project == "~" || strings.HasPrefix(project, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			if project == "~" {
				project = home
			} else {
				project = filepath.Join(home, strings.TrimPrefix(project, "~/"))
			}
		}
	}
	if abs, err := filepath.Abs(project); err == nil {
		return abs
	}
	return filepath.Clean(project)
}

func normalizeAgent(agentID string) string {
	return strings.TrimSpace(agentID)
}

func isTerminalSession(status contract.SessionStatus) bool {
	switch status {
	case contract.SessionStopped, contract.SessionErrored, contract.SessionInterrupted:
		return true
	default:
		return false
	}
}

func stringFromAny(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return ""
	}
}

func mapFromAny(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return typed
	default:
		return nil
	}
}

func boolFromAny(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true") || strings.TrimSpace(typed) == "1"
	default:
		return false
	}
}

func speechRouteWantsResponses(params map[string]any, route map[string]any) bool {
	if boolFromAny(params["speak_responses"]) ||
		boolFromAny(params["speakResponses"]) ||
		boolFromAny(params["tts_responses"]) ||
		boolFromAny(route["speak_responses"]) ||
		boolFromAny(route["speakResponses"]) ||
		boolFromAny(route["tts_responses"]) ||
		mapFromAny(params["response_tts"]) != nil ||
		mapFromAny(route["tts"]) != nil {
		return true
	}
	response := mapFromAny(route["response"])
	if response == nil {
		return false
	}
	return boolFromAny(response["speak"]) || boolFromAny(response["tts"]) || mapFromAny(response["tts"]) != nil
}
