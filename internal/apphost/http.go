package apphost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/benjaminwestern/agentic-control/pkg/contract"
)

type HTTPServer struct {
	core *Core
}

func NewHTTPServer(core *Core) *HTTPServer {
	return &HTTPServer{core: core}
}

func (s *HTTPServer) APIHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/projects", s.handleProjects)
	mux.HandleFunc("/api/snapshot", s.handleSnapshot)
	mux.HandleFunc("/api/agents/threads", s.handleAgentsThreads)
	mux.HandleFunc("/api/runtimes/status", s.handleRuntimesStatus)
	mux.HandleFunc("/api/attention", s.handleAttention)
	mux.HandleFunc("/api/events/recent", s.handleRecentEvents)
	mux.HandleFunc("/api/events/stream", s.handleEventStream)
	mux.HandleFunc("/api/control/describe", s.handleDescribe)
	mux.HandleFunc("/api/sessions/start", s.handleStartSession)
	mux.HandleFunc("/api/sessions/resume", s.handleResumeSession)
	mux.HandleFunc("/api/sessions/send", s.handleSendSessionInput)
	mux.HandleFunc("/api/sessions/respond", s.handleRespondSession)
	mux.HandleFunc("/api/sessions/interrupt", s.handleInterruptSession)
	mux.HandleFunc("/api/sessions/stop", s.handleStopSession)
	mux.HandleFunc("/api/interaction/call", s.handleInteractionCall)
	mux.HandleFunc("/api/speech/submit", s.handleSpeechSubmit)
	mux.HandleFunc("/api/speech/subscribe", s.handleSpeechSubscribe)
	mux.HandleFunc("/api/speech/unsubscribe", s.handleSpeechUnsubscribe)
	mux.HandleFunc("/api/speech/tts", s.handleTTS)
	mux.HandleFunc("/api/voices/status", s.handleVoiceStatus)
	mux.HandleFunc("/api/voices/claim", s.handleVoiceClaim)
	mux.HandleFunc("/api/voices/assign", s.handleVoiceAssign)
	mux.HandleFunc("/api/voices/release", s.handleVoiceRelease)
	mux.HandleFunc("/api/notifications/audio/catalog", s.handleNotificationAudioCatalog)
	mux.HandleFunc("/api/notifications/audio/play", s.handleNotificationAudioPlay)
	mux.HandleFunc("/api/notifications/audio/status", s.handleNotificationAudioStatus)
	mux.HandleFunc("/api/notifications/audio/stop", s.handleNotificationAudioStop)
	mux.HandleFunc("/api/court/catalog", s.handleCourtCatalog)
	mux.HandleFunc("/api/court/runs", s.handleCourtRuns)
	mux.HandleFunc("/api/court/runs/", s.handleCourtRunPath)
	mux.HandleFunc("/api/court/runtime-requests/respond", s.handleCourtRuntimeRespond)
	mux.HandleFunc("/api/court/workers/control", s.handleCourtWorkerControl)
	return localAPIMiddleware(mux)
}

func (s *HTTPServer) WebHandler(assets fs.FS) http.Handler {
	apiHandler := s.APIHandler()
	fileServer := http.FileServer(http.FS(assets))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			apiHandler.ServeHTTP(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			return
		}
		cleanPath := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if cleanPath == "." || cleanPath == "" {
			cleanPath = "index.html"
		}
		if cleanPath == "index.html" {
			serveIndex(w, r, assets)
			return
		}
		if fileExists(assets, cleanPath) {
			fileServer.ServeHTTP(w, r)
			return
		}
		serveIndex(w, r, assets)
	})
}

func serveIndex(w http.ResponseWriter, r *http.Request, assets fs.FS) {
	data, err := fs.ReadFile(assets, "index.html")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(data)
	}
}

func (s *HTTPServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "time": time.Now()})
}

func (s *HTTPServer) handleProjects(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.core.Projects(ProjectListRequest{Workspace: r.URL.Query().Get("workspace")}))
	case http.MethodPost:
		var req ProjectOpenRequest
		if !decodeJSONHandler(w, r, &req) {
			return
		}
		result, err := s.core.OpenProject(req)
		writeResult(w, result, err)
	default:
		writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
	}
}

func (s *HTTPServer) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	req := SnapshotRequest{
		Workspace: r.URL.Query().Get("workspace"),
		Backend:   r.URL.Query().Get("backend"),
		Limit:     intQuery(r, "limit", 100),
	}
	if r.Method == http.MethodPost {
		if err := decodeRequest(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
	}
	result, err := s.core.Snapshot(r.Context(), req)
	writeResult(w, result, err)
}

func (s *HTTPServer) handleAgentsThreads(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	archived, err := boolPtrQuery(r, "archived")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := s.core.AgentsThreads(r.Context(), AgentsThreadsRequest{
		Workspace: r.URL.Query().Get("workspace"),
		Backend:   r.URL.Query().Get("backend"),
		Runtime:   r.URL.Query().Get("runtime"),
		Archived:  archived,
	})
	writeResult(w, result, err)
}

func (s *HTTPServer) handleRuntimesStatus(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	result, err := s.core.RuntimeStatus(r.Context(), RuntimeStatusRequest{
		Workspace: r.URL.Query().Get("workspace"),
		Backend:   r.URL.Query().Get("backend"),
	})
	writeResult(w, result, err)
}

func (s *HTTPServer) handleAttention(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	result, err := s.core.Attention(r.Context(), AttentionRequest{
		Status:    contract.AttentionStatus(r.URL.Query().Get("status")),
		Action:    contract.AttentionAction(r.URL.Query().Get("action")),
		SessionID: r.URL.Query().Get("session_id"),
		Limit:     intQuery(r, "limit", 50),
	})
	writeResult(w, result, err)
}

func (s *HTTPServer) handleRecentEvents(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	writeJSON(w, http.StatusOK, s.core.RecentEventsFiltered(eventFilterFromRequest(r, 100)))
}

func (s *HTTPServer) handleEventStream(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	filter := eventFilterFromRequest(r, 100)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, errors.New("streaming is unavailable"))
		return
	}
	for _, event := range s.core.RecentEventsFiltered(filter) {
		if err := writeSSE(w, event); err != nil {
			return
		}
	}
	flusher.Flush()

	events, unsubscribe := s.core.SubscribeEvents(128)
	defer unsubscribe()
	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			_, _ = io.WriteString(w, ": heartbeat\n\n")
			flusher.Flush()
		case event, ok := <-events:
			if !ok {
				return
			}
			if !eventMatchesFilter(event, filter) {
				continue
			}
			if err := writeSSE(w, event); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (s *HTTPServer) handleDescribe(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	writeJSON(w, http.StatusOK, s.core.service.Describe())
}

func (s *HTTPServer) handleStartSession(w http.ResponseWriter, r *http.Request) {
	var req StartSessionRequest
	if !decodeJSONHandler(w, r, &req) {
		return
	}
	result, err := s.core.StartSession(r.Context(), req)
	writeResult(w, result, err)
}

func (s *HTTPServer) handleResumeSession(w http.ResponseWriter, r *http.Request) {
	var req ResumeSessionRequest
	if !decodeJSONHandler(w, r, &req) {
		return
	}
	result, err := s.core.ResumeSession(r.Context(), req)
	writeResult(w, result, err)
}

func (s *HTTPServer) handleSendSessionInput(w http.ResponseWriter, r *http.Request) {
	var req SendSessionInputRequest
	if !decodeJSONHandler(w, r, &req) {
		return
	}
	result, err := s.core.SendSessionInput(r.Context(), req)
	writeResult(w, result, err)
}

func (s *HTTPServer) handleRespondSession(w http.ResponseWriter, r *http.Request) {
	var req RespondSessionRequest
	if !decodeJSONHandler(w, r, &req) {
		return
	}
	result, err := s.core.RespondSession(r.Context(), req)
	writeResult(w, result, err)
}

func (s *HTTPServer) handleInterruptSession(w http.ResponseWriter, r *http.Request) {
	var req SessionIDRequest
	if !decodeJSONHandler(w, r, &req) {
		return
	}
	result, err := s.core.InterruptSession(r.Context(), req)
	writeResult(w, result, err)
}

func (s *HTTPServer) handleStopSession(w http.ResponseWriter, r *http.Request) {
	var req SessionIDRequest
	if !decodeJSONHandler(w, r, &req) {
		return
	}
	result, err := s.core.StopSession(r.Context(), req)
	writeResult(w, result, err)
}

func (s *HTTPServer) handleInteractionCall(w http.ResponseWriter, r *http.Request) {
	var req InteractionCallRequest
	if !decodeJSONHandler(w, r, &req) {
		return
	}
	result, err := s.core.InteractionCall(r.Context(), req)
	writeResult(w, result, err)
}

func (s *HTTPServer) handleSpeechSubmit(w http.ResponseWriter, r *http.Request) {
	var req SpeechRequest
	if !decodeJSONHandler(w, r, &req) {
		return
	}
	result, err := s.core.SpeechSubmit(r.Context(), req)
	writeResult(w, result, err)
}

func (s *HTTPServer) handleSpeechSubscribe(w http.ResponseWriter, r *http.Request) {
	var req SpeechRequest
	if !decodeJSONHandler(w, r, &req) {
		return
	}
	result, err := s.core.SpeechSubscribe(r.Context(), req)
	writeResult(w, result, err)
}

func (s *HTTPServer) handleSpeechUnsubscribe(w http.ResponseWriter, r *http.Request) {
	var req SpeechRequest
	if !decodeJSONHandler(w, r, &req) {
		return
	}
	result, err := s.core.SpeechUnsubscribe(r.Context(), req)
	writeResult(w, result, err)
}

func (s *HTTPServer) handleTTS(w http.ResponseWriter, r *http.Request) {
	var req SpeechRequest
	if !decodeJSONHandler(w, r, &req) {
		return
	}
	result, err := s.core.TTS(r.Context(), req)
	writeResult(w, result, err)
}

func (s *HTTPServer) handleVoiceStatus(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	result, err := s.core.VoiceStatus(r.Context())
	writeResult(w, result, err)
}

func (s *HTTPServer) handleVoiceClaim(w http.ResponseWriter, r *http.Request) {
	var req VoiceClaimRequest
	if !decodeJSONHandler(w, r, &req) {
		return
	}
	result, err := s.core.ClaimVoice(r.Context(), req)
	writeResult(w, result, err)
}

func (s *HTTPServer) handleVoiceAssign(w http.ResponseWriter, r *http.Request) {
	var req VoiceAssignRequest
	if !decodeJSONHandler(w, r, &req) {
		return
	}
	result, err := s.core.AssignVoice(req)
	writeResult(w, result, err)
}

func (s *HTTPServer) handleVoiceRelease(w http.ResponseWriter, r *http.Request) {
	var req VoiceReleaseRequest
	if !decodeJSONHandler(w, r, &req) {
		return
	}
	writeJSON(w, http.StatusOK, s.core.ReleaseVoice(req))
}

func (s *HTTPServer) handleNotificationAudioCatalog(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	result, err := s.core.NotificationAudioCatalog(r.Context())
	writeResult(w, result, err)
}

func (s *HTTPServer) handleNotificationAudioStatus(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	result, err := s.core.NotificationAudioStatus(r.Context())
	writeResult(w, result, err)
}

func (s *HTTPServer) handleNotificationAudioPlay(w http.ResponseWriter, r *http.Request) {
	var req NotificationRequest
	if !decodeJSONHandler(w, r, &req) {
		return
	}
	result, err := s.core.NotificationAudioPlay(r.Context(), req)
	writeResult(w, result, err)
}

func (s *HTTPServer) handleNotificationAudioStop(w http.ResponseWriter, r *http.Request) {
	var req NotificationRequest
	if !decodeJSONHandler(w, r, &req) {
		return
	}
	result, err := s.core.NotificationAudioStop(r.Context(), req)
	writeResult(w, result, err)
}

func (s *HTTPServer) handleCourtCatalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	req := CourtCatalogRequest{
		Workspace: r.URL.Query().Get("workspace"),
		Backend:   r.URL.Query().Get("backend"),
		PresetID:  firstNonEmpty(r.URL.Query().Get("preset_id"), r.URL.Query().Get("preset")),
	}
	if r.Method == http.MethodPost {
		if err := decodeRequest(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
	}
	result, err := s.core.CourtCatalog(r.Context(), req)
	writeResult(w, result, err)
}

func (s *HTTPServer) handleCourtRuns(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		result, err := s.core.ListCourtRuns(r.Context())
		writeResult(w, result, err)
	case http.MethodPost:
		var req StartCourtRunRequest
		if err := decodeRequest(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		result, err := s.core.StartCourtRun(r.Context(), req)
		writeResult(w, result, err)
	default:
		writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
	}
}

func (s *HTTPServer) handleCourtRunPath(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/court/runs/"), "/"), "/")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
		writeError(w, http.StatusNotFound, errors.New("unknown court run route"))
		return
	}
	runID := parts[0]
	switch parts[1] {
	case "monitor":
		result, err := s.core.CourtMonitor(r.Context(), CourtMonitorRequest{
			RunID:      runID,
			EventLimit: intQuery(r, "events", 40),
		})
		writeResult(w, result, err)
	case "trace":
		result, err := s.core.CourtTrace(r.Context(), runID)
		writeResult(w, result, err)
	default:
		writeError(w, http.StatusNotFound, errors.New("unknown court run route"))
	}
}

func (s *HTTPServer) handleCourtRuntimeRespond(w http.ResponseWriter, r *http.Request) {
	var req CourtRuntimeResponseRequest
	if !decodeJSONHandler(w, r, &req) {
		return
	}
	result, err := s.core.CourtRespondRuntimeRequest(r.Context(), req)
	writeResult(w, result, err)
}

func (s *HTTPServer) handleCourtWorkerControl(w http.ResponseWriter, r *http.Request) {
	var req CourtWorkerControlRequest
	if !decodeJSONHandler(w, r, &req) {
		return
	}
	result, err := s.core.CourtControlWorker(r.Context(), req)
	writeResult(w, result, err)
}

func decodeJSONHandler(w http.ResponseWriter, r *http.Request, target any) bool {
	if !requireMethod(w, r, http.MethodPost) {
		return false
	}
	if err := decodeRequest(r, target); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return false
	}
	return true
}

func decodeRequest(r *http.Request, target any) error {
	defer func() { _ = r.Body.Close() }()
	decoder := json.NewDecoder(io.LimitReader(r.Body, 4*1024*1024))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	return nil
}

func writeResult(w http.ResponseWriter, value any, err error) {
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"message": err.Error(),
			"status":  status,
		},
	})
}

func requireMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method == method {
		return true
	}
	writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
	return false
}

func writeSSE(w io.Writer, event ObservedEvent) error {
	encoded, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "id: %d\nevent: control-event\ndata: %s\n\n", event.Sequence, encoded)
	return err
}

func intQuery(r *http.Request, key string, fallback int) int {
	value, err := strconv.Atoi(r.URL.Query().Get(key))
	if err != nil {
		return fallback
	}
	return value
}

func int64Query(r *http.Request, key string, fallback int64) int64 {
	value, err := strconv.ParseInt(r.URL.Query().Get(key), 10, 64)
	if err != nil {
		return fallback
	}
	return value
}

func eventFilterFromRequest(r *http.Request, fallbackLimit int) EventFilter {
	return EventFilter{
		SessionID: strings.TrimSpace(r.URL.Query().Get("session_id")),
		After:     int64Query(r, "after", 0),
		Limit:     intQuery(r, "limit", fallbackLimit),
	}
}

func boolPtrQuery(r *http.Request, key string) (*bool, error) {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return nil, fmt.Errorf("%s must be a boolean", key)
	}
	return &value, nil
}

func fileExists(fsys fs.FS, name string) bool {
	file, err := fsys.Open(name)
	if err != nil {
		return false
	}
	_ = file.Close()
	return true
}

func localAPIMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "http://localhost:5173")
		w.Header().Set("Access-Control-Allow-Headers", "content-type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
