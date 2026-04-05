# Control-plane

Use the control-plane when your application owns the agent session lifecycle.
The passive harness owns observed hook and plugin translation. The
control-plane owns app-managed sessions.

The controller boundaries are:

- Codex via `codex app-server`
- Gemini via `gemini --acp`
- Claude via a local bridge to `@anthropic-ai/claude-agent-sdk`
- OpenCode via `opencode serve`

## What the control-plane provides

The Go controller is a long-lived Unix socket service that keeps an active
session registry and exposes a small RPC surface for host applications.

In practice, that gives your app one place to:

- discover runtime capabilities
- start or resume sessions
- send input and interrupt turns
- respond to approvals and user-input requests when the runtime supports them
- subscribe to a typed event stream

Supported methods:

- `system.ping`
- `system.describe`
- `events.subscribe`
- `events.unsubscribe`
- `session.start`
- `session.resume`
- `session.send`
- `session.interrupt`
- `session.respond`
- `session.stop`
- `session.list`

The socket server lives in
[`cmd/agent-control/main.go`](../cmd/agent-control/main.go), the service and
registry live in [`internal/controlplane/`](../internal/controlplane), and the
shared types live in [`pkg/contract/`](../pkg/contract) and
[`pkg/controlplane/`](../pkg/controlplane).

## Bootstrap and capabilities

Use `system.describe` as the first control-plane call. It returns the
schema version, the RPC wire version, the supported controller methods, and a
runtime descriptor for each registered provider.

That call is the first successful integration step because it tells your app
which runtimes are available and what each runtime can safely do today.

Each runtime descriptor includes:

- `runtime`
- `ownership`
- `transport`
- `capabilities`

The capability object makes controller defaults explicit. Clients don't need to
guess whether a runtime supports `resume`, `respond`, or immediate provider
session IDs. This is the main convention-over-configuration entrypoint for new
integrations, including HTTP-backed wrappers such as OpenCode server.

Example request:

```json
{
  "id": "describe-1",
  "method": "system.describe"
}
```

Example response shape:

```json
{
  "schema_version": "agentic-control.control-plane.v1",
  "wire_protocol_version": "agentic-control.rpc.v1",
  "methods": [
    "system.ping",
    "system.describe",
    "events.subscribe",
    "events.unsubscribe",
    "session.start",
    "session.resume",
    "session.send",
    "session.interrupt",
    "session.respond",
    "session.stop",
    "session.list"
  ],
  "runtimes": [
    {
      "schema_version": "agentic-control.control-plane.v1",
      "runtime": "codex",
      "ownership": "controlled",
      "transport": "app_server",
      "capabilities": {
        "start_session": true,
        "resume_session": true,
        "send_input": true,
        "interrupt": true,
        "respond": true,
        "stop_session": true,
        "list_sessions": true,
        "stream_events": true,
        "approval_requests": true,
        "user_input_requests": true,
        "immediate_provider_session": true,
        "resume_by_provider_id": true,
        "adopt_external_sessions": false
      }
    }
  ]
}
```

## Start the server

Build the binaries first:

```bash
mise run build
```

That build step installs the Claude bridge dependency automatically the first
time you compile `agent_control`.

Then run the controller:

```bash
SOCKET_PATH=/tmp/agentic-control.sock
.artifacts/bin/agent_control serve --socket-path "$SOCKET_PATH"
```

You can also use the convenience task:

```bash
SOCKET_PATH=/tmp/agentic-control.sock mise run control:serve
```

## First successful call

Once the server is running, call `system.describe` before you do anything else.
The simplest local path is:

```bash
SOCKET_PATH=/tmp/agentic-control.sock
.artifacts/bin/agent_control describe --socket-path "$SOCKET_PATH"
```

If the server is reachable, you will get back a single JSON response with the
available methods and runtime capability descriptors.

## RPC transport

The transport is newline-delimited JSON over a Unix domain socket. Each client
request is one JSON object. Each server response is one JSON object. Event
notifications are emitted as `{"method":"event","params":...}` after you call
`events.subscribe`.

Example request:

```json
{
  "id": "start-1",
  "method": "session.start",
  "params": {
    "runtime": "codex",
    "session_id": "voice-codex-1",
    "cwd": "/Users/benjaminwestern/code/personal/agentic-control"
  }
}
```

Example subscription request:

```json
{
  "id": "sub-1",
  "method": "events.subscribe"
}
```

## Request and response conventions

The controller keeps the outer RPC surface small, but the request model is
typed enough that provider wrappers don't have to invent their own side
channels.

`request.opened` events can include a structured `request` object with:

- `kind`
- `status`
- `tool`
- `options`
- `questions`
- `extensions`

`session.respond` uses a canonical action model:

- `allow`
- `deny`
- `submit`
- `cancel`
- `choose`

The server applies a small set of safe defaults:

- If you set `option_id` and omit `action`, the controller normalises the
  action to `choose`.
- If you set `text` or `answers` and omit `action`, the controller normalises
  the action to `submit`.
- If you omit `action` for an approval flow, the controller returns a
  validation error instead of auto-approving the request.

This keeps app code simple while avoiding unsafe implicit approvals.

Runtime events also include `session_state`, which gives you a typed session
status snapshot without scraping `payload.status` out of event-specific data.

## Runtime notes

Each runtime keeps its native transport. The controller normalises session and
event types above that transport, but it does not pretend every provider has
the same session model.

If you are integrating the control-plane for the first time, you can skip the
rest of this section on first read. The details below are runtime
status and parity notes, not required bootstrap steps.

<details>
<summary>Current runtime status and parity</summary>

The parity bar for app-managed sessions is the full controller surface
implemented today by Codex and Claude:

- start
- resume
- send
- interrupt
- respond to approval requests
- respond to user-input requests
- list active controller-owned sessions
- stop

Both runtimes also expose immediate provider session IDs and support resume by
provider session ID through the shared contract.

Codex is at parity with that bar:

- start
- resume
- send
- interrupt
- list active sessions
- respond to approvals and user-input requests
- stop

No controller-side parity work is pending for Codex.

Gemini is one feature short of that bar:

- start
- resume from a saved Gemini session with `session/load`
- send
- interrupt
- respond to permission requests
- list active controller-owned sessions
- stop

Released Gemini ACP builds do not expose host-owned user-input requests,
so the controller cannot yet surface `ask_user` or `exit_plan_mode` as shared
`request.opened` user-input requests. The upstream work to close that gap is
[google-gemini/gemini-cli#24664](https://github.com/google-gemini/gemini-cli/pull/24664),
but parity depends on that change shipping in a released Gemini CLI build.

The remaining Gemini parity steps are:

- ship a Gemini CLI release that includes PR `#24664`
- advertise the final Gemini host-input capability during `initialize`
- map `gemini/requestUserInput` into shared `request.opened` user-input
  requests for `ask_user` and `exit_plan_mode`
- route `session.respond` answers and cancellations back through the Gemini
  host-input path
- flip `user_input_requests` to `true` in `system.describe`
- add real integration coverage against the released ACP build

Gemini session discovery is controller-owned. The Gemini ACP
surface in `gemini 0.36.0` supports `session/new`, `session/load`,
`session/prompt`, `session/cancel`, and `session/request_permission`, but it
does not expose `session/list`, so the Go service treats its own active
session registry as the authoritative live-session view. `system.describe`
therefore reports Gemini capability truth from the controller, not from ACP.

Claude is at parity with the bar:

- start
- resume
- send
- interrupt
- respond to approval requests
- respond to user-input requests
- list active sessions
- stop

No controller-side parity work is pending for Claude.

Claude uses a small Node bridge around the official TypeScript Agent SDK. The
Go provider owns the canonical controller contract, active session
registry, and normalized events, but approvals and user input now flow through
the SDK `canUseTool` boundary instead of the CLI `stream-json` input format.
New Claude sessions also get a controller-assigned UUID at `session.start`, so
the provider session ID is available before the first user turn is sent.

This bridge path was validated on April 5, 2026 against `claude 2.1.84` and
`@anthropic-ai/claude-agent-sdk 0.2.92` with:

- a real SDK-backed turn start and streamed result
- a real `request.opened` approval event for a blocked Bash write
- a real `session.respond` allow path that completed the Bash call and returned
  a final assistant result

OpenCode is at parity with the same bar:

- start
- resume
- send
- interrupt
- respond to permission requests
- respond to user-input requests
- list active controller-owned sessions
- stop

The OpenCode provider starts one shared `opencode serve --pure` process and
uses the documented HTTP plus SSE server surface behind the same Go session and
event contract as the other runtimes. The provider creates or adopts OpenCode
session IDs through `/session`, sends turns through `/session/:id/prompt_async`,
interrupts through `/session/:id/abort`, responds to approvals through
`/session/:id/permissions/:permissionID`, responds to user-input requests
through `/question/:requestID/reply` and `/question/:requestID/reject`, and
watches `/event` for normalized session, message, tool, permission, and
question updates. The `/event` route is a live-only stream, not a replay
stream, so if the SSE connection drops the provider retries the subscription
and does a best-effort session-state resync. That recovery path can restore
terminal turn truth, but it cannot reconstruct every missed tool,
approval, or assistant-delta event from the gap. When recovery cannot
determine the terminal turn outcome, the controller emits a generic
`runtime.event` recovery notice instead of inventing success.

If the dropped window leaves the session in a generic busy state without enough
recoverable in-flight detail, the controller emits a recovery `runtime.event`
warning instead of pretending it can fully reconstruct the missing state.

OpenCode goes beyond Codex and Claude in one area: it can adopt idle or
previously detached external provider sessions by `provider_session_id`.
No controller-side parity work is pending for OpenCode.

OpenCode session discovery is controller-owned for the same reason as
Gemini: the Go service only reports sessions it started or adopted. The
underlying OpenCode server can list more history than the controller
adopts, but `session.list` intentionally returns the active controller-owned
view so follow-up control operations stay explicit.

OpenCode `session.resume` is limited to idle or previously detached
provider sessions. The controller rejects busy sessions during adoption because
the OpenCode server does not expose enough information to reconstruct
in-flight approval state safely. It also rejects adoption when it cannot verify
the remote session status at all.

`session.stop` for OpenCode ends the controller-owned session handle, but it
does not delete the underlying OpenCode session record. That keeps
`resume_by_provider_id` truthful and lets host applications re-attach later
with the provider session ID.

</details>
