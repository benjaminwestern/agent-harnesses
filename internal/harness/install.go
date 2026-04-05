package harness

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type InstallOptions struct {
	Runtime    Runtime
	Scope      string
	Defaulted  bool
	HelperArgs []string
}

type runtimeInstallPaths struct {
	WorkspaceRoot  string
	RuntimeRoot    string
	InstallRoot    string
	ManifestPath   string
	HelperBinary   string
	ConfigPath     string
	PluginPath     string
	PluginConfig   string
	PluginSource   string
	SettingsPath   string
	ClaudeSettings string
}

type installManifest struct {
	Runtime          string   `json:"runtime"`
	Scope            string   `json:"scope"`
	InstallRoot      string   `json:"install_root"`
	HelperBinary     string   `json:"helper_binary"`
	HelperCommand    string   `json:"helper_command"`
	HelperArgs       []string `json:"helper_args,omitempty"`
	ConfigPath       string   `json:"config_path,omitempty"`
	PluginPath       string   `json:"plugin_path,omitempty"`
	PluginConfigPath string   `json:"plugin_config_path,omitempty"`
	SettingsPath     string   `json:"settings_path,omitempty"`
	HookNames        []string `json:"hook_names,omitempty"`
	Events           []string `json:"events,omitempty"`
	ScopeDefaulted   bool     `json:"scope_defaulted,omitempty"`
}

func runInstall(args []string, stdout, stderr io.Writer) error {
	options, err := parseInstallArgs(args)
	if err != nil {
		if errors.Is(err, errHelpRequested) {
			printUsage(stdout)
			return nil
		}
		printUsage(stderr)
		return err
	}

	workspaceRoot, err := findWorkspaceRoot()
	if err != nil {
		return err
	}
	paths, err := resolveInstallPaths(options.Runtime, options.Scope, workspaceRoot)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(paths.InstallRoot, 0o755); err != nil {
		return err
	}
	if err := installHelperBinary(paths.HelperBinary); err != nil {
		return err
	}

	manifest := installManifest{
		Runtime:        string(options.Runtime),
		Scope:          options.Scope,
		InstallRoot:    paths.InstallRoot,
		HelperBinary:   paths.HelperBinary,
		ScopeDefaulted: options.Defaulted,
	}

	switch options.Runtime {
	case RuntimeCodex:
		manifest.HelperArgs = append([]string(nil), codexHelperArgs(options.HelperArgs)...)
		manifest.HelperCommand = shellCommand(paths.HelperBinary, manifest.HelperArgs)
		manifest.ConfigPath = paths.ConfigPath
		manifest.Events = []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse", "Stop"}
		if err := installCodexBundle(paths, manifest.HelperCommand); err != nil {
			return err
		}
	case RuntimeGemini:
		manifest.HelperArgs = append([]string(nil), geminiHelperArgs(options.HelperArgs)...)
		manifest.HelperCommand = shellCommand(paths.HelperBinary, manifest.HelperArgs)
		manifest.SettingsPath = paths.SettingsPath
		manifest.HookNames = geminiHookNames()
		if err := installGeminiBundle(paths, manifest.HelperCommand); err != nil {
			return err
		}
	case RuntimeClaude:
		manifest.HelperArgs = append([]string(nil), claudeHelperArgs(options.HelperArgs)...)
		manifest.HelperCommand = shellCommand(paths.HelperBinary, manifest.HelperArgs)
		manifest.SettingsPath = paths.ClaudeSettings
		if err := installClaudeBundle(paths, manifest.HelperCommand); err != nil {
			return err
		}
	case RuntimeOpenCode:
		manifest.HelperArgs = append([]string(nil), openCodeHelperArgs(options.HelperArgs)...)
		manifest.PluginPath = paths.PluginPath
		manifest.PluginConfigPath = paths.PluginConfig
		manifest.HelperCommand = shellCommand(paths.HelperBinary, manifest.HelperArgs)
		if err := installOpenCodeBundle(paths, manifest.HelperArgs); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported runtime: %s", options.Runtime)
	}

	if err := writeJSONFile(paths.ManifestPath, manifest); err != nil {
		return err
	}

	if _, err := fmt.Fprintf(stdout, "Installed %s bundle (%s)\n", options.Runtime, options.Scope); err != nil {
		return err
	}
	if options.Defaulted {
		if _, err := fmt.Fprintf(stdout, "Scope defaulted to %s for %s\n", options.Scope, options.Runtime); err != nil {
			return err
		}
	}
	if len(options.HelperArgs) > 0 {
		if _, err := fmt.Fprintf(stdout, "Embedded helper flags: %s\n", strings.Join(options.HelperArgs, " ")); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(stdout, "Managed paths:"); err != nil {
		return err
	}
	for _, path := range managedPaths(paths, options.Runtime) {
		if _, err := fmt.Fprintf(stdout, "- %s\n", path); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(stdout, "To remove this bundle later, run: ./.artifacts/bin/agent_harness uninstall --runtime %s --scope %s\n", options.Runtime, options.Scope); err != nil {
		return err
	}
	return nil
}

func runUninstall(args []string, stdout, stderr io.Writer) error {
	options, err := parseInstallArgs(args)
	if err != nil {
		if errors.Is(err, errHelpRequested) {
			printUsage(stdout)
			return nil
		}
		printUsage(stderr)
		return err
	}

	workspaceRoot, err := findWorkspaceRoot()
	if err != nil {
		return err
	}
	paths, err := resolveInstallPaths(options.Runtime, options.Scope, workspaceRoot)
	if err != nil {
		return err
	}
	if !bundleLikelyExists(paths, options.Runtime) {
		_, err := fmt.Fprintf(stdout, "No %s bundle found (%s)\n", options.Runtime, options.Scope)
		return err
	}

	manifest, err := readManifest(paths.ManifestPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	switch options.Runtime {
	case RuntimeCodex:
		if err := uninstallCodexBundle(paths, manifest); err != nil {
			return err
		}
	case RuntimeGemini:
		if err := uninstallGeminiBundle(paths, manifest); err != nil {
			return err
		}
	case RuntimeClaude:
		if err := uninstallClaudeBundle(paths); err != nil {
			return err
		}
	case RuntimeOpenCode:
		if err := uninstallOpenCodeBundle(paths); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported runtime: %s", options.Runtime)
	}

	_ = os.Remove(paths.ManifestPath)
	if isEmptyDir(paths.InstallRoot) {
		_ = os.RemoveAll(paths.InstallRoot)
	}
	if _, err := fmt.Fprintf(stdout, "Removed %s bundle (%s)\n", options.Runtime, options.Scope); err != nil {
		return err
	}
	return nil
}

func parseInstallArgs(args []string) (InstallOptions, error) {
	options := InstallOptions{}
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--runtime":
			index++
			if index >= len(args) {
				return InstallOptions{}, errors.New("missing runtime")
			}
			runtime, err := parseRuntime(args[index])
			if err != nil {
				return InstallOptions{}, err
			}
			options.Runtime = runtime
		case "--scope":
			index++
			if index >= len(args) {
				return InstallOptions{}, errors.New("missing scope")
			}
			options.Scope = args[index]
		case "--help", "-h":
			return InstallOptions{}, errHelpRequested
		case "--socket-path", "--socket-env", "--provenance", "--success-json", "--bind-env", "--bind-value":
			options.HelperArgs = append(options.HelperArgs, args[index])
			index++
			if index >= len(args) {
				return InstallOptions{}, fmt.Errorf("missing value for %s", args[index-1])
			}
			options.HelperArgs = append(options.HelperArgs, args[index])
		case "--stdout":
			options.HelperArgs = append(options.HelperArgs, args[index])
		case "--":
			options.HelperArgs = append(options.HelperArgs, args[index+1:]...)
			index = len(args)
		default:
			return InstallOptions{}, fmt.Errorf("unsupported install argument: %s", args[index])
		}
	}
	if options.Runtime == "" {
		return InstallOptions{}, errors.New("missing runtime")
	}
	if options.Scope == "" {
		options.Scope = defaultScopeForRuntime(options.Runtime)
		options.Defaulted = true
	}
	if options.Scope != "repo" && options.Scope != "global" {
		return InstallOptions{}, fmt.Errorf("unsupported scope: %s", options.Scope)
	}
	return options, nil
}

func defaultScopeForRuntime(runtime Runtime) string {
	if runtime == RuntimeOpenCode {
		return "global"
	}
	return "repo"
}

func resolveInstallPaths(runtime Runtime, scope, workspaceRoot string) (runtimeInstallPaths, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return runtimeInstallPaths{}, err
	}
	paths := runtimeInstallPaths{WorkspaceRoot: workspaceRoot}
	switch runtime {
	case RuntimeCodex:
		if scope == "global" {
			paths.RuntimeRoot = firstNonEmptyEnv("CODEX_HOME", filepath.Join(homeDir, ".codex"))
		} else {
			paths.RuntimeRoot = filepath.Join(workspaceRoot, ".codex")
		}
		paths.InstallRoot = filepath.Join(paths.RuntimeRoot, "agentic-control")
		paths.HelperBinary = filepath.Join(paths.InstallRoot, "bin", "agent_harness")
		paths.ManifestPath = filepath.Join(paths.InstallRoot, "install-manifest.json")
		paths.ConfigPath = filepath.Join(paths.RuntimeRoot, "hooks.json")
	case RuntimeGemini:
		if scope == "global" {
			paths.RuntimeRoot = firstNonEmptyEnv("GEMINI_HOME", filepath.Join(homeDir, ".gemini"))
		} else {
			paths.RuntimeRoot = filepath.Join(workspaceRoot, ".gemini")
		}
		paths.InstallRoot = filepath.Join(paths.RuntimeRoot, "agentic-control")
		paths.HelperBinary = filepath.Join(paths.InstallRoot, "bin", "agent_harness")
		paths.ManifestPath = filepath.Join(paths.InstallRoot, "install-manifest.json")
		paths.SettingsPath = filepath.Join(paths.RuntimeRoot, "settings.json")
	case RuntimeClaude:
		if scope == "global" {
			claudeRoot := firstNonEmptyEnv("CLAUDE_HOME", filepath.Join(homeDir, ".claude"))
			paths.InstallRoot = filepath.Join(claudeRoot, "agentic-control")
		} else {
			paths.InstallRoot = filepath.Join(workspaceRoot, ".agentic-control", "claude")
		}
		paths.HelperBinary = filepath.Join(paths.InstallRoot, "bin", "agent_harness")
		paths.ManifestPath = filepath.Join(paths.InstallRoot, "install-manifest.json")
		paths.ClaudeSettings = filepath.Join(paths.InstallRoot, "settings.json")
	case RuntimeOpenCode:
		if scope == "global" {
			paths.RuntimeRoot = firstNonEmptyEnv("OPENCODE_CONFIG_DIR", filepath.Join(homeDir, ".config", "opencode"))
		} else {
			paths.RuntimeRoot = filepath.Join(workspaceRoot, ".opencode")
		}
		paths.InstallRoot = filepath.Join(paths.RuntimeRoot, "agentic-control")
		paths.HelperBinary = filepath.Join(paths.InstallRoot, "bin", "agent_harness")
		paths.ManifestPath = filepath.Join(paths.InstallRoot, "install-manifest.json")
		paths.PluginPath = filepath.Join(paths.RuntimeRoot, "plugins", "agentic-control.js")
		paths.PluginConfig = filepath.Join(paths.InstallRoot, "plugin-config.json")
		paths.PluginSource = filepath.Join(workspaceRoot, "runtimes", "opencode", "plugin.js")
	default:
		return runtimeInstallPaths{}, fmt.Errorf("unsupported runtime: %s", runtime)
	}
	return paths, nil
}

func installHelperBinary(target string) error {
	source, err := os.Executable()
	if err != nil {
		return err
	}
	source, err = filepath.EvalSymlinks(source)
	if err != nil {
		source = source
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	if samePath(source, target) {
		return nil
	}
	return copyFile(source, target, 0o755)
}

func managedPaths(paths runtimeInstallPaths, runtime Runtime) []string {
	result := []string{paths.HelperBinary, paths.ManifestPath}
	switch runtime {
	case RuntimeCodex:
		result = append(result, paths.ConfigPath)
	case RuntimeGemini:
		result = append(result, paths.SettingsPath)
	case RuntimeClaude:
		result = append(result, paths.ClaudeSettings)
	case RuntimeOpenCode:
		result = append(result, paths.PluginPath, paths.PluginConfig)
	}
	return result
}

func bundleLikelyExists(paths runtimeInstallPaths, runtime Runtime) bool {
	for _, path := range managedPaths(paths, runtime) {
		if fileExists(path) || dirExists(path) {
			return true
		}
	}
	return false
}

func installCodexBundle(paths runtimeInstallPaths, helperCommand string) error {
	root, err := loadOrCreateJSON(paths.ConfigPath)
	if err != nil {
		return err
	}
	hooks := ensureMap(root, "hooks")
	entries := map[string]map[string]any{
		"SessionStart": {
			"matcher": "startup|resume",
			"hooks":   []any{map[string]any{"type": "command", "command": helperCommand, "statusMessage": "Agentic Control: listening for Codex hook events"}},
		},
		"UserPromptSubmit": {
			"matcher": ".*",
			"hooks":   []any{map[string]any{"type": "command", "command": helperCommand}},
		},
		"PreToolUse": {
			"matcher": "Bash",
			"hooks":   []any{map[string]any{"type": "command", "command": helperCommand}},
		},
		"PostToolUse": {
			"matcher": "Bash",
			"hooks":   []any{map[string]any{"type": "command", "command": helperCommand}},
		},
		"Stop": {
			"matcher": ".*",
			"hooks":   []any{map[string]any{"type": "command", "command": helperCommand}},
		},
	}
	for event, entry := range entries {
		items := removeManagedCodexEntries(arrayField(hooks, event), RuntimeCodex)
		hooks[event] = append(items, entry)
	}
	return writeJSONFile(paths.ConfigPath, root)
}

func uninstallCodexBundle(paths runtimeInstallPaths, manifest installManifest) error {
	root, err := loadOrCreateJSON(paths.ConfigPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	hooks := ensureMap(root, "hooks")
	for _, event := range []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse", "Stop"} {
		items := removeManagedCodexEntries(arrayField(hooks, event), RuntimeCodex)
		if len(items) == 0 {
			delete(hooks, event)
			continue
		}
		hooks[event] = items
	}
	cleanupEmptyMap(root, "hooks")
	return writeOrRemoveJSON(paths.ConfigPath, root)
}

func installGeminiBundle(paths runtimeInstallPaths, helperCommand string) error {
	root, err := loadOrCreateJSON(paths.SettingsPath)
	if err != nil {
		return err
	}
	hooks := ensureMap(root, "hooks")
	entries := map[string]map[string]any{
		"SessionStart": {"matcher": "*", "hooks": []any{map[string]any{"name": "agentic-control-session-start", "type": "command", "command": helperCommand}}},
		"SessionEnd":   {"matcher": "*", "hooks": []any{map[string]any{"name": "agentic-control-session-end", "type": "command", "command": helperCommand}}},
		"BeforeAgent":  {"matcher": "*", "hooks": []any{map[string]any{"name": "agentic-control-before-agent", "type": "command", "command": helperCommand}}},
		"AfterAgent":   {"matcher": "*", "hooks": []any{map[string]any{"name": "agentic-control-after-agent", "type": "command", "command": helperCommand}}},
		"BeforeTool":   {"matcher": "run_shell_command|write_file|replace", "hooks": []any{map[string]any{"name": "agentic-control-before-tool", "type": "command", "command": helperCommand}}},
		"AfterTool":    {"matcher": "run_shell_command|write_file|replace", "hooks": []any{map[string]any{"name": "agentic-control-after-tool", "type": "command", "command": helperCommand}}},
		"Notification": {"matcher": "*", "hooks": []any{map[string]any{"name": "agentic-control-notification", "type": "command", "command": helperCommand}}},
	}
	for event, entry := range entries {
		items := removeManagedGeminiEntries(arrayField(hooks, event))
		hooks[event] = append(items, entry)
	}
	return writeJSONFile(paths.SettingsPath, root)
}

func uninstallGeminiBundle(paths runtimeInstallPaths, manifest installManifest) error {
	root, err := loadOrCreateJSON(paths.SettingsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	hooks := ensureMap(root, "hooks")
	for _, event := range []string{"SessionStart", "SessionEnd", "BeforeAgent", "AfterAgent", "BeforeTool", "AfterTool", "Notification"} {
		items := removeManagedGeminiEntries(arrayField(hooks, event))
		if len(items) == 0 {
			delete(hooks, event)
			continue
		}
		hooks[event] = items
	}
	cleanupEmptyMap(root, "hooks")
	return writeOrRemoveJSON(paths.SettingsPath, root)
}

func installClaudeBundle(paths runtimeInstallPaths, helperCommand string) error {
	root := map[string]any{
		"hooks": map[string]any{
			"SessionStart":       []any{map[string]any{"hooks": []any{map[string]any{"type": "command", "command": helperCommand, "statusMessage": "Agentic Control: listening for Claude hook events"}}}},
			"UserPromptSubmit":   []any{map[string]any{"hooks": []any{map[string]any{"type": "command", "command": helperCommand}}}},
			"PreToolUse":         []any{map[string]any{"hooks": []any{map[string]any{"type": "command", "command": helperCommand}}}},
			"PermissionRequest":  []any{map[string]any{"hooks": []any{map[string]any{"type": "command", "command": helperCommand}}}},
			"PostToolUse":        []any{map[string]any{"hooks": []any{map[string]any{"type": "command", "command": helperCommand}}}},
			"PostToolUseFailure": []any{map[string]any{"hooks": []any{map[string]any{"type": "command", "command": helperCommand}}}},
			"Notification":       []any{map[string]any{"hooks": []any{map[string]any{"type": "command", "command": helperCommand}}}},
			"Stop":               []any{map[string]any{"hooks": []any{map[string]any{"type": "command", "command": helperCommand}}}},
			"StopFailure":        []any{map[string]any{"hooks": []any{map[string]any{"type": "command", "command": helperCommand}}}},
			"SessionEnd":         []any{map[string]any{"hooks": []any{map[string]any{"type": "command", "command": helperCommand}}}},
		},
	}
	return writeJSONFile(paths.ClaudeSettings, root)
}

func uninstallClaudeBundle(paths runtimeInstallPaths) error {
	if err := os.RemoveAll(paths.InstallRoot); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func installOpenCodeBundle(paths runtimeInstallPaths, helperArgs []string) error {
	if err := os.MkdirAll(filepath.Dir(paths.PluginPath), 0o755); err != nil {
		return err
	}
	legacyPlugin := filepath.Join(filepath.Dir(paths.PluginPath), "agent-harness.js")
	_ = os.Remove(legacyPlugin)
	if err := copyFile(paths.PluginSource, paths.PluginPath, 0o644); err != nil {
		return err
	}
	config := map[string]any{"helperArgs": helperArgs}
	return writeJSONFile(paths.PluginConfig, config)
}

func uninstallOpenCodeBundle(paths runtimeInstallPaths) error {
	_ = os.Remove(paths.PluginPath)
	_ = os.Remove(filepath.Join(filepath.Dir(paths.PluginPath), "agent-harness.js"))
	_ = os.Remove(paths.PluginConfig)
	if isEmptyDir(filepath.Dir(paths.PluginPath)) {
		_ = os.Remove(filepath.Dir(paths.PluginPath))
	}
	if err := os.RemoveAll(paths.InstallRoot); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func readManifest(path string) (installManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return installManifest{}, err
	}
	var manifest installManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return installManifest{}, err
	}
	return manifest, nil
}

func loadOrCreateJSON(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]any{}, nil
		}
		return nil, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return map[string]any{}, nil
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	if root == nil {
		return map[string]any{}, nil
	}
	return root, nil
}

func writeJSONFile(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func writeOrRemoveJSON(path string, root map[string]any) error {
	if len(root) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	return writeJSONFile(path, root)
}

func ensureMap(root map[string]any, key string) map[string]any {
	if existing, ok := root[key].(map[string]any); ok {
		return existing
	}
	created := map[string]any{}
	root[key] = created
	return created
}

func cleanupEmptyMap(root map[string]any, key string) {
	if existing, ok := root[key].(map[string]any); ok && len(existing) == 0 {
		delete(root, key)
	}
}

func arrayField(object map[string]any, key string) []any {
	values, ok := object[key].([]any)
	if !ok {
		return nil
	}
	return values
}

func removeManagedCodexEntries(entries []any, runtime Runtime) []any {
	result := make([]any, 0, len(entries))
	for _, entry := range entries {
		entryMap, ok := entry.(map[string]any)
		if !ok {
			result = append(result, entry)
			continue
		}
		if !entryContainsManagedCommand(entryMap, runtime) {
			result = append(result, entry)
		}
	}
	return result
}

func removeManagedGeminiEntries(entries []any) []any {
	result := make([]any, 0, len(entries))
	for _, entry := range entries {
		entryMap, ok := entry.(map[string]any)
		if !ok {
			result = append(result, entry)
			continue
		}
		if !entryContainsManagedGeminiHook(entryMap) {
			result = append(result, entry)
		}
	}
	return result
}

func entryContainsManagedCommand(entry map[string]any, runtime Runtime) bool {
	hooks, ok := entry["hooks"].([]any)
	if !ok {
		return false
	}
	for _, hook := range hooks {
		hookMap, ok := hook.(map[string]any)
		if !ok {
			continue
		}
		command, _ := hookMap["command"].(string)
		if isManagedHelperCommand(command, runtime) {
			return true
		}
	}
	return false
}

func entryContainsManagedGeminiHook(entry map[string]any) bool {
	hooks, ok := entry["hooks"].([]any)
	if !ok {
		return false
	}
	for _, hook := range hooks {
		hookMap, ok := hook.(map[string]any)
		if !ok {
			continue
		}
		name, _ := hookMap["name"].(string)
		if strings.HasPrefix(name, "agent-harness-") || strings.HasPrefix(name, "agentic-control-") {
			return true
		}
	}
	return false
}

func isManagedHelperCommand(command string, runtime Runtime) bool {
	command = strings.TrimSpace(command)
	if command == "" || !strings.Contains(command, "agent_harness") {
		return false
	}
	return strings.Contains(command, string(runtime))
}

func codexHelperArgs(extra []string) []string {
	args := []string{"--runtime", "codex"}
	return append(args, extra...)
}

func geminiHelperArgs(extra []string) []string {
	args := []string{"--runtime", "gemini", "--success-json", "{}"}
	return append(args, extra...)
}

func claudeHelperArgs(extra []string) []string {
	args := []string{"--runtime", "claude"}
	return append(args, extra...)
}

func openCodeHelperArgs(extra []string) []string {
	args := []string{"--runtime", "opencode", "--provenance", "native_plugin"}
	return append(args, extra...)
}

func geminiHookNames() []string {
	return []string{
		"agentic-control-session-start",
		"agentic-control-session-end",
		"agentic-control-before-agent",
		"agentic-control-after-agent",
		"agentic-control-before-tool",
		"agentic-control-after-tool",
		"agentic-control-notification",
	}
}

func shellCommand(binary string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, shellQuote(binary))
	for _, arg := range args {
		parts = append(parts, shellQuote(arg))
	}
	return strings.Join(parts, " ")
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	if strings.IndexFunc(value, func(r rune) bool {
		return !(r == '-' || r == '_' || r == '/' || r == '.' || r == ':' || r == '=' ||
			(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'))
	}) == -1 {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func copyFile(source, target string, mode os.FileMode) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, data, mode)
}

func samePath(left, right string) bool {
	leftEval, leftErr := filepath.EvalSymlinks(left)
	if leftErr == nil {
		left = leftEval
	}
	rightEval, rightErr := filepath.EvalSymlinks(right)
	if rightErr == nil {
		right = rightEval
	}
	return left == right
}

func isEmptyDir(path string) bool {
	entries, err := os.ReadDir(path)
	if err != nil {
		return false
	}
	return len(entries) == 0
}

func findWorkspaceRoot() (string, error) {
	current, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if fileExists(filepath.Join(current, "go.mod")) && dirExists(filepath.Join(current, "runtimes")) {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", errors.New("could not locate repository root from current directory")
		}
		current = parent
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func firstNonEmptyEnv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func sortedKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
