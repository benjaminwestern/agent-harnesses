# Codex

Codex is the reference runtime for this repository because it exposes both a
native hook surface and an optional plugin packaging model. The recommended
integration path here is hooks first, plugins second. Hooks give you the direct
lifecycle events you need for investigation and correlation. Plugins remain
useful when you want a cleaner distribution story for those same hook settings.

Official references:

- [Codex hooks](https://developers.openai.com/codex/hooks)
- [Codex plugins overview](https://developers.openai.com/codex/plugins)
- [Codex plugin packaging](https://developers.openai.com/codex/plugins/build)
- [OpenAI Codex repository](https://github.com/openai/codex)

This bundle was validated on April 5, 2026 against
`codex-cli 0.118.0`, as reported by `codex --version` on the validation
machine.

## Install Codex CLI

Install Codex CLI with the official quickstart before wiring Agentic Control
into it:

- [Codex CLI quickstart and install](https://github.com/openai/codex#quickstart)
- [Codex CLI releases](https://github.com/openai/codex/releases/latest)

The official quickstart documents these convenient install paths:

```bash
npm install -g @openai/codex
brew install --cask codex
```

> [!NOTE]
> Codex hooks are documented upstream as experimental. This repository
> treats them as the primary observability surface because they expose the
> lifecycle events that matter most for investigation, resume correlation, and
> approval-aware testing.

## What Agentic Control uses

Agentic Control uses the native Codex hook surface. It does not depend on MCP,
and it does not require a plugin to observe runtime activity.

When your application owns the Codex session directly, the repository also
ships a separate control-plane adapter. That adapter uses `codex app-server`
for app-managed sessions.

The default Codex bundle listens for these native events:

- `SessionStart`
- `UserPromptSubmit`
- `PreToolUse`
- `PostToolUse`
- `Stop`

The shared helper maps those into these normalised families:

- `session.started`
- `turn.user_prompt_submitted`
- `tool.started`
- `tool.finished`
- `turn.stopped`

## Codex control-plane boundary

The hook bundle is for passive observation. The Go controller is for
app-managed Codex sessions that need direct input, interruption, approvals, and
user-input requests.

The Codex controller supports:

- start
- resume
- send
- interrupt
- respond to approval requests
- respond to user-input requests
- list active controller-owned sessions
- stop

## Controller parity

Codex and Claude define the parity bar for app-managed sessions in
this repository. That bar is the full controller surface listed above.

Codex is at parity with that bar. No controller-side work is pending for Codex.
Codex is the reference
implementation for the app-server path in this repository.

## Hook lifecycle and event mapping

The Codex adapter maps native events as follows:

| Codex hook event | Normalised event type | What it usually means |
| --- | --- | --- |
| `SessionStart` | `session.started` | Codex has started or resumed a session and has native session metadata available. |
| `UserPromptSubmit` | `turn.user_prompt_submitted` | Codex has accepted a user prompt for a new turn. |
| `PreToolUse` | `tool.started` | Codex is about to execute a tool. The bundle listens for `Bash`. |
| `PostToolUse` | `tool.finished` | Codex has finished a tool call and can provide result metadata. |
| `Stop` | `turn.stopped` | Codex has hit the stop hook surface, which is useful for interruption and completion cues. |

The helper preserves the native hook name as `native_event_name`, so you can
distinguish Codex-specific events even when your application primarily
reasons over the normalised `event_type`.

## What Codex passes to the hook command

According to the Codex hooks reference, each hook command receives one JSON
object on `stdin`. The exact shape depends on the event, but the common fields
you should expect are:

- `session_id`
- `transcript_path`
- `cwd`
- `hook_event_name`
- `model`

Turn-scoped hooks also include:

- `turn_id`

Tool-scoped hooks also include fields such as:

- `tool_use_id`
- `tool_name`
- `tool_input`
- `tool_output`

For Bash interception, the fields that matter most are:

- `tool_name`
- `tool_use_id`
- `tool_input.command`
- `tool_output.exit_code`
- `tool_output.stdout`
- `tool_output.stderr`

Representative payloads in this repository:

- [`runtimes/codex/fixtures/session_start.json`](../runtimes/codex/fixtures/session_start.json)
- [`runtimes/codex/fixtures/pre_tool_use.json`](../runtimes/codex/fixtures/pre_tool_use.json)
- [`runtimes/codex/fixtures/post_tool_use.json`](../runtimes/codex/fixtures/post_tool_use.json)

The helper preserves those runtime-native fields when they are present and
leaves them absent when Codex does not send them. It does not invent fallback
placeholders.

## What Agentic Control emits downstream

After translation, the Codex adapter emits one normalised event with:

- the shared contract fields from [event contract](contract.md)
- the native Codex hook name
- native session, turn, and tool identifiers when present
- any optional `bindings` that your host application requested with
  `--bind-env` or `--bind-value`

That means you can correlate:

- `session_id` with resume workflows
- `turn_id` with per-turn UI state
- `tool_use_id` with tool execution tracking
- `bindings.launch_id` or equivalent app-owned keys with your terminal or
  process model

## Install the Codex bundle

The shared installer defaults Codex to a repo-local layout so you can test and
iterate without modifying `~/.codex`:

```bash
.artifacts/bin/agent_harness install --runtime codex --scope repo \
  --socket-env AGENT_HARNESS_SOCKET \
  --bind-env launch_id=APP_LAUNCH_ID \
  --bind-env app_session_id=APP_SESSION_ID \
  --bind-env actor_id=APP_ACTOR_ID \
  --bind-env host_id=APP_HOST_ID
```

That writes:

- `.codex/hooks.json`
- `.codex/agentic-control/bin/agent_harness`

If you explicitly want a global install:

```bash
.artifacts/bin/agent_harness install --runtime codex --scope global \
  --socket-env AGENT_HARNESS_SOCKET
```

That global path writes:

- `~/.codex/hooks.json`
- `~/.codex/agentic-control/bin/agent_harness`

The installer does not edit global feature configuration, and it does not
assume any product-specific environment variable names.

To remove only the Agentic Control Codex hook content later, run the matching
uninstall command:

```bash
.artifacts/bin/agent_harness uninstall --runtime codex --scope repo
```

## Enable hooks without editing global TOML

Codex hooks can be enabled at launch time instead of in
`~/.codex/config.toml`:

```bash
codex --enable codex_hooks ...
```

That keeps the integration self-contained and avoids modifying user-global
feature flags unless the user explicitly wants that behaviour.

## Launch-time contract

The installer embeds whatever helper flags you pass after `--runtime` and
`--scope`. That is how you bind app-owned metadata without teaching the helper
your app’s internal naming model.

Typical example:

```bash
.artifacts/bin/agent_harness install --runtime codex --scope repo \
  --socket-env AGENT_HARNESS_SOCKET \
  --bind-env launch_id=APP_LAUNCH_ID \
  --bind-env app_session_id=APP_SESSION_ID
```

Then launch Codex with hooks enabled:

```bash
codex --enable codex_hooks
```

If your app already knows the socket path directly, you can skip the socket
environment variable and use `--socket-path` instead.

## Live testing

For quick fixture replay:

```bash
mise run diag:fixtures:codex
```

For a real interactive session:

```bash
mise run diag:codex:smoke
mise run diag:codex:bash
mise run diag:codex:approval
```

Those tasks keep approvals enabled. They are intentionally not YOLO-mode paths
because the point of the harness is to verify real tool and approval behaviour.

Watch a live Codex session in two terminals:

```bash
# Terminal 1
cd ~/code/personal/agentic-control
.artifacts/bin/agent_harness listen --socket-path /tmp/agent-harness-codex.sock
```

```bash
# Terminal 2
cd ~/code/personal/agentic-control
.artifacts/bin/agent_harness install --runtime codex --scope repo --socket-env AGENT_HARNESS_SOCKET
PROMPT="$(cat runtimes/codex/prompts/approval-request.md)"
AGENT_HARNESS_SOCKET=/tmp/agent-harness-codex.sock \
codex --enable codex_hooks --no-alt-screen --ask-for-approval on-request \
  --sandbox workspace-write "$PROMPT"
```
