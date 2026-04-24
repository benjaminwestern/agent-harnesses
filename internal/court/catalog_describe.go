package court

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	api "github.com/benjaminwestern/agentic-control/pkg/controlplane"
)

type CatalogStatus string

const (
	CatalogStatusSupported CatalogStatus = "supported"
	CatalogStatusWarning   CatalogStatus = "warning"
	CatalogStatusBlocked   CatalogStatus = "blocked"
)

type CatalogSourceKind string

const (
	CatalogSourceBuiltin  CatalogSourceKind = "builtin"
	CatalogSourceConfig   CatalogSourceKind = "config"
	CatalogSourceMarkdown CatalogSourceKind = "markdown"
)

type CatalogDiagnosticSeverity string

const (
	CatalogDiagnosticWarning CatalogDiagnosticSeverity = "warning"
	CatalogDiagnosticError   CatalogDiagnosticSeverity = "error"
)

type CatalogSource struct {
	Kind CatalogSourceKind `json:"kind"`
	Root string            `json:"root,omitempty"`
	Path string            `json:"path,omitempty"`
}

type CatalogDiagnostic struct {
	Severity     CatalogDiagnosticSeverity `json:"severity"`
	Code         string                    `json:"code"`
	Message      string                    `json:"message"`
	ResourceType string                    `json:"resource_type,omitempty"`
	ResourceID   string                    `json:"resource_id,omitempty"`
	Backend      string                    `json:"backend,omitempty"`
	Provider     string                    `json:"provider,omitempty"`
	Model        string                    `json:"model,omitempty"`
	Path         string                    `json:"path,omitempty"`
}

type CatalogRuntimeTarget struct {
	Backend      string              `json:"backend"`
	Provider     string              `json:"provider,omitempty"`
	Model        string              `json:"model,omitempty"`
	ModelOptions RuntimeModelOptions `json:"model_options,omitempty"`
	Status       CatalogStatus       `json:"status"`
	Diagnostics  []CatalogDiagnostic `json:"diagnostics,omitempty"`
}

type CatalogRoleDescriptor struct {
	ID          string               `json:"id"`
	Kind        RoleKind             `json:"kind"`
	Title       string               `json:"title"`
	Brief       string               `json:"brief"`
	Agent       string               `json:"agent,omitempty"`
	Source      CatalogSource        `json:"source"`
	Target      CatalogRuntimeTarget `json:"target"`
	Status      CatalogStatus        `json:"status"`
	Diagnostics []CatalogDiagnostic  `json:"diagnostics,omitempty"`
}

type CatalogWorkflowDescriptor struct {
	ID              string                  `json:"id"`
	Title           string                  `json:"title"`
	Description     string                  `json:"description,omitempty"`
	Workflow        WorkflowMode            `json:"workflow"`
	Source          CatalogSource           `json:"source"`
	SelectedBackend string                  `json:"selected_backend"`
	DefaultBackend  string                  `json:"default_backend"`
	Status          CatalogStatus           `json:"status"`
	Roles           []CatalogRoleDescriptor `json:"roles,omitempty"`
	Diagnostics     []CatalogDiagnostic     `json:"diagnostics,omitempty"`
}

type CatalogWorkflowSummary struct {
	ID              string        `json:"id"`
	Title           string        `json:"title"`
	Description     string        `json:"description,omitempty"`
	Workflow        WorkflowMode  `json:"workflow"`
	Source          CatalogSource `json:"source"`
	SelectedBackend string        `json:"selected_backend"`
	DefaultBackend  string        `json:"default_backend"`
	Status          CatalogStatus `json:"status"`
	DiagnosticCount int           `json:"diagnostic_count,omitempty"`
}

type CatalogListResult struct {
	Workspace       string                   `json:"workspace"`
	Roots           []string                 `json:"roots"`
	SelectedBackend string                   `json:"selected_backend"`
	DefaultBackend  string                   `json:"default_backend"`
	Workflows       []CatalogWorkflowSummary `json:"workflows"`
}

type CatalogGetResult struct {
	Workspace       string                    `json:"workspace"`
	Roots           []string                  `json:"roots"`
	SelectedBackend string                    `json:"selected_backend"`
	DefaultBackend  string                    `json:"default_backend"`
	Workflow        CatalogWorkflowDescriptor `json:"workflow"`
}

type CatalogValidateResult struct {
	Workspace       string                      `json:"workspace"`
	Roots           []string                    `json:"roots"`
	SelectedBackend string                      `json:"selected_backend"`
	DefaultBackend  string                      `json:"default_backend"`
	Workflows       []CatalogWorkflowDescriptor `json:"workflows"`
	Diagnostics     []CatalogDiagnostic         `json:"diagnostics,omitempty"`
}

type catalogContext struct {
	workspace       string
	roots           []string
	defaultBackend  string
	selectedBackend string
	defaults        RuntimeBackendConfig
}

func (e *Engine) CatalogList(workspace string, backend string) (CatalogListResult, error) {
	ctx, err := e.catalogContext(workspace, backend)
	if err != nil {
		return CatalogListResult{}, err
	}
	presetIDs, err := e.catalogPresetIDs(ctx.roots)
	if err != nil {
		return CatalogListResult{}, err
	}
	workflows := make([]CatalogWorkflowSummary, 0, len(presetIDs))
	for _, presetID := range presetIDs {
		workflow, err := e.catalogWorkflowDescriptor(ctx, presetID)
		if err != nil {
			return CatalogListResult{}, err
		}
		workflows = append(workflows, CatalogWorkflowSummary{
			ID:              workflow.ID,
			Title:           workflow.Title,
			Description:     workflow.Description,
			Workflow:        workflow.Workflow,
			Source:          workflow.Source,
			SelectedBackend: workflow.SelectedBackend,
			DefaultBackend:  workflow.DefaultBackend,
			Status:          workflow.Status,
			DiagnosticCount: len(workflow.Diagnostics),
		})
	}
	return CatalogListResult{
		Workspace:       ctx.workspace,
		Roots:           ctx.roots,
		SelectedBackend: ctx.selectedBackend,
		DefaultBackend:  ctx.defaultBackend,
		Workflows:       workflows,
	}, nil
}

func (e *Engine) CatalogGet(workspace string, presetID string, backend string) (CatalogGetResult, error) {
	ctx, err := e.catalogContext(workspace, backend)
	if err != nil {
		return CatalogGetResult{}, err
	}
	workflow, err := e.catalogWorkflowDescriptor(ctx, strings.TrimSpace(presetID))
	if err != nil {
		return CatalogGetResult{}, err
	}
	if workflow.ID == "" {
		return CatalogGetResult{}, fmt.Errorf("preset %q not found", presetID)
	}
	return CatalogGetResult{
		Workspace:       ctx.workspace,
		Roots:           ctx.roots,
		SelectedBackend: ctx.selectedBackend,
		DefaultBackend:  ctx.defaultBackend,
		Workflow:        workflow,
	}, nil
}

func (e *Engine) CatalogValidate(workspace string, presetID string, backend string) (CatalogValidateResult, error) {
	ctx, err := e.catalogContext(workspace, backend)
	if err != nil {
		return CatalogValidateResult{}, err
	}
	ids := []string{strings.TrimSpace(presetID)}
	if ids[0] == "" {
		ids, err = e.catalogPresetIDs(ctx.roots)
		if err != nil {
			return CatalogValidateResult{}, err
		}
	}
	workflows := make([]CatalogWorkflowDescriptor, 0, len(ids))
	var diagnostics []CatalogDiagnostic
	for _, id := range ids {
		workflow, err := e.catalogWorkflowDescriptor(ctx, id)
		if err != nil {
			return CatalogValidateResult{}, err
		}
		if workflow.ID == "" {
			workflow = blockedWorkflowDescriptor(id, ctx.selectedBackend, ctx.defaultBackend, CatalogSource{}, CatalogDiagnostic{
				Severity:     CatalogDiagnosticError,
				Code:         "preset_not_found",
				Message:      fmt.Sprintf("preset %q not found", id),
				ResourceType: "preset",
				ResourceID:   id,
			})
		}
		workflows = append(workflows, workflow)
		diagnostics = append(diagnostics, workflow.Diagnostics...)
	}
	return CatalogValidateResult{
		Workspace:       ctx.workspace,
		Roots:           ctx.roots,
		SelectedBackend: ctx.selectedBackend,
		DefaultBackend:  ctx.defaultBackend,
		Workflows:       workflows,
		Diagnostics:     diagnostics,
	}, nil
}

func (e *Engine) catalogContext(workspace string, backend string) (catalogContext, error) {
	if strings.TrimSpace(workspace) == "" {
		workspace = "."
	}
	absWorkspace, err := filepath.Abs(workspace)
	if err != nil {
		return catalogContext{}, wrapErr("resolve workspace", err)
	}
	roots, err := e.catalogRoots(absWorkspace)
	if err != nil {
		return catalogContext{}, err
	}
	config, err := LoadCourtConfigFromRoots(roots)
	if err != nil {
		return catalogContext{}, err
	}
	defaultBackend := firstNonEmpty(config.Defaults.Backend, "opencode")
	selectedBackend := firstNonEmpty(strings.TrimSpace(backend), defaultBackend)
	return catalogContext{
		workspace:       absWorkspace,
		roots:           roots,
		defaultBackend:  defaultBackend,
		selectedBackend: selectedBackend,
		defaults:        config.Defaults.BackendDefaults(selectedBackend),
	}, nil
}

func (e *Engine) catalogPresetIDs(roots []string) ([]string, error) {
	config, err := LoadCourtConfigFromRoots(roots)
	if err != nil {
		return nil, err
	}
	ids := map[string]struct{}{}
	for id := range config.Presets {
		ids[id] = struct{}{}
	}
	for _, root := range roots {
		entries, err := os.ReadDir(filepath.Join(root, "presets"))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, wrapErr("read preset catalog directory", err)
		}
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != markdownExtension {
				continue
			}
			ids[strings.TrimSuffix(entry.Name(), markdownExtension)] = struct{}{}
		}
	}
	for _, preset := range ListPresets() {
		ids[preset.ID] = struct{}{}
	}
	out := sortedKeys(ids)
	sort.Strings(out)
	return out, nil
}

func (e *Engine) catalogWorkflowDescriptor(ctx catalogContext, presetID string) (CatalogWorkflowDescriptor, error) {
	preset, source, found, loadErr := e.loadCatalogPreset(ctx.roots, presetID)
	if !found {
		return CatalogWorkflowDescriptor{}, nil
	}
	if loadErr != nil {
		return blockedWorkflowDescriptor(presetID, ctx.selectedBackend, ctx.defaultBackend, source, CatalogDiagnostic{
			Severity:     CatalogDiagnosticError,
			Code:         "preset_load_failed",
			Message:      loadErr.Error(),
			ResourceType: "preset",
			ResourceID:   presetID,
			Path:         source.Path,
		}), nil
	}

	workflow := CatalogWorkflowDescriptor{
		ID:              preset.ID,
		Title:           preset.Title,
		Description:     preset.Description,
		Workflow:        preset.Workflow,
		Source:          source,
		SelectedBackend: ctx.selectedBackend,
		DefaultBackend:  ctx.defaultBackend,
		Status:          CatalogStatusSupported,
	}
	roles := make([]CatalogRoleDescriptor, 0, len(preset.Roles))
	for _, role := range preset.Roles {
		roleSource := e.locateRoleSource(ctx.roots, role.ID, source.Kind == CatalogSourceBuiltin)
		roleDescriptor := e.catalogRoleDescriptor(role, roleSource, ctx)
		roles = append(roles, roleDescriptor)
		workflow.Diagnostics = append(workflow.Diagnostics, roleDescriptor.Diagnostics...)
	}
	workflow.Roles = roles
	workflow.Status = catalogStatusFromDiagnostics(workflow.Diagnostics)
	return workflow, nil
}

func (e *Engine) loadCatalogPreset(roots []string, presetID string) (Preset, CatalogSource, bool, error) {
	presetID = strings.TrimSpace(presetID)
	if presetID == "" {
		return Preset{}, CatalogSource{}, false, nil
	}
	source := e.locatePresetSource(roots, presetID)
	preset, ok, err := LoadPresetFromRoots(roots, presetID)
	if err != nil {
		if source.Kind != "" {
			return Preset{}, source, true, err
		}
		return Preset{}, CatalogSource{}, false, err
	}
	if ok {
		return preset, source, true, nil
	}
	preset, err = ResolvePreset(presetID)
	if err != nil {
		return Preset{}, CatalogSource{}, false, nil
	}
	return preset, CatalogSource{Kind: CatalogSourceBuiltin, Path: "builtin:presets/" + preset.ID}, true, nil
}

func (e *Engine) catalogRoleDescriptor(role Role, source CatalogSource, ctx catalogContext) CatalogRoleDescriptor {
	target := resolveCatalogRuntimeTarget(role, ctx.selectedBackend, ctx.defaults)
	status, diagnostics := e.validateCatalogRuntimeTarget(role, source, target)
	return CatalogRoleDescriptor{
		ID:          role.ID,
		Kind:        role.Kind,
		Title:       role.Title,
		Brief:       role.Brief,
		Agent:       role.Agent,
		Source:      source,
		Target:      target,
		Status:      status,
		Diagnostics: diagnostics,
	}
}

func resolveCatalogRuntimeTarget(role Role, backend string, defaults RuntimeBackendConfig) CatalogRuntimeTarget {
	scoped := backendConfigFor(role.Backends, backend)
	roleModel := ""
	roleProvider := ""
	roleModelOptions := RuntimeModelOptions{}
	if role.Backend == "" || role.Backend == backend {
		roleModel = role.Model
		roleProvider = role.Provider
		roleModelOptions = role.ModelOptions
	}
	model := firstNonEmpty(scoped.Model, roleModel, defaults.Model)
	return CatalogRuntimeTarget{
		Backend:      backend,
		Provider:     firstNonEmpty(scoped.Provider, roleProvider, defaults.Provider, api.InferModelProvider(model)),
		Model:        model,
		ModelOptions: api.MergeModelOptions(scoped.ModelOptions, roleModelOptions, defaults.ModelOptions),
		Status:       CatalogStatusSupported,
	}
}

func (e *Engine) validateCatalogRuntimeTarget(role Role, source CatalogSource, target CatalogRuntimeTarget) (CatalogStatus, []CatalogDiagnostic) {
	validation := api.ValidateSessionTarget(e.controlPlane.Describe().Runtimes, api.RuntimeTarget{
		Backend:  target.Backend,
		Provider: target.Provider,
		Model:    target.Model,
		Options:  target.ModelOptions,
	})
	diagnostics := make([]CatalogDiagnostic, 0, len(validation.Issues))
	for _, issue := range validation.Issues {
		severity := CatalogDiagnosticWarning
		if issue.Severity == api.ValidationSeverityError {
			severity = CatalogDiagnosticError
		}
		diagnostics = append(diagnostics, CatalogDiagnostic{
			Severity:     severity,
			Code:         issue.Code,
			Message:      issue.Message,
			ResourceType: "role",
			ResourceID:   role.ID,
			Backend:      target.Backend,
			Provider:     target.Provider,
			Model:        target.Model,
			Path:         source.Path,
		})
	}
	return catalogStatusFromDiagnostics(diagnostics), diagnostics
}

func (e *Engine) locatePresetSource(roots []string, presetID string) CatalogSource {
	if source, ok, err := locateConfigCatalogSource(roots, "preset", presetID); err == nil && ok {
		return source
	}
	if source, ok := locateMarkdownCatalogSource(roots, []string{"presets"}, presetID); ok {
		return source
	}
	return CatalogSource{}
}

func (e *Engine) locateRoleSource(roots []string, roleID string, builtin bool) CatalogSource {
	if builtin {
		return CatalogSource{Kind: CatalogSourceBuiltin, Path: "builtin:roles/" + roleID}
	}
	if source, ok, err := locateConfigCatalogSource(roots, "role", roleID); err == nil && ok {
		return source
	}
	if source, ok := locateMarkdownCatalogSource(roots, []string{"roles", "jurors", "judges", "clerks"}, roleID); ok {
		return source
	}
	return CatalogSource{Kind: CatalogSourceBuiltin, Path: "builtin:roles/" + roleID}
}

func locateConfigCatalogSource(roots []string, resourceType string, id string) (CatalogSource, bool, error) {
	for i := len(roots) - 1; i >= 0; i-- {
		config, err := LoadCourtConfigFromRoots([]string{roots[i]})
		if err != nil {
			return CatalogSource{}, false, err
		}
		if configContainsResource(config, resourceType, id) {
			return CatalogSource{
				Kind: CatalogSourceConfig,
				Root: roots[i],
				Path: filepath.Join(roots[i], configFileName),
			}, true, nil
		}
	}
	return CatalogSource{}, false, nil
}

func locateMarkdownCatalogSource(roots []string, subdirs []string, id string) (CatalogSource, bool) {
	path, ok := findMarkdownFile(roots, subdirs, id)
	if !ok {
		return CatalogSource{}, false
	}
	return CatalogSource{
		Kind: CatalogSourceMarkdown,
		Root: filepath.Dir(filepath.Dir(path)),
		Path: path,
	}, true
}

func configContainsResource(config Config, resourceType string, id string) bool {
	switch resourceType {
	case "agent":
		_, ok := config.Agents[id]
		return ok
	case "role":
		_, ok := config.Roles[id]
		return ok
	case "jury":
		_, ok := config.Juries[id]
		return ok
	case "preset":
		_, ok := config.Presets[id]
		return ok
	default:
		return false
	}
}

func blockedWorkflowDescriptor(id string, selectedBackend string, defaultBackend string, source CatalogSource, diagnostic CatalogDiagnostic) CatalogWorkflowDescriptor {
	return CatalogWorkflowDescriptor{
		ID:              id,
		Title:           id,
		Workflow:        WorkflowParallelConsensus,
		Source:          source,
		SelectedBackend: selectedBackend,
		DefaultBackend:  defaultBackend,
		Status:          CatalogStatusBlocked,
		Diagnostics:     []CatalogDiagnostic{diagnostic},
	}
}

func catalogStatusFromDiagnostics(diagnostics []CatalogDiagnostic) CatalogStatus {
	status := CatalogStatusSupported
	for _, diagnostic := range diagnostics {
		switch diagnostic.Severity {
		case CatalogDiagnosticError:
			return CatalogStatusBlocked
		case CatalogDiagnosticWarning:
			status = CatalogStatusWarning
		}
	}
	return status
}
