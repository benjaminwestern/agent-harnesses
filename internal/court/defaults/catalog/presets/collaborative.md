---
id: collaborative
name: Collaborative jury
routing: clerk
jury: standard_review
clerk: clerk
verdictMode: pros_cons
assignmentStrategy: heuristic_auto
maxAssignmentRetries: 1
retryDelayMs: 3000
retryBackoff: 2
requireFinalJudgeForPromotion: true
minAverageFindingConfidence: 0.7
minVerdictConfidence: 0.75
maxCriticalSuggestions: 0
correctionsEnabled: false
nestedDelegationEnabled: true
delegationAllowedRoleKinds: clerk
delegationMaxDepth: 1
delegationMaxChildrenPerParent: 3
delegationMaxTotalChildren: 12
helperRecipes:
  - id: clerk_map
    title: Case map
    sessionName: clerk-map
    when: Use when the case boundary or repo slice is still fuzzy.
    autoPrune: on_docket
    roleKinds: clerk
    prompt: |
      Map the case boundary before docketing work.
      Identify the primary modules, entrypoints, files, and ownership seams that matter.
      Return only the facts the clerk needs to split the work cleanly.
  - id: clerk_evidence
    title: Missing evidence sweep
    sessionName: clerk-evidence
    when: Use when one missing fact set would materially improve the docket.
    autoPrune: on_docket
    roleKinds: clerk
    prompt: |
      Gather the smallest missing fact set that will materially improve docket quality.
      Focus on concrete evidence, not broad review conclusions.
      Return a short list of facts, files, and implications for task routing.
  - id: clerk_scope
    title: Decomposition check
    sessionName: clerk-scope
    when: Use when the best juror split or task shape is unclear.
    autoPrune: on_docket
    roleKinds: clerk
    prompt: |
      Test one alternative decomposition for this case.
      Compare likely task boundaries, overlaps, and handoff points.
      Recommend the cleanest assignment split for the clerk to keep or reject.
---

The clerk decomposes the case and routes work across the jury without a final judge. Missing juror targets are auto-filled heuristically, and failed juror-assignment lanes retry once with backoff.
