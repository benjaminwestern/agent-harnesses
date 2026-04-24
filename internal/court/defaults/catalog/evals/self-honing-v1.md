---
id: self_honing_v1
title: Court self-honing v1 baseline
presets: review,review_inline,collaborative_review
repetitions: 1
timeoutMs: 600000
promotePreset: collaborative_review
---

## routing_recovery | Routing and recovery semantics
Review the court runtime and confirm juror assignments cannot silently pass after
failure or after a juror completes without a structured finding. Explain what is
correct, what looks partial, and which edge cases should be tested
next.

## trace_scorecard | Trace and promotion gate review
Review whether the current trace and scorecard artifacts are strong enough for
supervised self-honing. Identify what is trustworthy, what is too
narrow, and whether the promotion gate looks too strict or too weak.

## ux_legibility | Operator UX and inspection
Review the operator experience of `court run`, `court trace`, `court status`,
`court verdict`, and artifact inspection. Recommend improvements that keep the
system convention-first, legible, and easy to inspect as the court grows.
