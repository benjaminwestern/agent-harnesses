# Court Subsystem

## Summary

Court lives inside Agentic Control as an internal workflow engine.

- Package: `internal/court`
- Runtime layer: `pkg/controlplane`
- Default runtime implementation: `pkg/controlplane/embedded`
- Product boundary: none
- Transport boundary: none

Court is an internal subsystem for advanced review-oriented workflow semantics,
not a separate product or service.

Court's user-facing CLI model is documented in the root README and the
`agent_control court --help` surface.

## Ownership

### Agentic Control core owns

- runtime and provider normalisation;
- backend, provider, and model definitions and validation;
- session lifecycle and event streaming;
- approval and user-input requests;
- speech/native orchestration;
- generic workflow orchestration primitives;
- concurrent worker fan-out and collection;
- operator shell and UI surfaces.

### Court owns

- workflow catalog resolution;
- presets, juries, roles, and agents;
- clerk, juror, and judge review semantics;
- run and worker lifecycle state;
- artifacts, traces, requests, and verdicts;
- persisted SQLite state.

For the broader orchestration picture, see the root README and
[`docs/orchestration-surface.md`](orchestration-surface.md).

## Package Layout

Court lives in:

```text
internal/court/
  agentic_control.go
  catalog.go
  catalog_describe.go
  config.go
  defaults/
  delegation.go
  doc.go
  engine.go
  errors.go
  lifecycle.go
  model_options.go
  operator.go
  paths.go
  presets.go
  runtime_requests.go
  setup.go
  store.go
  types.go
  worker_prompt.go
  worker_result.go
  worker_runtime.go
  workflow.go
```

The package intentionally does not expose a transport surface.

## Runtime Wiring

Court depends on a narrow runtime interface:

- `internal/court.RuntimeControlPlane`

That interface is implemented directly by Agentic Control's embedded control
plane.

Court also reuses Agentic Control core for runtime target ownership:

- backend definitions;
- upstream provider inference;
- model validation;
- model-option validation.

Court reuses `pkg/controlplane.ModelOptions` directly rather than
carrying its own duplicate model-option type and helper logic.

Those rules live in `pkg/controlplane`, not in Court-specific logic.

Default wiring in `internal/court.NewEngine(...)` is:

- if `EngineOptions.ControlPlane` is set, use it;
- otherwise use `embedded.New()`.

This keeps Court in process and reuses the existing generic runtime machinery.
Court should progressively sit on top of Agentic Control's native orchestration
layer where generic fan-out, aggregation, and scheduling code can be shared.

The reused core pieces include:

- `internal/orchestration.LaunchDetachedCommand(...)` for detached worker
  process launching;
- `internal/orchestration.HandlePendingSessionControls(...)` and
  `internal/orchestration.FlushQueuedRuntimeResponses(...)` for generic
  session-control and queued-response execution;
- `internal/orchestration` ledger vocabulary for run/worker status, events,
  artifacts, runtime request records, and control action/status enums;
- `internal/orchestration` observation wrappers for run traces, monitor
  snapshots, status views, and watch updates/options;
- `internal/orchestration.SQLiteLedgerStore` for persistence of worker controls,
  runtime requests, events, and artifacts;
- `internal/orchestration.ExecuteWorker(...)` for the generic load/run/update/
  reconcile worker lifecycle loop;
- `internal/orchestration.RuntimeIdentity` plus generic worker-running,
  worker-complete, worker-retry-reset, and worker-runtime-identity persistence;
- `internal/orchestration` worker row persistence and worker-attempt history,
  with Court converting only the review-specific role metadata around those
  records;
- `internal/orchestration.IsTerminalRunStatus(...)` and the generic run
  status/stage persistence helpers that back Court run-state transitions;
- `pkg/controlplane.ExtractStructuredJSON(...)` and
  `pkg/controlplane.JSONObjectCandidates(...)` for the generic JSON repair and
  extraction path used by Court worker-result parsing.

## What Was Intentionally Not Moved

These files and concepts were intentionally left behind from the Court repo:

- `api.go`
- `rpc.go`
- `runtime_remote_client.go`
- `cmd/court/serve.go`
- any Court-specific HTTP or JSON-RPC transport layer

Reason:

- Court is not a separate service boundary;
- Agentic Control consumes Court in process;
- the UI should call Go methods directly, not re-wrap Court over another local
  transport.

## Primary Integration Points

The rest of Agentic Control should call Court directly through Go.

### Engine lifecycle

- `court.NewEngine(...)`
- `(*Engine).Close()`

### Catalog and workflow selection

- `(*Engine).CatalogList(...)`
- `(*Engine).CatalogGet(...)`
- `(*Engine).CatalogValidate(...)`
- `(*Engine).ListAvailablePresets(...)`

### Run lifecycle

- `(*Engine).StartRunWithOptions(...)`
- `(*Engine).ListRuns(...)`
- `(*Engine).GetRun(...)`
- `(*Engine).RunStatus(...)`
- `(*Engine).ReconcileRun(...)`
- `(*Engine).CompletedVerdict(...)`

### Worker and trace inspection

- `(*Engine).ListWorkers(...)`
- `(*Engine).TraceRun(...)`
- `(*Engine).ListArtifacts(...)`
- `(*Engine).ListEvents(...)`
- `(*Engine).MonitorSnapshot(...)`

`TraceRun` reuses the control-plane session ledger and attaches tracked
runtime session summaries, including token-usage breakdowns by model and mode,
when those details are available.

### Operator controls

- `(*Engine).ListRuntimeRequests(...)`
- `(*Engine).RespondToRuntimeRequest(...)`
- `(*Engine).CancelWorker(...)`
- `(*Engine).InterruptWorker(...)`
- `(*Engine).ResumeWorker(...)`
- `(*Engine).RetryWorker(...)`

## Storage and Catalog Paths

Court keeps its own storage and catalog conventions.

- project catalog root: `.court/`
- global catalog root: `~/.config/court`
- SQLite path: `~/.local/share/court/court.db`

This is deliberate. Folding the product into Agentic Control does not require an
immediate rename of persisted paths.

## Integration Work

### UI and shell

- add a Court workflow picker using `CatalogList` and `CatalogGet`;
- add workflow validation and diagnostics before starting runs;
- add a Court run view using `RunStatus`, `TraceRun`, and `MonitorSnapshot`;
- surface pending runtime requests and worker controls in the same shell.

### CLI

- expand the new `agent_control court ...` surface with more operator flows;
- optionally add a thin compatibility `cmd/court` if headless use
  matters.

The baseline flow includes:

- `agent_control court init`
- `agent_control court presets`
- `agent_control court list-runs`
- `agent_control court run`
- `agent_control court status`
- `agent_control court monitor`
- `agent_control court trace`
- `agent_control court verdict`
- `agent_control court requests`
- `agent_control court respond`

### Data and migration

- decide whether old standalone Court data should be imported or reused in place;
- keep schema compatibility until that migration is explicit.

## Non-Goals

- do not make Court call Agentic Control over a local daemon protocol;
- do not reintroduce `court serve` inside this repo as the default integration;
- do not duplicate runtime contracts under `internal/court`;
- do not make the UI parse `.court` files directly.
