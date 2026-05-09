package apphost

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectManagerOpenPersistsAndSorts(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "projects.json")
	alphaPath := filepath.Join(t.TempDir(), "alpha")
	betaPath := filepath.Join(t.TempDir(), "beta")

	manager, err := NewProjectManager(storePath, alphaPath)
	if err != nil {
		t.Fatalf("NewProjectManager: %v", err)
	}
	if _, err := manager.Open(ProjectOpenRequest{Path: betaPath, Pinned: true}); err != nil {
		t.Fatalf("Open beta: %v", err)
	}

	page := manager.List(alphaPath)
	if page.Selected.Path != normalizeProject(alphaPath) {
		t.Fatalf("selected path = %q, want %q", page.Selected.Path, normalizeProject(alphaPath))
	}
	if len(page.Projects) != 2 {
		t.Fatalf("projects length = %d, want 2", len(page.Projects))
	}
	if page.Projects[0].Path != normalizeProject(betaPath) {
		t.Fatalf("first project = %q, want pinned beta", page.Projects[0].Path)
	}

	reloaded, err := NewProjectManager(storePath)
	if err != nil {
		t.Fatalf("reload NewProjectManager: %v", err)
	}
	reloadedPage := reloaded.List(betaPath)
	if len(reloadedPage.Projects) != 2 {
		t.Fatalf("reloaded projects length = %d, want 2", len(reloadedPage.Projects))
	}
}

func TestProjectHTTPHandlerListsAndOpens(t *testing.T) {
	core, err := New(Options{
		Workspace:    t.TempDir(),
		Backend:      "opencode",
		ProjectStore: filepath.Join(t.TempDir(), "projects.json"),
	})
	if err != nil {
		t.Fatalf("New core: %v", err)
	}
	defer func() { _ = core.Close() }()
	server := NewHTTPServer(core)

	projectPath := filepath.Join(t.TempDir(), "repo")
	request := httptest.NewRequest(http.MethodPost, "/api/projects", strings.NewReader(`{"path":`+quoteJSON(projectPath)+`}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.APIHandler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("POST /api/projects status = %d body = %s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/api/projects?workspace="+projectPath, nil)
	response = httptest.NewRecorder()
	server.APIHandler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("GET /api/projects status = %d body = %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), projectNameFromPath(projectPath)) {
		t.Fatalf("project list did not include opened project: %s", response.Body.String())
	}
}

func quoteJSON(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
}
