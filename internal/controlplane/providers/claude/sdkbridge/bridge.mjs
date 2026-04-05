import readline from "node:readline";
import { randomUUID } from "node:crypto";

import { query } from "@anthropic-ai/claude-agent-sdk";

class AsyncQueue {
  constructor() {
    this.items = [];
    this.waiters = [];
    this.closed = false;
  }

  push(item) {
    if (this.closed) {
      throw new Error("input queue is closed");
    }
    if (this.waiters.length > 0) {
      const resolve = this.waiters.shift();
      resolve({ value: item, done: false });
      return;
    }
    this.items.push(item);
  }

  close() {
    this.closed = true;
    while (this.waiters.length > 0) {
      const resolve = this.waiters.shift();
      resolve({ value: undefined, done: true });
    }
  }

  [Symbol.asyncIterator]() {
    return {
      next: () => {
        if (this.items.length > 0) {
          return Promise.resolve({ value: this.items.shift(), done: false });
        }
        if (this.closed) {
          return Promise.resolve({ value: undefined, done: true });
        }
        return new Promise((resolve) => {
          this.waiters.push(resolve);
        });
      },
    };
  }
}

class ClaudeBridge {
  constructor() {
    this.activeQuery = null;
    this.inputQueue = null;
    this.startConfig = null;
    this.pendingRequests = new Map();
    this.requestCounter = 0;
    this.writeChain = Promise.resolve();
    this.sessionID = "";
    this.model = "";
    this.cwd = "";
    this.closed = false;
  }

  async sendMessage(message) {
    const line = `${JSON.stringify(message)}\n`;
    this.writeChain = this.writeChain.then(
      () =>
        new Promise((resolve, reject) => {
          process.stdout.write(line, (error) => {
            if (error) {
              reject(error);
              return;
            }
            resolve();
          });
        }),
    );
    return this.writeChain;
  }

  async respond(id, result) {
    await this.sendMessage({ id, result });
  }

  async respondError(id, error) {
    await this.sendMessage({
      id,
      error: {
        code: -32000,
        message: error instanceof Error ? error.message : String(error),
      },
    });
  }

  async notify(method, params) {
    await this.sendMessage({ method, params });
  }

  assertQuery() {
    if (!this.activeQuery || !this.inputQueue) {
      throw new Error("Claude session is not initialized");
    }
  }

  nextRequestID() {
    this.requestCounter += 1;
    return `claude-req-${this.requestCounter}`;
  }

  async startSession(params = {}) {
    if (this.activeQuery || this.startConfig) {
      throw new Error("Claude session is already initialized");
    }

    this.startConfig = {
      cwd: typeof params.cwd === "string" ? params.cwd : "",
      model: typeof params.model === "string" ? params.model : "",
      sessionID:
        typeof params.session_id === "string" && params.session_id.trim() !== ""
          ? params.session_id
          : randomUUID(),
      resumeSessionID:
        typeof params.resume_session_id === "string" ? params.resume_session_id : "",
      claudePath:
        typeof params.claude_path === "string" ? params.claude_path : "",
    };

    return {
      session_id: this.startConfig.resumeSessionID || this.startConfig.sessionID,
      model: this.startConfig.model,
      cwd: this.startConfig.cwd,
    };
  }

  ensureStarted() {
    if (this.activeQuery) {
      return;
    }
    if (!this.startConfig) {
      throw new Error("Claude session is not initialized");
    }

    this.inputQueue = new AsyncQueue();
    const options = {
      includePartialMessages: true,
      canUseTool: (toolName, input, toolOptions) =>
        this.handleToolRequest(toolName, input, toolOptions),
    };
    if (this.startConfig.cwd.trim() !== "") {
      options.cwd = this.startConfig.cwd;
    }
    if (this.startConfig.model.trim() !== "") {
      options.model = this.startConfig.model;
    }
    if (this.startConfig.resumeSessionID.trim() !== "") {
      options.resume = this.startConfig.resumeSessionID;
    } else if (this.startConfig.sessionID.trim() !== "") {
      options.sessionId = this.startConfig.sessionID;
    }
    if (this.startConfig.claudePath.trim() !== "") {
      options.pathToClaudeCodeExecutable = this.startConfig.claudePath;
    }

    this.activeQuery = query({
      prompt: this.inputQueue,
      options,
    });

    this.consumeQuery(this.activeQuery).catch(async (error) => {
      await this.notify("runtime.error", {
        message: error instanceof Error ? error.message : String(error),
      });
    });
  }

  async consumeQuery(activeQuery) {
    try {
      for await (const message of activeQuery) {
        if (typeof message.session_id === "string" && message.session_id !== "") {
          this.sessionID = message.session_id;
        }
        if (message.type === "system" && message.subtype === "init") {
          this.sessionID = message.session_id ?? this.sessionID;
          this.model = message.model ?? this.model;
          this.cwd = message.cwd ?? this.cwd;
        }

        await this.notify("sdk.message", message);
      }
    } catch (error) {
      throw error;
    } finally {
      this.closePendingRequests("session closed");
      this.activeQuery = null;
    }
  }

  closePendingRequests(reason) {
    for (const [requestID, pending] of this.pendingRequests.entries()) {
      this.pendingRequests.delete(requestID);
      pending.resolve({
        behavior: "deny",
        message: reason,
        interrupt: true,
        toolUseID: pending.toolUseID,
        decisionClassification: "user_reject",
      });
      void this.notify("request.closed", {
        request_id: requestID,
        reason,
      });
    }
  }

  async handleToolRequest(toolName, input, toolOptions) {
    const requestID = this.nextRequestID();
    const metadata = {
      tool_name: toolName,
      tool_input: input,
      suggestions: toolOptions.suggestions ?? [],
      blocked_path: toolOptions.blockedPath ?? "",
      decision_reason: toolOptions.decisionReason ?? "",
      title: toolOptions.title ?? "",
      display_name: toolOptions.displayName ?? "",
      description: toolOptions.description ?? "",
      tool_use_id: toolOptions.toolUseID,
      agent_id: toolOptions.agentID ?? "",
    };
    if (
      toolName === "AskUserQuestion" &&
      input &&
      typeof input === "object" &&
      Array.isArray(input.questions)
    ) {
      metadata.questions = input.questions;
    }

    const summary =
      metadata.title ||
      (toolName === "AskUserQuestion"
        ? "Claude requested user input"
        : `Claude requested tool approval: ${toolName}`);
    const kind =
      toolName === "AskUserQuestion" ? "user_input.tool" : "approval.tool";

    const responsePromise = new Promise((resolve) => {
      const abort = () => {
        if (!this.pendingRequests.has(requestID)) {
          return;
        }
        this.pendingRequests.delete(requestID);
        void this.notify("request.closed", {
          request_id: requestID,
          reason: "aborted",
        });
        resolve({
          behavior: "deny",
          message: "Interrupted by host application",
          interrupt: true,
          toolUseID: toolOptions.toolUseID,
          decisionClassification: "user_reject",
        });
      };

      toolOptions.signal.addEventListener("abort", abort, { once: true });
      this.pendingRequests.set(requestID, {
        resolve: (result) => {
          toolOptions.signal.removeEventListener("abort", abort);
          resolve(result);
        },
        kind,
        metadata,
        summary,
        toolUseID: toolOptions.toolUseID,
      });
    });

    await this.notify("request.opened", {
      request_id: requestID,
      kind,
      native_method: "canUseTool",
      summary,
      created_at_ms: Date.now(),
      metadata,
    });

    return responsePromise;
  }

  async handleRespond(params = {}) {
    const requestID = params.request_id;
    if (typeof requestID !== "string" || requestID.trim() === "") {
      throw new Error("session.respond requires request_id");
    }

    const pending = this.pendingRequests.get(requestID);
    if (!pending) {
      throw new Error("unknown pending Claude request");
    }

    const result = buildResponseResult(pending, params);
    this.pendingRequests.delete(requestID);
    pending.resolve(result);
  }

  async handleRequest(request) {
    const { id, method, params } = request;

    try {
      switch (method) {
        case "session.start": {
          const result = await this.startSession(params);
          await this.respond(id, result);
          return;
        }
        case "session.resume": {
          const result = await this.startSession(params);
          await this.respond(id, result);
          return;
        }
        case "session.send": {
          this.ensureStarted();
          this.assertQuery();
          const text = typeof params?.text === "string" ? params.text : "";
          if (text.trim() === "") {
            throw new Error("session.send requires text");
          }
          this.inputQueue.push({
            type: "user",
            message: {
              role: "user",
              content: text,
            },
            parent_tool_use_id: null,
          });
          await this.respond(id, { queued: true });
          return;
        }
        case "session.interrupt": {
          this.assertQuery();
          await this.activeQuery.interrupt();
          await this.respond(id, { interrupted: true });
          return;
        }
        case "session.respond": {
          this.assertQuery();
          await this.handleRespond(params);
          await this.respond(id, { ok: true });
          return;
        }
        case "session.stop": {
          await this.shutdown();
          await this.respond(id, { stopped: true });
          return;
        }
        default:
          throw new Error(`unsupported method: ${method}`);
      }
    } catch (error) {
      await this.respondError(id, error);
    }
  }

  async shutdown() {
    if (this.closed) {
      return;
    }
    this.closed = true;
    this.closePendingRequests("session closed");
    if (this.inputQueue) {
      this.inputQueue.close();
    }
    if (this.activeQuery) {
      this.activeQuery.close();
      this.activeQuery = null;
    }
    this.startConfig = null;
  }
}

function normalizeAction(action) {
  const value = String(action ?? "").trim().toLowerCase();
  switch (value) {
    case "allow":
    case "approve":
    case "approved":
    case "yes":
      return "allow";
    case "always":
    case "always_allow":
    case "approve_always":
    case "allow_always":
    case "remember_allow":
      return "allow_always";
    case "deny":
    case "reject":
    case "decline":
    case "no":
    case "cancel":
      return "deny";
    default:
      return value;
  }
}

function buildPermissionResult(pending, params) {
  const action = normalizeAction(params.action);
  const metadata = params.metadata && typeof params.metadata === "object" ? params.metadata : {};

  if (action === "allow" || action === "allow_always") {
    const result = {
      behavior: "allow",
      updatedInput: {},
      toolUseID: pending.toolUseID,
      decisionClassification:
        action === "allow_always" ? "user_permanent" : "user_temporary",
    };

    if (
      metadata.updated_input &&
      typeof metadata.updated_input === "object" &&
      !Array.isArray(metadata.updated_input)
    ) {
      result.updatedInput = metadata.updated_input;
    }

    const updatedPermissions = Array.isArray(metadata.updated_permissions)
      ? metadata.updated_permissions
      : pending.metadata.suggestions;
    if (action === "allow_always" && Array.isArray(updatedPermissions) && updatedPermissions.length > 0) {
      result.updatedPermissions = updatedPermissions;
    }

    return result;
  }

  return {
    behavior: "deny",
    message:
      typeof params.text === "string" && params.text.trim() !== ""
        ? params.text.trim()
        : "Denied by host application",
    interrupt: Boolean(metadata.interrupt),
    toolUseID: pending.toolUseID,
    decisionClassification: "user_reject",
  };
}

function buildAskUserQuestionResult(pending, params) {
  const action = normalizeAction(params.action);
  if (action === "deny") {
    return {
      behavior: "deny",
      message:
        typeof params.text === "string" && params.text.trim() !== ""
          ? params.text.trim()
          : "Denied by host application",
      interrupt: true,
      toolUseID: pending.toolUseID,
      decisionClassification: "user_reject",
    };
  }

  const metadata = params.metadata && typeof params.metadata === "object" ? params.metadata : {};
  const questions = Array.isArray(pending.metadata.questions)
    ? pending.metadata.questions
    : [];

  let answers = null;
  if (
    metadata.answers &&
    typeof metadata.answers === "object" &&
    !Array.isArray(metadata.answers)
  ) {
    answers = metadata.answers;
  } else if (typeof params.text === "string" && params.text.trim() !== "") {
    if (questions.length === 0 || typeof questions[0].question !== "string") {
      throw new Error(
        "AskUserQuestion response requires metadata.answers when no question metadata is available",
      );
    }
    answers = {
      [questions[0].question]: params.text.trim(),
    };
  }

  if (!answers) {
    throw new Error("AskUserQuestion response requires metadata.answers or text");
  }

  return {
    behavior: "allow",
    toolUseID: pending.toolUseID,
    decisionClassification: "user_temporary",
    updatedInput: {
      questions,
      answers,
    },
  };
}

function buildResponseResult(pending, params) {
  if (pending.kind === "user_input.tool") {
    return buildAskUserQuestionResult(pending, params);
  }
  return buildPermissionResult(pending, params);
}

const bridge = new ClaudeBridge();

process.on("SIGTERM", async () => {
  await bridge.shutdown();
  process.exit(0);
});

process.on("SIGINT", async () => {
  await bridge.shutdown();
  process.exit(0);
});

const input = readline.createInterface({
  input: process.stdin,
  crlfDelay: Infinity,
});

for await (const line of input) {
  const trimmed = line.trim();
  if (trimmed === "") {
    continue;
  }

  try {
    const request = JSON.parse(trimmed);
    await bridge.handleRequest(request);
  } catch (error) {
    await bridge.sendMessage({
      error: {
        code: -32700,
        message: error instanceof Error ? error.message : String(error),
      },
    });
  }
}

await bridge.shutdown();
