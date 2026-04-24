// Package court provides Court runtime functionality.
package court

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	api "github.com/benjaminwestern/agentic-control/pkg/controlplane"
	"gopkg.in/yaml.v3"
)

const markdownExtension = ".md"

type markdownDoc struct {
	Frontmatter map[string]string
	Lists       map[string][]string
	Body        string
}

// Jury defines Court runtime data.
type Jury struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	JurorIDs    []string `json:"juror_ids"`
	ClerkID     string   `json:"clerk_id,omitempty"`
	JudgeID     string   `json:"judge_id,omitempty"`
}

// AgentConfig defines Court runtime data.
type AgentConfig struct {
	ID           string                          `json:"id"`
	Title        string                          `json:"title,omitempty"`
	Backend      string                          `json:"backend,omitempty"`
	Provider     string                          `json:"provider,omitempty"`
	Model        string                          `json:"model,omitempty"`
	ModelOptions RuntimeModelOptions             `json:"model_options,omitempty"`
	Backends     map[string]RuntimeBackendConfig `json:"backends,omitempty"`
}

// LoadPreset provides Court runtime functionality.
func LoadPreset(rootDir string, id string) (Preset, bool, error) {
	return LoadPresetFromRoots([]string{rootDir}, id)
}

// LoadPresetFromRoots provides Court runtime functionality.
func LoadPresetFromRoots(rootDirs []string, id string) (Preset, bool, error) {
	if id == "" {
		return Preset{}, false, nil
	}
	rootDirs = cleanPathList(rootDirs)
	if preset, ok, err := LoadConfigPresetFromRoots(rootDirs, id); err != nil {
		return Preset{}, false, err
	} else if ok {
		return preset, true, nil
	}
	path, ok := findMarkdownFile(rootDirs, []string{"presets"}, id)
	if !ok {
		return Preset{}, false, nil
	}
	doc, err := readMarkdownDoc(path)
	if err != nil {
		return Preset{}, false, err
	}

	presetID := firstNonEmpty(doc.Frontmatter["id"], doc.Frontmatter["name"], id)
	workflow := workflowFromRouting(doc.Frontmatter["workflow"], doc.Frontmatter["routing_mode"], doc.Frontmatter["routing"])
	jury, err := loadPresetJury(rootDirs, doc)
	if err != nil {
		return Preset{}, false, err
	}
	roleIDs := jury.JurorIDs
	roles := make([]Role, 0, len(roleIDs)+1)
	for _, roleID := range roleIDs {
		role, ok, err := LoadRoleFromRoots(rootDirs, roleID)
		if err != nil {
			return Preset{}, false, err
		}
		if !ok {
			return Preset{}, false, fmt.Errorf("preset %q references missing role %q", id, roleID)
		}
		roles = append(roles, role)
	}
	if judgeID := firstNonEmpty(
		doc.Frontmatter["final_judge"],
		doc.Frontmatter["finalJudge"],
		doc.Frontmatter["judge"],
		doc.Frontmatter["judge_juror"],
		doc.Frontmatter["inlineJudge"],
		jury.JudgeID,
	); judgeID != "" {
		role, ok, err := LoadRoleFromRoots(rootDirs, judgeID)
		if err != nil {
			return Preset{}, false, err
		}
		if !ok {
			return Preset{}, false, fmt.Errorf("preset %q references missing judge %q", id, judgeID)
		}
		role.Kind = RoleJudge
		roles = append(roles, role)
	}
	if clerkID := firstNonEmpty(doc.Frontmatter["clerk"], jury.ClerkID); clerkID != "" {
		role, ok, err := LoadRoleFromRoots(rootDirs, clerkID)
		if err != nil {
			return Preset{}, false, err
		}
		if !ok {
			return Preset{}, false, fmt.Errorf("preset %q references missing clerk %q", id, clerkID)
		}
		role.Kind = RoleClerk
		roles = append([]Role{role}, roles...)
	}

	return Preset{
		ID:          presetID,
		Title:       firstNonEmpty(doc.Frontmatter["title"], doc.Frontmatter["name"], presetID),
		Description: firstParagraph(doc.Body),
		Workflow:    workflow,
		Roles:       roles,
	}, true, nil
}

// ListMarkdownPresets provides Court runtime functionality.
func ListMarkdownPresets(rootDir string) ([]Preset, error) {
	return ListMarkdownPresetsFromRoots([]string{rootDir})
}

// ListMarkdownPresetsFromRoots provides Court runtime functionality.
func ListMarkdownPresetsFromRoots(rootDirs []string) ([]Preset, error) {
	rootDirs = cleanPathList(rootDirs)
	configPresets, err := ListConfigPresetsFromRoots(rootDirs)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	for _, preset := range configPresets {
		seen[preset.ID] = struct{}{}
	}
	ids := map[string]struct{}{}
	for _, rootDir := range rootDirs {
		entries, err := os.ReadDir(filepath.Join(rootDir, "presets"))
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
			id := strings.TrimSuffix(entry.Name(), markdownExtension)
			if _, ok := seen[id]; !ok {
				ids[id] = struct{}{}
			}
		}
	}
	var names []string
	for id := range ids {
		names = append(names, id)
	}
	sort.Strings(names)
	out := append([]Preset{}, configPresets...)
	for _, id := range names {
		preset, ok, err := LoadPresetFromRoots(rootDirs, id)
		if err != nil {
			return nil, wrapErr("read jury catalog directory", err)
		}
		if ok {
			out = append(out, preset)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// LoadJury provides Court runtime functionality.
func LoadJury(rootDir string, id string) (Jury, bool, error) {
	return LoadJuryFromRoots([]string{rootDir}, id)
}

// LoadJuryFromRoots provides Court runtime functionality.
func LoadJuryFromRoots(rootDirs []string, id string) (Jury, bool, error) {
	if id == "" {
		return Jury{}, false, nil
	}
	rootDirs = cleanPathList(rootDirs)
	if jury, ok, err := LoadConfigJuryFromRoots(rootDirs, id); err != nil {
		return Jury{}, false, err
	} else if ok {
		return jury, true, nil
	}
	path, ok := findMarkdownFile(rootDirs, []string{"juries"}, id)
	if !ok {
		return Jury{}, false, nil
	}
	doc, err := readMarkdownDoc(path)
	if err != nil {
		return Jury{}, false, err
	}
	jurorIDs := firstListOrCSV(doc, "jurors", "jury", "members", "roles")
	return Jury{
		ID:          firstNonEmpty(doc.Frontmatter["id"], doc.Frontmatter["name"], id),
		Title:       firstNonEmpty(doc.Frontmatter["title"], doc.Frontmatter["name"], id),
		Description: firstParagraph(doc.Body),
		JurorIDs:    jurorIDs,
		ClerkID:     doc.Frontmatter["clerk"],
		JudgeID:     firstNonEmpty(doc.Frontmatter["final_judge"], doc.Frontmatter["finalJudge"], doc.Frontmatter["judge"], doc.Frontmatter["judge_juror"], doc.Frontmatter["inlineJudge"]),
	}, true, nil
}

// ListJuriesFromRoots provides Court runtime functionality.
func ListJuriesFromRoots(rootDirs []string) ([]Jury, error) {
	rootDirs = cleanPathList(rootDirs)
	config, err := LoadCourtConfigFromRoots(rootDirs)
	if err != nil {
		return nil, err
	}
	ids := map[string]struct{}{}
	for id := range config.Juries {
		ids[id] = struct{}{}
	}
	for _, rootDir := range rootDirs {
		dir := filepath.Join(rootDir, "juries")
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, wrapErr("read jury catalog directory", err)
		}
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != markdownExtension {
				continue
			}
			ids[strings.TrimSuffix(entry.Name(), markdownExtension)] = struct{}{}
		}
	}
	names := sortedKeys(ids)
	out := make([]Jury, 0, len(names))
	for _, id := range names {
		jury, ok, err := LoadJuryFromRoots(rootDirs, id)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, jury)
		}
	}
	return out, nil
}

// LoadRole provides Court runtime functionality.
func LoadRole(rootDir string, id string) (Role, bool, error) {
	return LoadRoleFromRoots([]string{rootDir}, id)
}

// LoadRoleFromRoots provides Court runtime functionality.
func LoadRoleFromRoots(rootDirs []string, id string) (Role, bool, error) {
	rootDirs = cleanPathList(rootDirs)
	if role, ok, err := LoadConfigRoleFromRoots(rootDirs, id); err != nil {
		return Role{}, false, err
	} else if ok {
		return role, true, nil
	}
	path, ok := findMarkdownFile(rootDirs, []string{"roles", "jurors", "judges", "clerks"}, id)
	if !ok {
		return Role{}, false, nil
	}
	doc, err := readMarkdownDoc(path)
	if err != nil {
		return Role{}, false, err
	}
	dir := filepath.Base(filepath.Dir(path))
	kind := roleKindFromString(doc.Frontmatter["kind"], dir)
	agentID := doc.Frontmatter["agent"]
	var agentConfig AgentConfig
	if agentID != "" {
		if config, ok, err := LoadAgentConfigFromRoots(rootDirs, agentID); err != nil {
			return Role{}, false, err
		} else if ok {
			agentConfig = config
		}
	}
	roleBackends := backendConfigsFromFrontmatter(doc)
	backendConfigs := mergeBackendConfigs(agentConfig.Backends, roleBackends)
	defaultBackend := firstNonEmpty(doc.Frontmatter["backend"], agentConfig.Backend, singleBackendName(roleBackends), singleBackendName(agentConfig.Backends))
	roleBackend := backendConfigFor(roleBackends, defaultBackend)
	agentBackend := backendConfigFor(agentConfig.Backends, defaultBackend)
	model := firstNonEmpty(doc.Frontmatter["model"], roleBackend.Model, agentBackend.Model, agentConfig.Model)
	provider := firstNonEmpty(doc.Frontmatter["provider"], doc.Frontmatter["customProvider"], roleBackend.Provider, agentBackend.Provider, agentConfig.Provider, api.InferModelProvider(model))
	roleID := firstNonEmpty(doc.Frontmatter["id"], doc.Frontmatter["name"], id)
	return Role{
		ID:           roleID,
		Kind:         kind,
		Title:        firstNonEmpty(doc.Frontmatter["title"], roleID),
		Brief:        strings.TrimSpace(doc.Body),
		Backend:      defaultBackend,
		Provider:     provider,
		Model:        model,
		ModelOptions: api.MergeModelOptions(modelOptionsFromFrontmatter(doc.Frontmatter), roleBackend.ModelOptions, agentBackend.ModelOptions, agentConfig.ModelOptions),
		Agent:        agentID,
		Backends:     backendConfigs,
	}, true, nil
}

// ListRolesFromRoots provides Court runtime functionality.
func ListRolesFromRoots(rootDirs []string) ([]Role, error) {
	rootDirs = cleanPathList(rootDirs)
	config, err := LoadCourtConfigFromRoots(rootDirs)
	if err != nil {
		return nil, err
	}
	ids := map[string]struct{}{}
	for id := range config.Roles {
		ids[id] = struct{}{}
	}
	for _, rootDir := range rootDirs {
		for _, subdir := range []string{"roles", "jurors", "judges", "clerks"} {
			dir := filepath.Join(rootDir, subdir)
			entries, err := os.ReadDir(dir)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return nil, wrapErr("read role catalog directory", err)
			}
			for _, entry := range entries {
				if entry.IsDir() || filepath.Ext(entry.Name()) != markdownExtension {
					continue
				}
				ids[strings.TrimSuffix(entry.Name(), markdownExtension)] = struct{}{}
			}
		}
	}
	names := sortedKeys(ids)
	out := make([]Role, 0, len(names))
	for _, id := range names {
		role, ok, err := LoadRoleFromRoots(rootDirs, id)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, role)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind == out[j].Kind {
			return out[i].ID < out[j].ID
		}
		return out[i].Kind < out[j].Kind
	})
	return out, nil
}

// LoadAgentModel provides Court runtime functionality.
func LoadAgentModel(rootDir string, id string) (string, bool, error) {
	config, ok, err := LoadAgentConfig(rootDir, id)
	if err != nil || !ok {
		return "", ok, err
	}
	return config.Model, config.Model != "", nil
}

// LoadAgentConfig provides Court runtime functionality.
func LoadAgentConfig(rootDir string, id string) (AgentConfig, bool, error) {
	return LoadAgentConfigFromRoots([]string{rootDir}, id)
}

// LoadAgentConfigFromRoots provides Court runtime functionality.
func LoadAgentConfigFromRoots(rootDirs []string, id string) (AgentConfig, bool, error) {
	rootDirs = cleanPathList(rootDirs)
	if config, ok, err := LoadConfigAgentFromRoots(rootDirs, id); err != nil {
		return AgentConfig{}, false, err
	} else if ok {
		return config, true, nil
	}
	path, ok := findMarkdownFile(rootDirs, []string{"agents"}, id)
	if !ok {
		return AgentConfig{}, false, nil
	}
	doc, err := readMarkdownDoc(path)
	if err != nil {
		return AgentConfig{}, false, err
	}
	backends := backendConfigsFromFrontmatter(doc)
	defaultBackend := firstNonEmpty(doc.Frontmatter["backend"], singleBackendName(backends))
	backendConfig := backendConfigFor(backends, defaultBackend)
	model := firstNonEmpty(doc.Frontmatter["model"], backendConfig.Model)
	config := AgentConfig{
		ID:           id,
		Title:        firstNonEmpty(doc.Frontmatter["title"], id),
		Backend:      defaultBackend,
		Provider:     firstNonEmpty(doc.Frontmatter["provider"], doc.Frontmatter["customProvider"], backendConfig.Provider, api.InferModelProvider(model)),
		Model:        model,
		ModelOptions: api.MergeModelOptions(modelOptionsFromFrontmatter(doc.Frontmatter), backendConfig.ModelOptions),
		Backends:     backends,
	}
	return config, config.Model != "" || config.Backend != "" || config.Provider != "" || len(config.Backends) > 0, nil
}

func loadPresetJury(rootDirs []string, doc markdownDoc) (Jury, error) {
	roleIDs := firstList(doc.Lists, "jury", "jurors", "members", "roles")
	if len(roleIDs) == 0 {
		roleIDs = firstListOrCSV(doc, "jurors", "members", "roles")
	}
	if len(roleIDs) > 0 {
		return Jury{JurorIDs: roleIDs}, nil
	}
	juryID := firstNonEmpty(doc.Frontmatter["jury"], doc.Frontmatter["juries"])
	if juryID == "" {
		return Jury{}, nil
	}
	jury, ok, err := LoadJuryFromRoots(rootDirs, juryID)
	if err != nil {
		return Jury{}, err
	}
	if !ok {
		return Jury{}, fmt.Errorf("preset references missing jury %q", juryID)
	}
	return jury, nil
}

func findMarkdownFile(rootDirs []string, subdirs []string, id string) (string, bool) {
	for i := len(rootDirs) - 1; i >= 0; i-- {
		for _, subdir := range subdirs {
			path := filepath.Join(rootDirs[i], subdir, id+markdownExtension)
			if info, err := os.Stat(path); err == nil && !info.IsDir() {
				return path, true
			}
		}
	}
	for i := len(rootDirs) - 1; i >= 0; i-- {
		for _, subdir := range subdirs {
			dir := filepath.Join(rootDirs[i], subdir)
			entries, err := os.ReadDir(dir)
			if err != nil {
				continue
			}
			for _, entry := range entries {
				if entry.IsDir() || filepath.Ext(entry.Name()) != markdownExtension {
					continue
				}
				path := filepath.Join(dir, entry.Name())
				doc, err := readMarkdownDoc(path)
				if err != nil {
					continue
				}
				if catalogDocMatchesID(doc, id) {
					return path, true
				}
			}
		}
	}
	return "", false
}

func catalogDocMatchesID(doc markdownDoc, id string) bool {
	for _, candidate := range []string{doc.Frontmatter["id"], doc.Frontmatter["name"]} {
		if candidate == id {
			return true
		}
	}
	for _, alias := range splitAliases(doc.Frontmatter["aliases"]) {
		if alias == id {
			return true
		}
	}
	return false
}

func splitAliases(value string) []string {
	var out []string
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func sortedKeys(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for key := range values {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func readMarkdownDoc(path string) (markdownDoc, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return markdownDoc{}, wrapErr("read markdown document", err)
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	doc := markdownDoc{
		Frontmatter: map[string]string{},
		Lists:       map[string][]string{},
		Body:        strings.TrimSpace(text),
	}
	if !strings.HasPrefix(text, "---\n") {
		return doc, nil
	}
	rest := text[len("---\n"):]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return doc, nil
	}
	rawFrontmatter := rest[:end]
	doc.Body = strings.TrimSpace(rest[end+len("\n---"):])
	if err := parseFrontmatter(rawFrontmatter, &doc); err != nil {
		return markdownDoc{}, fmt.Errorf("%s: %w", path, err)
	}
	return doc, nil
}

func firstList(values map[string][]string, keys ...string) []string {
	for _, key := range keys {
		if items := values[key]; len(items) > 0 {
			return items
		}
	}
	return nil
}

func firstListOrCSV(doc markdownDoc, keys ...string) []string {
	if values := firstList(doc.Lists, keys...); len(values) > 0 {
		return values
	}
	for _, key := range keys {
		if values := splitListValue(doc.Frontmatter[key]); len(values) > 0 {
			return values
		}
	}
	return nil
}

func splitListValue(value string) []string {
	var out []string
	for _, item := range strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == '\n'
	}) {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func parseFrontmatter(raw string, doc *markdownDoc) error {
	values := map[string]any{}
	if err := yaml.Unmarshal([]byte(raw), &values); err != nil {
		return wrapErr("parse markdown frontmatter", err)
	}
	for key, value := range values {
		switch typed := value.(type) {
		case nil:
			continue
		case []any:
			items := make([]string, 0, len(typed))
			for _, item := range typed {
				text, ok := scalarFrontmatterValue(item)
				if !ok {
					continue
				}
				if text != "" {
					items = append(items, text)
				}
			}
			if len(items) > 0 {
				doc.Lists[key] = items
			}
		default:
			text, ok := scalarFrontmatterValue(typed)
			if !ok {
				encoded, err := json.Marshal(typed)
				if err != nil {
					continue
				}
				text = string(encoded)
			}
			doc.Frontmatter[key] = text
		}
	}
	return nil
}

func backendConfigsFromFrontmatter(doc markdownDoc) map[string]RuntimeBackendConfig {
	raw := firstNonEmpty(doc.Frontmatter["backends"], doc.Frontmatter["backendConfigs"], doc.Frontmatter["backend_configs"])
	if raw == "" {
		return nil
	}
	var decoded map[string]map[string]any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return nil
	}
	out := map[string]RuntimeBackendConfig{}
	for backend, values := range decoded {
		backend = strings.TrimSpace(backend)
		if backend == "" {
			continue
		}
		config := RuntimeBackendConfig{}
		if text, ok := scalarFrontmatterValue(values["provider"]); ok {
			config.Provider = text
		}
		if config.Provider == "" {
			if text, ok := scalarFrontmatterValue(values["customProvider"]); ok {
				config.Provider = text
			}
		}
		if config.Provider == "" {
			if text, ok := scalarFrontmatterValue(values["custom_provider"]); ok {
				config.Provider = text
			}
		}
		if text, ok := scalarFrontmatterValue(values["model"]); ok {
			config.Model = text
		}
		config.ModelOptions = runtimeModelOptionsFromValueMap(values)
		config.Provider = firstNonEmpty(config.Provider)
		config.Model = firstNonEmpty(config.Model)
		if config.Provider == "" && config.Model == "" && !api.HasModelOptions(config.ModelOptions) {
			continue
		}
		out[backend] = config
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func modelOptionsFromFrontmatter(values map[string]string) RuntimeModelOptions {
	options := RuntimeModelOptions{
		ReasoningEffort: firstNonEmpty(values["reasoning_effort"], values["reasoningEffort"]),
		ThinkingLevel:   firstNonEmpty(values["thinking_level"], values["thinkingLevel"]),
	}
	if text := firstNonEmpty(values["thinking_budget"], values["thinkingBudget"]); text != "" {
		if parsed, err := strconv.Atoi(text); err == nil {
			options.ThinkingBudget = &parsed
		}
	}
	return options
}

func mergeBackendConfigs(base map[string]RuntimeBackendConfig, override map[string]RuntimeBackendConfig) map[string]RuntimeBackendConfig {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}
	out := map[string]RuntimeBackendConfig{}
	for backend, config := range base {
		out[backend] = config
	}
	for backend, config := range override {
		current := out[backend]
		out[backend] = RuntimeBackendConfig{
			Provider:     firstNonEmpty(config.Provider, current.Provider),
			Model:        firstNonEmpty(config.Model, current.Model),
			ModelOptions: api.MergeModelOptions(config.ModelOptions, current.ModelOptions),
		}
	}
	return out
}

func backendConfigFor(configs map[string]RuntimeBackendConfig, backend string) RuntimeBackendConfig {
	backend = strings.TrimSpace(backend)
	if backend == "" || len(configs) == 0 {
		return RuntimeBackendConfig{}
	}
	if config, ok := configs[backend]; ok {
		return config
	}
	for candidate, config := range configs {
		if strings.EqualFold(candidate, backend) {
			return config
		}
	}
	return RuntimeBackendConfig{}
}

func singleBackendName(configs map[string]RuntimeBackendConfig) string {
	if len(configs) != 1 {
		return ""
	}
	for backend := range configs {
		return backend
	}
	return ""
}

func scalarFrontmatterValue(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed), true
	case bool:
		return strconv.FormatBool(typed), true
	case int:
		return strconv.Itoa(typed), true
	case int64:
		return strconv.FormatInt(typed, 10), true
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64), true
	default:
		return "", false
	}
}

func roleKindFromString(value string, dir string) RoleKind {
	switch strings.ToLower(value) {
	case "clerk":
		return RoleClerk
	case "judge", "final_judge", "inline_judge", "finaljudge", "inlinejudge":
		return RoleJudge
	case "juror":
		return RoleJuror
	}
	switch dir {
	case "judges":
		return RoleJudge
	case "clerks":
		return RoleClerk
	default:
		return RoleJuror
	}
}

func workflowFromRouting(values ...string) WorkflowMode {
	for _, value := range values {
		switch strings.ToLower(value) {
		case "broadcast", "parallel", "parallel_consensus":
			return WorkflowParallelConsensus
		case "routed", "clerk", "clerk_routed":
			return WorkflowRouted
		case "role_scoped":
			return WorkflowRoleScoped
		case "bounded", "bounded_correction":
			return WorkflowBoundedCorrection
		case "review_only":
			return WorkflowReviewOnly
		}
	}
	return WorkflowParallelConsensus
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstParagraph(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	for _, block := range strings.Split(trimmed, "\n\n") {
		block = strings.TrimSpace(block)
		if block != "" && !strings.HasPrefix(block, "#") {
			return collapseWhitespace(block)
		}
	}
	return collapseWhitespace(trimmed)
}
