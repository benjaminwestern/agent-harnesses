// Package court provides Court runtime functionality.
package court

import (
	"strings"
	"time"
)

// PlanArtifact defines Court runtime data.
type PlanArtifact struct {
	Headline       string        `json:"headline"`
	Summary        string        `json:"summary"`
	OperatorPrompt string        `json:"operator_prompt"`
	CreatedAt      time.Time     `json:"created_at"`
	Elements       []PlanElement `json:"elements"`
}

// PlanElement defines Court runtime data.
type PlanElement struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Detail   string   `json:"detail"`
	Keywords []string `json:"keywords,omitempty"`
}

// DocketSource defines Court runtime data.
type DocketSource string

const (
	// DocketClerkHeuristic defines a Court runtime value.
	DocketClerkHeuristic DocketSource = "clerk_heuristic"
	// DocketClerkDelegated defines a Court runtime value.
	DocketClerkDelegated DocketSource = "clerk_delegated"
	// DocketDeterministicLocal defines a Court runtime value.
	DocketDeterministicLocal DocketSource = "deterministic_fallback"
)

// DocketTargetKind defines Court runtime data.
type DocketTargetKind string

const (
	// DocketTargetJuror defines a Court runtime value.
	DocketTargetJuror DocketTargetKind = "juror"
	// DocketTargetJury defines a Court runtime value.
	DocketTargetJury DocketTargetKind = "jury"
	// DocketTargetJudge defines a Court runtime value.
	DocketTargetJudge DocketTargetKind = "judge"
	// DocketTargetCourt defines a Court runtime value.
	DocketTargetCourt DocketTargetKind = "court"
)

// DocketArtifact defines Court runtime data.
type DocketArtifact struct {
	WorkflowMode    WorkflowMode       `json:"workflow_mode"`
	DelegationScope DelegationScope    `json:"delegation_scope,omitempty"`
	Source          DocketSource       `json:"source"`
	Summary         string             `json:"summary"`
	ClerkNotes      string             `json:"clerk_notes,omitempty"`
	JuryIDs         []string           `json:"jury_ids,omitempty"`
	JudgeIDs        []string           `json:"judge_ids,omitempty"`
	Assignments     []DocketAssignment `json:"assignments"`
}

// DocketAssignment defines Court runtime data.
type DocketAssignment struct {
	ID                  string           `json:"id"`
	Title               string           `json:"title"`
	Instructions        string           `json:"instructions"`
	TargetKind          DocketTargetKind `json:"target_kind"`
	TargetIDs           []string         `json:"target_ids"`
	PlanElementIDs      []string         `json:"plan_element_ids"`
	ExpectedDeliverable string           `json:"expected_deliverable,omitempty"`
	Rationale           string           `json:"rationale,omitempty"`
}

// PhaseInput defines Court runtime data.
type PhaseInput struct {
	RequireClerk           bool
	InlineReviewEnabled    bool
	VerdictEnabled         bool
	VerdictDisabled        bool
	DocketReady            bool
	InlineReviewReady      bool
	VerdictReady           bool
	CorrectionTargetCount  int
	CorrectionAppliedCount int
}

// ParticipantState defines Court runtime data.
type ParticipantState struct {
	Role               RoleKind
	Present            bool
	Required           bool
	PromptReady        bool
	AttentionRequested bool
	BlockedOnUser      bool
	CommandRunning     bool
	Unhealthy          bool
}

// PhaseResult defines Court runtime data.
type PhaseResult struct {
	Phase                Phase `json:"phase"`
	Blocked              bool  `json:"blocked"`
	BlockedRequiredCount int   `json:"blocked_required_count"`
	JurorTotalCount      int   `json:"juror_total_count"`
	JurorReadyCount      int   `json:"juror_ready_count"`
	JurorRunningCount    int   `json:"juror_running_count"`
	ClerkPresent         bool  `json:"clerk_present"`
	JudgePresent         bool  `json:"judge_present"`
	CanInlineReview      bool  `json:"can_inline_review"`
	CanFinalizeVerdict   bool  `json:"can_finalize_verdict"`
}

// ResolveWorkflowMode provides Court runtime functionality.
func ResolveWorkflowMode(value string, fallback WorkflowMode) WorkflowMode {
	switch WorkflowMode(value) {
	case WorkflowParallelConsensus, WorkflowRouted, WorkflowRoleScoped, WorkflowBoundedCorrection, WorkflowReviewOnly:
		return WorkflowMode(value)
	}
	if fallback != "" {
		return fallback
	}
	return WorkflowParallelConsensus
}

// ParseWorkflowMode provides Court runtime functionality.
func ParseWorkflowMode(value string) (WorkflowMode, bool) {
	trimmed := strings.TrimSpace(value)
	switch WorkflowMode(trimmed) {
	case WorkflowParallelConsensus, WorkflowRouted, WorkflowRoleScoped, WorkflowBoundedCorrection, WorkflowReviewOnly:
		return WorkflowMode(trimmed), true
	default:
		return "", false
	}
}

// ResolveDelegationScope provides Court runtime functionality.
func ResolveDelegationScope(value string, fallback DelegationScope) DelegationScope {
	if scope, ok := ParseDelegationScope(value); ok {
		return scope
	}
	if fallback != "" {
		return fallback
	}
	return DelegationScopePreset
}

// ParseDelegationScope provides Court runtime functionality.
func ParseDelegationScope(value string) (DelegationScope, bool) {
	trimmed := strings.TrimSpace(value)
	switch DelegationScope(trimmed) {
	case "", DelegationScopePreset:
		return DelegationScopePreset, true
	case DelegationScopeWorkspace:
		return DelegationScopeWorkspace, true
	case DelegationScopeGlobal:
		return DelegationScopeGlobal, true
	default:
		return "", false
	}
}

// BuildPlan provides Court runtime functionality.
func BuildPlan(task string) PlanArtifact {
	trimmed := strings.TrimSpace(task)
	lines := strings.Split(trimmed, "\n")
	var bodies []string
	for _, line := range lines {
		item := strings.TrimSpace(line)
		if strings.HasPrefix(item, "- ") || strings.HasPrefix(item, "* ") {
			bodies = append(bodies, strings.TrimSpace(item[2:]))
		}
	}
	if len(bodies) == 0 {
		paragraphs := strings.Split(trimmed, "\n\n")
		for _, paragraph := range paragraphs {
			if text := collapseWhitespace(paragraph); text != "" {
				bodies = append(bodies, text)
			}
		}
	}
	if len(bodies) == 0 {
		bodies = []string{trimmed}
	}

	elements := make([]PlanElement, 0, len(bodies))
	for i, body := range bodies {
		title := body
		if idx := strings.Index(title, ":"); idx >= 0 {
			title = title[:idx]
		}
		title = truncate(collapseWhitespace(title), 72)
		if title == "" {
			title = "task"
		}
		elements = append(elements, PlanElement{
			ID:       "plan-" + itoa(i+1),
			Title:    title,
			Detail:   collapseWhitespace(body),
			Keywords: workflowKeywords(body),
		})
	}

	headline := elements[0].Title
	summary := truncate(elements[0].Detail, 180)
	return PlanArtifact{
		Headline:       headline,
		Summary:        summary,
		OperatorPrompt: trimmed,
		CreatedAt:      time.Now(),
		Elements:       elements,
	}
}

// BuildDocket provides Court runtime functionality.
func BuildDocket(plan PlanArtifact, preset Preset, workflow WorkflowMode) DocketArtifact {
	jurors := jurorRoles(preset)
	assignments := make([]DocketAssignment, 0, len(jurors))
	for i, role := range jurors {
		if role.Kind == RoleJudge || role.Kind == RoleClerk {
			continue
		}
		planIDs := allPlanIDs(plan)
		instructions := plan.OperatorPrompt
		rationale := "parallel consensus broadcasts the same prompt to each juror"
		switch workflow {
		case WorkflowRouted:
			element := plan.Elements[i%len(plan.Elements)]
			planIDs = []string{element.ID}
			instructions = element.Detail
			rationale = "deterministic local routing until clerk routing is online"
		case WorkflowRoleScoped:
			instructions = "Review the full task through this role scope:\n\n" + role.Brief + "\n\nTask:\n" + plan.OperatorPrompt
			rationale = "role-scoped workflow keeps the full task visible while specializing each juror by role brief"
		case WorkflowParallelConsensus, WorkflowBoundedCorrection, WorkflowReviewOnly:
		}
		assignments = append(assignments, DocketAssignment{
			ID:                  role.ID,
			Title:               role.Title,
			Instructions:        instructions,
			TargetKind:          DocketTargetJuror,
			TargetIDs:           []string{role.ID},
			PlanElementIDs:      planIDs,
			ExpectedDeliverable: "Structured findings, risks, next actions, and a direct recommendation.",
			Rationale:           rationale,
		})
	}
	return DocketArtifact{
		WorkflowMode:    workflow,
		DelegationScope: DelegationScopePreset,
		Source:          DocketDeterministicLocal,
		Summary:         plan.Summary,
		Assignments:     assignments,
	}
}

// EvaluatePhase provides Court runtime functionality.
func EvaluatePhase(input PhaseInput, participants []ParticipantState) PhaseResult {
	if !input.VerdictEnabled && !input.VerdictDisabled {
		input.VerdictEnabled = true
	}
	result := PhaseResult{Phase: PhaseIdle}
	for _, participant := range participants {
		if !participant.Present {
			continue
		}
		eligible := reviewEligible(participant)
		running := participant.CommandRunning || (participant.Present && !eligible && !participant.BlockedOnUser)
		switch participant.Role {
		case RoleJuror:
			result.JurorTotalCount++
			if eligible {
				result.JurorReadyCount++
			}
			if running {
				result.JurorRunningCount++
			}
		case RoleClerk:
			result.ClerkPresent = true
		case RoleJudge:
			result.JudgePresent = true
		}
		if participant.Required && (participant.BlockedOnUser || participant.AttentionRequested) {
			result.BlockedRequiredCount++
		}
	}

	result.Blocked = result.BlockedRequiredCount > 0
	result.CanInlineReview = result.JurorTotalCount > 0 &&
		result.JurorReadyCount == result.JurorTotalCount &&
		!result.Blocked
	result.CanFinalizeVerdict = input.VerdictEnabled &&
		result.CanInlineReview &&
		(!input.InlineReviewEnabled || input.InlineReviewReady) &&
		input.CorrectionAppliedCount >= input.CorrectionTargetCount

	switch {
	case result.Blocked:
		result.Phase = PhaseBlocked
	case input.VerdictReady:
		result.Phase = PhaseComplete
	case input.RequireClerk && !input.DocketReady:
		result.Phase = PhaseClerk
	case input.InlineReviewEnabled && input.InlineReviewReady && input.CorrectionAppliedCount < input.CorrectionTargetCount:
		result.Phase = PhaseCorrections
	case result.JurorRunningCount > 0:
		if input.InlineReviewEnabled && input.InlineReviewReady {
			result.Phase = PhaseCorrections
		} else {
			result.Phase = PhaseJurors
		}
	case input.InlineReviewEnabled && !input.InlineReviewReady && result.CanInlineReview:
		result.Phase = PhaseInlineJudge
	case input.VerdictEnabled && !input.VerdictReady && result.CanFinalizeVerdict:
		result.Phase = PhaseVerdict
	case (!input.VerdictEnabled && result.CanInlineReview) || (input.VerdictEnabled && result.CanFinalizeVerdict && input.VerdictReady):
		result.Phase = PhaseComplete
	case result.JurorTotalCount > 0:
		result.Phase = PhaseJurors
	default:
		result.Phase = PhaseIdle
	}
	return result
}

// ParticipantStatesFromWorkers provides Court runtime functionality.
func ParticipantStatesFromWorkers(workers []Worker) []ParticipantState {
	states := make([]ParticipantState, 0, len(workers))
	for _, worker := range workers {
		states = append(states, ParticipantState{
			Role:           worker.RoleKind,
			Present:        true,
			Required:       true,
			PromptReady:    worker.Status == WorkerCompleted,
			CommandRunning: worker.Status == WorkerQueued || worker.Status == WorkerRunning,
			Unhealthy:      worker.Status == WorkerFailed,
		})
	}
	return states
}

func reviewEligible(participant ParticipantState) bool {
	return participant.Present &&
		!participant.Unhealthy &&
		!participant.BlockedOnUser &&
		!participant.AttentionRequested &&
		participant.PromptReady
}

func allPlanIDs(plan PlanArtifact) []string {
	out := make([]string, 0, len(plan.Elements))
	for _, element := range plan.Elements {
		out = append(out, element.ID)
	}
	return out
}

func workflowKeywords(text string) []string {
	banned := map[string]struct{}{
		"about": {}, "after": {}, "agent": {}, "along": {}, "build": {}, "check": {},
		"court": {}, "judge": {}, "juror": {}, "should": {}, "their": {}, "there": {},
		"these": {}, "those": {}, "under": {}, "with": {}, "work": {}, "would": {},
	}
	var out []string
	for _, raw := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	}) {
		if len(raw) < 4 {
			continue
		}
		if _, ok := banned[raw]; ok {
			continue
		}
		if !contains(out, raw) {
			out = append(out, raw)
		}
		if len(out) == 6 {
			break
		}
	}
	return out
}

func collapseWhitespace(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return strings.TrimSpace(value[:limit])
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func itoa(value int) string {
	const digits = "0123456789"
	if value == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for value > 0 {
		i--
		buf[i] = digits[value%10]
		value /= 10
	}
	return string(buf[i:])
}
