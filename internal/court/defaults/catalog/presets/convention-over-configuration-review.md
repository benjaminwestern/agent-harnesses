---
id: convention_over_configuration_review
aliases: coc-review,grug-review
name: Convention over configuration review
routing: clerk
jury: convention_over_configuration_jury
clerk: convention_over_configuration_clerk
finalJudge: convention_over_configuration_judge
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
  - id: clerk_conventions
    title: Convention inventory
    sessionName: clerk-conventions
    when: Use when the dominant conventions and their breakpoints are not yet clear.
    autoPrune: on_docket
    roleKinds: clerk
    prompt: |
      Inventory the dominant conventions in scope and where the codebase breaks them.
      Focus on conventions that matter for maintainability, onboarding, and consistency.
      Return routing evidence the clerk can turn into clean juror assignments.
  - id: clerk_escapehatches
    title: Escape-hatch scan
    sessionName: clerk-escapehatches
    when: Use when bespoke exceptions or local escape hatches need a dedicated lane.
    autoPrune: on_docket
    roleKinds: clerk
    prompt: |
      Identify bespoke escape hatches, local exceptions, or hand-rolled deviations from the dominant path.
      Explain which ones deserve dedicated review attention.
      Keep the output concise and assignment-oriented.
  - id: clerk_stack_shape
    title: Stack-shape map
    sessionName: clerk-stack-shape
    when: Use when the clerk needs a fast map of framework-integrated versus hand-rolled seams.
    autoPrune: on_docket
    roleKinds: clerk
    prompt: |
      Map where the stack follows framework conventions versus where it diverges into hand-rolled seams.
      Highlight the boundaries that should influence assignment scope.
      Return only the facts the clerk needs for docket shaping.
---

Clerk-routed court for convention-over-configuration and omakase /
Rails-doctrine-inspired reviews across the whole repository.
