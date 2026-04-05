package harness

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/benjaminwestern/agentic-control/pkg/contract"
)

type Runtime string

var errHelpRequested = errors.New("help requested")

const (
	RuntimeCodex    Runtime = "codex"
	RuntimeGemini   Runtime = "gemini"
	RuntimeClaude   Runtime = "claude"
	RuntimeOpenCode Runtime = "opencode"
)

type bindingSpecKind int

const (
	bindingEnv bindingSpecKind = iota
	bindingValue
)

type BindingSpec struct {
	Kind  bindingSpecKind
	Key   string
	Value string
}

type EmitOptions struct {
	Runtime      Runtime
	Provenance   string
	SocketPath   string
	SocketEnv    string
	EmitStdout   bool
	SuccessJSON  string
	BindingSpecs []BindingSpec
}

type ListenOptions struct {
	SocketPath string
}

type ToolInput struct {
	Command *string `json:"command"`
}

type ToolOutput struct {
	ExitCode *int    `json:"exit_code"`
	Stdout   *string `json:"stdout"`
	Stderr   *string `json:"stderr"`
}

type CodexHookPayload struct {
	SessionID      *string     `json:"session_id"`
	TranscriptPath *string     `json:"transcript_path"`
	CWD            *string     `json:"cwd"`
	HookEventName  *string     `json:"hook_event_name"`
	Model          *string     `json:"model"`
	Source         *string     `json:"source"`
	TurnID         *string     `json:"turn_id"`
	UserPrompt     *string     `json:"user_prompt"`
	ToolName       *string     `json:"tool_name"`
	ToolUseID      *string     `json:"tool_use_id"`
	ToolInput      *ToolInput  `json:"tool_input"`
	ToolOutput     *ToolOutput `json:"tool_output"`
	StopHookActive *bool       `json:"stop_hook_active"`
}

func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		printUsage(stderr)
		return errors.New("invalid arguments")
	}
	if args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		printUsage(stdout)
		return nil
	}

	switch args[0] {
	case "emit":
		options, err := parseEmitArgs(args[1:])
		if err != nil {
			if errors.Is(err, errHelpRequested) {
				printUsage(stdout)
				return nil
			}
			printUsage(stderr)
			return err
		}
		return emitEvent(stdin, stdout, options)
	case "listen":
		options, err := parseListenArgs(args[1:])
		if err != nil {
			printUsage(stderr)
			return err
		}
		return runListener(options, stdout)
	case "install":
		return runInstall(args[1:], stdout, stderr)
	case "uninstall":
		return runUninstall(args[1:], stdout, stderr)
	case "run":
		return runRuntime(args[1:], stdout, stderr)
	}

	options, err := parseEmitArgs(args)
	if err != nil {
		if errors.Is(err, errHelpRequested) {
			printUsage(stdout)
			return nil
		}
		printUsage(stderr)
		return err
	}
	return emitEvent(stdin, stdout, options)
}

func printUsage(w io.Writer) {
	fmt.Fprint(w,
		"Usage:\n"+
			"  agent_harness emit --runtime <codex|gemini|claude|opencode> [emit flags]\n"+
			"  agent_harness install --runtime <codex|gemini|claude|opencode> [--scope repo|global] [helper flags]\n"+
			"  agent_harness uninstall --runtime <codex|gemini|claude|opencode> [--scope repo|global]\n"+
			"  agent_harness run --runtime <codex|gemini|claude|opencode> [--scenario smoke|bash|approval] [run flags]\n"+
			"  agent_harness listen --socket-path <path>\n\n"+
			"Primary use cases:\n"+
			"  install     Safely add only Agentic Control-managed hook or plugin config.\n"+
			"  uninstall   Remove only Agentic Control-managed hook or plugin config.\n"+
			"  listen      Inspect passive hook or plugin events locally.\n"+
			"  run         Diagnostic live-run helper for repo scenarios.\n"+
			"  emit        Low-level runtime entrypoint for hooks or plugins.\n\n"+
			"Helper flags accepted by install:\n"+
			"  --socket-path <path>\n"+
			"  --socket-env <ENV_NAME>\n"+
			"  --bind-env <key=ENV_NAME>\n"+
			"  --bind-value <key=value>\n"+
			"  --provenance <label>\n"+
			"  --stdout\n"+
			"  --success-json <payload>\n\n"+
			"Diagnostic run flags:\n"+
			"  --scenario smoke|bash|approval\n"+
			"  --prompt-file <path>\n"+
			"  --approval-policy <policy>\n"+
			"  --sandbox <mode>\n"+
			"  --gemini-approval-mode <mode>\n"+
			"  --claude-permission-mode <mode>\n"+
			"  --extra-arg <arg>\n\n"+
			"For backwards compatibility, the bare emit form also works:\n"+
			"  agent_harness --runtime <codex|gemini|claude|opencode> [emit flags]\n")
}

func parseEmitArgs(args []string) (EmitOptions, error) {
	options := EmitOptions{
		Provenance: "native_hook",
	}

	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--runtime":
			index++
			if index >= len(args) {
				return EmitOptions{}, errors.New("missing runtime")
			}
			runtime, err := parseRuntime(args[index])
			if err != nil {
				return EmitOptions{}, err
			}
			options.Runtime = runtime
		case "--provenance":
			index++
			if index >= len(args) {
				return EmitOptions{}, errors.New("missing provenance")
			}
			options.Provenance = args[index]
		case "--socket-path":
			index++
			if index >= len(args) {
				return EmitOptions{}, errors.New("missing socket path")
			}
			options.SocketPath = args[index]
		case "--socket-env":
			index++
			if index >= len(args) {
				return EmitOptions{}, errors.New("missing socket env")
			}
			options.SocketEnv = args[index]
		case "--stdout":
			options.EmitStdout = true
		case "--success-json":
			index++
			if index >= len(args) {
				return EmitOptions{}, errors.New("missing success json")
			}
			options.SuccessJSON = args[index]
		case "--bind-env":
			index++
			if index >= len(args) {
				return EmitOptions{}, errors.New("missing binding spec")
			}
			spec, err := parseBindingSpec(bindingEnv, args[index])
			if err != nil {
				return EmitOptions{}, err
			}
			options.BindingSpecs = append(options.BindingSpecs, spec)
		case "--bind-value":
			index++
			if index >= len(args) {
				return EmitOptions{}, errors.New("missing binding spec")
			}
			spec, err := parseBindingSpec(bindingValue, args[index])
			if err != nil {
				return EmitOptions{}, err
			}
			options.BindingSpecs = append(options.BindingSpecs, spec)
		case "--help", "-h":
			return EmitOptions{}, errHelpRequested
		default:
			return EmitOptions{}, fmt.Errorf("invalid argument: %s", args[index])
		}
	}

	if options.Runtime == "" {
		return EmitOptions{}, errors.New("missing runtime")
	}

	return options, nil
}

func parseListenArgs(args []string) (ListenOptions, error) {
	var options ListenOptions
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--socket-path":
			index++
			if index >= len(args) {
				return ListenOptions{}, errors.New("missing socket path")
			}
			options.SocketPath = args[index]
		default:
			return ListenOptions{}, fmt.Errorf("invalid argument: %s", args[index])
		}
	}
	if options.SocketPath == "" {
		return ListenOptions{}, errors.New("missing socket path")
	}
	return options, nil
}

func parseRuntime(value string) (Runtime, error) {
	switch value {
	case string(RuntimeCodex):
		return RuntimeCodex, nil
	case string(RuntimeGemini):
		return RuntimeGemini, nil
	case string(RuntimeClaude):
		return RuntimeClaude, nil
	case string(RuntimeOpenCode):
		return RuntimeOpenCode, nil
	default:
		return "", fmt.Errorf("invalid runtime: %s", value)
	}
}

func parseBindingSpec(kind bindingSpecKind, raw string) (BindingSpec, error) {
	index := strings.IndexByte(raw, '=')
	if index <= 0 || index+1 >= len(raw) {
		return BindingSpec{}, errors.New("invalid binding spec")
	}
	return BindingSpec{
		Kind:  kind,
		Key:   raw[:index],
		Value: raw[index+1:],
	}, nil
}

func emitEvent(stdin io.Reader, stdout io.Writer, options EmitOptions) error {
	payload, err := io.ReadAll(io.LimitReader(stdin, 1024*1024))
	if err != nil {
		return err
	}

	event, err := NormalizePayload(options.Runtime, options.Provenance, payload)
	if err != nil {
		return err
	}
	event.RuntimePID = detectRuntimePID()
	event.Bindings = collectBindings(options.BindingSpecs)

	line, err := json.Marshal(event)
	if err != nil {
		return err
	}
	line = append(line, '\n')

	if socketPath := resolveSocketPath(options); socketPath != "" {
		if err := sendToUnixSocket(socketPath, line); err != nil &&
			!errors.Is(err, os.ErrNotExist) &&
			!errors.Is(err, syscall.ENOENT) &&
			!errors.Is(err, syscall.ECONNREFUSED) {
			return err
		}
	}

	switch {
	case options.SuccessJSON != "":
		if _, err := fmt.Fprintln(stdout, options.SuccessJSON); err != nil {
			return err
		}
	case options.EmitStdout:
		if _, err := stdout.Write(line); err != nil {
			return err
		}
	}

	return nil
}

func NormalizePayload(runtime Runtime, provenance string, payload []byte) (*contract.HarnessEvent, error) {
	switch runtime {
	case RuntimeCodex:
		return normalizeCodexPayload(provenance, payload)
	case RuntimeGemini:
		return normalizeGeminiPayload(provenance, payload)
	case RuntimeClaude:
		return normalizeClaudePayload(provenance, payload)
	case RuntimeOpenCode:
		return normalizeOpenCodePayload(provenance, payload)
	default:
		return nil, fmt.Errorf("unsupported runtime: %s", runtime)
	}
}

func normalizeCodexPayload(provenance string, payload []byte) (*contract.HarnessEvent, error) {
	var decoded CodexHookPayload
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return nil, err
	}

	nativeEventName := valueOr(decoded.HookEventName, "Unknown")
	eventType := codexEventType(nativeEventName)
	summary := codexSummary(nativeEventName, decoded)

	var command *string
	if decoded.ToolInput != nil {
		command = decoded.ToolInput.Command
	}

	var exitCode *int
	if decoded.ToolOutput != nil {
		exitCode = decoded.ToolOutput.ExitCode
	}

	return &contract.HarnessEvent{
		SchemaVersion:   contract.HarnessSchemaVersion,
		RecordedAtMS:    time.Now().UnixMilli(),
		Runtime:         string(RuntimeCodex),
		Provenance:      provenance,
		NativeEventName: nativeEventName,
		EventType:       eventType,
		Summary:         summary,
		SessionID:       decoded.SessionID,
		TurnID:          decoded.TurnID,
		ToolCallID:      decoded.ToolUseID,
		ToolName:        decoded.ToolName,
		Command:         command,
		PromptText:      decoded.UserPrompt,
		CWD:             decoded.CWD,
		Model:           decoded.Model,
		TranscriptPath:  decoded.TranscriptPath,
		SessionSource:   decoded.Source,
		ExitCode:        exitCode,
		StopHookActive:  decoded.StopHookActive,
	}, nil
}

func normalizeGeminiPayload(provenance string, payload []byte) (*contract.HarnessEvent, error) {
	root, err := decodeJSONObject(payload)
	if err != nil {
		return nil, err
	}

	nativeEventName := valueOr(stringFromObject(root, "hook_event_name"), "Unknown")
	eventType := geminiEventType(nativeEventName)
	summary := geminiSummary(nativeEventName, root)

	toolInput := objectFromObject(root, "tool_input")
	toolOutput := objectFromObject(root, "tool_output")
	if toolOutput == nil {
		toolOutput = objectFromObject(root, "tool_result")
	}

	var command *string
	if toolInput != nil {
		command = stringFromObject(toolInput, "command")
	}

	var exitCode *int
	if toolOutput != nil {
		exitCode = intFromObject(toolOutput, "exit_code")
	}

	return &contract.HarnessEvent{
		SchemaVersion:   contract.HarnessSchemaVersion,
		RecordedAtMS:    time.Now().UnixMilli(),
		Runtime:         string(RuntimeGemini),
		Provenance:      provenance,
		NativeEventName: nativeEventName,
		EventType:       eventType,
		Summary:         summary,
		SessionID:       stringFromObject(root, "session_id"),
		TurnID:          firstNonNilString(stringFromObject(root, "invocation_id"), stringFromObject(root, "turn_id")),
		ToolCallID:      stringFromObject(root, "tool_call_id"),
		ToolName:        stringFromObject(root, "tool_name"),
		Command:         command,
		PromptText:      stringFromObject(root, "prompt"),
		CWD:             stringFromObject(root, "cwd"),
		Model:           stringFromObject(root, "model"),
		TranscriptPath:  stringFromObject(root, "transcript_path"),
		SessionSource:   stringFromObject(root, "source"),
		ExitCode:        exitCode,
	}, nil
}

func normalizeClaudePayload(provenance string, payload []byte) (*contract.HarnessEvent, error) {
	root, err := decodeJSONObject(payload)
	if err != nil {
		return nil, err
	}

	nativeEventName := valueOr(stringFromObject(root, "hook_event_name"), "Unknown")
	eventType := claudeEventType(nativeEventName)
	summary := claudeSummary(nativeEventName, root)

	toolInput := objectFromObject(root, "tool_input")
	toolResponse := objectFromObject(root, "tool_response")

	var command *string
	if toolInput != nil {
		command = stringFromObject(toolInput, "command")
	}

	var exitCode *int
	if toolResponse != nil {
		exitCode = intFromObject(toolResponse, "exit_code")
	}

	return &contract.HarnessEvent{
		SchemaVersion:   contract.HarnessSchemaVersion,
		RecordedAtMS:    time.Now().UnixMilli(),
		Runtime:         string(RuntimeClaude),
		Provenance:      provenance,
		NativeEventName: nativeEventName,
		EventType:       eventType,
		Summary:         summary,
		SessionID:       stringFromObject(root, "session_id"),
		TurnID:          stringFromObject(root, "turn_id"),
		ToolCallID:      stringFromObject(root, "tool_use_id"),
		ToolName:        stringFromObject(root, "tool_name"),
		Command:         command,
		PromptText:      stringFromObject(root, "prompt"),
		CWD:             stringFromObject(root, "cwd"),
		Model:           stringFromObject(root, "model"),
		TranscriptPath:  stringFromObject(root, "transcript_path"),
		SessionSource:   stringFromObject(root, "source"),
		PermissionMode:  stringFromObject(root, "permission_mode"),
		Reason:          firstNonNilString(stringFromObject(root, "reason"), stringFromObject(root, "error")),
		ExitCode:        exitCode,
		StopHookActive:  boolFromObject(root, "stop_hook_active"),
	}, nil
}

func normalizeOpenCodePayload(provenance string, payload []byte) (*contract.HarnessEvent, error) {
	root, err := decodeJSONObject(payload)
	if err != nil {
		return nil, err
	}

	nativeEventName := valueOr(stringFromObject(root, "hook_event_name"), "Unknown")
	eventType := openCodeEventType(nativeEventName)
	summary := openCodeSummary(nativeEventName, root)

	eventObject := objectFromObject(root, "event")
	input := objectFromObject(root, "input")
	output := objectFromObject(root, "output")
	var eventProperties map[string]any
	if eventObject != nil {
		eventProperties = objectFromObject(eventObject, "properties")
	}

	return &contract.HarnessEvent{
		SchemaVersion:   contract.HarnessSchemaVersion,
		RecordedAtMS:    time.Now().UnixMilli(),
		Runtime:         string(RuntimeOpenCode),
		Provenance:      provenance,
		NativeEventName: nativeEventName,
		EventType:       eventType,
		Summary:         summary,
		SessionID: firstNonNilString(
			stringFromObject(root, "session_id"),
			objectFieldString(input, "sessionID"),
			objectFieldString(eventProperties, "sessionID"),
		),
		TurnID: firstNonNilString(
			stringFromObject(root, "turn_id"),
			nestedString(eventProperties, "messageID"),
		),
		ToolCallID: firstNonNilString(
			stringFromObject(root, "tool_call_id"),
			objectFieldString(input, "callID"),
			nestedString(eventProperties, "tool", "callID"),
		),
		ToolName: firstNonNilString(
			stringFromObject(root, "tool_name"),
			objectFieldString(input, "tool"),
			objectFieldString(eventProperties, "permission"),
		),
		Command: firstNonNilString(
			stringFromObject(root, "command"),
			nestedString(output, "args", "command"),
			nestedString(input, "args", "command"),
			nestedString(eventProperties, "metadata", "command"),
			firstStringFromArrayField(eventProperties, "patterns"),
		),
		PromptText:     stringFromObject(root, "prompt_text"),
		CWD:            firstNonNilString(stringFromObject(root, "cwd"), stringFromObject(root, "directory")),
		Model:          stringFromObject(root, "model"),
		TranscriptPath: stringFromObject(root, "transcript_path"),
		SessionSource:  stringFromObject(root, "session_source"),
		PermissionMode: stringFromObject(root, "permission_mode"),
		Reason: firstNonNilString(
			stringFromObject(root, "reason"),
			objectFieldString(eventProperties, "reply"),
			nestedString(eventProperties, "error", "type"),
			nestedString(eventProperties, "error", "message"),
		),
		ExitCode: firstNonNilInt(
			intFromObject(root, "exit_code"),
			nestedInt(output, "metadata", "exitCode"),
			nestedInt(output, "metadata", "exit_code"),
		),
	}, nil
}

func collectBindings(specs []BindingSpec) map[string]string {
	if len(specs) == 0 {
		return nil
	}

	bindings := make(map[string]string)
	for _, spec := range specs {
		switch spec.Kind {
		case bindingEnv:
			if value, ok := os.LookupEnv(spec.Value); ok {
				bindings[spec.Key] = value
			}
		case bindingValue:
			bindings[spec.Key] = spec.Value
		}
	}
	if len(bindings) == 0 {
		return nil
	}
	return bindings
}

func detectRuntimePID() *int {
	parentPID := os.Getppid()
	if parentPID <= 0 {
		return nil
	}
	return intPtr(parentPID)
}

func resolveSocketPath(options EmitOptions) string {
	if options.SocketPath != "" {
		return options.SocketPath
	}
	if options.SocketEnv == "" {
		return ""
	}
	return os.Getenv(options.SocketEnv)
}

func codexEventType(nativeEventName string) string {
	switch nativeEventName {
	case "SessionStart":
		return "session.started"
	case "UserPromptSubmit":
		return "turn.user_prompt_submitted"
	case "PreToolUse":
		return "tool.started"
	case "PostToolUse":
		return "tool.finished"
	case "Stop":
		return "turn.stopped"
	default:
		return "runtime.event"
	}
}

func geminiEventType(nativeEventName string) string {
	switch nativeEventName {
	case "SessionStart":
		return "session.started"
	case "SessionEnd":
		return "session.ended"
	case "BeforeAgent":
		return "turn.user_prompt_submitted"
	case "AfterAgent":
		return "turn.finished"
	case "BeforeTool":
		return "tool.started"
	case "AfterTool":
		return "tool.finished"
	case "Notification":
		return "notification"
	default:
		return "runtime.event"
	}
}

func claudeEventType(nativeEventName string) string {
	switch nativeEventName {
	case "SessionStart":
		return "session.started"
	case "SessionEnd":
		return "session.ended"
	case "UserPromptSubmit":
		return "turn.user_prompt_submitted"
	case "PreToolUse":
		return "tool.started"
	case "PermissionRequest":
		return "tool.permission_requested"
	case "PostToolUse":
		return "tool.finished"
	case "PostToolUseFailure":
		return "tool.failed"
	case "Stop":
		return "turn.stopped"
	case "StopFailure":
		return "turn.failed"
	case "Notification":
		return "notification"
	default:
		return "runtime.event"
	}
}

func openCodeEventType(nativeEventName string) string {
	switch nativeEventName {
	case "session.created":
		return "session.started"
	case "session.idle":
		return "turn.finished"
	case "session.error":
		return "turn.failed"
	case "permission.asked":
		return "tool.permission_requested"
	case "tool.execute.before":
		return "tool.started"
	case "tool.execute.after":
		return "tool.finished"
	default:
		return "runtime.event"
	}
}

func codexSummary(nativeEventName string, payload CodexHookPayload) string {
	switch nativeEventName {
	case "SessionStart":
		return fmt.Sprintf("Codex session started via %s", valueOr(payload.Source, "startup"))
	case "UserPromptSubmit":
		if payload.UserPrompt != nil {
			return fmt.Sprintf("User prompt submitted: %s", truncate(*payload.UserPrompt, 120))
		}
		return "User prompt submitted"
	case "PreToolUse":
		if payload.ToolInput != nil && payload.ToolInput.Command != nil {
			return fmt.Sprintf("About to run Bash: %s", truncate(*payload.ToolInput.Command, 160))
		}
		return "About to run Bash"
	case "PostToolUse":
		if payload.ToolOutput != nil && payload.ToolOutput.ExitCode != nil {
			return fmt.Sprintf("Finished Bash tool with exit code %d", *payload.ToolOutput.ExitCode)
		}
		return "Finished Bash tool"
	case "Stop":
		if payload.StopHookActive != nil {
			return fmt.Sprintf("Codex turn stopped (hook active: %t)", *payload.StopHookActive)
		}
		return "Codex turn stopped"
	default:
		return nativeEventName
	}
}

func geminiSummary(nativeEventName string, root map[string]any) string {
	switch nativeEventName {
	case "SessionStart":
		return "Gemini session started"
	case "SessionEnd":
		return "Gemini session ended"
	case "BeforeAgent":
		if prompt := stringFromObject(root, "prompt"); prompt != nil {
			return fmt.Sprintf("Gemini prompt submitted: %s", truncate(*prompt, 120))
		}
		return "Gemini prompt submitted"
	case "AfterAgent":
		if response := stringFromObject(root, "response"); response != nil {
			return fmt.Sprintf("Gemini agent turn finished: %s", truncate(*response, 120))
		}
		return "Gemini agent turn finished"
	case "BeforeTool":
		toolName := valueOr(stringFromObject(root, "tool_name"), "tool")
		if input := objectFromObject(root, "tool_input"); input != nil {
			if command := stringFromObject(input, "command"); command != nil {
				return fmt.Sprintf("Gemini about to run %s: %s", toolName, truncate(*command, 160))
			}
		}
		return fmt.Sprintf("Gemini about to run %s", toolName)
	case "AfterTool":
		toolName := valueOr(stringFromObject(root, "tool_name"), "tool")
		if result := objectFromObject(root, "tool_result"); result != nil {
			if content := stringFromObject(result, "llmContent"); content != nil {
				return fmt.Sprintf("Gemini finished %s: %s", toolName, truncate(*content, 120))
			}
			if display := stringFromObject(result, "returnDisplay"); display != nil {
				return fmt.Sprintf("Gemini finished %s: %s", toolName, truncate(*display, 120))
			}
		}
		return fmt.Sprintf("Gemini finished %s", toolName)
	case "Notification":
		if message := stringFromObject(root, "message"); message != nil {
			return fmt.Sprintf("Gemini notification: %s", truncate(*message, 120))
		}
		if kind := stringFromObject(root, "type"); kind != nil {
			return fmt.Sprintf("Gemini notification: %s", *kind)
		}
		return "Gemini notification"
	default:
		return nativeEventName
	}
}

func claudeSummary(nativeEventName string, root map[string]any) string {
	switch nativeEventName {
	case "SessionStart":
		return fmt.Sprintf("Claude session started via %s", valueOr(stringFromObject(root, "source"), "startup"))
	case "SessionEnd":
		return fmt.Sprintf("Claude session ended: %s", valueOr(stringFromObject(root, "reason"), "other"))
	case "UserPromptSubmit":
		if prompt := stringFromObject(root, "prompt"); prompt != nil {
			return fmt.Sprintf("Claude prompt submitted: %s", truncate(*prompt, 120))
		}
		return "Claude prompt submitted"
	case "PreToolUse":
		return claudeToolSummary("Claude about to run", root)
	case "PermissionRequest":
		return claudeToolSummary("Claude requested permission for", root)
	case "PostToolUse":
		return claudeToolSummary("Claude finished", root)
	case "PostToolUseFailure":
		toolName := valueOr(stringFromObject(root, "tool_name"), "tool")
		if errorText := stringFromObject(root, "error"); errorText != nil {
			return fmt.Sprintf("Claude %s failed: %s", toolName, truncate(*errorText, 120))
		}
		return fmt.Sprintf("Claude %s failed", toolName)
	case "Notification":
		if message := stringFromObject(root, "message"); message != nil {
			return fmt.Sprintf("Claude notification: %s", truncate(*message, 120))
		}
		if notificationType := stringFromObject(root, "notification_type"); notificationType != nil {
			return fmt.Sprintf("Claude notification: %s", *notificationType)
		}
		return "Claude notification"
	case "Stop":
		if message := stringFromObject(root, "last_assistant_message"); message != nil {
			return fmt.Sprintf("Claude turn stopped: %s", truncate(*message, 120))
		}
		return "Claude turn stopped"
	case "StopFailure":
		if errorKind := stringFromObject(root, "error"); errorKind != nil {
			return fmt.Sprintf("Claude turn failed: %s", *errorKind)
		}
		return "Claude turn failed"
	default:
		return nativeEventName
	}
}

func openCodeSummary(nativeEventName string, root map[string]any) string {
	event := objectFromObject(root, "event")
	var eventProperties map[string]any
	if event != nil {
		eventProperties = objectFromObject(event, "properties")
	}
	input := objectFromObject(root, "input")
	output := objectFromObject(root, "output")

	switch nativeEventName {
	case "session.created":
		if title := nestedString(eventProperties, "info", "title"); title != nil {
			return fmt.Sprintf("OpenCode session created: %s", truncate(*title, 120))
		}
		return "OpenCode session created"
	case "session.status":
		if statusType := nestedString(eventProperties, "status", "type"); statusType != nil {
			if *statusType == "retry" {
				return fmt.Sprintf("OpenCode session retrying: %s", valueOr(nestedString(eventProperties, "status", "message"), "retry"))
			}
			return fmt.Sprintf("OpenCode session status: %s", *statusType)
		}
		return "OpenCode session status changed"
	case "session.idle":
		return "OpenCode session is idle"
	case "session.error":
		if message := nestedString(eventProperties, "error", "message"); message != nil {
			return fmt.Sprintf("OpenCode session error: %s", truncate(*message, 120))
		}
		if errorType := nestedString(eventProperties, "error", "type"); errorType != nil {
			return fmt.Sprintf("OpenCode session error: %s", *errorType)
		}
		return "OpenCode session error"
	case "permission.asked":
		permissionName := valueOr(
			firstNonNilString(stringFromObject(root, "tool_name"), objectFieldString(eventProperties, "permission")),
			"permission",
		)
		command := firstNonNilString(
			stringFromObject(root, "command"),
			nestedString(eventProperties, "metadata", "command"),
			firstStringFromArrayField(eventProperties, "patterns"),
		)
		if command != nil {
			return fmt.Sprintf("OpenCode requested permission for %s: %s", permissionName, truncate(*command, 160))
		}
		return fmt.Sprintf("OpenCode requested permission for %s", permissionName)
	case "permission.replied":
		if reply := objectFieldString(eventProperties, "reply"); reply != nil {
			return fmt.Sprintf("OpenCode permission reply: %s", *reply)
		}
		return "OpenCode permission replied"
	case "tool.execute.before":
		toolName := valueOr(
			firstNonNilString(stringFromObject(root, "tool_name"), objectFieldString(input, "tool")),
			"tool",
		)
		command := firstNonNilString(
			stringFromObject(root, "command"),
			nestedString(output, "args", "command"),
		)
		if command != nil {
			return fmt.Sprintf("OpenCode about to run %s: %s", toolName, truncate(*command, 160))
		}
		return fmt.Sprintf("OpenCode about to run %s", toolName)
	case "tool.execute.after":
		toolName := valueOr(
			firstNonNilString(stringFromObject(root, "tool_name"), objectFieldString(input, "tool")),
			"tool",
		)
		if title := objectFieldString(output, "title"); title != nil {
			return fmt.Sprintf("OpenCode finished %s: %s", toolName, truncate(*title, 120))
		}
		if content := objectFieldString(output, "output"); content != nil {
			return fmt.Sprintf("OpenCode finished %s: %s", toolName, truncate(*content, 120))
		}
		return fmt.Sprintf("OpenCode finished %s", toolName)
	default:
		return nativeEventName
	}
}

func claudeToolSummary(prefix string, root map[string]any) string {
	toolName := valueOr(stringFromObject(root, "tool_name"), "tool")
	if input := objectFromObject(root, "tool_input"); input != nil {
		if target := toolTargetFromObject(input); target != nil {
			return fmt.Sprintf("%s %s: %s", prefix, toolName, truncate(*target, 160))
		}
	}
	return fmt.Sprintf("%s %s", prefix, toolName)
}

func truncate(value string, maxLen int) string {
	if len(value) <= maxLen {
		return value
	}
	return value[:maxLen]
}

func decodeJSONObject(payload []byte) (map[string]any, error) {
	var root map[string]any
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if err := decoder.Decode(&root); err != nil {
		return nil, err
	}
	return root, nil
}

func stringFromObject(object map[string]any, field string) *string {
	if object == nil {
		return nil
	}
	return stringValue(object[field])
}

func objectFromObject(object map[string]any, field string) map[string]any {
	if object == nil {
		return nil
	}
	return objectValue(object[field])
}

func boolFromObject(object map[string]any, field string) *bool {
	if object == nil {
		return nil
	}
	return boolValue(object[field])
}

func intFromObject(object map[string]any, field string) *int {
	if object == nil {
		return nil
	}
	return intValue(object[field])
}

func nestedString(object map[string]any, path ...string) *string {
	if object == nil || len(path) == 0 {
		return nil
	}
	current := object
	for _, part := range path[:len(path)-1] {
		current = objectFromObject(current, part)
		if current == nil {
			return nil
		}
	}
	return stringFromObject(current, path[len(path)-1])
}

func nestedInt(object map[string]any, path ...string) *int {
	if object == nil || len(path) == 0 {
		return nil
	}
	current := object
	for _, part := range path[:len(path)-1] {
		current = objectFromObject(current, part)
		if current == nil {
			return nil
		}
	}
	return intFromObject(current, path[len(path)-1])
}

func firstStringFromArrayField(object map[string]any, field string) *string {
	if object == nil {
		return nil
	}
	values, ok := object[field].([]any)
	if !ok {
		return nil
	}
	for _, item := range values {
		if value := stringValue(item); value != nil {
			return value
		}
	}
	return nil
}

func toolTargetFromObject(object map[string]any) *string {
	return firstNonNilString(
		stringFromObject(object, "command"),
		stringFromObject(object, "file_path"),
		stringFromObject(object, "path"),
		stringFromObject(object, "url"),
		stringFromObject(object, "query"),
		stringFromObject(object, "description"),
	)
}

func stringValue(value any) *string {
	switch typed := value.(type) {
	case string:
		return stringPtr(typed)
	case json.Number:
		value := typed.String()
		return &value
	default:
		return nil
	}
}

func objectValue(value any) map[string]any {
	typed, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	return typed
}

func boolValue(value any) *bool {
	typed, ok := value.(bool)
	if !ok {
		return nil
	}
	return boolPtr(typed)
}

func intValue(value any) *int {
	switch typed := value.(type) {
	case int:
		return intPtr(typed)
	case int32:
		return intPtr(int(typed))
	case int64:
		return intPtr(int(typed))
	case float64:
		return intPtr(int(typed))
	case json.Number:
		if integer, err := typed.Int64(); err == nil {
			return intPtr(int(integer))
		}
	}
	return nil
}

func sendToUnixSocket(path string, payload []byte) error {
	conn, err := net.Dial("unix", path)
	if err != nil {
		return err
	}
	defer conn.Close()

	_, err = conn.Write(payload)
	return err
}

func runListener(options ListenOptions, stdout io.Writer) error {
	_ = os.Remove(options.SocketPath)
	if err := os.MkdirAll(filepath.Dir(options.SocketPath), 0o755); err != nil {
		return err
	}

	listener, err := net.Listen("unix", options.SocketPath)
	if err != nil {
		return err
	}
	defer func() {
		listener.Close()
		_ = os.Remove(options.SocketPath)
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			return err
		}
		if _, err := io.Copy(stdout, conn); err != nil {
			conn.Close()
			return err
		}
		conn.Close()
	}
}

func objectFieldString(object map[string]any, field string) *string {
	return stringFromObject(object, field)
}

func firstNonNilString(values ...*string) *string {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func firstNonNilInt(values ...*int) *int {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func valueOr(value *string, fallback string) string {
	if value == nil {
		return fallback
	}
	return *value
}

func stringPtr(value string) *string {
	return &value
}

func intPtr(value int) *int {
	return &value
}

func boolPtr(value bool) *bool {
	return &value
}
