// Package court provides Court runtime functionality.
package court

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	api "github.com/benjaminwestern/agentic-control/pkg/controlplane"
)

const configFileName = "config.toml"

// Config defines Court runtime data.
type Config struct {
	Features map[string]bool
	Defaults ConfigDefaults
	Agents   map[string]ConfigAgent
	Roles    map[string]ConfigRole
	Juries   map[string]ConfigJury
	Presets  map[string]ConfigPreset
}

// ConfigDefaults defines Court runtime data.
type ConfigDefaults struct {
	Preset          string
	Workflow        string
	DelegationScope string
	Backend         string
	Provider        string
	Model           string
	ModelOptions    RuntimeModelOptions
	Backends        map[string]RuntimeBackendConfig
}

// ConfigAgent defines Court runtime data.
type ConfigAgent struct {
	ID           string
	Title        string
	Backend      string
	Provider     string
	Model        string
	ModelOptions RuntimeModelOptions
	Backends     map[string]RuntimeBackendConfig
}

// ConfigRole defines Court runtime data.
type ConfigRole struct {
	ID           string
	Kind         RoleKind
	Title        string
	Prompt       string
	Agent        string
	Backend      string
	Provider     string
	Model        string
	ModelOptions RuntimeModelOptions
	Backends     map[string]RuntimeBackendConfig
}

// ConfigJury defines Court runtime data.
type ConfigJury struct {
	ID          string
	Title       string
	Description string
	JurorIDs    []string
	ClerkID     string
	JudgeID     string
}

// ConfigPreset defines Court runtime data.
type ConfigPreset struct {
	ID          string
	Title       string
	Description string
	Workflow    string
	JuryID      string
	JurorIDs    []string
	ClerkID     string
	JudgeID     string
}

// LoadCourtConfig provides Court runtime functionality.
func LoadCourtConfig(rootDir string) (Config, error) {
	return LoadCourtConfigFromRoots([]string{rootDir})
}

// LoadCourtConfigFromRoots provides Court runtime functionality.
func LoadCourtConfigFromRoots(rootDirs []string) (Config, error) {
	config := emptyCourtConfig()
	for _, rootDir := range cleanPathList(rootDirs) {
		path := filepath.Join(rootDir, configFileName)
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return Config{}, wrapErr("read court config", err)
		}
		parsed, err := parseCourtConfig(data)
		if err != nil {
			return Config{}, fmt.Errorf("%s: %w", path, err)
		}
		config = mergeCourtConfig(config, parsed)
	}
	return config, nil
}

func emptyCourtConfig() Config {
	return Config{
		Features: map[string]bool{},
		Agents:   map[string]ConfigAgent{},
		Roles:    map[string]ConfigRole{},
		Juries:   map[string]ConfigJury{},
		Presets:  map[string]ConfigPreset{},
	}
}

func parseCourtConfig(data []byte) (Config, error) {
	raw := map[string]any{}
	if _, err := toml.Decode(string(data), &raw); err != nil {
		return Config{}, wrapErr("decode court config", err)
	}
	config := emptyCourtConfig()
	if values := mapValue(raw["features"]); values != nil {
		for k, v := range values {
			if b, ok := v.(bool); ok {
				config.Features[k] = b
			}
		}
	}
	if values := mapValue(raw["defaults"]); values != nil {
		config.Defaults = parseConfigDefaults(values)
	}
	config.Agents = parseConfigAgents(raw)
	config.Roles = parseConfigRoles(raw)
	config.Juries = parseConfigJuries(raw)
	config.Presets = parseConfigPresets(raw)
	return config, nil
}

func mergeCourtConfig(base Config, override Config) Config {
	out := emptyCourtConfig()
	out.Defaults = mergeConfigDefaults(base.Defaults, override.Defaults)
	for id, value := range base.Features {
		out.Features[id] = value
	}
	for id, value := range override.Features {
		out.Features[id] = value
	}
	for id, value := range base.Agents {
		out.Agents[id] = value
	}
	for id, value := range override.Agents {
		out.Agents[id] = value
	}
	for id, value := range base.Roles {
		out.Roles[id] = value
	}
	for id, value := range override.Roles {
		out.Roles[id] = value
	}
	for id, value := range base.Juries {
		out.Juries[id] = value
	}
	for id, value := range override.Juries {
		out.Juries[id] = value
	}
	for id, value := range base.Presets {
		out.Presets[id] = value
	}
	for id, value := range override.Presets {
		out.Presets[id] = value
	}
	return out
}

func mergeConfigDefaults(base ConfigDefaults, override ConfigDefaults) ConfigDefaults {
	out := ConfigDefaults{
		Preset:          firstNonEmpty(override.Preset, base.Preset),
		Workflow:        firstNonEmpty(override.Workflow, base.Workflow),
		DelegationScope: firstNonEmpty(override.DelegationScope, base.DelegationScope),
		Backend:         firstNonEmpty(override.Backend, base.Backend),
		Provider:        firstNonEmpty(override.Provider, base.Provider),
		Model:           firstNonEmpty(override.Model, base.Model),
		ModelOptions:    api.MergeModelOptions(override.ModelOptions, base.ModelOptions),
	}
	if len(base.Backends) > 0 || len(override.Backends) > 0 {
		out.Backends = map[string]RuntimeBackendConfig{}
		for backend, config := range base.Backends {
			out.Backends[backend] = config
		}
		for backend, config := range override.Backends {
			current := out.Backends[backend]
			out.Backends[backend] = RuntimeBackendConfig{
				Provider:     firstNonEmpty(config.Provider, current.Provider),
				Model:        firstNonEmpty(config.Model, current.Model),
				ModelOptions: api.MergeModelOptions(config.ModelOptions, current.ModelOptions),
			}
		}
	}
	return out
}

func parseConfigDefaults(values map[string]any) ConfigDefaults {
	return ConfigDefaults{
		Preset:          firstNonEmpty(stringValue(values["preset"]), stringValue(values["court"])),
		Workflow:        firstNonEmpty(stringValue(values["workflow"]), stringValue(values["routing_mode"]), stringValue(values["routing"])),
		DelegationScope: firstNonEmpty(stringValue(values["delegation_scope"]), stringValue(values["delegationScope"]), stringValue(values["delegate_scope"]), stringValue(values["delegateScope"])),
		Backend:         firstNonEmpty(stringValue(values["backend"]), stringValue(values["runtime"])),
		Provider:        firstNonEmpty(stringValue(values["provider"]), stringValue(values["customProvider"]), stringValue(values["custom_provider"])),
		Model:           stringValue(values["model"]),
		ModelOptions:    runtimeModelOptionsFromValueMap(values),
		Backends:        runtimeBackendConfigsFromValue(values["backends"]),
	}
}

func parseConfigAgents(raw map[string]any) map[string]ConfigAgent {
	out := map[string]ConfigAgent{}
	for id, values := range namedConfigEntries(raw, "agents", "agent") {
		agent := ConfigAgent{
			ID:           firstNonEmpty(stringValue(values["id"]), id),
			Title:        firstNonEmpty(stringValue(values["title"]), stringValue(values["name"]), id),
			Backend:      firstNonEmpty(stringValue(values["backend"]), stringValue(values["runtime"])),
			Provider:     firstNonEmpty(stringValue(values["provider"]), stringValue(values["customProvider"]), stringValue(values["custom_provider"])),
			Model:        stringValue(values["model"]),
			ModelOptions: runtimeModelOptionsFromValueMap(values),
			Backends:     runtimeBackendConfigsFromValue(values["backends"]),
		}
		if agent.ID != "" {
			out[agent.ID] = agent
		}
	}
	return out
}

func parseConfigRoles(raw map[string]any) map[string]ConfigRole {
	out := map[string]ConfigRole{}
	for _, section := range []struct {
		plural   string
		singular string
		kind     RoleKind
	}{
		{"roles", "role", ""},
		{"jurors", "juror", RoleJuror},
		{"judges", "judge", RoleJudge},
		{"clerks", "clerk", RoleClerk},
	} {
		for id, values := range namedConfigEntries(raw, section.plural, section.singular) {
			role := ConfigRole{
				ID:           firstNonEmpty(stringValue(values["id"]), id),
				Kind:         roleKindFromString(firstNonEmpty(stringValue(values["kind"]), string(section.kind)), ""),
				Title:        firstNonEmpty(stringValue(values["title"]), stringValue(values["name"]), id),
				Prompt:       firstNonEmpty(stringValue(values["prompt"]), stringValue(values["brief"]), stringValue(values["body"])),
				Agent:        stringValue(values["agent"]),
				Backend:      firstNonEmpty(stringValue(values["backend"]), stringValue(values["runtime"])),
				Provider:     firstNonEmpty(stringValue(values["provider"]), stringValue(values["customProvider"]), stringValue(values["custom_provider"])),
				Model:        stringValue(values["model"]),
				ModelOptions: runtimeModelOptionsFromValueMap(values),
				Backends:     runtimeBackendConfigsFromValue(values["backends"]),
			}
			if role.ID != "" {
				out[role.ID] = role
			}
		}
	}
	return out
}

func parseConfigJuries(raw map[string]any) map[string]ConfigJury {
	out := map[string]ConfigJury{}
	for id, values := range namedConfigEntries(raw, "juries", "jury") {
		jury := ConfigJury{
			ID:          firstNonEmpty(stringValue(values["id"]), stringValue(values["name"]), id),
			Title:       firstNonEmpty(stringValue(values["title"]), stringValue(values["name"]), id),
			Description: firstNonEmpty(stringValue(values["description"]), stringValue(values["prompt"]), stringValue(values["brief"])),
			JurorIDs:    firstStringList(values, "jurors", "jury", "members", "roles"),
			ClerkID:     stringValue(values["clerk"]),
			JudgeID:     firstNonEmpty(stringValue(values["final_judge"]), stringValue(values["finalJudge"]), stringValue(values["judge"]), stringValue(values["judge_juror"]), stringValue(values["inlineJudge"])),
		}
		if jury.ID != "" {
			out[jury.ID] = jury
		}
	}
	return out
}

func parseConfigPresets(raw map[string]any) map[string]ConfigPreset {
	out := map[string]ConfigPreset{}
	for id, values := range namedConfigEntries(raw, "presets", "preset") {
		preset := ConfigPreset{
			ID:          firstNonEmpty(stringValue(values["id"]), stringValue(values["name"]), id),
			Title:       firstNonEmpty(stringValue(values["title"]), stringValue(values["name"]), id),
			Description: firstNonEmpty(stringValue(values["description"]), stringValue(values["prompt"]), stringValue(values["brief"])),
			Workflow:    firstNonEmpty(stringValue(values["workflow"]), stringValue(values["routing_mode"]), stringValue(values["routing"])),
			JuryID:      firstNonEmpty(stringValue(values["jury"]), stringValue(values["juries"])),
			JurorIDs:    firstStringList(values, "jurors", "members", "roles"),
			ClerkID:     stringValue(values["clerk"]),
			JudgeID:     firstNonEmpty(stringValue(values["final_judge"]), stringValue(values["finalJudge"]), stringValue(values["judge"]), stringValue(values["judge_juror"]), stringValue(values["inlineJudge"])),
		}
		if preset.ID != "" {
			out[preset.ID] = preset
		}
	}
	return out
}

func namedConfigEntries(raw map[string]any, plural string, singular string) map[string]map[string]any {
	out := map[string]map[string]any{}
	if values := mapValue(raw[plural]); values != nil {
		for id, value := range values {
			if entry := mapValue(value); entry != nil {
				out[id] = entry
			}
		}
	}
	for _, value := range listValue(raw[singular]) {
		entry := mapValue(value)
		if entry == nil {
			continue
		}
		id := firstNonEmpty(stringValue(entry["id"]), stringValue(entry["name"]))
		if id != "" {
			out[id] = entry
		}
	}
	return out
}

// LoadConfigAgentFromRoots provides Court runtime functionality.
func LoadConfigAgentFromRoots(rootDirs []string, id string) (AgentConfig, bool, error) {
	config, err := LoadCourtConfigFromRoots(rootDirs)
	if err != nil {
		return AgentConfig{}, false, err
	}
	agent, ok := config.Agents[id]
	if !ok {
		return AgentConfig{}, false, nil
	}
	backendConfig := backendConfigFor(agent.Backends, agent.Backend)
	model := firstNonEmpty(agent.Model, backendConfig.Model)
	provider := firstNonEmpty(agent.Provider, backendConfig.Provider, api.InferModelProvider(model))
	return AgentConfig{
		ID:           agent.ID,
		Title:        firstNonEmpty(agent.Title, agent.ID),
		Backend:      firstNonEmpty(agent.Backend, singleBackendName(agent.Backends)),
		Provider:     provider,
		Model:        model,
		ModelOptions: api.MergeModelOptions(agent.ModelOptions, backendConfig.ModelOptions),
		Backends:     agent.Backends,
	}, true, nil
}

// LoadConfigRoleFromRoots provides Court runtime functionality.
func LoadConfigRoleFromRoots(rootDirs []string, id string) (Role, bool, error) {
	config, err := LoadCourtConfigFromRoots(rootDirs)
	if err != nil {
		return Role{}, false, err
	}
	roleConfig, ok := config.Roles[id]
	if !ok {
		return Role{}, false, nil
	}
	var agentConfig AgentConfig
	if roleConfig.Agent != "" {
		if config, ok, err := LoadAgentConfigFromRoots(rootDirs, roleConfig.Agent); err != nil {
			return Role{}, false, err
		} else if ok {
			agentConfig = config
		}
	}
	backendConfigs := mergeBackendConfigs(agentConfig.Backends, roleConfig.Backends)
	defaultBackend := firstNonEmpty(roleConfig.Backend, agentConfig.Backend, singleBackendName(roleConfig.Backends), singleBackendName(agentConfig.Backends))
	roleBackend := backendConfigFor(roleConfig.Backends, defaultBackend)
	agentBackend := backendConfigFor(agentConfig.Backends, defaultBackend)
	model := firstNonEmpty(roleConfig.Model, roleBackend.Model, agentBackend.Model, agentConfig.Model)
	provider := firstNonEmpty(roleConfig.Provider, roleBackend.Provider, agentBackend.Provider, agentConfig.Provider, api.InferModelProvider(model))
	return Role{
		ID:           id,
		Kind:         roleConfig.Kind,
		Title:        firstNonEmpty(roleConfig.Title, id),
		Brief:        roleConfig.Prompt,
		Backend:      defaultBackend,
		Provider:     provider,
		Model:        model,
		ModelOptions: api.MergeModelOptions(roleConfig.ModelOptions, roleBackend.ModelOptions, agentBackend.ModelOptions, agentConfig.ModelOptions),
		Agent:        roleConfig.Agent,
		Backends:     backendConfigs,
	}, true, nil
}

// LoadConfigJuryFromRoots provides Court runtime functionality.
func LoadConfigJuryFromRoots(rootDirs []string, id string) (Jury, bool, error) {
	config, err := LoadCourtConfigFromRoots(rootDirs)
	if err != nil {
		return Jury{}, false, err
	}
	jury, ok := config.Juries[id]
	if !ok {
		return Jury{}, false, nil
	}
	return Jury{
		ID:          jury.ID,
		Title:       firstNonEmpty(jury.Title, jury.ID),
		Description: jury.Description,
		JurorIDs:    jury.JurorIDs,
		ClerkID:     jury.ClerkID,
		JudgeID:     jury.JudgeID,
	}, true, nil
}

// LoadConfigPresetFromRoots provides Court runtime functionality.
func LoadConfigPresetFromRoots(rootDirs []string, id string) (Preset, bool, error) {
	config, err := LoadCourtConfigFromRoots(rootDirs)
	if err != nil {
		return Preset{}, false, err
	}
	presetConfig, ok := config.Presets[id]
	if !ok {
		return Preset{}, false, nil
	}
	jury := Jury{}
	if presetConfig.JuryID != "" {
		loaded, ok, err := LoadJuryFromRoots(rootDirs, presetConfig.JuryID)
		if err != nil {
			return Preset{}, false, err
		}
		if !ok {
			return Preset{}, false, fmt.Errorf("preset %q references missing jury %q", id, presetConfig.JuryID)
		}
		jury = loaded
	}

	roleIDs := presetConfig.JurorIDs
	if len(roleIDs) == 0 {
		roleIDs = jury.JurorIDs
	}
	roles := make([]Role, 0, len(roleIDs)+2)
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
	if judgeID := firstNonEmpty(presetConfig.JudgeID, jury.JudgeID); judgeID != "" {
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
	if clerkID := firstNonEmpty(presetConfig.ClerkID, jury.ClerkID); clerkID != "" {
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
	workflow := workflowFromRouting(presetConfig.Workflow, config.Defaults.Workflow)
	return Preset{
		ID:          presetConfig.ID,
		Title:       firstNonEmpty(presetConfig.Title, presetConfig.ID),
		Description: presetConfig.Description,
		Workflow:    workflow,
		Roles:       roles,
	}, true, nil
}

// ListConfigPresetsFromRoots provides Court runtime functionality.
func ListConfigPresetsFromRoots(rootDirs []string) ([]Preset, error) {
	config, err := LoadCourtConfigFromRoots(rootDirs)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(config.Presets))
	for id := range config.Presets {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	presets := make([]Preset, 0, len(ids))
	for _, id := range ids {
		preset, ok, err := LoadConfigPresetFromRoots(rootDirs, id)
		if err != nil {
			return nil, err
		}
		if ok {
			presets = append(presets, preset)
		}
	}
	return presets, nil
}

// BackendDefaults provides Court runtime functionality.
func (d ConfigDefaults) BackendDefaults(backend string) RuntimeBackendConfig {
	scoped := backendConfigFor(d.Backends, backend)
	model := firstNonEmpty(scoped.Model, d.Model)
	provider := firstNonEmpty(scoped.Provider, d.Provider, api.InferModelProvider(model))
	return RuntimeBackendConfig{
		Provider:     provider,
		Model:        model,
		ModelOptions: api.MergeModelOptions(scoped.ModelOptions, d.ModelOptions),
	}
}

func firstStringList(values map[string]any, keys ...string) []string {
	for _, key := range keys {
		if list := stringListValue(values[key]); len(list) > 0 {
			return list
		}
	}
	return nil
}

func runtimeBackendConfigsFromValue(value any) map[string]RuntimeBackendConfig {
	values := mapValue(value)
	if len(values) == 0 {
		return nil
	}
	out := map[string]RuntimeBackendConfig{}
	for backend, raw := range values {
		configValues := mapValue(raw)
		if len(configValues) == 0 {
			continue
		}
		config := RuntimeBackendConfig{
			Provider:     firstNonEmpty(stringValue(configValues["provider"]), stringValue(configValues["customProvider"]), stringValue(configValues["custom_provider"])),
			Model:        stringValue(configValues["model"]),
			ModelOptions: runtimeModelOptionsFromValueMap(configValues),
		}
		if config.Provider != "" || config.Model != "" || api.HasModelOptions(config.ModelOptions) {
			out[backend] = config
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func runtimeModelOptionsFromValueMap(values map[string]any) RuntimeModelOptions {
	if nested := mapValue(firstNonEmptyValue(values["model_options"], values["modelOptions"])); len(nested) > 0 {
		values = mergeValueMaps(values, nested)
	}
	options := RuntimeModelOptions{
		ReasoningEffort: firstNonEmpty(
			stringValue(values["reasoning_effort"]),
			stringValue(values["reasoningEffort"]),
		),
		ThinkingLevel: firstNonEmpty(
			stringValue(values["thinking_level"]),
			stringValue(values["thinkingLevel"]),
		),
	}
	if budget, ok := intPointerValue(firstNonEmptyValue(values["thinking_budget"], values["thinkingBudget"])); ok {
		options.ThinkingBudget = budget
	}
	return options
}

func firstNonEmptyValue(values ...any) any {
	for _, value := range values {
		if stringValue(value) != "" || mapValue(value) != nil {
			return value
		}
	}
	return nil
}

func mergeValueMaps(base map[string]any, override map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range base {
		out[key] = value
	}
	for key, value := range override {
		out[key] = value
	}
	return out
}

func intPointerValue(value any) (*int, bool) {
	switch typed := value.(type) {
	case int:
		return &typed, true
	case int64:
		converted := int(typed)
		return &converted, true
	case float64:
		converted := int(typed)
		return &converted, true
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil {
			return nil, false
		}
		return &parsed, true
	default:
		return nil, false
	}
}

func mapValue(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return typed
	default:
		return nil
	}
}

func listValue(value any) []any {
	switch typed := value.(type) {
	case []any:
		return typed
	case []map[string]any:
		out := make([]any, 0, len(typed))
		for _, value := range typed {
			out = append(out, value)
		}
		return out
	default:
		return nil
	}
}

func stringListValue(value any) []string {
	switch typed := value.(type) {
	case []string:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if item = strings.TrimSpace(item); item != "" {
				out = append(out, item)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := stringValue(item); text != "" {
				out = append(out, text)
			}
		}
		return out
	case string:
		return splitListValue(typed)
	default:
		return nil
	}
}

func stringValue(value any) string {
	text, ok := scalarFrontmatterValue(value)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}
