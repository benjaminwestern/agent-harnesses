# Smoke Testing

## Summary

Agentic Control has a first-class CLI smoke command:

```bash
agent_control smoke [--socket-path <path>]
```

This runs a small real provider/runtime matrix through the same orchestration
surface that product features use.

## Default Matrix

The default smoke targets are:

- `claude=claude-sonnet-4-6`
- `gemini=gemini-3-flash-preview`
- `codex=gpt-5.4`
- `opencode=google/gemini-3-flash-preview`
- `opencode=openai/gpt-5.4`

## Why This Exists

Builds and unit tests are not enough for this product.

We need a repeatable way to verify:

- runtime availability;
- model validation;
- session creation;
- turn completion;
- normalized text output;
- token usage surfaces;
- cost surfaces where available.

## Live Behavior

At the time of writing, the smoke matrix verifies that all five default targets
return the expected text successfully.

- Claude: normalized token usage is available;
- Gemini: normalized token usage is available;
- Codex: normalized token usage is available;
- OpenCode Google/OpenAI: normalized token usage is available.

Cost visibility is provider-dependent. OpenCode Google exposes the strongest
cost signal in the default matrix.

## Recommended Usage

Use smoke checks:

- after runtime adapter changes;
- after orchestration changes;
- after session/thread persistence changes;
- before claiming a provider surface is stable.

## Example

```bash
agent_control serve --socket-path /tmp/agentic-control.sock
agent_control wait-ready --socket-path /tmp/agentic-control.sock
agent_control smoke --socket-path /tmp/agentic-control.sock
```

Typed selection default path:

```bash
agent_control smoke \
  --socket-path /tmp/agentic-control.sock \
  --provider opencode \
  --model-selection google/gemini-3-flash-preview
```

That path uses the same typed selection contract that powers
`orchestrate` and `court run`, so the smoke surface validates the same core
selection logic the rest of the product uses.

Use `wait-ready` in scripts and local smoke flows to avoid startup races between
the daemon creating the socket and being ready to answer JSON-RPC calls.

## Advanced Usage

You can override the matrix with repeated `--target` flags or use the typed
selection path.

```bash
agent_control smoke \
  --socket-path /tmp/agentic-control.sock \
  --target gemini=gemini-3-flash-preview \
  --target opencode=google/gemini-3-flash-preview
```
