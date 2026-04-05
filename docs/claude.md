# Claude

Claude is the third runtime wired into Agentic Control, and it is the cleanest
fit for a sidecar settings model. Claude Code already supports
project-local hooks, session-local settings injection through `--settings`, and
a broad event surface that includes approval requests, notifications, turn end
events, and session teardown.

Official references:

- [Claude Code hooks reference](https://code.claude.com/docs/en/hooks)

This bundle was validated on April 5, 2026 against
`2.1.84 (Claude Code)`, as reported by `claude --version` on the validation
machine.

## Install Claude Code

Install Claude Code with the official setup guide before wiring Agentic Control
into it:

- [Claude Code setup](https://docs.claude.com/en/docs/claude-code/setup)
- [Claude Code quickstart](https://docs.claude.com/en/docs/claude-code/quickstart)

The official setup guide documents these convenient install paths:

```bash
curl -fsSL https://claude.ai/install.sh | bash
brew install --cask claude-code
winget install Anthropic.ClaudeCode
```

The upstream documentation is the source of truth for event names, matcher
semantics, hook input payloads, and decision control. This page explains the
subset that Agentic Control uses and how that subset maps into the shared
contract.

## What Agentic Control uses

Agentic Control uses Claude Code command hooks loaded through an explicit
sidecar settings file.

The bundle listens for these native events:

- `SessionStart`
- `UserPromptSubmit`
- `PreToolUse`
- `PermissionRequest`
- `PostToolUse`
- `PostToolUseFailure`
- `Notification`
- `Stop`
- `StopFailure`
- `SessionEnd`

The shared helper maps those into these normalised families:

- `session.started`
- `session.ended`
- `turn.user_prompt_submitted`
- `turn.stopped`
- `turn.failed`
- `tool.started`
- `tool.permission_requested`
- `tool.finished`
- `tool.failed`
- `notification`

When your application owns the Claude session directly, the repository also
ships a separate control-plane adapter. That adapter uses a local Node
bridge around the official TypeScript Claude Agent SDK for app-managed
sessions.

## Claude control-plane boundary

The hook bundle is for passive observation. The Go controller is for
app-managed Claude sessions that need direct input, interruption, and an
active-session registry.

The Claude controller supports:

- start
- resume
- send
- interrupt
- respond to approval requests
- respond to user-input requests
- list active controller-owned sessions
- stop

The Go adapter uses a small Node bridge around
`@anthropic-ai/claude-agent-sdk`, not the old CLI `stream-json` transport. The
Go provider owns the shared session and event contract, but the bridge handles
Claude's real approval callback path so `session.respond` works
for app-managed sessions. New Claude sessions also get a controller-assigned
UUID at `session.start`, so `provider_session_id` is available immediately
instead of appearing only after the first turn begins.

This bridge path was validated on April 5, 2026 against `claude 2.1.84` and
`@anthropic-ai/claude-agent-sdk 0.2.92`. The validation covered:

- a real SDK-backed turn start and streamed result
- a real approval callback for a blocked Bash write outside the working
  directory
- a real `session.respond` allow flow that unblocked the tool call and
  completed the assistant turn

## Controller parity

Claude and Codex define the parity bar for app-managed sessions in
this repository. That bar is the full controller surface Claude already
implements today:

- start
- resume
- send
- interrupt
- respond to approval requests
- respond to user-input requests
- list active controller-owned sessions
- stop

Claude is at parity with that bar. No controller-side work is pending for
Claude. Claude is the reference implementation for the SDK-bridge path in this
repository.

## Hook lifecycle and event mapping

The Claude adapter maps native events as follows:

| Claude hook event | Normalised event type | What it usually means |
| --- | --- | --- |
| `SessionStart` | `session.started` | Claude has started or resumed a session and can expose session metadata. |
| `UserPromptSubmit` | `turn.user_prompt_submitted` | Claude has accepted a prompt and is about to begin the turn. |
| `PreToolUse` | `tool.started` | Claude has prepared a tool call and is about to execute it. |
| `PermissionRequest` | `tool.permission_requested` | Claude is about to show a permission dialogue to the user. |
| `PostToolUse` | `tool.finished` | Claude has completed a tool call successfully. |
| `PostToolUseFailure` | `tool.failed` | Claude attempted a tool call and it failed. |
| `Notification` | `notification` | Claude raised a user-facing or runtime-facing notification. |
| `Stop` | `turn.stopped` | Claude has finished the turn and is about to stop. |
| `StopFailure` | `turn.failed` | Claude could not finish the turn because the runtime hit an API-level failure. |
| `SessionEnd` | `session.ended` | Claude ended the session and can expose teardown metadata. |

The helper preserves the original hook name as `native_event_name`, so your
application can still reason over Claude-specific states even when it primarily
consumes the shared `event_type`.

## What Claude passes to the hook command

Claude Code sends one JSON object to each command hook on `stdin`. The common
fields documented upstream are:

- `session_id`
- `transcript_path`
- `cwd`
- `hook_event_name`

Many Claude events also expose:

- `permission_mode`
- `tool_name`
- `tool_input`
- `tool_use_id`
- `prompt`
- `message`
- `source`
- `model`
- `reason`
- `error`
- `last_assistant_message`

The bundle in this repository relies most heavily on these payload shapes:

- `SessionStart` for `source` and `model`
- `UserPromptSubmit` for `prompt`
- `PreToolUse` for `tool_name`, `tool_input`, and `tool_use_id`
- `PermissionRequest` for approval-sensitive tool metadata
- `PostToolUse` for `tool_input` and `tool_response`
- `PostToolUseFailure` for `error`
- `Notification` for `message` and `notification_type`
- `Stop` for `stop_hook_active` and `last_assistant_message`
- `StopFailure` for `error` and `error_details`
- `SessionEnd` for `reason`

Representative payloads in this repository:

- [`runtimes/claude/fixtures/session_start.json`](../runtimes/claude/fixtures/session_start.json)
- [`runtimes/claude/fixtures/permission_request.json`](../runtimes/claude/fixtures/permission_request.json)
- [`runtimes/claude/fixtures/post_tool_use.json`](../runtimes/claude/fixtures/post_tool_use.json)
- [`runtimes/claude/fixtures/stop_failure.json`](../runtimes/claude/fixtures/stop_failure.json)
- [`runtimes/claude/fixtures/session_end.json`](../runtimes/claude/fixtures/session_end.json)

The helper preserves fields such as `permission_mode` and `reason` when Claude
provides them. It does not invent values when Claude omits them.

## What Agentic Control emits downstream

After translation, the Claude adapter emits one normalised event with:

- the shared contract fields from [event contract](contract.md)
- the native Claude hook name
- native session, tool, and approval metadata when Claude provides it
- any optional `bindings` requested by your host application

That gives you a stable app-facing contract while still preserving the runtime
fields you need for approval tracking, resume workflows, and investigation.

## Install the Claude bundle

The Claude bundle defaults to a repo-local sidecar layout instead of writing to
an existing `.claude/settings.json` file:

```bash
.artifacts/bin/agent_harness install --runtime claude --scope repo \
  --socket-env AGENT_HARNESS_SOCKET \
  --bind-env launch_id=APP_LAUNCH_ID \
  --bind-env app_session_id=APP_SESSION_ID \
  --bind-env actor_id=APP_ACTOR_ID \
  --bind-env host_id=APP_HOST_ID
```

That writes:

- `.agentic-control/claude/settings.json`
- `.agentic-control/claude/bin/agent_harness`

If you explicitly want a global sidecar install:

```bash
.artifacts/bin/agent_harness install --runtime claude --scope global \
  --socket-env AGENT_HARNESS_SOCKET
```

That global path writes:

- `~/.claude/agentic-control/settings.json`
- `~/.claude/agentic-control/bin/agent_harness`

The settings path honours `CLAUDE_HOME` if you override it.

To remove the Claude sidecar bundle later, run:

```bash
.artifacts/bin/agent_harness uninstall --runtime claude --scope repo
```

## Launch-time contract

The recommended launch path is explicit:

```bash
claude --settings ./.agentic-control/claude/settings.json
```

That keeps the hook wiring self-contained and avoids editing any existing
Claude settings file unless you explicitly choose to merge the generated
sidecar yourself.

The installer embeds whatever helper flags you pass after `--runtime` and
`--scope`. That is how you attach app-owned correlation values without
teaching the helper anything about your internal naming model.

If your application already knows the socket path directly, you can use
`--socket-path` instead of `--socket-env`.

## Approval-aware behaviour

Claude is the first runtime in this repository that exposes a dedicated
`PermissionRequest` hook in addition to `PreToolUse`.

That distinction matters:

- `PreToolUse` tells you a tool is about to run.
- `PermissionRequest` tells you the runtime is about to ask the user to allow
  that tool.
- `PostToolUse` and `PostToolUseFailure` tell you how the tool ended.

That makes Claude particularly useful for approval-aware investigation because
you can distinguish tool planning, permission gating, and final execution
outcomes without relying on screen scraping.

## Live testing

For quick fixture replay:

```bash
mise run diag:fixtures:claude
```

For repo-local installation:

```bash
mise run diag:install:claude
```

For a real interactive session:

```bash
mise run diag:claude:smoke
mise run diag:claude:bash
mise run diag:claude:approval
```

Those tasks keep Claude in an approval-capable mode. They are intentionally not
short-circuit paths because the point of the harness is to verify real tool and
approval behaviour.

Watch a live Claude session in two terminals:

```bash
# Terminal 1
cd ~/code/personal/agentic-control
.artifacts/bin/agent_harness listen --socket-path /tmp/agent-harness-claude.sock
```

```bash
# Terminal 2
cd ~/code/personal/agentic-control
.artifacts/bin/agent_harness install --runtime claude --scope repo --socket-env AGENT_HARNESS_SOCKET
PROMPT="$(cat runtimes/claude/prompts/approval.md)"
AGENT_HARNESS_SOCKET=/tmp/agent-harness-claude.sock \
claude --settings ./.agentic-control/claude/settings.json \
  --permission-mode default "$PROMPT"
```
