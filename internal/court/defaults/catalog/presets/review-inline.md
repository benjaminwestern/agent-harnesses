---
id: review_inline
name: Broadcast review with inline judge
routing: broadcast
jury: standard_review
inlineJudge: inline_judge
finalJudge: final_judge
verdictMode: best_answer
assignmentStrategy: heuristic_auto
maxAssignmentRetries: 1
retryDelayMs: 3000
retryBackoff: 2
requireFinalJudgeForPromotion: true
minAverageFindingConfidence: 0.7
minVerdictConfidence: 0.75
maxCriticalSuggestions: 0
correctionsEnabled: true
nestedDelegationEnabled: false
delegationAllowedRoleKinds: clerk
delegationMaxDepth: 0
delegationMaxChildrenPerParent: 0
delegationMaxTotalChildren: 0
---

Broadcast review plus inline supervision and final-judge evaluation.
