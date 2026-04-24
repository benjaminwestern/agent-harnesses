---
id: security_threat_model_review
name: Sweeping security threat model
routing: clerk
jury: security_threat_model_jury
clerk: security_threat_model_clerk
finalJudge: security_threat_model_judge
verdictMode: merge
assignmentStrategy: heuristic_auto
maxAssignmentRetries: 1
retryDelayMs: 3000
retryBackoff: 2
correctionsEnabled: false
nestedDelegationEnabled: true
delegationAllowedRoleKinds: clerk
delegationMaxDepth: 1
delegationMaxChildrenPerParent: 3
delegationMaxTotalChildren: 12
helperRecipes:
  - id: clerk_boundaries
    title: Trust-boundary map
    sessionName: clerk-boundaries
    when: Use when assets, trust boundaries, or privileged transitions need a first pass.
    autoPrune: on_docket
    roleKinds: clerk
    prompt: |
      Map the trust boundaries, assets, and privileged transitions relevant to this case.
      Focus on the boundary model the clerk needs before assigning deeper threat analysis.
      Keep the output concise and evidence-heavy.
  - id: clerk_abuse_paths
    title: Abuse-path sketch
    sessionName: clerk-abuse-paths
    when: Use when a few concrete abuse paths would improve docket quality before full analysis.
    autoPrune: on_docket
    roleKinds: clerk
    prompt: |
      Enumerate a small set of concrete abuse paths relevant to this case.
      Focus on the paths most likely to influence how the clerk splits the threat model.
      Return concise evidence, not a full mitigation plan.
  - id: clerk_ops_surface
    title: Operational surface
    sessionName: clerk-ops-surface
    when: Use when local, build, or non-network operational surfaces should influence the lane split.
    autoPrune: on_docket
    roleKinds: clerk
    prompt: |
      Isolate the operational surfaces in scope, including local workflows, build steps, CI, release, and other non-network paths when relevant.
      Highlight the surfaces that deserve dedicated threat-model attention.
      Keep the output routing-oriented and concrete.
---

Sweeping threat-model court based on the security-threat-model skill. Covers
runtime, non-network, and operational trust boundaries in addition to
internet-facing surfaces.
