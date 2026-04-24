package main

import (
	"github.com/benjaminwestern/agentic-control/internal/court"
	"github.com/benjaminwestern/agentic-control/pkg/contract"
	api "github.com/benjaminwestern/agentic-control/pkg/controlplane"
)

type archiveThreadResult struct {
	OK       bool   `json:"ok"`
	ThreadID string `json:"thread_id"`
	Archived bool   `json:"archived"`
}

type renameThreadResult struct {
	OK       bool   `json:"ok"`
	ThreadID string `json:"thread_id"`
	Name     string `json:"name"`
}

type threadMetadataResult struct {
	OK       bool                    `json:"ok"`
	ThreadID string                  `json:"thread_id"`
	Metadata contract.ThreadMetadata `json:"metadata"`
}

type modelSelectionNormalizationResult struct {
	Selection contract.ModelSelection      `json:"selection"`
	Target    api.RuntimeTarget            `json:"target"`
	Issues    []api.RuntimeValidationIssue `json:"issues,omitempty"`
}

type smokeTargetResult struct {
	Target  string              `json:"target"`
	Backend string              `json:"backend"`
	Model   string              `json:"model,omitempty"`
	Passed  bool                `json:"passed"`
	Text    string              `json:"text,omitempty"`
	Error   string              `json:"error,omitempty"`
	Usage   contract.TokenUsage `json:"usage,omitempty"`
	CostUSD float64             `json:"cost_usd,omitempty"`
}

type smokeResult struct {
	Passed       bool                `json:"passed"`
	TargetCount  int                 `json:"target_count"`
	Targets      []smokeTargetResult `json:"targets"`
	TotalUsage   contract.TokenUsage `json:"total_usage,omitempty"`
	TotalCostUSD float64             `json:"total_cost_usd,omitempty"`
}

type courtStatusResult struct {
	Status court.RunStatusView `json:"status"`
	Usage  court.UsageSummary  `json:"usage"`
}

type courtMonitorResult struct {
	Snapshot court.MonitorSnapshot `json:"snapshot"`
	Usage    court.UsageSummary    `json:"usage"`
}

type courtTraceResult struct {
	Trace court.RunTrace     `json:"trace"`
	Usage court.UsageSummary `json:"usage"`
}

type sessionResumeResult struct {
	Session contract.RuntimeSession `json:"session"`
	Tracked contract.TrackedSession `json:"tracked"`
	Event   *contract.RuntimeEvent  `json:"event,omitempty"`
}

type courtContinueResult struct {
	Promotion court.PromotedThreadResult `json:"promotion"`
	Resume    *sessionResumeResult       `json:"resume,omitempty"`
}
