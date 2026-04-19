# pi

pi gives Agentic Control two useful integration surfaces: an auto-discovered
TypeScript extension for passive observation, and `pi --mode rpc` for
app-managed sessions. Use the extension bundle when you want passive runtime
telemetry from unmanaged sessions. Use the Go control-plane provider when your
application owns the pi process and needs to send prompts, interrupt work, and
resume saved sessions by session file path.

Official references:

- [pi README](https://github.com/badlogic/pi-mono/tree/main/packages/coding-agent)
- [pi RPC mode](https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/docs/rpc.md)
- [pi extensions](https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/docs/extensions.md)
- [pi session format](https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/docs/session.md)

This bundle was validated against `@mariozechner/pi-coding-agent 0.67.68`, the
installed version on the validation machine.

## Install pi

Install pi before wiring Agentic Control into it:

```bash
npm install -g @mariozechner/pi-coding-agent
```

The upstream documentation is the source of truth for CLI flags, RPC command
shapes, extension APIs, and session storage details. This page explains how
Agentic Control uses those surfaces.

## What Agentic Control uses

Agentic Control uses a project-local or global pi extension for passive
observation. The installer writes `agentic-control.ts` into pi's standard
extension discovery path, so pi loads it automatically without changing
`settings.json`.

The extension forwards these lifecycle signals into the shared harness:

- `session_start`
- `session_shutdown`
- `input`
- `tool_execution_start`
- `tool_execution_end`
- `agent_end`

The shared helper maps those into these normalised families:

- `session.started`
- `session.ended`
- `turn.user_prompt_submitted`
- `tool.started`
- `tool.finished`
- `tool.failed`
- `turn.finished`
- `turn.failed`
- `turn.stopped`

The extension emits the pi session file path as both the native session
identifier and `transcript_path`. That keeps resume and investigation work
explicit because pi's session persistence is file-based.

## pi control-plane boundary

The Go control-plane provider launches `pi --mode rpc` for app-managed
sessions. It uses the RPC command stream for:

- `get_state`
- `new_session`
- `switch_session`
- `prompt`
- `abort`

It also consumes streamed RPC events such as `message_update`,
`tool_execution_start`, `tool_execution_end`, and `agent_end`.

The pi controller supports:

- start
- resume by session file path
- send
- interrupt
- list active controller-owned sessions
- stop

The current provider does not expose host-owned approval or user-input
requests. pi can build those workflows with extensions and the extension UI
sub-protocol, but Agentic Control does not impose a generic approval policy on
pi sessions today.

The controller starts pi with `--no-extensions` so app-managed sessions stay
predictable and do not inherit arbitrary user or project extensions. Passive
observation remains available through the separate `agent_harness` extension
bundle.

Model inventory is a known control-plane gap. pi does maintain a model
registry and exposes it through `/model` and `pi --list-models`, including
auth-filtered built-ins and custom `~/.pi/agent/models.json` entries. It does
not currently document a stable JSON/RPC inventory method for host
integrations, so Agentic Control does not expose pi models through
`system.describe` yet. The probe reports pi install/version state with
`model_source: "runtime_default"` until a stable machine-readable contract is
available.

## What pi passes to the harness extension

The bundled extension emits a small, app-neutral JSON envelope to
`agent_harness`. The fields you will most commonly care about are:

- `hook_event_name`
- `session_file`
- `cwd`
- `model`
- `prompt_text`
- `tool_call_id`
- `tool_name`
- `args.command`
- `result_text`
- `stop_reason`
- `error_message`

Representative payloads in this repository:

- [`runtimes/pi/fixtures/session_start.json`](../runtimes/pi/fixtures/session_start.json)
- [`runtimes/pi/fixtures/input.json`](../runtimes/pi/fixtures/input.json)
- [`runtimes/pi/fixtures/tool_execution_start.json`](../runtimes/pi/fixtures/tool_execution_start.json)
- [`runtimes/pi/fixtures/tool_execution_end.json`](../runtimes/pi/fixtures/tool_execution_end.json)
- [`runtimes/pi/fixtures/agent_end.json`](../runtimes/pi/fixtures/agent_end.json)

## Install the pi bundle

The shared installer defaults pi to a repo-local layout:

```bash
.artifacts/bin/agent_harness install --runtime pi --scope repo \
  --socket-env AGENT_HARNESS_SOCKET \
  --bind-env launch_id=APP_LAUNCH_ID \
  --bind-env app_session_id=APP_SESSION_ID \
  --bind-env actor_id=APP_ACTOR_ID \
  --bind-env host_id=APP_HOST_ID
```

That writes:

- `.pi/extensions/agentic-control.ts`
- `.pi/agentic-control/extension-config.json`
- `.pi/agentic-control/bin/agent_harness`

If you explicitly want a global install:

```bash
.artifacts/bin/agent_harness install --runtime pi --scope global \
  --socket-env AGENT_HARNESS_SOCKET
```

That global path writes:

- `~/.pi/agent/extensions/agentic-control.ts`
- `~/.pi/agent/agentic-control/extension-config.json`
- `~/.pi/agent/agentic-control/bin/agent_harness`

To remove only the Agentic Control bundle later, run the matching uninstall
command:

```bash
.artifacts/bin/agent_harness uninstall --runtime pi --scope repo
.artifacts/bin/agent_harness uninstall --runtime pi --scope global
```

## Local verification

Replay the sample fixtures first:

```bash
mise run diag:fixtures:pi
```

Install the bundle for live use:

```bash
mise run diag:install:pi
```

Run the sample scenarios:

```bash
mise run diag:pi:smoke
mise run diag:pi:bash
mise run diag:pi:approval
```

The pi live scenarios use `pi -p` so they do not require an interactive TTY.
The `approval` scenario is a file-write probe, not a native runtime approval
flow.

## Next steps

If your application owns the pi process, start with
[control-plane guide](./control-plane.md). If you only need passive runtime
telemetry from pi sessions launched elsewhere, install the pi extension bundle
and point it at your local `agent_harness` socket.
