package apphost

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	internalcp "github.com/benjaminwestern/agentic-control/internal/controlplane"
	"github.com/benjaminwestern/agentic-control/pkg/contract"
)

func TestAttentionPageFiltersLimitsAndReportsTotal(t *testing.T) {
	core := newReadModelTestCore(t)
	lowPriority := core.service.EnqueueAttention(contract.AttentionItem{
		Status:    contract.AttentionStatusQueued,
		Action:    contract.AttentionActionSpeak,
		SessionID: "session-1",
		Priority:  10,
		Text:      "low",
	})
	highPriority := core.service.EnqueueAttention(contract.AttentionItem{
		Status:    contract.AttentionStatusQueued,
		Action:    contract.AttentionActionSpeak,
		SessionID: "session-1",
		Priority:  50,
		Text:      "high",
	})
	core.service.EnqueueAttention(contract.AttentionItem{
		Status:    contract.AttentionStatusActive,
		Action:    contract.AttentionActionSpeak,
		SessionID: "session-1",
		Priority:  100,
		Text:      "active",
	})

	page, err := core.Attention(context.Background(), AttentionRequest{
		Status: contract.AttentionStatusQueued,
		Action: contract.AttentionActionSpeak,
		Limit:  1,
	})
	if err != nil {
		t.Fatalf("Attention returned error: %v", err)
	}
	if page.Total != 2 {
		t.Fatalf("total = %d, want 2", page.Total)
	}
	if len(page.Items) != 1 {
		t.Fatalf("items length = %d, want 1", len(page.Items))
	}
	if page.Items[0].ID != highPriority.ID {
		t.Fatalf("first item = %q, want high priority %q", page.Items[0].ID, highPriority.ID)
	}
	if page.Items[0].ID == lowPriority.ID {
		t.Fatalf("low priority item should be limited out")
	}
	if page.Filters.Limit != 1 {
		t.Fatalf("limit filter = %d, want 1", page.Filters.Limit)
	}
}

func TestHTTPAttentionEndpointUsesQueryFilters(t *testing.T) {
	core := newReadModelTestCore(t)
	wanted := core.service.EnqueueAttention(contract.AttentionItem{
		Status:    contract.AttentionStatusQueued,
		Action:    contract.AttentionActionAsk,
		SessionID: "session-1",
		Priority:  20,
		Text:      "first",
	})
	core.service.EnqueueAttention(contract.AttentionItem{
		Status:    contract.AttentionStatusQueued,
		Action:    contract.AttentionActionAsk,
		SessionID: "session-2",
		Priority:  40,
		Text:      "second",
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/attention?status=queued&action=ask&session_id=session-1&limit=10", nil)
	NewHTTPServer(core).APIHandler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}

	var page AttentionPage
	if err := json.NewDecoder(recorder.Body).Decode(&page); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("page total/items = %d/%d, want 1/1", page.Total, len(page.Items))
	}
	if page.Items[0].ID != wanted.ID {
		t.Fatalf("item = %q, want %q", page.Items[0].ID, wanted.ID)
	}
	if page.Filters.SessionID != "session-1" {
		t.Fatalf("session filter = %q, want session-1", page.Filters.SessionID)
	}
}

func TestHTTPAgentsThreadsEndpointParsesArchivedFilter(t *testing.T) {
	core := newReadModelTestCore(t)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/agents/threads?archived=false", nil)
	NewHTTPServer(core).APIHandler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}

	var page AgentsThreadsPage
	if err := json.NewDecoder(recorder.Body).Decode(&page); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if page.Filters.Archived == nil || *page.Filters.Archived {
		t.Fatalf("archived filter = %#v, want false pointer", page.Filters.Archived)
	}
}

func TestHTTPAgentsThreadsEndpointRejectsInvalidArchivedFilter(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/agents/threads?archived=maybe", nil)
	NewHTTPServer(&Core{}).APIHandler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func newReadModelTestCore(t *testing.T) *Core {
	t.Helper()
	t.Setenv("AGENTIC_CONTROL_STATE_DB", filepath.Join(t.TempDir(), "controlplane.db"))
	t.Setenv("AGENTIC_CONTROL_EVENT_LOG", "")
	t.Setenv("AGENTIC_INTERACTION_RPC_SOCKET", filepath.Join(t.TempDir(), "missing.sock"))

	voices, err := NewVoiceManager(filepath.Join(t.TempDir(), "voices.json"))
	if err != nil {
		t.Fatalf("NewVoiceManager: %v", err)
	}
	service := internalcp.NewService()
	t.Cleanup(func() {
		if err := service.Close(); err != nil {
			t.Fatalf("close service: %v", err)
		}
	})
	return &Core{
		service:   service,
		voices:    voices,
		workspace: filepath.Join(t.TempDir(), "workspace"),
		backend:   "opencode",
	}
}
