# Event contract

Agentic Control emits one normalised JSON object per native runtime event. The
contract is designed to be stable across runtimes while preserving the native
identifiers that matter for correlation and resume workflows.

The prose contract on this page is the source of truth for the event shape. The
Go implementation mirrors it in [`pkg/contract/`](../pkg/contract). The
harness event schema version is
`agentic-control.harness.v1`.

## Core fields

Every event includes the following top-level fields:

- `schema_version`
- `recorded_at_ms`
- `runtime`
- `provenance`
- `native_event_name`
- `event_type`
- `summary`

Depending on the runtime and the specific event, the payload can also include:

- `session_id`
- `turn_id`
- `tool_call_id`
- `tool_name`
- `command`
- `prompt_text`
- `cwd`
- `model`
- `transcript_path`
- `session_source`
- `permission_mode`
- `reason`
- `exit_code`
- `stop_hook_active`
- `runtime_pid`

`provenance` is usually `native_hook` for hook-based runtimes and
`native_plugin` for plugin-based runtimes such as OpenCode.

## Bindings

Bindings are optional app-owned correlation values. The helper never requires
them, and it never assumes any specific environment variable names.

Bindings appear under:

```json
{
  "bindings": {
    "launch_id": "launch-123",
    "app_session_id": "session-456",
    "actor_id": "agent-alpha",
    "host_id": "bridge-789"
  }
}
```

The recommended binding keys are:

- `launch_id`
- `app_session_id`
- `actor_id`
- `host_id`

You can also emit your own keys if another app needs a different shape.

## Normalised event types

The helper maps native runtime events into stable normalised event types. The
families are:

- `session.started`
- `session.ended`
- `turn.user_prompt_submitted`
- `turn.finished`
- `turn.failed`
- `turn.stopped`
- `tool.started`
- `tool.finished`
- `tool.failed`
- `tool.permission_requested`
- `notification`
- `runtime.event`

Runtimes can emit richer native events than the normalised contract exposes
today. In that case, the native event name is still preserved so you can extend
your app-side policy without waiting for a schema rewrite.

## Example

This is a representative event:

```json
{
  "schema_version": "agentic-control.harness.v1",
  "recorded_at_ms": 1775200000000,
  "runtime": "codex",
  "provenance": "native_hook",
  "native_event_name": "PreToolUse",
  "event_type": "tool.started",
  "summary": "About to run Bash: git status --short",
  "session_id": "sess_123",
  "turn_id": "turn_456",
  "tool_call_id": "toolu_789",
  "tool_name": "Bash",
  "command": "git status --short",
  "cwd": "/workspace/repo",
  "transcript_path": "/tmp/codex-session.json",
  "permission_mode": "default",
  "runtime_pid": 91234,
  "bindings": {
    "launch_id": "launch-123",
    "app_session_id": "session-456",
    "actor_id": "agent-alpha"
  }
}
```
