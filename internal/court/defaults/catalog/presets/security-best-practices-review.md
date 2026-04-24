---
id: security_best_practices_review
aliases: security-review
name: Sweeping security best-practices review
routing: clerk
jury: security_best_practices_jury
clerk: security_best_practices_clerk
finalJudge: security_best_practices_judge
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
  - id: clerk_surface_map
    title: Surface map
    sessionName: clerk-surface-map
    when: Use when auth, secrets, storage, CI, release, or local execution surfaces need a first pass.
    autoPrune: on_docket
    roleKinds: clerk
    prompt: |
      Inventory the relevant security surfaces for this case.
      Cover identity, secrets, storage, CI, release, local execution, and runtime posture when they are in scope.
      Return only the map the clerk needs to route deeper security review lanes.
  - id: clerk_runtime_posture
    title: Runtime posture
    sessionName: clerk-runtime-posture
    when: Use when configuration, environment, and deployment posture should shape the docket.
    autoPrune: on_docket
    roleKinds: clerk
    prompt: |
      Pull together the runtime posture relevant to this case.
      Focus on configuration, environment assumptions, deployment shape, and trust-sensitive execution paths.
      Keep the output concise and routing-oriented.
  - id: clerk_identity_scope
    title: Identity slice
    sessionName: clerk-identity-scope
    when: Use when identity or access control likely deserves a dedicated juror lane.
    autoPrune: on_docket
    roleKinds: clerk
    prompt: |
      Isolate the identity and access-control slice relevant to this case.
      Highlight the assets, boundaries, and controls that should drive a dedicated review lane.
      Return evidence the clerk can use to assign work, not a final security verdict.
---

Sweeping security review court based on the security-best-practices skill.
Covers network, browser, local execution, files, workers, build, CI, release,
identity, secrets, and runtime posture rather than only HTTP/TCP surfaces.
