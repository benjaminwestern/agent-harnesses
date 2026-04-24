---
id: performance_flow_jury_only
aliases: performance-jury
name: Performance, flow, and grug jury only
routing: clerk
jury: performance_flow_jury
clerk: performance_flow_clerk
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
  - id: clerk_hotpaths
    title: Hot path scan
    sessionName: clerk-hotpaths
    when: Use when latency suspects, queueing, or contention need a first pass before routing work.
    autoPrune: on_docket
    roleKinds: clerk
    prompt: |
      Identify likely hot paths, queueing points, and obvious latency suspects relevant to this case.
      Focus on concrete runtime paths and contention surfaces.
      Return only the evidence the clerk needs to split deeper review lanes.
  - id: clerk_dataflow
    title: Data-flow map
    sessionName: clerk-dataflow
    when: Use when one critical state or request flow should drive the assignment graph.
    autoPrune: on_docket
    roleKinds: clerk
    prompt: |
      Map one critical request, state, or data flow relevant to this case.
      Identify boundaries, fan-out points, and places where work should be split.
      Keep the output routing-oriented rather than a full performance review.
  - id: clerk_scope
    title: Simplification seam
    sessionName: clerk-scope
    when: Use when the worst duplication or fragmentation seam is not yet obvious.
    autoPrune: on_docket
    roleKinds: clerk
    prompt: |
      Isolate the most important duplication, fragmentation, or simplification seam in scope.
      Explain why it deserves a dedicated juror lane.
      Return a tight recommendation the clerk can use when shaping the docket.
---

The same performance, flow, and grug-specialist jury without a final judge.
Use this when you want the raw specialist findings rather than a synthesized
verdict.
