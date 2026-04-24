package orchestration

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/benjaminwestern/agentic-control/pkg/contract"
	api "github.com/benjaminwestern/agentic-control/pkg/controlplane"
)

type FanoutController interface {
	Describe() contract.SystemDescriptor
	SubscribeEvents(buffer int) (<-chan contract.RuntimeEvent, func())
	StartSession(context.Context, string, api.StartSessionRequest) (*contract.RuntimeSession, error)
	SendInput(context.Context, api.SendInputRequest) (*contract.RuntimeEvent, error)
	StopSession(context.Context, string) (*contract.RuntimeEvent, error)
	GetTrackedSession(context.Context, string, string) (*contract.TrackedSession, error)
}

type FanoutTarget struct {
	Backend   string                   `json:"backend"`
	Model     string                   `json:"model,omitempty"`
	Label     string                   `json:"label,omitempty"`
	Options   api.ModelOptions         `json:"options,omitempty"`
	Selection *contract.ModelSelection `json:"selection,omitempty"`
}

type FanoutTargetResult struct {
	Target          FanoutTarget             `json:"target"`
	Session         *contract.TrackedSession `json:"session,omitempty"`
	Text            string                   `json:"text,omitempty"`
	Error           string                   `json:"error,omitempty"`
	Stopped         bool                     `json:"stopped,omitempty"`
	StopError       string                   `json:"stop_error,omitempty"`
	EventCount      int                      `json:"event_count,omitempty"`
	RecordedUsage   contract.TokenUsage      `json:"recorded_usage,omitempty"`
	RecordedCostUSD float64                  `json:"recorded_cost_usd,omitempty"`
}

type FanoutResult struct {
	Prompt       string               `json:"prompt"`
	Targets      []FanoutTargetResult `json:"targets"`
	TotalUsage   contract.TokenUsage  `json:"total_usage,omitempty"`
	TotalCostUSD float64              `json:"total_cost_usd,omitempty"`
	TargetCount  int                  `json:"target_count"`
}

type FanoutOptions struct {
	Targets      []FanoutTarget
	Prompt       string
	Repeat       int
	ModelOptions api.ModelOptions
	KeepSessions bool
	Metadata     map[string]any
	EventBuffer  int
}

type fanoutTargetState struct {
	result      FanoutTargetResult
	sessionID   string
	done        bool
	accumulator api.TurnAccumulator
}

func ResolveFanoutTargets(descriptors []contract.RuntimeDescriptor, requested []FanoutTarget) ([]FanoutTarget, error) {
	if len(requested) == 0 {
		requested = defaultFanoutTargets(descriptors)
	}
	if len(requested) == 0 {
		return nil, fmt.Errorf("no available session-capable runtimes were discovered")
	}
	resolved := make([]FanoutTarget, 0, len(requested))
	registry := api.BuildModelRegistry(descriptors)
	for _, target := range requested {
		explicitModel := strings.TrimSpace(target.Model)
		if target.Selection != nil {
			explicitModel = strings.TrimSpace(target.Selection.Model)
		}
		validation := api.RuntimeTargetValidationResult{}
		if target.Selection != nil {
			normalized := api.NormalizeModelSelection(registry, *target.Selection)
			validation = api.ValidateSessionTargetWithRegistry(registry, api.RuntimeTargetFromSelection(normalized))
			target.Selection = &normalized
		} else {
			validation = api.ValidateSessionTargetWithRegistry(registry, api.RuntimeTarget{
				Backend: target.Backend,
				Model:   target.Model,
				Options: target.Options,
			})
		}
		if validation.HasErrors() {
			return nil, fmt.Errorf("target %q is invalid: %s", target.Backend, validation.Issues[0].Message)
		}
		if explicitModel != "" && validation.HasIssueCode("model_unlisted") {
			return nil, fmt.Errorf("target %q is invalid: %s", target.Backend, validation.Issues[0].Message)
		}
		resolved = append(resolved, FanoutTarget{
			Backend:   validation.Target.Backend,
			Model:     validation.Target.Model,
			Options:   validation.Target.Options,
			Selection: target.Selection,
		})
	}
	return resolved, nil
}

func RunFanout(ctx context.Context, controller FanoutController, options FanoutOptions) (FanoutResult, error) {
	if strings.TrimSpace(options.Prompt) == "" {
		return FanoutResult{}, fmt.Errorf("prompt is required")
	}
	descriptor := controller.Describe()
	targets, err := ResolveFanoutTargets(descriptor.Runtimes, options.Targets)
	if err != nil {
		return FanoutResult{}, err
	}
	targets = repeatFanoutTargets(targets, options.Repeat)
	assignFanoutLabels(targets)
	for i := range targets {
		targets[i].Options = api.MergeModelOptions(options.ModelOptions, targets[i].Options)
	}
	buffer := options.EventBuffer
	if buffer <= 0 {
		buffer = 2048
	}
	events, unsubscribe := controller.SubscribeEvents(buffer)
	defer unsubscribe()

	states := make(map[string]*fanoutTargetState, len(targets))
	results := make([]FanoutTargetResult, 0, len(targets))
	for _, target := range targets {
		session, startErr := controller.StartSession(ctx, target.Backend, api.StartSessionRequest{
			SessionID:    "fanout-" + randomFanoutID(),
			Model:        target.Model,
			ModelOptions: target.Options,
			Prompt:       options.Prompt,
			Metadata: mergeFanoutMetadata(options.Metadata, map[string]any{
				"thread_name":   fanoutLabelBase(target),
				"thread_kind":   "orchestration_target",
				"workflow":      "fanout",
				"workflow_mode": "fanout",
				"target_label":  fanoutLabelBase(target),
				"task":          options.Prompt,
			}),
		})
		result := FanoutTargetResult{Target: target}
		if startErr != nil {
			result.Error = startErr.Error()
			results = append(results, result)
			continue
		}
		state := &fanoutTargetState{result: result, sessionID: session.SessionID}
		states[session.SessionID] = state
	}

	remaining := len(states)
	for remaining > 0 {
		select {
		case <-ctx.Done():
			return FanoutResult{}, ctx.Err()
		case event, ok := <-events:
			if !ok {
				return FanoutResult{}, fmt.Errorf("control plane event stream closed")
			}
			state, exists := states[event.SessionID]
			if !exists || state.done {
				continue
			}
			state.accumulator.Add(event)
			switch {
			case contract.IsTurnCompletedEvent(event):
				state.done = true
				state.result.Text = firstNonEmptyFanout(state.accumulator.FinalText, state.accumulator.JoinedDelta(), event.Summary)
				state.result.EventCount = len(state.accumulator.EventLines)
				finishFanoutTarget(ctx, controller, options.KeepSessions, state)
				results = append(results, state.result)
				remaining--
			case contract.IsTurnErroredEvent(event):
				state.done = true
				state.result.Text = state.accumulator.JoinedDelta()
				state.result.Error = contract.EventErrorText(event)
				state.result.EventCount = len(state.accumulator.EventLines)
				finishFanoutTarget(ctx, controller, options.KeepSessions, state)
				results = append(results, state.result)
				remaining--
			}
		}
	}

	output := FanoutResult{
		Prompt:      options.Prompt,
		Targets:     results,
		TargetCount: len(results),
	}
	for _, result := range results {
		output.TotalUsage = addUsage(output.TotalUsage, result.RecordedUsage)
		output.TotalCostUSD += result.RecordedCostUSD
	}
	return output, nil
}

func finishFanoutTarget(ctx context.Context, controller FanoutController, keepSessions bool, state *fanoutTargetState) {
	tracked, err := controller.GetTrackedSession(ctx, state.sessionID, "")
	if err == nil {
		state.result.Session = tracked
		state.result.RecordedUsage = tracked.Session.Usage
		state.result.RecordedCostUSD = tracked.Session.CostUSD
	}
	if keepSessions {
		return
	}
	if _, err := controller.StopSession(ctx, state.sessionID); err != nil {
		state.result.StopError = err.Error()
		return
	}
	state.result.Stopped = true
	if tracked, err := controller.GetTrackedSession(ctx, state.sessionID, ""); err == nil {
		state.result.Session = tracked
		state.result.RecordedUsage = tracked.Session.Usage
		state.result.RecordedCostUSD = tracked.Session.CostUSD
	}
}

func defaultFanoutTargets(descriptors []contract.RuntimeDescriptor) []FanoutTarget {
	var targets []FanoutTarget
	for _, runtime := range descriptors {
		if !runtime.Capabilities.StartSession || !runtime.Capabilities.StreamEvents {
			continue
		}
		if runtime.Probe != nil && !runtime.Probe.Installed {
			continue
		}
		targets = append(targets, FanoutTarget{Backend: runtime.Runtime, Model: defaultRuntimeModel(runtime)})
	}
	return targets
}

func repeatFanoutTargets(targets []FanoutTarget, repeat int) []FanoutTarget {
	if repeat <= 1 {
		return targets
	}
	expanded := make([]FanoutTarget, 0, len(targets)*repeat)
	for _, target := range targets {
		for i := 0; i < repeat; i++ {
			expanded = append(expanded, target)
		}
	}
	return expanded
}

func assignFanoutLabels(targets []FanoutTarget) {
	totals := make(map[string]int)
	for _, target := range targets {
		totals[fanoutLabelBase(target)]++
	}
	seen := make(map[string]int)
	for i := range targets {
		base := fanoutLabelBase(targets[i])
		seen[base]++
		if totals[base] > 1 {
			targets[i].Label = fmt.Sprintf("%s#%d", base, seen[base])
		} else {
			targets[i].Label = base
		}
	}
}

func fanoutLabelBase(target FanoutTarget) string {
	if strings.TrimSpace(target.Label) != "" {
		return strings.TrimSpace(target.Label)
	}
	if strings.TrimSpace(target.Model) != "" {
		return fmt.Sprintf("%s=%s", target.Backend, target.Model)
	}
	return target.Backend
}

func mergeFanoutMetadata(base map[string]any, extra map[string]any) map[string]any {
	merged := make(map[string]any, len(base)+len(extra))
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range extra {
		merged[key] = value
	}
	return merged
}

func defaultRuntimeModel(runtime contract.RuntimeDescriptor) string {
	if runtime.Probe == nil {
		return ""
	}
	for _, model := range runtime.Probe.Models {
		if model.Default {
			return model.ID
		}
	}
	for _, model := range runtime.Probe.Models {
		if !model.Custom {
			return model.ID
		}
	}
	return ""
}

func addUsage(left contract.TokenUsage, right contract.TokenUsage) contract.TokenUsage {
	return contract.TokenUsage{
		InputTokens:     left.InputTokens + right.InputTokens,
		OutputTokens:    left.OutputTokens + right.OutputTokens,
		ReasoningTokens: left.ReasoningTokens + right.ReasoningTokens,
		CachedTokens:    left.CachedTokens + right.CachedTokens,
		TotalTokens:     left.TotalTokens + right.TotalTokens,
	}
}

func randomFanoutID() string {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return "fallback"
	}
	return hex.EncodeToString(buf)
}

func firstNonEmptyFanout(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
