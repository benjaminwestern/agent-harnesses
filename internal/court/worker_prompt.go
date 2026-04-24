// Package court provides Court runtime functionality.
package court

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

func (e *Engine) buildWorkerPrompt(run Run, worker Worker, role Role) (string, error) {
	var b strings.Builder
	b.WriteString("Runtime identity:\n")
	b.WriteString("- court_run_id: ")
	b.WriteString(run.ID)
	b.WriteString("\n- court_worker_id: ")
	b.WriteString(worker.ID)
	b.WriteString("\n- court_launch_id: ")
	b.WriteString(worker.LaunchID)
	b.WriteString("\n\n")
	b.WriteString("Task:\n")
	b.WriteString(run.Task)
	b.WriteString("\n\n")
	if err := e.appendRolePromptContext(&b, run, worker); err != nil {
		return "", err
	}
	b.WriteString("Role:\n")
	b.WriteString(worker.RoleTitle)
	b.WriteString("\n\n")
	b.WriteString("Brief:\n")
	b.WriteString(role.Brief)
	b.WriteString("\n\n")
	if worker.RoleKind == RoleJudge {
		workers, err := e.store.ListWorkers(context.Background(), run.ID)
		if err != nil {
			return "", err
		}
		b.WriteString("Evidence from completed workers as JSON. Use this as the source of truth:\n")
		b.WriteString(judgeEvidenceJSON(workers))
		b.WriteString("\n\n")
	}
	b.WriteString("Final output contract:\n")
	b.WriteString("- End your turn with exactly one Court WorkerResult JSON object and no markdown fence or surrounding prose.\n")
	b.WriteString("- Use tools only for investigation. Do not finish with tool calls, progress notes, or a prose summary.\n")
	b.WriteString("- Do not call or wait for court_submit_finding, court_submit_verdict, court_submit_docket, or any other court_submit_* tool. The Go Court runtime persists your final JSON response directly.\n")
	b.WriteString("- Replace every placeholder with concrete values from your work. Placeholder JSON is rejected.\n")
	b.WriteString("- Do not add extra fields.\n")
	b.WriteString("Required JSON shape:\n")
	b.WriteString(WorkerResultSchemaExample())
	b.WriteString("\n\n")
	b.WriteString("Example value style, shown without JSON braces so it cannot be copied as the result:\n")
	b.WriteString(`schema_version=1, summary="Runtime execution stays behind agentic-control.", findings=["Court starts and observes workers through the control plane."], risks=["Backend-specific model metadata needs regression coverage."], next_actions=["Add tests for backend-scoped model resolution."], verdict="revise_then_decide", confidence="medium"`)
	b.WriteString("\n\n")
	b.WriteString("Confidence must be one of: ")
	b.WriteString(strings.Join(WorkerResultConfidenceValues(), ", "))
	b.WriteString(".\n\n")
	b.WriteString("Be direct. Prefer concrete findings over general commentary. Do not wait for more input.\n")
	return b.String(), nil
}

func (e *Engine) appendRolePromptContext(b *strings.Builder, run Run, worker Worker) error {
	switch worker.RoleKind {
	case RoleClerk:
		return e.appendClerkPromptContext(b, run)
	case RoleJuror:
		e.appendJurorPromptContext(b, run, worker)
	case RoleJudge:
	}
	return nil
}

func (e *Engine) appendClerkPromptContext(b *strings.Builder, run Run) error {
	b.WriteString("Clerk duty:\n")
	b.WriteString("Prepare the docket for this run. Identify the work slices, routing concerns, and evidence the jurors should produce. Your structured result will be persisted as clerk notes and used to release the juror phase.\n\n")
	if !delegationScopeEnabled(run.DelegationScope) {
		return nil
	}
	catalog, err := e.delegationCatalogForRun(run)
	if err != nil {
		return err
	}
	manifest, _ := json.MarshalIndent(catalog.manifest, "", "  ")
	b.WriteString("Delegation scope:\n")
	b.WriteString(string(run.DelegationScope))
	b.WriteString("\n\n")
	b.WriteString("Available delegation catalog as JSON. Select the best jury, juror, and judge combination from these IDs only:\n")
	b.Write(manifest)
	b.WriteString("\n\n")
	b.WriteString("Delegation decision requirement:\n")
	b.WriteString("- Include exactly one `next_actions` string that starts with `court_delegation:` followed immediately by minified JSON.\n")
	b.WriteString("- The delegation JSON schema is: ")
	b.WriteString(`{"schema_version":1,"scope":"workspace","jury_ids":["jury_id"],"juror_ids":["juror_id"],"judge_ids":["judge_id"],"assignments":[{"target_id":"juror_id","instructions":"specific assignment","expected_deliverable":"expected evidence","rationale":"why this juror"}],"rationale":"why this combination fits the task"}`)
	b.WriteString("\n")
	b.WriteString("- Use `jury_ids` when a named jury fits; use `juror_ids` for individual additions or replacements; use `judge_ids` for the best final reviewer(s).\n")
	b.WriteString("- Every selected ID must exist in the available delegation catalog.\n\n")
	return nil
}

func (e *Engine) appendJurorPromptContext(b *strings.Builder, run Run, worker Worker) {
	assignment, ok := e.assignmentForWorker(context.Background(), run.ID, worker.RoleID)
	if !ok {
		return
	}
	b.WriteString("Docket assignment:\n")
	b.WriteString(assignment.Instructions)
	b.WriteString("\n\n")
	if len(assignment.PlanElementIDs) > 0 {
		b.WriteString("Plan elements: ")
		b.WriteString(strings.Join(assignment.PlanElementIDs, ", "))
		b.WriteString("\n")
	}
	if assignment.ExpectedDeliverable != "" {
		b.WriteString("Expected deliverable: ")
		b.WriteString(assignment.ExpectedDeliverable)
		b.WriteString("\n")
	}
	if assignment.Rationale != "" {
		b.WriteString("Routing rationale: ")
		b.WriteString(assignment.Rationale)
		b.WriteString("\n")
	}
	b.WriteString("\n")
}

func courtWorkerSystemPrompt() string {
	var b strings.Builder
	b.WriteString("You are a Court background worker running under the Go Court orchestrator.\n")
	b.WriteString("Court is the orchestrator. You are one assigned worker. Work independently and return structured evidence.\n")
	b.WriteString("The runtime may provide normal investigation tools, but Go Court does not expose court_submit_* tools. Never call or wait for court_submit_finding, court_submit_verdict, court_submit_docket, or similar tools.\n")
	b.WriteString("Your final assistant message must be exactly one valid Court WorkerResult JSON object. Do not wrap it in markdown. Do not include prose before or after it. Do not return placeholders.\n")
	b.WriteString("Court validates these exact fields only: ")
	b.WriteString(strings.Join(WorkerResultRequiredFields(), ", "))
	b.WriteString(". Confidence must be one of: ")
	b.WriteString(strings.Join(WorkerResultConfidenceValues(), ", "))
	b.WriteString(".")
	return b.String()
}

func (e *Engine) roleForWorker(run Run, roleID string) (Role, error) {
	preset, err := e.resolvePresetForWorkspace(run.Preset, run.Workspace)
	if err != nil {
		return Role{}, err
	}
	for _, role := range preset.Roles {
		if role.ID == roleID {
			return role, nil
		}
	}
	roots, err := e.delegationRoots(run)
	if err == nil {
		if role, ok, err := LoadRoleFromRoots(roots, roleID); err != nil {
			return Role{}, err
		} else if ok {
			return role, nil
		}
	}
	return Role{}, fmt.Errorf("role %q not found in preset %q", roleID, run.Preset)
}

func (e *Engine) assignmentForWorker(ctx context.Context, runID string, roleID string) (DocketAssignment, bool) {
	artifacts, err := e.store.ListArtifacts(ctx, runID)
	if err != nil {
		return DocketAssignment{}, false
	}
	for i := len(artifacts) - 1; i >= 0; i-- {
		artifact := artifacts[i]
		if artifact.Kind != "docket" || artifact.Format != "json" {
			continue
		}
		var docket DocketArtifact
		if err := json.Unmarshal([]byte(artifact.Content), &docket); err != nil {
			continue
		}
		for _, assignment := range docket.Assignments {
			if assignment.ID == roleID || contains(assignment.TargetIDs, roleID) {
				return assignment, true
			}
		}
	}
	return DocketAssignment{}, false
}
