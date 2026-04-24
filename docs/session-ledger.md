# Session Ledger

## Summary

Agentic Control keeps a reusable downstream session ledger in the
control-plane layer.

This ledger tracks:

- downstream session identity;
- runtime and provider session IDs;
- session status, model, and mode;
- accumulated token usage;
- token usage breakdown by model;
- token usage breakdown by mode;
- completed and errored sessions after they disappear from live provider lists.

This is useful both for:

- the generic Agentic Control product surface;
- Court, which attaches tracked runtime session summaries to worker
  traces.

## Why

Provider `ListSessions` results only show the current live runtime view.

That is not enough for:

- session economics;
- historical auditing;
- completed-session inspection;
- reuse by higher-level workflow modules such as Court.

## Data Model

The control-plane contract includes:

- `contract.TokenUsage`
- `contract.TokenUsageBreakdown`
- `contract.TrackedSession`

`TrackedSession` contains the latest session snapshot plus usage breakdowns by:

- model;
- mode.

## How Usage Is Counted

The ledger treats token-usage events as cumulative usage snapshots for a
session.

When a new usage snapshot arrives:

1. the latest session total is updated;
2. the delta since the previous snapshot is calculated;
3. that delta is attributed to the session's current model and mode.

This allows economics views such as:

- session X used N tokens total;
- model A consumed Y tokens within the session;
- mode `review` consumed P tokens and mode `judge` consumed Q tokens.

## Provider Requirements

Providers should emit normalised token usage in event payloads where possible.

Preferred payload shape:

```json
{
  "usage": {
    "input_tokens": 1200,
    "output_tokens": 300,
    "reasoning_tokens": 400,
    "cached_tokens": 0,
    "total_tokens": 1900
  }
}
```

If a provider only emits raw usage/cost metrics, that provider needs
uplift work before token economics become fully comparable across runtimes.

## Control-Plane Surface

The service supports:

- `session.get`
- `session.history`

And the embedded in-process control-plane exposes:

- `GetTrackedSession(...)`
- `ListTrackedSessions(...)`

## Court Reuse

Court reuses the session ledger through its runtime control-plane boundary.

`internal/court.TraceRun(...)` attaches a tracked runtime session summary to each
worker trace when one is available.

This avoids re-implementing session history or token accounting inside Court.

## Recommended Next Work

1. Normalize token-usage events in more providers.
2. Surface tracked session economics in the Agentic Control UI.
3. Add stage- and run-level economics rollups in Court views.
4. Consider persisting the ledger if long-lived economics history becomes a
   product requirement.
