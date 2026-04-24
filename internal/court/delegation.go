// Package court provides Court runtime functionality.
package court

import (
	"encoding/json"
	"fmt"
	"strings"

	api "github.com/benjaminwestern/agentic-control/pkg/controlplane"
)

const delegationDecisionPrefix = "court_delegation:"

// DelegationManifest defines Court runtime data.
type DelegationManifest struct {
	Scope  DelegationScope `json:"scope"`
	Juries []JurySummary   `json:"juries"`
	Jurors []RoleSummary   `json:"jurors"`
	Judges []RoleSummary   `json:"judges"`
}

// JurySummary defines Court runtime data.
type JurySummary struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	JurorIDs    []string `json:"juror_ids"`
	JudgeID     string   `json:"judge_id,omitempty"`
}

// RoleSummary defines Court runtime data.
type RoleSummary struct {
	ID       string   `json:"id"`
	Kind     RoleKind `json:"kind"`
	Title    string   `json:"title"`
	Brief    string   `json:"brief,omitempty"`
	Backend  string   `json:"backend,omitempty"`
	Provider string   `json:"provider,omitempty"`
	Model    string   `json:"model,omitempty"`
}

// ClerkDelegationDecision defines Court runtime data.
type ClerkDelegationDecision struct {
	SchemaVersion int                    `json:"schema_version"`
	Scope         DelegationScope        `json:"scope,omitempty"`
	JuryIDs       []string               `json:"jury_ids,omitempty"`
	JurorIDs      []string               `json:"juror_ids,omitempty"`
	JudgeIDs      []string               `json:"judge_ids,omitempty"`
	Assignments   []ClerkDelegationRoute `json:"assignments,omitempty"`
	Rationale     string                 `json:"rationale,omitempty"`
}

// ClerkDelegationRoute defines Court runtime data.
type ClerkDelegationRoute struct {
	TargetID            string   `json:"target_id,omitempty"`
	TargetIDs           []string `json:"target_ids,omitempty"`
	Instructions        string   `json:"instructions"`
	ExpectedDeliverable string   `json:"expected_deliverable,omitempty"`
	Rationale           string   `json:"rationale,omitempty"`
}

type delegationCatalog struct {
	manifest DelegationManifest
	roles    map[string]Role
	juries   map[string]Jury
}

func delegationScopeEnabled(scope DelegationScope) bool {
	return scope == DelegationScopeWorkspace || scope == DelegationScopeGlobal
}

func (e *Engine) delegationRoots(run Run) ([]string, error) {
	if run.DelegationScope == DelegationScopeGlobal {
		return cleanPathList([]string{e.configDir}), nil
	}
	return e.catalogRoots(run.Workspace)
}

func (e *Engine) delegationCatalogForRun(run Run) (delegationCatalog, error) {
	scope := ResolveDelegationScope(string(run.DelegationScope), DelegationScopePreset)
	roots, err := e.delegationRoots(run)
	if err != nil {
		return delegationCatalog{}, err
	}
	roles, err := ListRolesFromRoots(roots)
	if err != nil {
		return delegationCatalog{}, err
	}
	juries, err := ListJuriesFromRoots(roots)
	if err != nil {
		return delegationCatalog{}, err
	}
	catalog := delegationCatalog{
		manifest: DelegationManifest{Scope: scope},
		roles:    map[string]Role{},
		juries:   map[string]Jury{},
	}
	for _, role := range roles {
		catalog.roles[role.ID] = role
		summary := RoleSummary{
			ID:       role.ID,
			Kind:     role.Kind,
			Title:    role.Title,
			Brief:    truncate(collapseWhitespace(role.Brief), 260),
			Backend:  role.Backend,
			Provider: role.Provider,
			Model:    role.Model,
		}
		switch role.Kind {
		case RoleJuror:
			catalog.manifest.Jurors = append(catalog.manifest.Jurors, summary)
		case RoleJudge:
			catalog.manifest.Judges = append(catalog.manifest.Judges, summary)
		case RoleClerk:
			continue
		}
	}
	for _, jury := range juries {
		catalog.juries[jury.ID] = jury
		catalog.manifest.Juries = append(catalog.manifest.Juries, JurySummary{
			ID:          jury.ID,
			Title:       jury.Title,
			Description: jury.Description,
			JurorIDs:    jury.JurorIDs,
			JudgeID:     jury.JudgeID,
		})
	}
	return catalog, nil
}

func clerkDelegationDecision(resultJSON string) (ClerkDelegationDecision, bool, error) {
	var result WorkerResult
	if err := json.Unmarshal([]byte(resultJSON), &result); err != nil {
		return ClerkDelegationDecision{}, false, wrapErr("parse worker delegation result", err)
	}
	values := append(append([]string{}, result.Findings...), result.NextActions...)
	values = append(values, result.Risks...)
	for _, value := range values {
		idx := strings.Index(value, delegationDecisionPrefix)
		if idx < 0 {
			continue
		}
		raw := strings.TrimSpace(value[idx+len(delegationDecisionPrefix):])
		for _, candidate := range api.JSONObjectCandidates(raw) {
			var decision ClerkDelegationDecision
			if err := json.Unmarshal([]byte(candidate), &decision); err != nil {
				continue
			}
			if decision.SchemaVersion != 1 {
				return ClerkDelegationDecision{}, true, fmt.Errorf("delegation decision schema_version must be 1")
			}
			return decision, true, nil
		}
		return ClerkDelegationDecision{}, true, fmt.Errorf("delegation decision after %q is not valid JSON", delegationDecisionPrefix)
	}
	return ClerkDelegationDecision{}, false, nil
}

func buildDelegatedDocket(plan PlanArtifact, run Run, preset Preset, catalog delegationCatalog, decision ClerkDelegationDecision) (DocketArtifact, []Role, error) {
	jurorIDs := make([]string, 0, len(decision.JurorIDs))
	judgeIDs := make([]string, 0, len(decision.JudgeIDs))
	seenJurors := map[string]struct{}{}
	seenJudges := map[string]struct{}{}

	for _, juryID := range cleanStringList(decision.JuryIDs) {
		jury, ok := catalog.juries[juryID]
		if !ok {
			return DocketArtifact{}, nil, fmt.Errorf("delegation references unknown jury %q", juryID)
		}
		for _, jurorID := range jury.JurorIDs {
			addUniqueID(&jurorIDs, seenJurors, jurorID)
		}
		if strings.TrimSpace(jury.JudgeID) != "" && len(decision.JudgeIDs) == 0 {
			addUniqueID(&judgeIDs, seenJudges, jury.JudgeID)
		}
	}
	for _, jurorID := range cleanStringList(decision.JurorIDs) {
		addUniqueID(&jurorIDs, seenJurors, jurorID)
	}
	for _, route := range decision.Assignments {
		for _, targetID := range routeTargetIDs(route) {
			role, ok := catalog.roles[targetID]
			if ok && role.Kind == RoleJuror {
				addUniqueID(&jurorIDs, seenJurors, targetID)
			}
		}
	}
	if len(jurorIDs) == 0 {
		return DocketArtifact{}, nil, fmt.Errorf("delegation selected no jurors")
	}

	jurors := make([]Role, 0, len(jurorIDs))
	for _, jurorID := range jurorIDs {
		role, ok := catalog.roles[jurorID]
		if !ok {
			return DocketArtifact{}, nil, fmt.Errorf("delegation references unknown juror %q", jurorID)
		}
		if role.Kind != RoleJuror {
			return DocketArtifact{}, nil, fmt.Errorf("delegation target %q is %s, not juror", jurorID, role.Kind)
		}
		jurors = append(jurors, role)
	}

	for _, judgeID := range cleanStringList(decision.JudgeIDs) {
		addUniqueID(&judgeIDs, seenJudges, judgeID)
	}
	if len(judgeIDs) == 0 {
		if judge, ok := judgeRole(preset); ok {
			addUniqueID(&judgeIDs, seenJudges, judge.ID)
		}
	}
	for _, judgeID := range judgeIDs {
		role, ok := catalog.roles[judgeID]
		if ok && role.Kind != RoleJudge {
			return DocketArtifact{}, nil, fmt.Errorf("delegation judge %q is %s, not judge", judgeID, role.Kind)
		}
	}

	assignments := make([]DocketAssignment, 0, len(jurors))
	for _, role := range jurors {
		route, hasRoute := routeForRole(decision.Assignments, role.ID)
		instructions := plan.OperatorPrompt
		expected := "Structured findings, risks, next actions, and a direct recommendation."
		rationale := firstNonEmpty(decision.Rationale, "selected by clerk delegation")
		if hasRoute {
			instructions = firstNonEmpty(route.Instructions, instructions)
			expected = firstNonEmpty(route.ExpectedDeliverable, expected)
			rationale = firstNonEmpty(route.Rationale, rationale)
		}
		assignments = append(assignments, DocketAssignment{
			ID:                  role.ID,
			Title:               role.Title,
			Instructions:        instructions,
			TargetKind:          DocketTargetJuror,
			TargetIDs:           []string{role.ID},
			PlanElementIDs:      allPlanIDs(plan),
			ExpectedDeliverable: expected,
			Rationale:           rationale,
		})
	}
	return DocketArtifact{
		WorkflowMode:    run.Workflow,
		DelegationScope: run.DelegationScope,
		Source:          DocketClerkDelegated,
		Summary:         firstNonEmpty(decision.Rationale, plan.Summary),
		JuryIDs:         cleanStringList(decision.JuryIDs),
		JudgeIDs:        judgeIDs,
		Assignments:     assignments,
	}, jurors, nil
}

func cleanStringList(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func addUniqueID(out *[]string, seen map[string]struct{}, id string) {
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	if _, ok := seen[id]; ok {
		return
	}
	seen[id] = struct{}{}
	*out = append(*out, id)
}

func routeTargetIDs(route ClerkDelegationRoute) []string {
	ids := cleanStringList(route.TargetIDs)
	if strings.TrimSpace(route.TargetID) != "" {
		ids = append([]string{strings.TrimSpace(route.TargetID)}, ids...)
	}
	return cleanStringList(ids)
}

func routeForRole(routes []ClerkDelegationRoute, roleID string) (ClerkDelegationRoute, bool) {
	for _, route := range routes {
		for _, targetID := range routeTargetIDs(route) {
			if targetID == roleID {
				return route, true
			}
		}
	}
	return ClerkDelegationRoute{}, false
}
