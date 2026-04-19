# Gemini

Gemini is the second runtime wired into Agentic Control. The overall
integration model is similar to Codex, but the configuration surface and hook
success contract are different. Gemini hooks live in `settings.json`, and each
hook command must return a valid success payload on `stdout`.

Official references:

- [Gemini CLI hooks guide](https://geminicli.com/docs/hooks/)
- [Gemini CLI hooks reference](https://geminicli.com/docs/hooks/reference/)

This bundle was validated on April 19, 2026 against `gemini 0.38.2`,
as reported by `gemini --version` on the validation machine.

## Install Gemini CLI

Install Gemini CLI with the official installation guide before wiring Agentic
Control into it:

- [Gemini CLI installation](https://geminicli.com/docs/get-started/installation/)
- [Gemini CLI quickstart](https://geminicli.com/docs/get-started/)

The official installation guide documents these convenient install
paths:

```bash
npm install -g @google/gemini-cli
brew install gemini-cli
```

The upstream documentation is the source of truth for the event list and hook
command contract. This page explains how Agentic Control uses that surface and
what it preserves for host applications.

## What Agentic Control uses

Agentic Control uses Gemini CLI hooks configured in `settings.json`.

The bundle listens for these native events:

- `SessionStart`
- `SessionEnd`
- `BeforeAgent`
- `AfterAgent`
- `BeforeTool`
- `AfterTool`
- `Notification`

The shared helper maps those into these normalised families:

- `session.started`
- `session.ended`
- `turn.user_prompt_submitted`
- `turn.finished`
- `tool.started`
- `tool.finished`
- `notification`

When your application owns the Gemini session directly, the repository also
ships a separate control-plane adapter. That adapter uses `gemini --acp`
instead of hooks.

## Gemini control-plane boundary

The hook bundle is for passive observation. The Go controller is for
app-managed Gemini sessions that need direct input, interruption, and approval
handling.

The Gemini controller supports:

- start
- resume from a saved Gemini session
- send
- interrupt
- respond to permission requests
- list active controller-owned sessions
- stop

The Gemini ACP adapter uses the native ACP methods exposed by the CLI:

- `initialize`
- `session/new`
- `session/load`
- `session/prompt`
- `session/cancel`
- `session/request_permission`

Gemini thinking controls are configured through control-plane
`model_options`. Gemini CLI does not expose a per-prompt thinking knob, so the
provider creates a temporary `GEMINI_CLI_SYSTEM_SETTINGS_PATH` file with model
aliases before it starts `gemini --acp`.

Supported Gemini model options are:

- `thinking_level: "LOW" | "HIGH"` for Gemini 3 models
- `thinking_budget: -1 | 0 | 512` for Gemini 2.5 models

When an option applies, the provider sends Gemini the generated alias but keeps
the public session `model` set to the requested model ID.

`system.describe` exposes the same information for UI discovery. Gemini's
probe includes a built-in Gemini model catalog with the supported thinking
levels or budgets for each model family, while the provider still creates the
runtime alias lazily only for sessions that request a thinking option.

The Gemini CLI ACP surface does not expose `session/list`, so the
controller uses its own active-session registry for live session targeting.
That means ACP is the right boundary for sessions your app launches or resumes,
while hooks remain the right boundary for unmanaged external Gemini sessions.

Completed Gemini ACP turns attempt to snapshot the native Gemini session file
under `~/.gemini/tmp/*/chats`. The `turn.completed` payload includes
`snapshot_session_id` and `snapshot_file_path` when a clone is available, and
the session metadata keeps a bounded list of recent snapshots. Host
applications can resume from a snapshot by passing the snapshot session ID as
`provider_session_id` to `session.resume`.

## Controller parity

Codex and Claude define the parity bar for app-managed sessions in
this repository. Gemini matches that bar everywhere except host-owned
user-input requests.

Released Gemini ACP builds can surface approval requests through
`session/request_permission`, but they disable ACP-owned `ask_user`
flows. The upstream work to close that gap is
[google-gemini/gemini-cli#24664](https://github.com/google-gemini/gemini-cli/pull/24664).
That pull request adds an opt-in `gemini/requestUserInput` path so an ACP host
can answer `ask_user` and `exit_plan_mode` itself. Gemini does not reach
controller parity until that change lands in a released Gemini CLI build.

The remaining Gemini parity steps are:

- ship a Gemini CLI release that includes PR `#24664`
- advertise the final Gemini host-input capability during `initialize`
- map `gemini/requestUserInput` into shared `request.opened` user-input
  requests for `ask_user` and `exit_plan_mode`
- route `session.respond` answers and cancellations back through the Gemini
  host-input path
- flip `user_input_requests` to `true` in `system.describe`
- add real integration coverage against the released ACP build

## Hook lifecycle and event mapping

The Gemini adapter maps native events as follows:

| Gemini hook event | Normalised event type | What it usually means |
| --- | --- | --- |
| `SessionStart` | `session.started` | Gemini has created a session and can expose session metadata. |
| `SessionEnd` | `session.ended` | Gemini has ended the session and can expose teardown context. |
| `BeforeAgent` | `turn.user_prompt_submitted` | Gemini has accepted a prompt and is about to begin agent work. |
| `AfterAgent` | `turn.finished` | Gemini has completed a turn and can expose response metadata. |
| `BeforeTool` | `tool.started` | Gemini is about to execute a tool. |
| `AfterTool` | `tool.finished` | Gemini has completed a tool call and can expose tool result metadata. |
| `Notification` | `notification` | Gemini has raised a user-facing or runtime-facing notification. |

The helper preserves the original hook name as `native_event_name`, so your
application can reason over both the shared contract and the runtime-specific
surface.

## What Gemini passes to the hook command

Gemini CLI passes a JSON object to each hook command on `stdin`. The exact
payload depends on the event, but the fields you will most commonly care about
are:

- `session_id`
- `hook_event_name`
- `cwd`
- `prompt`
- `tool_name`
- `tool_input.command`
- `tool_result.returnDisplay`
- `tool_result.llmContent`
- `message`

Representative payloads in this repository:

- [`runtimes/gemini/fixtures/session_start.json`](../runtimes/gemini/fixtures/session_start.json)
- [`runtimes/gemini/fixtures/before_tool.json`](../runtimes/gemini/fixtures/before_tool.json)
- [`runtimes/gemini/fixtures/after_tool.json`](../runtimes/gemini/fixtures/after_tool.json)
- [`runtimes/gemini/fixtures/notification.json`](../runtimes/gemini/fixtures/notification.json)

Different Gemini events expose different subsets of those fields. The helper
keeps missing fields absent rather than inventing placeholders.

## Gemini hook success behaviour

Gemini hooks are slightly stricter than Codex hooks. The command must write a
valid success response to `stdout` for the hook to be considered successful.

That is why the Gemini installer always appends:

```bash
--success-json '{}'
```

The shared helper handles that detail, so you do not need to maintain a
Gemini-specific wrapper binary just to satisfy the hook contract.

## What Agentic Control emits downstream

After translation, the Gemini adapter emits one normalised event with:

- the shared contract fields from [event contract](contract.md)
- the native Gemini hook name
- native session, tool, and notification metadata when Gemini provides it
- any optional `bindings` requested by your host application

That gives you a stable app-facing contract without hiding the runtime-native
session and tool identifiers that are useful for resume or investigation work.

## Install the Gemini bundle

The shared installer defaults Gemini to a repo-local layout:

```bash
.artifacts/bin/agent_harness install --runtime gemini --scope repo \
  --socket-env AGENT_HARNESS_SOCKET \
  --bind-env launch_id=APP_LAUNCH_ID \
  --bind-env app_session_id=APP_SESSION_ID \
  --bind-env actor_id=APP_ACTOR_ID \
  --bind-env host_id=APP_HOST_ID
```

That writes:

- `.gemini/settings.json`
- `.gemini/agentic-control/bin/agent_harness`

If you explicitly want a global install:

```bash
.artifacts/bin/agent_harness install --runtime gemini --scope global \
  --socket-env AGENT_HARNESS_SOCKET
```

That global path writes:

- `~/.gemini/settings.json`
- `~/.gemini/agentic-control/bin/agent_harness`

The installer keeps the runtime-specific detail small. It writes the hook
configuration, ensures the helper binary is present, and embeds whatever
binding flags you pass. It only manages the Agentic Control hook content inside
`settings.json`; it does not replace unrelated Gemini settings.

To remove only the Agentic Control Gemini hook content later, run the matching
uninstall command:

```bash
.artifacts/bin/agent_harness uninstall --runtime gemini --scope repo
```

## Launch-time contract

Unlike Codex, Gemini does not need an extra experimental feature flag at launch
time for the hook integration used here. The main runtime-specific launch rule
is that interactive scenarios still need a real TTY. That matters for the live
approval and prompt-aware harness runs.

The installer embeds the helper flags you pass after `--runtime` and
`--scope`. That is how you attach app-owned correlation values without
teaching the helper your application’s internal naming model.

## Live testing

For quick fixture replay:

```bash
mise run diag:fixtures:gemini
```

For a real interactive session:

```bash
mise run diag:gemini:smoke
mise run diag:gemini:bash
mise run diag:gemini:approval
```

Those tasks keep Gemini in an approval-capable mode so you can observe the
native event flow instead of bypassing it.

Watch a live Gemini session in two terminals:

```bash
# Terminal 1
cd ~/code/personal/agentic-control
.artifacts/bin/agent_harness listen --socket-path /tmp/agent-harness-gemini.sock
```

```bash
# Terminal 2
cd ~/code/personal/agentic-control
.artifacts/bin/agent_harness install --runtime gemini --scope repo --socket-env AGENT_HARNESS_SOCKET
PROMPT="$(cat runtimes/gemini/prompts/approval.md)"
AGENT_HARNESS_SOCKET=/tmp/agent-harness-gemini.sock \
gemini --sandbox --approval-mode default --prompt-interactive "$PROMPT"
```
