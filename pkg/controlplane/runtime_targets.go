package controlplane

import (
	"fmt"
	"slices"
	"strings"

	"github.com/benjaminwestern/agentic-control/pkg/contract"
)

type ValidationSeverity string

const (
	ValidationSeverityWarning ValidationSeverity = "warning"
	ValidationSeverityError   ValidationSeverity = "error"
)

type RuntimeValidationIssue struct {
	Severity ValidationSeverity `json:"severity"`
	Code     string             `json:"code"`
	Message  string             `json:"message"`
}

type RuntimeTarget struct {
	Backend  string       `json:"backend,omitempty"`
	Provider string       `json:"provider,omitempty"`
	Model    string       `json:"model,omitempty"`
	Options  ModelOptions `json:"options,omitempty"`
}

type RuntimeTargetValidationResult struct {
	Target  RuntimeTarget               `json:"target"`
	Runtime *contract.RuntimeDescriptor `json:"runtime,omitempty"`
	Model   *contract.RuntimeModel      `json:"model_descriptor,omitempty"`
	Issues  []RuntimeValidationIssue    `json:"issues,omitempty"`
}

func (result RuntimeTargetValidationResult) HasErrors() bool {
	for _, issue := range result.Issues {
		if issue.Severity == ValidationSeverityError {
			return true
		}
	}
	return false
}

func (result RuntimeTargetValidationResult) HasIssueCode(code string) bool {
	for _, issue := range result.Issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func NormalizeRuntimeBackend(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "claudeagent", "claude-agent", "claude_code", "claudecode":
		return "claude"
	case "open-code":
		return "opencode"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func NormalizeRuntimeProvider(value string) string {
	return NormalizeRuntimeBackend(value)
}

func InferRuntimeBackend(model string) string {
	model = strings.ToLower(strings.TrimSpace(model))
	switch {
	case model == "":
		return ""
	case strings.HasPrefix(model, "claude-"):
		return "claude"
	case strings.HasPrefix(model, "gemini-"), strings.HasPrefix(model, "auto-gemini-"):
		return "gemini"
	case strings.HasPrefix(model, "gpt-"), strings.HasPrefix(model, "o1"), strings.HasPrefix(model, "o3"), strings.HasPrefix(model, "o4"):
		return "codex"
	case strings.Contains(model, "/"):
		return "opencode"
	default:
		return ""
	}
}

func InferRuntimeProvider(model string) string {
	return InferRuntimeBackend(model)
}

func InferModelProvider(model string) string {
	model = strings.ToLower(strings.TrimSpace(model))
	switch {
	case model == "":
		return ""
	case strings.Contains(model, "/"):
		prefix, _, _ := strings.Cut(model, "/")
		return strings.TrimSpace(prefix)
	case strings.HasPrefix(model, "claude-"):
		return "anthropic"
	case strings.HasPrefix(model, "gemini-"), strings.HasPrefix(model, "auto-gemini-"):
		return "google"
	case strings.HasPrefix(model, "gpt-"), strings.HasPrefix(model, "o1"), strings.HasPrefix(model, "o3"), strings.HasPrefix(model, "o4"):
		return "openai"
	default:
		return ""
	}
}

func SupportedSessionBackends(descriptors []contract.RuntimeDescriptor) []string {
	backends := make([]string, 0, len(descriptors))
	for _, runtime := range descriptors {
		if !runtime.Capabilities.StartSession || !runtime.Capabilities.StreamEvents {
			continue
		}
		backends = append(backends, runtime.Runtime)
	}
	slices.Sort(backends)
	return backends
}

func ValidateSessionBackend(descriptors []contract.RuntimeDescriptor, backend string) (*contract.RuntimeDescriptor, error) {
	if strings.TrimSpace(backend) == "" {
		backend = "opencode"
	}
	for _, runtime := range descriptors {
		if runtime.Runtime != backend {
			continue
		}
		if !runtime.Capabilities.StartSession || !runtime.Capabilities.StreamEvents {
			return nil, fmt.Errorf("backend %q is registered by agentic-control but does not support session start and event streaming", backend)
		}
		if runtime.Probe != nil && !runtime.Probe.Installed {
			message := firstNonEmpty(runtime.Probe.Message, fmt.Sprintf("%s runtime binary was not found", backend))
			return nil, fmt.Errorf("backend %q is registered by agentic-control but unavailable locally: %s", backend, message)
		}
		runtimeCopy := runtime
		return &runtimeCopy, nil
	}
	return nil, fmt.Errorf("unsupported backend %q; supported backends are %s", backend, strings.Join(SupportedSessionBackends(descriptors), ", "))
}

func ResolveRuntimeTarget(target RuntimeTarget) RuntimeTarget {
	resolved := target
	if strings.TrimSpace(resolved.Backend) == "" {
		resolved.Backend = "opencode"
	}
	resolved.Provider = strings.ToLower(strings.TrimSpace(resolved.Provider))
	return resolved
}

func ValidateSessionTarget(descriptors []contract.RuntimeDescriptor, target RuntimeTarget) RuntimeTargetValidationResult {
	return ValidateSessionTargetWithRegistry(BuildModelRegistry(descriptors), target)
}

func validateRuntimeModelOptions(target RuntimeTarget, model contract.RuntimeModel) []RuntimeValidationIssue {
	var issues []RuntimeValidationIssue
	capabilities := model.Capabilities
	if effort := strings.TrimSpace(target.Options.ReasoningEffort); effort != "" && len(capabilities.ReasoningEffortLevels) > 0 {
		if !runtimeModelOptionExists(capabilities.ReasoningEffortLevels, effort) {
			issues = append(issues, RuntimeValidationIssue{
				Severity: ValidationSeverityError,
				Code:     "unsupported_reasoning_effort",
				Message:  fmt.Sprintf("reasoning effort %q is not supported by model %q on backend %q", effort, target.Model, target.Backend),
			})
		}
	}
	if level := strings.TrimSpace(target.Options.ThinkingLevel); level != "" {
		if !capabilities.SupportsThinkingLevel {
			issues = append(issues, RuntimeValidationIssue{
				Severity: ValidationSeverityError,
				Code:     "unsupported_thinking_level",
				Message:  fmt.Sprintf("thinking level is not supported by model %q on backend %q", target.Model, target.Backend),
			})
		} else if len(capabilities.SupportedThinkingLevels) > 0 && !containsString(capabilities.SupportedThinkingLevels, level) {
			issues = append(issues, RuntimeValidationIssue{
				Severity: ValidationSeverityError,
				Code:     "invalid_thinking_level",
				Message:  fmt.Sprintf("thinking level %q is not supported by model %q on backend %q", level, target.Model, target.Backend),
			})
		}
	}
	if target.Options.ThinkingBudget != nil {
		budget := *target.Options.ThinkingBudget
		if !capabilities.SupportsThinkingBudget {
			issues = append(issues, RuntimeValidationIssue{
				Severity: ValidationSeverityError,
				Code:     "unsupported_thinking_budget",
				Message:  fmt.Sprintf("thinking budget is not supported by model %q on backend %q", target.Model, target.Backend),
			})
		} else if len(capabilities.SupportedThinkingBudgets) > 0 && !containsInt(capabilities.SupportedThinkingBudgets, budget) {
			issues = append(issues, RuntimeValidationIssue{
				Severity: ValidationSeverityError,
				Code:     "invalid_thinking_budget",
				Message:  fmt.Sprintf("thinking budget %d is not supported by model %q on backend %q", budget, target.Model, target.Backend),
			})
		}
	}
	return issues
}

func runtimeDescriptorByName(descriptors []contract.RuntimeDescriptor, backend string) (*contract.RuntimeDescriptor, bool) {
	for _, runtime := range descriptors {
		if runtime.Runtime != backend {
			continue
		}
		runtimeCopy := runtime
		return &runtimeCopy, true
	}
	return nil, false
}

func runtimeModelByID(models []contract.RuntimeModel, id string) (*contract.RuntimeModel, bool) {
	for _, model := range models {
		if model.ID != id {
			continue
		}
		modelCopy := model
		return &modelCopy, true
	}
	return nil, false
}

func runtimeAllowsCustomModels(models []contract.RuntimeModel) bool {
	for _, model := range models {
		if model.Custom {
			return true
		}
	}
	return false
}

func runtimeModelOptionExists(options []contract.RuntimeModelOption, value string) bool {
	for _, option := range options {
		if option.Value == value {
			return true
		}
	}
	return false
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsInt(values []int, want int) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
