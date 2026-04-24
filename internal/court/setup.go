// Package court provides Court runtime functionality.
package court

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	api "github.com/benjaminwestern/agentic-control/pkg/controlplane"
)

//go:embed defaults/catalog
var defaultCatalog embed.FS

// SetupScope defines Court runtime data.
type SetupScope string

const (
	// SetupScopeGlobal defines a Court runtime value.
	SetupScopeGlobal SetupScope = "global"
	// SetupScopeProject defines a Court runtime value.
	SetupScopeProject SetupScope = "project"
)

// SetupFileAction defines Court runtime data.
type SetupFileAction string

const (
	// SetupFileCreated defines a Court runtime value.
	SetupFileCreated SetupFileAction = "created"
	// SetupFileOverwritten defines a Court runtime value.
	SetupFileOverwritten SetupFileAction = "overwritten"
	// SetupFileSkipped defines a Court runtime value.
	SetupFileSkipped SetupFileAction = "skipped"
	// SetupFileWouldCreate defines a Court runtime value.
	SetupFileWouldCreate SetupFileAction = "would_create"
	// SetupFileWouldOverwrite defines a Court runtime value.
	SetupFileWouldOverwrite SetupFileAction = "would_overwrite"
	// SetupFileWouldSkip defines a Court runtime value.
	SetupFileWouldSkip SetupFileAction = "would_skip"
)

// SetupDefaultsRequest defines Court runtime data.
type SetupDefaultsRequest struct {
	Scope     SetupScope `json:"scope,omitempty"`
	TargetDir string     `json:"target_dir,omitempty"`
	Workspace string     `json:"workspace,omitempty"`
	Force     bool       `json:"force,omitempty"`
	DryRun    bool       `json:"dry_run,omitempty"`
}

// SetupDefaultsResult defines Court runtime data.
type SetupDefaultsResult struct {
	Scope        SetupScope        `json:"scope"`
	TargetDir    string            `json:"target_dir"`
	Files        []SetupFileResult `json:"files"`
	PresetCount  int               `json:"preset_count"`
	JuryCount    int               `json:"jury_count"`
	RoleCount    int               `json:"role_count"`
	AgentCount   int               `json:"agent_count"`
	CreatedCount int               `json:"created_count"`
	UpdatedCount int               `json:"updated_count"`
	SkippedCount int               `json:"skipped_count"`
	DryRun       bool              `json:"dry_run,omitempty"`
}

// InitDefaultsResult defines Court runtime data.
type InitDefaultsResult struct {
	Setup  SetupDefaultsResult `json:"setup"`
	Config SetupFileResult     `json:"config"`
}

// SetupFileResult defines Court runtime data.
type SetupFileResult struct {
	Path   string          `json:"path"`
	Action SetupFileAction `json:"action"`
}

// SetupDefaults provides Court runtime functionality.
func SetupDefaults(req SetupDefaultsRequest) (SetupDefaultsResult, error) {
	scope := req.Scope
	if scope == "" {
		scope = SetupScopeGlobal
	}
	targetDir, err := setupTargetDir(scope, req.TargetDir, req.Workspace)
	if err != nil {
		return SetupDefaultsResult{}, err
	}
	result := SetupDefaultsResult{
		Scope:     scope,
		TargetDir: targetDir,
		DryRun:    req.DryRun,
	}

	err = fs.WalkDir(defaultCatalog, "defaults/catalog", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return wrapErr("walk embedded defaults", walkErr)
		}
		if path == "defaults/catalog" {
			return nil
		}
		rel, err := filepath.Rel("defaults/catalog", path)
		if err != nil {
			return wrapErr("resolve embedded default path", err)
		}
		target := filepath.Join(targetDir, filepath.FromSlash(rel))
		if entry.IsDir() {
			if req.DryRun {
				return nil
			}
			return wrapErr("create setup target directory", os.MkdirAll(target, 0o750))
		}
		if !strings.HasSuffix(entry.Name(), ".md") {
			return nil
		}
		action, err := writeDefaultCatalogFile(path, target, req.Force, req.DryRun)
		if err != nil {
			return err
		}
		result.Files = append(result.Files, SetupFileResult{Path: target, Action: action})
		switch action {
		case SetupFileCreated, SetupFileWouldCreate:
			result.CreatedCount++
		case SetupFileOverwritten, SetupFileWouldOverwrite:
			result.UpdatedCount++
		case SetupFileSkipped, SetupFileWouldSkip:
			result.SkippedCount++
		}
		countSetupFile(&result, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return SetupDefaultsResult{}, wrapErr("walk embedded catalog", err)
	}
	return result, nil
}

func setupTargetDir(scope SetupScope, targetDir string, workspace string) (string, error) {
	if strings.TrimSpace(targetDir) != "" {
		abs, err := filepath.Abs(targetDir)
		return abs, wrapErr("resolve setup target", err)
	}
	switch scope {
	case SetupScopeGlobal:
		return DefaultConfigDir()
	case SetupScopeProject:
		if strings.TrimSpace(workspace) == "" {
			workspace = "."
		}
		abs, err := filepath.Abs(workspace)
		if err != nil {
			return "", wrapErr("resolve setup workspace", err)
		}
		if info, err := os.Stat(abs); err == nil && !info.IsDir() {
			abs = filepath.Dir(abs)
		}
		return filepath.Join(abs, projectConfigDirName), nil
	default:
		return "", fmt.Errorf("unknown setup scope %q", scope)
	}
}

func writeDefaultCatalogFile(sourcePath string, targetPath string, force bool, dryRun bool) (SetupFileAction, error) {
	data, err := defaultCatalog.ReadFile(sourcePath)
	if err != nil {
		return "", wrapErr("read embedded catalog file", err)
	}
	exists, err := fileExists(targetPath)
	if err != nil {
		return "", err
	}
	if exists {
		return writeExistingFile(targetPath, data, force, dryRun)
	}
	if dryRun {
		return SetupFileWouldCreate, nil
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o750); err != nil {
		return "", wrapErr("create setup target directory", err)
	}
	if err := os.WriteFile(targetPath, data, 0o600); err != nil {
		return "", wrapErr("write setup target file", err)
	}
	return SetupFileCreated, nil
}

// WriteDefaultConfig provides Court runtime functionality.
func WriteDefaultConfig(scope SetupScope, targetDir string, workspace string, backend string, model string, force bool, dryRun bool) (SetupFileResult, error) {
	if scope == "" {
		scope = SetupScopeGlobal
	}
	targetDir, err := setupTargetDir(scope, targetDir, workspace)
	if err != nil {
		return SetupFileResult{}, err
	}
	if backend == "" {
		backend = "opencode"
	}
	if model == "" {
		model = "opencode/gemini-3-flash"
	}
	target := filepath.Join(targetDir, configFileName)
	action, err := writeConfigFile(target, DefaultConfigTOML(backend, model), force, dryRun)
	if err != nil {
		return SetupFileResult{}, err
	}
	return SetupFileResult{Path: target, Action: action}, nil
}

// DefaultConfigTOML provides Court runtime functionality.
func DefaultConfigTOML(backend string, model string) string {
	provider := api.InferModelProvider(model)
	var b strings.Builder
	b.WriteString("# Court runtime defaults.\n")
	b.WriteString("# Prefer markdown files under roles/, juries/, presets/, and agents/ for larger prompts.\n")
	b.WriteString("# TOML definitions are supported for compact overrides and take precedence over markdown files.\n\n")
	b.WriteString("[defaults]\n")
	b.WriteString("backend = ")
	b.WriteString(quotedTOMLString(backend))
	b.WriteString("\n")
	b.WriteString("# Set to \"workspace\" or \"global\" to let clerk-backed runs select from a wider catalog.\n")
	b.WriteString("delegation_scope = \"preset\"\n")
	b.WriteString("model = ")
	b.WriteString(quotedTOMLString(model))
	b.WriteString("\n\n")
	b.WriteString("[defaults.backends.")
	b.WriteString(backend)
	b.WriteString("]\n")
	if provider != "" {
		b.WriteString("provider = ")
		b.WriteString(quotedTOMLString(provider))
		b.WriteString("\n")
	}
	b.WriteString("model = ")
	b.WriteString(quotedTOMLString(model))
	b.WriteString("\n")
	return b.String()
}

func writeConfigFile(targetPath string, content string, force bool, dryRun bool) (SetupFileAction, error) {
	exists, err := fileExists(targetPath)
	if err != nil {
		return "", err
	}
	data := []byte(content)
	if exists {
		return writeExistingFile(targetPath, data, force, dryRun)
	}
	if dryRun {
		return SetupFileWouldCreate, nil
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o750); err != nil {
		return "", wrapErr("create config target directory", err)
	}
	if err := os.WriteFile(targetPath, data, 0o600); err != nil {
		return "", wrapErr("write config file", err)
	}
	return SetupFileCreated, nil
}

func fileExists(path string) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		return true, nil
	} else if !os.IsNotExist(err) {
		return false, wrapErr("stat setup target", err)
	}
	return false, nil
}

func writeExistingFile(path string, data []byte, force bool, dryRun bool) (SetupFileAction, error) {
	if !force {
		if dryRun {
			return SetupFileWouldSkip, nil
		}
		return SetupFileSkipped, nil
	}
	if dryRun {
		return SetupFileWouldOverwrite, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return "", wrapErr("create setup target directory", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", wrapErr("overwrite setup target file", err)
	}
	return SetupFileOverwritten, nil
}

func quotedTOMLString(value string) string {
	return strconv.Quote(value)
}

func countSetupFile(result *SetupDefaultsResult, rel string) {
	switch {
	case strings.HasPrefix(rel, "presets/"):
		result.PresetCount++
	case strings.HasPrefix(rel, "juries/"):
		result.JuryCount++
	case strings.HasPrefix(rel, "roles/"):
		result.RoleCount++
	case strings.HasPrefix(rel, "agents/"):
		result.AgentCount++
	}
}
