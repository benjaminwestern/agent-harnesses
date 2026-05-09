package apphost

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

type ProjectManager struct {
	mu       sync.RWMutex
	path     string
	projects map[string]Project
}

type Project struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Path           string `json:"path"`
	Pinned         bool   `json:"pinned,omitempty"`
	CreatedAtMS    int64  `json:"created_at_ms"`
	UpdatedAtMS    int64  `json:"updated_at_ms"`
	LastOpenedAtMS int64  `json:"last_opened_at_ms"`
}

type ProjectListRequest struct {
	Workspace string `json:"workspace,omitempty"`
}

type ProjectListPage struct {
	GeneratedAt time.Time `json:"generated_at"`
	Selected    Project   `json:"selected"`
	Projects    []Project `json:"projects"`
}

type ProjectOpenRequest struct {
	Path   string `json:"path"`
	Name   string `json:"name,omitempty"`
	Pinned bool   `json:"pinned,omitempty"`
}

type projectStateFile struct {
	Projects []Project `json:"projects"`
}

func NewProjectManager(path string, initialProjects ...string) (*ProjectManager, error) {
	if strings.TrimSpace(path) == "" {
		configDir, err := os.UserConfigDir()
		if err != nil {
			return nil, err
		}
		path = filepath.Join(configDir, "agentic-control", "projects.json")
	}
	manager := &ProjectManager{
		path:     path,
		projects: make(map[string]Project),
	}
	if err := manager.load(); err != nil {
		return nil, err
	}
	for _, projectPath := range initialProjects {
		if _, err := manager.Open(ProjectOpenRequest{Path: projectPath}); err != nil {
			return nil, err
		}
	}
	return manager, nil
}

func (m *ProjectManager) List(selectedPath string) ProjectListPage {
	selectedPath = normalizeProject(selectedPath)

	m.mu.RLock()
	defer m.mu.RUnlock()

	projects := make([]Project, 0, len(m.projects))
	var selected Project
	for _, project := range m.projects {
		projects = append(projects, project)
		if project.Path == selectedPath {
			selected = project
		}
	}
	if selected.Path == "" && selectedPath != "" {
		selected = newProject(selectedPath, "", false)
		projects = append(projects, selected)
	}
	sortProjects(projects)
	return ProjectListPage{
		GeneratedAt: time.Now(),
		Selected:    selected,
		Projects:    projects,
	}
}

func (m *ProjectManager) Open(req ProjectOpenRequest) (Project, error) {
	projectPath := normalizeProject(req.Path)
	if projectPath == "" {
		return Project{}, errors.New("path is required")
	}
	now := time.Now().UnixMilli()

	m.mu.Lock()
	defer m.mu.Unlock()

	project := m.projects[projectPath]
	if project.Path == "" {
		project = newProject(projectPath, req.Name, req.Pinned)
		project.CreatedAtMS = now
	} else {
		project.Name = firstNonEmpty(strings.TrimSpace(req.Name), project.Name, projectNameFromPath(projectPath))
		project.Pinned = project.Pinned || req.Pinned
	}
	project.UpdatedAtMS = now
	project.LastOpenedAtMS = now
	m.projects[projectPath] = project
	if err := m.saveLocked(); err != nil {
		return Project{}, err
	}
	return project, nil
}

func (m *ProjectManager) load() error {
	data, err := os.ReadFile(m.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var state projectStateFile
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}
	for _, project := range state.Projects {
		project.Path = normalizeProject(project.Path)
		if project.Path == "" {
			continue
		}
		if project.Name == "" {
			project.Name = projectNameFromPath(project.Path)
		}
		if project.ID == "" {
			project.ID = projectID(project.Path)
		}
		m.projects[project.Path] = project
	}
	return nil
}

func (m *ProjectManager) saveLocked() error {
	projects := make([]Project, 0, len(m.projects))
	for _, project := range m.projects {
		projects = append(projects, project)
	}
	sortProjects(projects)
	data, err := json.MarshalIndent(projectStateFile{Projects: projects}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(m.path), 0o750); err != nil {
		return err
	}
	return os.WriteFile(m.path, append(data, '\n'), 0o600)
}

func newProject(path string, name string, pinned bool) Project {
	now := time.Now().UnixMilli()
	return Project{
		ID:             projectID(path),
		Name:           firstNonEmpty(strings.TrimSpace(name), projectNameFromPath(path)),
		Path:           path,
		Pinned:         pinned,
		CreatedAtMS:    now,
		UpdatedAtMS:    now,
		LastOpenedAtMS: now,
	}
}

func projectNameFromPath(path string) string {
	path = strings.TrimSpace(filepath.Clean(path))
	if path == "." || path == string(filepath.Separator) {
		return path
	}
	if base := filepath.Base(path); base != "" && base != "." {
		return base
	}
	return path
}

func projectID(path string) string {
	path = normalizeProject(path)
	replacer := strings.NewReplacer("/", "-", "\\", "-", ":", "-", " ", "-")
	id := strings.Trim(replacer.Replace(path), "-")
	if id == "" {
		return "project"
	}
	return id
}

func sortProjects(projects []Project) {
	slices.SortFunc(projects, func(left, right Project) int {
		if left.Pinned != right.Pinned {
			if left.Pinned {
				return -1
			}
			return 1
		}
		if left.LastOpenedAtMS != right.LastOpenedAtMS {
			if left.LastOpenedAtMS > right.LastOpenedAtMS {
				return -1
			}
			return 1
		}
		return strings.Compare(left.Name, right.Name)
	})
}

func (c *Core) Projects(req ProjectListRequest) ProjectListPage {
	workspace := firstNonEmpty(req.Workspace, c.workspace)
	return c.projects.List(workspace)
}

func (c *Core) OpenProject(req ProjectOpenRequest) (Project, error) {
	return c.projects.Open(req)
}
