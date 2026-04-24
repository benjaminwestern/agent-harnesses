package orchestration

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/benjaminwestern/agentic-control/pkg/contract"
	api "github.com/benjaminwestern/agentic-control/pkg/controlplane"
)

type ReductionMode string

const (
	ReductionModeCompare   ReductionMode = "compare"
	ReductionModeSummarize ReductionMode = "summarize"
	ReductionModeBestOfN   ReductionMode = "best_of_n"
)

type ReductionResult struct {
	Mode            ReductionMode            `json:"mode"`
	Target          FanoutTarget             `json:"target"`
	Session         *contract.TrackedSession `json:"session,omitempty"`
	Text            string                   `json:"text,omitempty"`
	JSON            string                   `json:"json,omitempty"`
	Error           string                   `json:"error,omitempty"`
	Stopped         bool                     `json:"stopped,omitempty"`
	StopError       string                   `json:"stop_error,omitempty"`
	RecordedUsage   contract.TokenUsage      `json:"recorded_usage,omitempty"`
	RecordedCostUSD float64                  `json:"recorded_cost_usd,omitempty"`
}

type ReviewedFanoutResult struct {
	Fanout       FanoutResult        `json:"fanout"`
	Reduction    ReductionResult     `json:"reduction"`
	TotalUsage   contract.TokenUsage `json:"total_usage,omitempty"`
	TotalCostUSD float64             `json:"total_cost_usd,omitempty"`
}

type ReviewedFanoutOptions struct {
	Fanout          FanoutOptions
	Mode            ReductionMode
	ReductionTarget FanoutTarget
}

func RunReviewedFanout(ctx context.Context, controller FanoutController, options ReviewedFanoutOptions) (ReviewedFanoutResult, error) {
	fanout, err := RunFanout(ctx, controller, options.Fanout)
	if err != nil {
		return ReviewedFanoutResult{}, err
	}
	reduction, err := RunReduction(ctx, controller, options.Mode, fanout, options.ReductionTarget, options.Fanout.KeepSessions)
	if err != nil {
		return ReviewedFanoutResult{}, err
	}
	return ReviewedFanoutResult{
		Fanout:       fanout,
		Reduction:    reduction,
		TotalUsage:   addUsage(fanout.TotalUsage, reduction.RecordedUsage),
		TotalCostUSD: fanout.TotalCostUSD + reduction.RecordedCostUSD,
	}, nil
}

func RunReduction(ctx context.Context, controller FanoutController, mode ReductionMode, fanout FanoutResult, target FanoutTarget, keepSession bool) (ReductionResult, error) {
	if mode == "" {
		return ReductionResult{}, fmt.Errorf("reduction mode is required")
	}
	resolved, err := resolveReductionTarget(controller.Describe().Runtimes, target)
	if err != nil {
		return ReductionResult{}, err
	}
	result, err := api.RunStructuredSession(ctx, controller, resolved.Backend, api.StartSessionRequest{
		SessionID:    "reduce-" + randomFanoutID(),
		Model:        resolved.Model,
		ModelOptions: resolved.Options,
		Prompt:       reductionPrompt(mode, fanout),
		Metadata: map[string]any{
			"thread_name":    "reducer",
			"thread_kind":    "orchestration_reducer",
			"workflow":       "fanout_reduce",
			"workflow_mode":  string(mode),
			"reduction_mode": string(mode),
		},
	}, api.StructuredSessionOptions{
		Extract:        reductionExtractor(mode),
		RepairPrompt:   reductionRepairPrompt(mode),
		RepairMetadata: api.MetadataForNoToolTurn(map[string]any{"workflow": "fanout_reduce", "reduction_mode": string(mode)}),
		MaxRepairTurns: 1,
	})
	output := ReductionResult{Mode: mode, Target: resolved}
	if err != nil {
		output.Error = err.Error()
		return output, nil
	}
	output.Text = result.Text
	output.JSON = result.JSON
	if tracked, err := controller.GetTrackedSession(ctx, result.Session.SessionID, result.Session.ProviderSessionID); err == nil {
		output.Session = tracked
		output.RecordedUsage = tracked.Session.Usage
		output.RecordedCostUSD = tracked.Session.CostUSD
	}
	if keepSession {
		return output, nil
	}
	if _, err := controller.StopSession(ctx, result.Session.SessionID); err != nil {
		output.StopError = err.Error()
		return output, nil
	}
	output.Stopped = true
	if tracked, err := controller.GetTrackedSession(ctx, result.Session.SessionID, result.Session.ProviderSessionID); err == nil {
		output.Session = tracked
		output.RecordedUsage = tracked.Session.Usage
		output.RecordedCostUSD = tracked.Session.CostUSD
	}
	return output, nil
}

func resolveReductionTarget(descriptors []contract.RuntimeDescriptor, requested FanoutTarget) (FanoutTarget, error) {
	if strings.TrimSpace(requested.Backend) == "" {
		for _, runtime := range descriptors {
			if runtime.Runtime == "opencode" && runtime.Capabilities.StartSession && runtime.Capabilities.StreamEvents && (runtime.Probe == nil || runtime.Probe.Installed) {
				return FanoutTarget{Backend: runtime.Runtime, Model: defaultRuntimeModel(runtime), Options: requested.Options, Label: "reducer"}, nil
			}
		}
	}
	targets, err := ResolveFanoutTargets(descriptors, []FanoutTarget{requested})
	if err != nil {
		return FanoutTarget{}, err
	}
	resolved := targets[0]
	if resolved.Label == "" {
		resolved.Label = "reducer"
	}
	return resolved, nil
}

func reductionPrompt(mode ReductionMode, fanout FanoutResult) string {
	var builder strings.Builder
	builder.WriteString("You are synthesizing multiple candidate outputs for the same task.\n")
	builder.WriteString("Return exactly one JSON object and no surrounding prose.\n\n")
	builder.WriteString("Task:\n")
	builder.WriteString(fanout.Prompt)
	builder.WriteString("\n\n")
	builder.WriteString("Candidates as JSON:\n")
	encoded, _ := json.MarshalIndent(fanout.Targets, "", "  ")
	builder.Write(encoded)
	builder.WriteString("\n\n")
	switch mode {
	case ReductionModeCompare:
		builder.WriteString("Return this shape:\n")
		builder.WriteString(`{"summary":"...","comparison":"...","recommendation":"...","ranked_labels":["candidate-1","candidate-2"]}`)
	case ReductionModeSummarize:
		builder.WriteString("Return this shape:\n")
		builder.WriteString(`{"summary":"...","synthesis":"...","highlights":["..."]}`)
	case ReductionModeBestOfN:
		builder.WriteString("Return this shape:\n")
		builder.WriteString(`{"summary":"...","winner_label":"candidate-1","rationale":"...","recommended_next_step":"..."}`)
	}
	return builder.String()
}

func reductionRepairPrompt(mode ReductionMode) string {
	return "Your previous turn did not return the required JSON object. Reply again with exactly one valid JSON object matching the requested shape and no extra prose."
}

func reductionExtractor(mode ReductionMode) api.StructuredResultExtractor {
	return func(values ...string) (string, string) {
		return api.ExtractStructuredJSON(strings.Join(values, "\n"), func(candidate string) (string, string, bool) {
			return parseReductionResult(mode, candidate)
		})
	}
}

func parseReductionResult(mode ReductionMode, candidate string) (string, string, bool) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(candidate), &raw); err != nil {
		return "", "", false
	}
	switch mode {
	case ReductionModeCompare:
		if strings.TrimSpace(stringValue(raw["summary"])) == "" || strings.TrimSpace(stringValue(raw["comparison"])) == "" {
			return "", "", false
		}
		return renderCompareResult(raw), normaliseJSON(raw), true
	case ReductionModeSummarize:
		if strings.TrimSpace(stringValue(raw["summary"])) == "" || strings.TrimSpace(stringValue(raw["synthesis"])) == "" {
			return "", "", false
		}
		return renderSummarizeResult(raw), normaliseJSON(raw), true
	case ReductionModeBestOfN:
		if strings.TrimSpace(stringValue(raw["winner_label"])) == "" || strings.TrimSpace(stringValue(raw["rationale"])) == "" {
			return "", "", false
		}
		return renderBestOfNResult(raw), normaliseJSON(raw), true
	default:
		return "", "", false
	}
}

func renderCompareResult(values map[string]any) string {
	var b strings.Builder
	b.WriteString("# Comparison\n\n")
	b.WriteString(stringValue(values["summary"]))
	b.WriteString("\n\n## Comparison\n\n")
	b.WriteString(stringValue(values["comparison"]))
	if recommendation := strings.TrimSpace(stringValue(values["recommendation"])); recommendation != "" {
		b.WriteString("\n\n## Recommendation\n\n")
		b.WriteString(recommendation)
	}
	return strings.TrimSpace(b.String())
}

func renderSummarizeResult(values map[string]any) string {
	var b strings.Builder
	b.WriteString("# Summary\n\n")
	b.WriteString(stringValue(values["summary"]))
	b.WriteString("\n\n## Synthesis\n\n")
	b.WriteString(stringValue(values["synthesis"]))
	if highlights, ok := values["highlights"].([]any); ok && len(highlights) > 0 {
		b.WriteString("\n\n## Highlights\n")
		for _, highlight := range highlights {
			text := strings.TrimSpace(stringValue(highlight))
			if text == "" {
				continue
			}
			b.WriteString("- ")
			b.WriteString(text)
			b.WriteString("\n")
		}
	}
	return strings.TrimSpace(b.String())
}

func renderBestOfNResult(values map[string]any) string {
	var b strings.Builder
	b.WriteString("# Best Of N\n\n")
	if summary := strings.TrimSpace(stringValue(values["summary"])); summary != "" {
		b.WriteString(summary)
		b.WriteString("\n\n")
	}
	b.WriteString("## Winner\n\n")
	b.WriteString(stringValue(values["winner_label"]))
	b.WriteString("\n\n## Rationale\n\n")
	b.WriteString(stringValue(values["rationale"]))
	if next := strings.TrimSpace(stringValue(values["recommended_next_step"])); next != "" {
		b.WriteString("\n\n## Recommended Next Step\n\n")
		b.WriteString(next)
	}
	return strings.TrimSpace(b.String())
}

func normaliseJSON(values map[string]any) string {
	encoded, err := json.Marshal(values)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return ""
		}
		return string(encoded)
	}
}
