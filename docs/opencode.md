# OpenCode

OpenCode is the one runtime in Agentic Control that uses two native
integration surfaces:

- the plugin system for passive observation through `agent_harness`
- `opencode serve` for app-managed sessions through `agent_control`

Unlike Codex, Gemini, and Claude, OpenCode does not need a shell-command hook
file to expose passive lifecycle events. Its plugin system already publishes
session, permission, and tool events directly, so the OpenCode bundle acts as
a hook-equivalent plugin that forwards those native events into the shared Go
helper.

Official references:

- [OpenCode install guide](https://opencode.ai/docs/)
- [OpenCode config](https://opencode.ai/docs/config/)
- [OpenCode plugins](https://opencode.ai/docs/plugins/)
- [OpenCode permissions](https://opencode.ai/docs/permissions/)
- [OpenCode server](https://opencode.ai/docs/server/)
- [OpenCode repository](https://github.com/anomalyco/opencode)

This bundle was validated on April 5, 2026 against `1.3.15`, as
reported by `opencode --version` on the validation machine.

## Install OpenCode

Install OpenCode with the official install guide before wiring Agentic Control
into it:

- [OpenCode install guide](https://opencode.ai/docs/)
- [OpenCode repository installation section](https://github.com/anomalyco/opencode#installation)

The official install guide and repository document these convenient
install paths:

```bash
curl -fsSL https://opencode.ai/install | bash
npm install -g opencode-ai
brew install anomalyco/tap/opencode
brew install opencode
```

## OpenCode control-plane boundary

Agentic Control uses the OpenCode plugin bundle for passive observation and the
OpenCode server for app-managed sessions.

The Go control-plane provider starts one shared `opencode serve --pure`
process, talks to it over HTTP, and subscribes to `/event` over server-sent
events. That gives the control-plane the same app-managed session surface it
already exposes for Codex, Gemini, and Claude, while keeping the passive plugin
lane separate. If the SSE stream drops while the server is still alive, the
provider retries the subscription and does a best-effort session-state resync.

The `/event` route is a live-only stream, not a replay stream. Reconnect gives
the provider a fresh subscription plus best-effort status recovery, but it may
not reconstruct every missed tool, approval, or assistant-delta event from the
gap. When recovery cannot determine the terminal turn outcome, the controller
emits a generic recovery `runtime.event` instead of inventing success.

If the dropped window leaves the session in a generic busy state without enough
recoverable in-flight detail, the controller emits a recovery `runtime.event`
warning instead of pretending it can fully reconstruct the missing state.

The OpenCode controller supports:

- start
- resume
- send
- interrupt
- respond to permission requests
- respond to user-input requests
- list active controller-owned sessions
- stop

## Controller parity

Codex and Claude define the parity bar for app-managed sessions in this
repository. OpenCode matches that bar, and it goes beyond the
bar by allowing the controller to adopt idle or previously detached external
provider sessions by `provider_session_id`.

No controller-side parity work is pending for OpenCode.

The provider uses these OpenCode server routes:

- `POST /session`
- `GET /session/:id`
- `GET /session/status`
- `GET /session/:id/message`
- `POST /session/:id/prompt_async`
- `POST /session/:id/abort`
- `POST /session/:id/permissions/:permissionID`
- `GET /question`
- `POST /question/:requestID/reply`
- `POST /question/:requestID/reject`
- `GET /event`

`session.stop` detaches the controller-owned session from Agentic Control, but
it does not delete the underlying OpenCode session record. That preserves the
resume path for host applications that want to re-attach later by provider
session ID.

OpenCode `session.resume` is limited to idle or previously detached
provider sessions. The controller rejects busy sessions during adoption because
the OpenCode server does not expose enough information to reconstruct
in-flight approval state safely. It also rejects adoption when it cannot verify
the remote session status at all.

## Why Agentic Control uses a plugin

OpenCode’s native extension point is its plugin system. The plugin API exposes
both a generic event stream and specific tool hooks, which is exactly what
Agentic Control needs for investigation support.

The OpenCode bundle therefore does not emulate shell hooks. It installs one
small plugin that forwards OpenCode-native events and tool executions to the
shared `agent_harness` binary. The translation into the shared contract
happens in Go, not in the plugin.

## What Agentic Control listens to

The passive OpenCode bundle forwards this minimum event set:

| OpenCode native event or hook | Normalised event type | What it usually means |
| --- | --- | --- |
| `session.created` | `session.started` | OpenCode created a session and can expose session metadata. |
| `session.status` | `runtime.event` | OpenCode updated session execution status, such as `busy` or `retry`. |
| `session.idle` | `turn.finished` | OpenCode has finished the active turn and is waiting for more input. |
| `session.error` | `turn.failed` | OpenCode could not complete the turn because the session hit an error. |
| `permission.asked` | `tool.permission_requested` | OpenCode needs user approval before continuing. |
| `permission.replied` | `runtime.event` | The user responded to a permission request with `once`, `always`, or `reject`. |
| `tool.execute.before` | `tool.started` | OpenCode is about to run a tool. |
| `tool.execute.after` | `tool.finished` | OpenCode has finished running a tool. |

This repository deliberately starts with the highest-signal surfaces. The goal
is to capture enough native state to replace heuristic “terminal finished”
logic, not to mirror every internal event OpenCode can emit.

## What OpenCode passes to the plugin

OpenCode’s plugin API exposes both generic events and specific tool hooks. The
official plugin package types them as:

- `event: async ({ event }) => { ... }`
- `"tool.execute.before": async (input, output) => { ... }`
- `"tool.execute.after": async (input, output) => { ... }`

The most relevant shapes for the Agentic Control bundle are:

- `event`
  - `permission.asked`
    - `properties.id`
    - `properties.sessionID`
    - `properties.permission`
    - `properties.patterns`
    - `properties.metadata`
    - `properties.always`
    - `properties.tool.messageID`
    - `properties.tool.callID`
  - `permission.replied`
    - `properties.sessionID`
    - `properties.requestID`
    - `properties.reply`
  - `session.created`
    - `properties.sessionID`
    - `properties.info`
  - `session.status`
    - `properties.sessionID`
    - `properties.status`
  - `session.idle`
    - `properties.sessionID`
  - `session.error`
    - `properties.sessionID`
    - `properties.error`
- `tool.execute.before`
  - `input.tool`
  - `input.sessionID`
  - `input.callID`
  - `output.args`
- `tool.execute.after`
  - `input.tool`
  - `input.sessionID`
  - `input.callID`
  - `input.args`
  - `output.title`
  - `output.output`
  - `output.metadata`

The plugin forwards those native shapes plus a few convenience fields such as
`session_id`, `tool_call_id`, `tool_name`, `command`, `reason`, and
`exit_code`. The helper does the normalisation work after that point.

Representative fixtures live in:

- [`runtimes/opencode/fixtures/session_created.json`](../runtimes/opencode/fixtures/session_created.json)
- [`runtimes/opencode/fixtures/session_status.json`](../runtimes/opencode/fixtures/session_status.json)
- [`runtimes/opencode/fixtures/permission_asked.json`](../runtimes/opencode/fixtures/permission_asked.json)
- [`runtimes/opencode/fixtures/permission_replied.json`](../runtimes/opencode/fixtures/permission_replied.json)
- [`runtimes/opencode/fixtures/tool_execute_before.json`](../runtimes/opencode/fixtures/tool_execute_before.json)
- [`runtimes/opencode/fixtures/tool_execute_after.json`](../runtimes/opencode/fixtures/tool_execute_after.json)

## Install the OpenCode bundle

OpenCode is the one runtime in this repository that defaults to a global
install, because its plugin system already uses a dedicated global directory:

```bash
.artifacts/bin/agent_harness install --runtime opencode --scope global \
  --socket-env AGENT_HARNESS_SOCKET
```

That writes:

- `~/.config/opencode/plugins/agentic-control.js`
- `~/.config/opencode/agentic-control/plugin-config.json`
- `~/.config/opencode/agentic-control/bin/agent_harness`

If you want an isolated repo-local install instead:

```bash
.artifacts/bin/agent_harness install --runtime opencode --scope repo \
  --socket-env AGENT_HARNESS_SOCKET
```

That writes under:

- `.opencode/plugins/agentic-control.js`
- `.opencode/agentic-control/bin/agent_harness`
- `.opencode/agentic-control/plugin-config.json`

The installer does not edit `opencode.json`. OpenCode auto-loads plugins from
the standard plugin directories documented in its config reference.

If you install both a global bundle and a repo-local bundle, the repo-local
bundle is the active bundle for that repository. The global plugin detects a
matching `.opencode/plugins/agentic-control.js` in the current project and does
not emit duplicate events.

To remove the OpenCode plugin bundle later, run:

```bash
.artifacts/bin/agent_harness uninstall --runtime opencode --scope global
```

## How the plugin finds the helper

The plugin does not need to know your application’s binding variable names. It
only needs to run the shared helper and inherit the surrounding process
environment. The launch side of your host application is responsible for
setting any binding environment variables and the socket path.

The OpenCode plugin therefore works like this:

1. It loads a small sidecar JSON file that contains helper flags such as
   `--runtime opencode` and `--socket-env AGENT_HARNESS_SOCKET`.
2. It resolves the helper binary from `agentic-control/bin/agent_harness`
   adjacent to the installed plugin bundle.
3. It forwards native OpenCode events or tool hook payloads to the helper on
   `stdin`.
4. The helper reads any configured binding variables directly from the
   inherited environment.

That keeps the plugin generic and keeps the application-specific correlation
contract inside the shared helper.

## Approval testing

OpenCode defaults to permissive tool execution, so the live approval harness
uses an inline config override to force `bash` into `ask` mode:

```bash
export OPENCODE_CONFIG_CONTENT='{"$schema":"https://opencode.ai/config.json","permission":{"bash":{"*":"ask"}}}'
opencode run "Use bash to create .artifacts/example.txt"
```

That matches OpenCode’s documented config precedence and permission model:

- [OpenCode config](https://opencode.ai/docs/config/)
- [OpenCode permissions](https://opencode.ai/docs/permissions/)

## Known gap

OpenCode’s plugin events are already strong enough for a real investigation
lane, but MCP tool calls do not trigger the same tool execution hooks
as standard tools. Track that upstream behaviour here:

- [OpenCode issue: MCP tool calls do not trigger tool.execute.before/after](https://github.com/anomalyco/opencode/issues/2319)

## Quick checks

Replay the OpenCode fixtures:

```bash
mise run diag:fixtures:opencode
```

Install the global plugin bundle:

```bash
mise run diag:install:opencode
```

Run the live scenarios:

```bash
mise run diag:opencode:smoke
mise run diag:opencode:bash
mise run diag:opencode:approval
```

Watch a live OpenCode session in two terminals:

```bash
# Terminal 1
cd ~/code/personal/agentic-control
.artifacts/bin/agent_harness listen --socket-path /tmp/agent-harness-opencode.sock
```

```bash
# Terminal 2
cd ~/code/personal/agentic-control
.artifacts/bin/agent_harness install --runtime opencode --scope repo --socket-env AGENT_HARNESS_SOCKET
AGENT_HARNESS_SOCKET=/tmp/agent-harness-opencode.sock \
OPENCODE_CONFIG_CONTENT='{"$schema":"https://opencode.ai/config.json","permission":{"bash":{"*":"ask"}}}' \
opencode run "Use bash to create .artifacts/opencode-hook-approval.txt with the text opencode approval probe"
```
