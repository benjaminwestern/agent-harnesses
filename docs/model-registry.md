# Model Registry

## Summary

Agentic Control owns a unified backend/provider/model registry in core.

This registry merges:

- runtime/backend descriptors;
- dynamic probed model inventory;
- built-in model catalog defaults where available;
- backend-specific alias and default rules.

Court reuses this registry for target validation instead of validating directly
 against raw runtime probe data.

## Why

The product needs one canonical place for:

- backend names;
- provider names;
- available models per backend;
- default model selection;
- alias resolution;
- UI-ready model/provider grouping.

Without this, each surface drifts:

- Court catalog validation;
- generic orchestration;
- CLI model listing;
- UI model selectors.

## Core Surface

### Shared types

- `contract.ModelRegistry`
- `contract.RuntimeBackendRegistry`
- `contract.RuntimeProviderRegistry`
- `contract.ModelAlias`

### Shared builder and validation

- `controlplane.BuildModelRegistry(...)`
- `controlplane.ValidateSessionTargetWithRegistry(...)`

### Daemon surface

- RPC method: `models.list`

### CLI surface

- `agent_control models`

Examples:

```bash
agent_control models --socket-path /tmp/agentic-control.sock
agent_control models --socket-path /tmp/agentic-control.sock --runtime opencode
agent_control models --socket-path /tmp/agentic-control.sock --runtime opencode --provider google
```

## Session Surface

To mirror the thread/session shape seen in richer client apps, Agentic Control
exposes first-class tracked session commands:

- `agent_control sessions list`
- `agent_control sessions get`
- `agent_control sessions resume`

And a durable thread surface:

- `agent_control threads list`
- `agent_control threads get`
- `agent_control threads events`
- `agent_control threads archive`
- `agent_control threads unarchive`

This lets operators:

- inspect preserved fan-out sessions;
- identify the winning session;
- continue chatting from that session as a base.

## Important Limitation

Thread metadata and stored event history are durable:

- thread metadata survives daemon restarts in SQLite;
- event history survives daemon restarts in SQLite;
- provider session IDs are retained for audit and continuation workflows.

The main remaining limitation is provider-side adoption semantics. Some runtimes
may refuse to resume a provider session after restart if the upstream runtime
cannot verify that session for adoption. In those cases the product should show
the provider session ID clearly so the operator can continue directly in the
provider's own surface.

## Thread Naming, Metadata, Forking, and Rollback

The durable thread surface supports:

- `agent_control threads name`
- `agent_control threads metadata`
- `agent_control threads fork`
- `agent_control threads archive`
- `agent_control threads unarchive`
- `agent_control threads events`

Fork behavior is intentionally explicit:

- forks are logical child threads;
- they preserve parent linkage and provider session IDs for auditability;
- they do not yet imply provider-native history branching.

Rollback is a scaffolded surface that returns a clear unsupported
error until provider adapters gain provider-native rollback semantics.
