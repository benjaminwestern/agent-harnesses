---
id: parallel
aliases: quick-review
name: Parallel jury
routing: broadcast
jury: standard_review
verdictMode: best_answer
assignmentStrategy: heuristic_auto
maxAssignmentRetries: 1
retryDelayMs: 3000
retryBackoff: 2
requireFinalJudgeForPromotion: true
minAverageFindingConfidence: 0.7
minVerdictConfidence: 0.75
maxCriticalSuggestions: 0
correctionsEnabled: false
nestedDelegationEnabled: false
delegationAllowedRoleKinds: clerk
delegationMaxDepth: 0
delegationMaxChildrenPerParent: 0
delegationMaxTotalChildren: 0
---

Run the jury in parallel with no clerk and no final judge.
