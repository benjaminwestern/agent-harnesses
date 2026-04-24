package court

import (
	"context"
	"slices"

	"github.com/benjaminwestern/agentic-control/pkg/contract"
)

type WorkerUsage struct {
	WorkerID          string                         `json:"worker_id"`
	RoleID            string                         `json:"role_id"`
	RoleTitle         string                         `json:"role_title"`
	Backend           string                         `json:"backend"`
	Model             string                         `json:"model,omitempty"`
	RuntimeSessionID  string                         `json:"runtime_session_id,omitempty"`
	ProviderSessionID string                         `json:"provider_session_id,omitempty"`
	Usage             contract.TokenUsage            `json:"usage,omitempty"`
	CostUSD           float64                        `json:"cost_usd,omitempty"`
	UsageByModel      []contract.TokenUsageBreakdown `json:"usage_by_model,omitempty"`
	UsageByMode       []contract.TokenUsageBreakdown `json:"usage_by_mode,omitempty"`
	CostByModel       []contract.CostBreakdown       `json:"cost_by_model,omitempty"`
	CostByMode        []contract.CostBreakdown       `json:"cost_by_mode,omitempty"`
}

type UsageSummary struct {
	TotalUsage   contract.TokenUsage            `json:"total_usage,omitempty"`
	TotalCostUSD float64                        `json:"total_cost_usd,omitempty"`
	ByModel      []contract.TokenUsageBreakdown `json:"by_model,omitempty"`
	ByMode       []contract.TokenUsageBreakdown `json:"by_mode,omitempty"`
	CostByModel  []contract.CostBreakdown       `json:"cost_by_model,omitempty"`
	CostByMode   []contract.CostBreakdown       `json:"cost_by_mode,omitempty"`
	ByWorker     []WorkerUsage                  `json:"by_worker,omitempty"`
}

func (e *Engine) UsageSummary(ctx context.Context, runID string) (UsageSummary, error) {
	trace, err := e.TraceRun(ctx, runID)
	if err != nil {
		return UsageSummary{}, err
	}
	return UsageSummaryFromTrace(trace), nil
}

func UsageSummaryFromTrace(trace RunTrace) UsageSummary {
	byModel := make(map[string]contract.TokenUsage)
	byMode := make(map[string]contract.TokenUsage)
	costByModel := make(map[string]float64)
	costByMode := make(map[string]float64)
	byWorker := make([]WorkerUsage, 0, len(trace.Workers))
	var total contract.TokenUsage
	var totalCost float64
	for _, worker := range trace.Workers {
		if worker.RuntimeSession == nil {
			continue
		}
		tracked := worker.RuntimeSession
		usage := tracked.Session.Usage
		cost := tracked.Session.CostUSD
		total = addTraceUsage(total, usage)
		totalCost += cost
		workerUsage := WorkerUsage{
			WorkerID:          worker.Worker.ID,
			RoleID:            worker.Worker.RoleID,
			RoleTitle:         worker.Worker.RoleTitle,
			Backend:           worker.Worker.Backend,
			Model:             worker.Worker.Model,
			RuntimeSessionID:  tracked.Session.SessionID,
			ProviderSessionID: tracked.Session.ProviderSessionID,
			Usage:             usage,
			CostUSD:           cost,
			UsageByModel:      tracked.UsageByModel,
			UsageByMode:       tracked.UsageByMode,
			CostByModel:       tracked.CostByModel,
			CostByMode:        tracked.CostByMode,
		}
		byWorker = append(byWorker, workerUsage)
		for _, breakdown := range tracked.UsageByModel {
			byModel[breakdown.Key] = addTraceUsage(byModel[breakdown.Key], breakdown.Usage)
		}
		for _, breakdown := range tracked.UsageByMode {
			byMode[breakdown.Key] = addTraceUsage(byMode[breakdown.Key], breakdown.Usage)
		}
		for _, breakdown := range tracked.CostByModel {
			costByModel[breakdown.Key] += breakdown.CostUSD
		}
		for _, breakdown := range tracked.CostByMode {
			costByMode[breakdown.Key] += breakdown.CostUSD
		}
	}
	return UsageSummary{
		TotalUsage:   total,
		TotalCostUSD: totalCost,
		ByModel:      tokenBreakdowns(byModel),
		ByMode:       tokenBreakdowns(byMode),
		CostByModel:  costBreakdowns(costByModel),
		CostByMode:   costBreakdowns(costByMode),
		ByWorker:     byWorker,
	}
}

func addTraceUsage(left contract.TokenUsage, right contract.TokenUsage) contract.TokenUsage {
	return contract.TokenUsage{
		InputTokens:     left.InputTokens + right.InputTokens,
		OutputTokens:    left.OutputTokens + right.OutputTokens,
		ReasoningTokens: left.ReasoningTokens + right.ReasoningTokens,
		CachedTokens:    left.CachedTokens + right.CachedTokens,
		TotalTokens:     left.TotalTokens + right.TotalTokens,
	}
}

func tokenBreakdowns(values map[string]contract.TokenUsage) []contract.TokenUsageBreakdown {
	breakdowns := make([]contract.TokenUsageBreakdown, 0, len(values))
	for key, usage := range values {
		if key == "" {
			continue
		}
		breakdowns = append(breakdowns, contract.TokenUsageBreakdown{Key: key, Usage: usage})
	}
	slices.SortFunc(breakdowns, func(left, right contract.TokenUsageBreakdown) int {
		switch {
		case left.Usage.TotalTokens > right.Usage.TotalTokens:
			return -1
		case left.Usage.TotalTokens < right.Usage.TotalTokens:
			return 1
		case left.Key < right.Key:
			return -1
		case left.Key > right.Key:
			return 1
		default:
			return 0
		}
	})
	return breakdowns
}

func costBreakdowns(values map[string]float64) []contract.CostBreakdown {
	breakdowns := make([]contract.CostBreakdown, 0, len(values))
	for key, cost := range values {
		if key == "" {
			continue
		}
		breakdowns = append(breakdowns, contract.CostBreakdown{Key: key, CostUSD: cost})
	}
	slices.SortFunc(breakdowns, func(left, right contract.CostBreakdown) int {
		switch {
		case left.CostUSD > right.CostUSD:
			return -1
		case left.CostUSD < right.CostUSD:
			return 1
		case left.Key < right.Key:
			return -1
		case left.Key > right.Key:
			return 1
		default:
			return 0
		}
	})
	return breakdowns
}
