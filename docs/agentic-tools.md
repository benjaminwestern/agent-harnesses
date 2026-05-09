# Agentic Tools & Workspace Primitives

## Summary

Agentic Control provides a native, highly granular suite of workspace coordination tools (primitives) specifically designed for model-scale AI orchestration.

- Package: `pkg/contract` and `internal/controlplane`
- Runtime layer: `pkg/controlplane`
- Persistence: `workspace.db` (SQLite)
- Protocol: Model Context Protocol (MCP) & JSON-RPC

These primitives are not a separate product; they are native capabilities integrated directly into the `agentic-control` control plane. They exist to replace external coordination apps and give AI workers safe, predictable, and highly targeted ways to manipulate shared workspace state.

## Ownership

### The Workspace Store owns
- High-performance SQLite persistence with WAL mode.
- Document optimistic concurrency control via `revision` tracking.
- Schema migrations for the 5 core primitives.
- Cross-agent lease (lock) management and expiry.

### The JSON-RPC Control Plane owns
- Secure, socket-based exposure of all CRUD and granular operations.
- Translating incoming requests into workspace store operations.
- Maintaining the single source of truth for the local machine.

### The MCP Server owns
- The standard `2024-11-05` Model Context Protocol lifecycle over `stdio`.
- Translating `agentic-control` Documents into MCP Resources (`resources/*`).
- Translating the granular operational endpoints into fully typed, self-describing MCP Tools (`tools/*`).
- Trapping tool logic failures and safely returning them to the LLM via standard `isError: true` tool results (rather than crashing the protocol with JSON-RPC errors).

## The Five Core Primitives

We built the core capabilities of an ideal AI coordination layer down to five robust primitives:

1. **Memory (Shared KV State)**
   - Used for workspace variables, lookup tables, and small shared cursors.
2. **Documents (Durable Text Buffers)**
   - Used for storing large artifacts, code snippets, or plans. They use revision checks to prevent AI agents from accidentally overwriting each other's work.
3. **Tasks (Cross-Agent Run Boards)**
   - Used to track discrete units of work, manage status (open, in_progress, blocked, completed), assign actors, and link blockers. They also support threaded `TaskComments` to capture dialogue or execution logs.
4. **Wakeups (Durable Timers)**
   - Used to persist future reminders for callers or external schedulers. Wakeups can be paused, resumed, cancelled, or reset.
5. **Leases (Distributed Locks)**
   - Used for safe cross-agent coordination. Agents acquire a lock with an expiry time before modifying shared physical files or executing critical path operations.

## Granular AI "Dumb" Endpoints

A core design principle of Agentic Tools is that **AI needs dumb, granular endpoints to be effective**.

Instead of forcing an LLM to read a massive JSON object, modify a single array, and push the entire object back correctly (which is highly prone to hallucination and context loss), we exposed highly targeted "dumb endpoints".

These allow agents to confidently manipulate discrete elements:
- `documents_append`: Append a string to a document without rewriting the whole file.
- `add_metadata`: Add or update a specific JSON key/value pair on a Document or Task.
- `documents_rename`: Safely update just the title.
- `tasks_add_tag` / `tasks_remove_tag`: Push or pull a single string tag safely.
- `tasks_add_blocker` / `tasks_remove_blocker`: Target and append a single task ID to the blockers list.
- `tasks_lock` / `tasks_unlock`: Lock a task with a specific `actor_id`.

## Triple Exposure Surfaces

The Agentic Tools are available everywhere they are needed.

### 1. JSON-RPC Socket
All primitives and granular endpoints are exposed over the internal Unix socket (`/tmp/agentic-control.sock`), exactly like `StartSession` or `SendInput`.

### 2. Native CLI
Humans and shell scripts can interact with the state locally using standard commands:
```bash
agent_control tasks create "Implement new auth flow"
agent_control tasks add-blocker "task-123" "task-456"
agent_control memory set "api_url" "https://localhost:8080"
agent_control documents write "design-spec" "# Title..."
```

### 3. Native MCP Server
The `agentic-control` binary can be run as a standard MCP server without a third-party MCP SDK.

```bash
agent_control mcp --workspace="default"
```

Once started, any LLM connecting to it over `stdio` will instantly understand exactly how to manipulate the workspace in tiny, predictable chunks via the dynamically generated tool and resource schemas.
