# Control-plane

Use the control-plane when your application owns the agent session lifecycle.
The passive harness owns observed hook, plugin, and extension translation.
The control-plane owns app-managed sessions.

The controller boundaries are:

- Codex via `codex app-server`
- Gemini via `gemini --acp`
- Claude via a local bridge to `@anthropic-ai/claude-agent-sdk`
- OpenCode via `opencode serve`
- pi via `pi --mode rpc`

## What the control-plane provides

The Go controller is a long-lived Unix socket service that keeps an active
session registry and exposes a typed RPC surface plus an interaction bridge for
host applications.

In practice, that gives your app one place to:

- discover runtime capabilities
- start or resume sessions
- send input and interrupt turns
- respond to approvals and user-input requests when the runtime supports them
- subscribe to a typed event stream

Current core controller methods:

- `system.ping`
- `system.describe`
- `models.list`
- `thread.list`
- `thread.get`
- `thread.archive`
- `thread.set_name`
- `thread.set_metadata`
- `thread.fork`
- `thread.rollback`
- `thread.events`
- `thread.read`
- `events.subscribe`
- `events.unsubscribe`
- `session.start`
- `session.resume`
- `session.send`
- `session.get`
- `session.history`
- `session.interrupt`
- `session.respond`
- `session.stop`
- `session.list`

Current interaction bridge and promoted native helpers:

- `interaction.call`
- `interaction.subscribe`
- `interaction.unsubscribe`
- `speech.tts.enqueue`
- `speech.tts.cancel`
- `speech.tts.status`
- `speech.tts.voices.list`
- `speech.tts.config.get`
- `speech.tts.config.set`
- `speech.stt.start`
- `speech.stt.stop`
- `speech.stt.status`
- `speech.stt.submit`
- `speech.stt.subscribe`
- `speech.stt.unsubscribe`
- `speech.stt.models.list`
- `speech.stt.model.get`
- `speech.stt.model.set`
- `speech.stt.model.download`
- `app.open`
- `app.activate`
- `insert.targets.list`
- `insert.enqueue`
- `screen.observe`
- `screen.click`
- `attention.enqueue`
- `attention.list`
- `attention.update`

## Interaction layer status

Agentic Interaction is part of the control-plane runtime substrate, with a
partial typed product surface:

- the daemon always probes Agentic Interaction and exposes its status from
  `system.describe`;
- the Go SDK and `interaction.call` / `interaction.subscribe` mirror the current
  Agentic Interaction JSON-RPC surface;
- a smaller set of native workflows has been promoted into dedicated
  control-plane methods such as `speech.*`, `app.open`, `insert.targets.list`,
  and `screen.observe`;
- the CLI does not expose the full interaction surface as first-class command
  groups, so many native methods are available through the Go
  packages and daemon bridge rather than through dedicated CLI verbs.

The interaction layer is in the control-plane core as an execution substrate
and bridge, with selected workflows promoted into typed CLI and RPC surfaces.

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
- `probe`

The capability object makes controller defaults explicit. Clients don't need to
guess whether a runtime supports `resume`, `respond`, or immediate provider
session IDs. This is the main convention-over-configuration entrypoint for new
integrations, including HTTP-backed wrappers such as OpenCode server.

The optional `probe` object is a cached runtime health check. The controller
checks the configured binary path, captures a version string when the runtime
supports `--version`, and caches the result for five minutes. Probe states are
advisory: `capabilities` describes what the provider implementation supports,
while `probe` describes the current local machine.

Probe fields are designed for upstream UX. A host can render one install and
model picker from:

- `installed`, `status`, `version`, and `binary_path` for local install state
- `auth.status`, `auth.type`, `auth.label`, `auth.method`, and `auth.message`
  for authentication state
- `models[]` and `model_source` for the models that the runtime can expose
  through the control-plane

Static providers such as Codex, Claude, and Gemini publish a built-in model
catalog with per-model capabilities. OpenCode enriches its probe from the
running `opencode serve` `/provider` endpoint when available, so upstream apps
can show connected OpenAI, Anthropic, Google, or other provider models without
hard-coding that inventory. pi documents RPC `get_available_models`, but
Agentic Control needs a short-lived probe implementation before it can
surface pi models through `system.describe`.

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
      },
      "probe": {
        "installed": true,
        "status": "ready",
        "version": "codex-cli 0.124.0",
        "binary_path": "/opt/homebrew/bin/codex",
        "auth": {
          "status": "authenticated",
          "method": "login status"
        },
        "model_source": "built_in",
        "models": [
          {
            "id": "gpt-5.4",
            "label": "GPT-5.4",
            "provider": "codex",
            "default": true,
            "capabilities": {
              "reasoning_effort_levels": [
                {"value": "xhigh", "label": "Extra High"},
                {"value": "high", "label": "High", "is_default": true},
                {"value": "medium", "label": "Medium"},
                {"value": "low", "label": "Low"}
              ],
              "supports_fast_mode": true
            }
          }
        ],
        "message": "Runtime binary found",
        "probed_at_ms": 1775200000000
      }
    }
  ]
}
```

## Text generation router

The public Go package also exposes a small unified text-generation router in
[`pkg/controlplane/textgen.go`](../pkg/controlplane/textgen.go). Upstream
services can use it for the shared text-generation surfaces:

- commit messages
- pull-request content
- branch names
- thread titles

Callers pass a `TextGenerationModelSelection` with optional `provider`,
`model`, `model_options`, and `fallbacks`. The router resolves in this order:

1. explicit provider
2. provider inferred from the model ID
3. fallback providers
4. router default provider

Model inference keeps common upstream rules centralised:

- `claude-*` routes to Claude
- `gemini-*` and `auto-gemini-*` route to Gemini
- `gpt-*` and OpenAI reasoning model prefixes route to Codex
- provider-scoped model IDs such as `anthropic/claude-sonnet-4-6` route to
  OpenCode

The resolved selection is passed into the selected provider, so upstream
services can keep their own role or worker configuration simple while still
preserving the chosen model and model options.

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
agent_control serve --socket-path "$SOCKET_PATH"
agent_control wait-ready --socket-path "$SOCKET_PATH"
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
agent_control describe --socket-path "$SOCKET_PATH"
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
    "cwd": "/path/to/agentic-control",
    "model": "gpt-5.4",
    "model_options": {
      "reasoning_effort": "high"
    }
  }
}
```

`model_options` is intentionally generic. Providers ignore unsupported fields.
Gemini uses `thinking_level` for Gemini 3 models and `thinking_budget` for
Gemini 2.5 models. For example:

```json
{
  "id": "start-gemini-1",
  "method": "session.start",
  "params": {
    "runtime": "gemini",
    "session_id": "gemini-research-1",
    "cwd": "/workspace/repo",
    "model": "gemini-2.5-pro",
    "model_options": {
      "thinking_budget": -1
    }
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

Richer controller events are emitted when the runtime exposes enough detail:

- `assistant.message.delta`
- `assistant.thought.delta`
- `thread.token-usage.updated`
- `turn.plan.updated`
- `tool.progress`
- `session.mode.changed`

## Event Logging

Set `AGENTIC_CONTROL_EVENT_LOG` to append every control-plane event to an
NDJSON file:

```bash
AGENTIC_CONTROL_EVENT_LOG=/tmp/agentic-control/events.ndjson \
  agent_control serve --socket-path /tmp/agentic-control.sock
```

Each line includes a UTC timestamp, runtime, session ID, and the full
normalised event. This is intended for local debugging, ACP protocol
investigation, and lightweight analytics capture.

## Text Generation Router

Host applications that use agentic-control for repository workflows can use the
Go text generation router in `pkg/controlplane`. It defines one provider
interface for commit messages, PR content, branch names, and thread titles, and
routes by provider name with a configured default fallback. This keeps
provider-specific prompting out of application workflow code while preserving
the app's choice of provider.

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
so the controller cannot surface `ask_user` or `exit_plan_mode` as shared
`request.opened` user-input requests. In the reviewed `0.39.0` package,
[google-gemini/gemini-cli#24664](https://github.com/google-gemini/gemini-cli/pull/24664)
is closed and not merged, so parity depends on a released ACP host-input path.

The Gemini parity gaps are:

- ship a released Gemini CLI host-input ACP path
- advertise the final Gemini host-input capability during `initialize`
- map `gemini/requestUserInput` into shared `request.opened` user-input
  requests for `ask_user` and `exit_plan_mode`
- route `session.respond` answers and cancellations back through the Gemini
  host-input path
- flip `user_input_requests` to `true` in `system.describe`
- add real integration coverage against the released ACP build

Gemini session discovery is controller-owned. The Gemini ACP
surface in `gemini 0.38.2` supports `session/new`, `session/load`,
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
registry, and normalized events, but approvals and user input flow through
the SDK `canUseTool` boundary instead of the CLI `stream-json` input format.
New Claude sessions also get a controller-assigned UUID at `session.start`, so
the provider session ID is available before the first user turn is sent.

This bridge path was refreshed on April 24, 2026 against local
`claude 2.1.104` and `@anthropic-ai/claude-agent-sdk 0.2.119` with:

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

pi supports a narrower app-managed surface:

- start
- resume by session file path
- send
- interrupt
- list active controller-owned sessions
- stop

The pi provider runs `pi --mode rpc --no-extensions`, uses `get_state` for
bootstrap, `new_session` for fresh controller-owned sessions, and
`switch_session` for resume by provider session ID. The provider session ID is
pi's session file path, not an opaque remote token. The controller also reads
streamed RPC events such as `message_update`, `tool_execution_start`,
`tool_execution_end`, and `agent_end` to produce normalized runtime events.

pi does not reach the Codex and Claude parity bar because Agentic Control does
not expose generic host-owned approval or user-input workflows for
pi. pi can build those flows with extensions and its extension UI sub-protocol,
but the controller intentionally keeps managed pi sessions deterministic by
starting them with `--no-extensions`.

</details>
