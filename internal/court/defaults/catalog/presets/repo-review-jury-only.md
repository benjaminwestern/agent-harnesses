---
id: repo_review_jury_only
name: Repository understanding jury only
routing: clerk
jury: repo_review_jury
clerk: repo_review_clerk
verdictMode: merge
assignmentStrategy: heuristic_auto
maxAssignmentRetries: 1
retryDelayMs: 3000
retryBackoff: 2
correctionsEnabled: false
requireFinalJudgeForPromotion: false
nestedDelegationEnabled: true
delegationAllowedRoleKinds: clerk
delegationMaxDepth: 1
delegationMaxChildrenPerParent: 3
delegationMaxTotalChildren: 12
helperRecipes:
  - id: clerk_map
    title: Repository map
    sessionName: clerk-map
    when: Use before routing work when module boundaries, entrypoints, or ownership are unclear.
    autoPrune: on_docket
    roleKinds: clerk
    prompt: |
      Map the repository slice relevant to this case.
      Identify modules, boundaries, entrypoints, and ownership seams the clerk should respect.
      Return only the facts needed to split follow-up review lanes cleanly.
  - id: clerk_flow
    title: Flow trace
    sessionName: clerk-flow
    when: Use when one runtime or feature flow should shape the docket.
    autoPrune: on_docket
    roleKinds: clerk
    prompt: |
      Trace one runtime or feature flow that the docket should decompose across specialists.
      Highlight the critical files, handoffs, and risk points in that flow.
      Keep the output focused on routing evidence rather than a full review.
  - id: clerk_compare
    title: Comparison context
    sessionName: clerk-compare
    when: Use when the case names a comparison target and the clerk needs only that context.
    autoPrune: on_docket
    roleKinds: clerk
    prompt: |
      Extract only the comparison context that matters for this case.
      Focus on relevant differences in structure, workflow, or architecture.
      Return concise evidence the clerk can use when assigning juror lanes.
---

The same repo-understanding court without a final judge. Use this when you want
raw jury findings and the orchestrator view rather than a synthesized verdict.
