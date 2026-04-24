package controlplane

import (
	"fmt"
	"slices"
	"strings"

	"github.com/benjaminwestern/agentic-control/internal/controlplane/modelcatalog"
	"github.com/benjaminwestern/agentic-control/pkg/contract"
)

var defaultModelByBackend = map[string]string{
	"codex":    "gpt-5.4",
	"claude":   "claude-sonnet-4-6",
	"gemini":   "auto-gemini-3",
	"opencode": "opencode/gemini-3-flash",
}

var modelAliasesByBackend = map[string]map[string]string{
	"opencode": {
		"gemini-3-flash":                "opencode/gemini-3-flash",
		"gemini-3.1-pro":                "opencode/gemini-3.1-pro",
		"google/gemini-3-flash-preview": "google/gemini-3-flash-preview",
	},
	"gemini": {
		"gemini-3-flash":        "gemini-3-flash-preview",
		"gemini-3.1-pro":        "gemini-3.1-pro-preview",
		"gemini-3.1-flash-lite": "gemini-3.1-flash-lite-preview",
	},
}

func BuildModelRegistry(descriptors []contract.RuntimeDescriptor) contract.ModelRegistry {
	backends := make([]contract.RuntimeBackendRegistry, 0, len(descriptors))
	for _, descriptor := range descriptors {
		backends = append(backends, buildBackendRegistry(descriptor))
	}
	slices.SortFunc(backends, func(left, right contract.RuntimeBackendRegistry) int {
		if left.Backend < right.Backend {
			return -1
		}
		if left.Backend > right.Backend {
			return 1
		}
		return 0
	})
	return contract.ModelRegistry{
		SchemaVersion: contract.ControlPlaneSchemaVersion,
		Backends:      backends,
	}
}

func ValidateSessionTargetWithRegistry(registry contract.ModelRegistry, target RuntimeTarget) RuntimeTargetValidationResult {
	result := RuntimeTargetValidationResult{Target: ResolveRuntimeTarget(target)}
	backend, ok := registryBackend(registry, result.Target.Backend)
	if !ok {
		result.Issues = append(result.Issues, RuntimeValidationIssue{
			Severity: ValidationSeverityError,
			Code:     "unsupported_backend",
			Message:  fmt.Sprintf("backend %q is not registered by agentic-control", result.Target.Backend),
		})
		return result
	}
	if !backend.SupportsSession {
		result.Issues = append(result.Issues, RuntimeValidationIssue{
			Severity: ValidationSeverityError,
			Code:     "backend_missing_session_capabilities",
			Message:  fmt.Sprintf("backend %q does not support session start and event streaming", result.Target.Backend),
		})
	}
	if !backend.Installed {
		message := fmt.Sprintf("backend %q is registered but unavailable locally", result.Target.Backend)
		if len(backend.Issues) > 0 {
			message = backend.Issues[0]
		}
		result.Issues = append(result.Issues, RuntimeValidationIssue{
			Severity: ValidationSeverityError,
			Code:     "backend_unavailable",
			Message:  message,
		})
	}
	if strings.TrimSpace(result.Target.Model) == "" {
		result.Target.Model = backend.DefaultModel
		result.Target.Provider = firstNonEmpty(result.Target.Provider, backend.DefaultProvider)
		if strings.TrimSpace(result.Target.Model) == "" {
			result.Issues = append(result.Issues, RuntimeValidationIssue{
				Severity: ValidationSeverityWarning,
				Code:     "model_unspecified",
				Message:  fmt.Sprintf("no explicit model was provided for backend %q; the runtime default will apply", result.Target.Backend),
			})
			return result
		}
	}
	modelID := resolveRegistryModelAlias(backend, result.Target.Model)
	model, found := registryModel(backend, modelID)
	if !found {
		customAllowed := false
		for _, item := range backend.Models {
			if item.Custom {
				customAllowed = true
				break
			}
		}
		severity := ValidationSeverityWarning
		message := fmt.Sprintf("model %q is not present in the registry for backend %q", result.Target.Model, result.Target.Backend)
		if !customAllowed {
			severity = ValidationSeverityError
			message = fmt.Sprintf("model %q is not supported by backend %q according to the current registry", result.Target.Model, result.Target.Backend)
		}
		result.Issues = append(result.Issues, RuntimeValidationIssue{Severity: severity, Code: "model_unlisted", Message: message})
		return result
	}
	result.Target.Model = model.ID
	result.Target.Provider = firstNonEmpty(result.Target.Provider, NormalizeRuntimeProvider(model.Provider), backend.DefaultProvider)
	if result.Target.Provider != "" && model.Provider != "" && result.Target.Provider != NormalizeRuntimeProvider(model.Provider) {
		result.Issues = append(result.Issues, RuntimeValidationIssue{
			Severity: ValidationSeverityError,
			Code:     "provider_model_mismatch",
			Message:  fmt.Sprintf("model %q belongs to provider %q on backend %q, not %q", result.Target.Model, model.Provider, result.Target.Backend, result.Target.Provider),
		})
	}
	result.Model = model
	result.Issues = append(result.Issues, validateRuntimeModelOptions(result.Target, *model)...)
	return result
}

func buildBackendRegistry(descriptor contract.RuntimeDescriptor) contract.RuntimeBackendRegistry {
	models := mergeRuntimeModels(descriptor.Runtime, builtInModels(descriptor.Runtime), probeModels(descriptor))
	providers := groupModelsByProvider(models)
	registry := contract.RuntimeBackendRegistry{
		Backend:         descriptor.Runtime,
		DisplayName:     backendDisplayName(descriptor.Runtime),
		Installed:       descriptor.Probe == nil || descriptor.Probe.Installed,
		SupportsSession: descriptor.Capabilities.StartSession && descriptor.Capabilities.StreamEvents,
		ModelSource:     modelSource(descriptor),
		Models:          models,
		Providers:       providers,
		Issues:          backendIssues(descriptor),
	}
	registry.DefaultModel = backendDefaultModel(descriptor.Runtime, models)
	registry.DefaultProvider = defaultProviderFromModel(models, registry.DefaultModel)
	registry.Aliases = backendAliases(descriptor.Runtime, models)
	for i := range registry.Providers {
		registry.Providers[i].DefaultModel = providerDefaultModel(registry.Providers[i])
	}
	return registry
}

func builtInModels(runtime string) []contract.RuntimeModel {
	switch runtime {
	case "codex":
		return modelcatalog.Codex()
	case "claude":
		return modelcatalog.Claude()
	case "gemini":
		return modelcatalog.Gemini()
	case "pi":
		return modelcatalog.Pi()
	default:
		return nil
	}
}

func probeModels(descriptor contract.RuntimeDescriptor) []contract.RuntimeModel {
	if descriptor.Probe == nil {
		return nil
	}
	return descriptor.Probe.Models
}

func mergeRuntimeModels(runtime string, primary []contract.RuntimeModel, secondary []contract.RuntimeModel) []contract.RuntimeModel {
	merged := make(map[string]contract.RuntimeModel)
	for _, model := range secondary {
		merged[model.ID] = model
	}
	for _, model := range primary {
		current, ok := merged[model.ID]
		if !ok {
			merged[model.ID] = model
			continue
		}
		if current.Label == "" {
			current.Label = model.Label
		}
		if current.Provider == "" {
			current.Provider = model.Provider
		}
		if !current.Default && model.Default {
			current.Default = true
		}
		if runtimeModelCapabilitiesEmpty(current.Capabilities) {
			current.Capabilities = model.Capabilities
		}
		merged[model.ID] = current
	}
	out := make([]contract.RuntimeModel, 0, len(merged))
	for _, model := range merged {
		out = append(out, model)
	}
	slices.SortFunc(out, func(left, right contract.RuntimeModel) int {
		if left.Default != right.Default {
			if left.Default {
				return -1
			}
			return 1
		}
		if left.Provider != right.Provider {
			if left.Provider < right.Provider {
				return -1
			}
			return 1
		}
		if left.ID < right.ID {
			return -1
		}
		if left.ID > right.ID {
			return 1
		}
		return 0
	})
	return out
}

func groupModelsByProvider(models []contract.RuntimeModel) []contract.RuntimeProviderRegistry {
	byProvider := map[string][]contract.RuntimeModel{}
	for _, model := range models {
		provider := NormalizeRuntimeProvider(model.Provider)
		if provider == "" {
			provider = InferModelProvider(model.ID)
		}
		byProvider[provider] = append(byProvider[provider], model)
	}
	providers := make([]contract.RuntimeProviderRegistry, 0, len(byProvider))
	for provider, models := range byProvider {
		slices.SortFunc(models, func(left, right contract.RuntimeModel) int {
			if left.Default != right.Default {
				if left.Default {
					return -1
				}
				return 1
			}
			if left.ID < right.ID {
				return -1
			}
			if left.ID > right.ID {
				return 1
			}
			return 0
		})
		providers = append(providers, contract.RuntimeProviderRegistry{
			Provider:    provider,
			DisplayName: providerDisplayName(provider),
			Models:      models,
		})
	}
	slices.SortFunc(providers, func(left, right contract.RuntimeProviderRegistry) int {
		if left.Provider < right.Provider {
			return -1
		}
		if left.Provider > right.Provider {
			return 1
		}
		return 0
	})
	return providers
}

func backendDefaultModel(runtime string, models []contract.RuntimeModel) string {
	for _, model := range models {
		if model.Default {
			return model.ID
		}
	}
	if configured := defaultModelByBackend[runtime]; configured != "" {
		for _, model := range models {
			if model.ID == configured {
				return model.ID
			}
		}
	}
	for _, model := range models {
		if !model.Custom {
			return model.ID
		}
	}
	if len(models) > 0 {
		return models[0].ID
	}
	return ""
}

func providerDefaultModel(provider contract.RuntimeProviderRegistry) string {
	for _, model := range provider.Models {
		if model.Default {
			return model.ID
		}
	}
	if len(provider.Models) > 0 {
		return provider.Models[0].ID
	}
	return ""
}

func defaultProviderFromModel(models []contract.RuntimeModel, modelID string) string {
	for _, model := range models {
		if model.ID == modelID {
			return NormalizeRuntimeProvider(model.Provider)
		}
	}
	return ""
}

func backendAliases(runtime string, models []contract.RuntimeModel) []contract.ModelAlias {
	aliases := make([]contract.ModelAlias, 0)
	configured := modelAliasesByBackend[runtime]
	for alias, modelID := range configured {
		for _, model := range models {
			if model.ID == modelID {
				aliases = append(aliases, contract.ModelAlias{Alias: alias, Model: modelID})
				break
			}
		}
	}
	slices.SortFunc(aliases, func(left, right contract.ModelAlias) int {
		if left.Alias < right.Alias {
			return -1
		}
		if left.Alias > right.Alias {
			return 1
		}
		return 0
	})
	return aliases
}

func backendIssues(descriptor contract.RuntimeDescriptor) []string {
	var issues []string
	if descriptor.Probe != nil && descriptor.Probe.Message != "" {
		issues = append(issues, descriptor.Probe.Message)
	}
	return issues
}

func modelSource(descriptor contract.RuntimeDescriptor) string {
	if descriptor.Probe != nil && descriptor.Probe.ModelSource != "" {
		return descriptor.Probe.ModelSource
	}
	if len(builtInModels(descriptor.Runtime)) > 0 {
		return "built_in"
	}
	return ""
}

func backendDisplayName(runtime string) string {
	switch runtime {
	case "codex":
		return "Codex"
	case "claude":
		return "Claude"
	case "gemini":
		return "Gemini"
	case "opencode":
		return "OpenCode"
	case "pi":
		return "Pi"
	default:
		return strings.ToUpper(runtime[:1]) + runtime[1:]
	}
}

func providerDisplayName(provider string) string {
	switch provider {
	case "openai":
		return "OpenAI"
	case "anthropic":
		return "Anthropic"
	case "google":
		return "Google"
	case "opencode":
		return "OpenCode"
	case "codex":
		return "Codex"
	case "claude":
		return "Claude"
	case "gemini":
		return "Gemini"
	default:
		return strings.ToUpper(provider[:1]) + provider[1:]
	}
}

func registryBackend(registry contract.ModelRegistry, backend string) (contract.RuntimeBackendRegistry, bool) {
	for _, item := range registry.Backends {
		if item.Backend == backend {
			return item, true
		}
	}
	return contract.RuntimeBackendRegistry{}, false
}

func registryModel(backend contract.RuntimeBackendRegistry, modelID string) (*contract.RuntimeModel, bool) {
	for _, model := range backend.Models {
		if model.ID == modelID {
			modelCopy := model
			return &modelCopy, true
		}
	}
	return nil, false
}

func resolveRegistryModelAlias(backend contract.RuntimeBackendRegistry, modelID string) string {
	for _, alias := range backend.Aliases {
		if alias.Alias == modelID {
			return alias.Model
		}
	}
	return modelID
}

func runtimeModelCapabilitiesEmpty(value contract.RuntimeModelCapabilities) bool {
	return len(value.ReasoningEffortLevels) == 0 &&
		len(value.ContextWindowOptions) == 0 &&
		len(value.VariantOptions) == 0 &&
		len(value.AgentOptions) == 0 &&
		len(value.PromptInjectedEfforts) == 0 &&
		!value.SupportsFastMode &&
		!value.SupportsThinkingToggle &&
		!value.SupportsThinkingLevel &&
		!value.SupportsThinkingBudget &&
		len(value.SupportedThinkingLevels) == 0 &&
		len(value.SupportedThinkingBudgets) == 0
}
