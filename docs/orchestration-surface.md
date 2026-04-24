# Orchestration Surface

## Summary

Agentic Control has a native non-Court orchestration surface:

- `agent_control orchestrate fanout`
- `agent_control orchestrate compare`
- `agent_control orchestrate summarize`
- `agent_control orchestrate best-of-n`

This is the first concrete proof that `internal/orchestration` is a product
layer, not just an extraction destination for Court internals.

## First Success

```bash
agent_control serve --socket-path /tmp/agentic-control.sock
agent_control wait-ready --socket-path /tmp/agentic-control.sock

agent_control orchestrate fanout \
  --socket-path /tmp/agentic-control.sock \
  --task "summarize this repository" \
  --provider opencode \
  --model-selection google/gemini-3-flash-preview
```

If no targets are specified, fan-out defaults to all available session-capable
runtimes using their default models.

## Feature Development Flow

If you want multiple implementation attempts for the same task, use compare with
repeat:

```bash
agent_control orchestrate compare \
  --socket-path /tmp/agentic-control.sock \
  --task "implement feature X" \
  --provider opencode \
  --model-selection google/gemini-3-flash-preview \
  --repeat 3
```

That generic flow gives you:

- three attempts;
- per-attempt outputs;
- per-attempt tracked sessions;
- per-attempt token usage;
- one reducer pass that compares the attempts together.

Court can then build a more opinionated semantic review flow on top of the same
substrate.

Generic orchestration may compare, summarize, or select candidates, but Court's
semantic handoff flow should remain stricter: a default continuation should only
be chosen automatically when Court has an explicit semantic winner.

## Explicit Targeting

Users can override the default fan-out target set with:

- provider-specific selection flags;
- repeated `--selection-json` values;
- repeated legacy `--target` flags when needed.

The default path is:

1. provider-specific selection flags
2. `--selection-json` for fully specified typed selections
3. raw `--target` only when low-level compatibility is needed

Examples:

```bash
agent_control orchestrate fanout \
  --task "compare these backends" \
  --target opencode \
  --target gemini \
  --target codex
```

```bash
agent_control orchestrate fanout \
  --socket-path /tmp/agentic-control.sock \
  --task "compare models" \
  --selection-json '{"provider":"opencode","model":"openai/gpt-5.4"}' \
  --selection-json '{"provider":"opencode","model":"google/gemini-3-flash-preview"}'
```

## Advanced Dials

Advanced model controls are available as explicit flags and apply to all fan-out
targets unless a target-specific override surface is added:

- `--reasoning-effort`
- `--thinking-level`
- `--thinking-budget`
- `--keep-sessions`

## Output Model

The fan-out result reports:

- resolved target list;
- final text per target;
- session details where available;
- per-target recorded token usage;
- aggregate token usage across targets.

This reuses the same session ledger and token economics machinery that Court
uses indirectly.

Reduction commands additionally return:

- a reducer session;
- reducer token usage;
- reducer cost where available;
- reduced structured output for compare, summarize, or best-of-N.

When used with `--keep-sessions` and a daemon `--socket-path`, the resulting
threads can be:

- listed;
- named;
- annotated with metadata;
- logically forked into child threads.

## Why This Matters

This flow proves the intended layering:

- Agentic Control core owns generic orchestration;
- Court is the advanced semantic review layer on top;
- token/session economics belong to the shared substrate and are reused in both.

## Recommended Next UX Work

1. Add a UI workflow card for fan-out orchestration.
2. Add per-target status streaming in the shell.
3. Surface recorded token usage and cost estimates more prominently.
4. Add target labels and richer reducer prompts if feature-development flows need
   more review structure.
