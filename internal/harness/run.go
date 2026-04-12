package harness

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

type RunOptions struct {
	Runtime              Runtime
	Scenario             string
	PromptFile           string
	ApprovalPolicy       string
	Sandbox              string
	GeminiApprovalMode   string
	ClaudePermissionMode string
	ExtraArgs            []string
}

func runRuntime(args []string, stdout, stderr io.Writer) error {
	options, err := parseRunArgs(args)
	if err != nil {
		if errors.Is(err, errHelpRequested) {
			printUsage(stdout)
			return nil
		}
		printUsage(stderr)
		return err
	}
	workspaceRoot, err := findWorkspaceRoot()
	if err != nil {
		return err
	}
	promptPath, err := runtimePromptPath(workspaceRoot, options)
	if err != nil {
		return err
	}
	promptTextBytes, err := os.ReadFile(promptPath)
	if err != nil {
		return err
	}
	if _, err := exec.LookPath(string(options.Runtime)); err != nil {
		return fmt.Errorf("%s is not installed or is not on PATH", options.Runtime)
	}
	if err := ensureRuntimeBundle(workspaceRoot, options.Runtime); err != nil {
		return err
	}
	if err := ensureInteractiveRunEnvironment(options.Runtime); err != nil {
		return err
	}

	socketPath := fmt.Sprintf("/tmp/agent-harness-%s-%d.sock", options.Runtime, os.Getpid())
	stopListener, err := startDebugListener(socketPath, stdout)
	if err != nil {
		return err
	}
	defer stopListener()

	env := os.Environ()
	env = append(env, "AGENT_HARNESS_SOCKET="+socketPath)
	env = append(env, runtimeEnv(options)...)

	command, err := runtimeCommand(workspaceRoot, string(promptTextBytes), options)
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintf(stdout, "Running %s harness scenario: %s\n", options.Runtime, options.Scenario); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "Prompt file: %s\n", promptPath); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "Hook socket: %s\n", socketPath); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(stdout, "Debug mode is live. Watch for `[hook]` lines while the agent runs."); err != nil {
		return err
	}
	if options.Scenario == "approval" {
		if _, err := fmt.Fprintln(stdout, "Expect the runtime to ask for approval before the write command runs."); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(stdout); err != nil {
		return err
	}

	cmd := exec.Command(command[0], command[1:]...)
	cmd.Dir = workspaceRoot
	cmd.Env = env
	cmd.Stdin = os.Stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			_, _ = fmt.Fprintf(stdout, "\n%s exited with status %d\n", options.Runtime, exitError.ExitCode())
			return err
		}
		return err
	}
	_, _ = fmt.Fprintf(stdout, "\n%s exited with status 0\n", options.Runtime)
	return nil
}

func parseRunArgs(args []string) (RunOptions, error) {
	options := RunOptions{
		Scenario:             "smoke",
		ApprovalPolicy:       "on-request",
		Sandbox:              "workspace-write",
		GeminiApprovalMode:   "default",
		ClaudePermissionMode: "default",
	}
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--runtime":
			index++
			if index >= len(args) {
				return RunOptions{}, errors.New("missing runtime")
			}
			runtime, err := parseRuntime(args[index])
			if err != nil {
				return RunOptions{}, err
			}
			options.Runtime = runtime
		case "--scenario":
			index++
			if index >= len(args) {
				return RunOptions{}, errors.New("missing scenario")
			}
			options.Scenario = args[index]
		case "--prompt-file":
			index++
			if index >= len(args) {
				return RunOptions{}, errors.New("missing prompt file")
			}
			options.PromptFile = args[index]
		case "--approval-policy":
			index++
			if index >= len(args) {
				return RunOptions{}, errors.New("missing approval policy")
			}
			options.ApprovalPolicy = args[index]
		case "--sandbox":
			index++
			if index >= len(args) {
				return RunOptions{}, errors.New("missing sandbox")
			}
			options.Sandbox = args[index]
		case "--gemini-approval-mode":
			index++
			if index >= len(args) {
				return RunOptions{}, errors.New("missing Gemini approval mode")
			}
			options.GeminiApprovalMode = args[index]
		case "--claude-permission-mode":
			index++
			if index >= len(args) {
				return RunOptions{}, errors.New("missing Claude permission mode")
			}
			options.ClaudePermissionMode = args[index]
		case "--extra-arg":
			index++
			if index >= len(args) {
				return RunOptions{}, errors.New("missing extra arg")
			}
			options.ExtraArgs = append(options.ExtraArgs, args[index])
		case "--help", "-h":
			return RunOptions{}, errHelpRequested
		default:
			return RunOptions{}, fmt.Errorf("invalid argument: %s", args[index])
		}
	}
	if options.Runtime == "" {
		return RunOptions{}, errors.New("missing runtime")
	}
	if options.Scenario != "smoke" && options.Scenario != "bash" && options.Scenario != "approval" {
		return RunOptions{}, fmt.Errorf("unsupported scenario: %s", options.Scenario)
	}
	return options, nil
}

func runtimePromptPath(workspaceRoot string, options RunOptions) (string, error) {
	if strings.TrimSpace(options.PromptFile) != "" {
		return filepath.Abs(options.PromptFile)
	}
	if options.Runtime == RuntimeCodex {
		codexPrompts := map[string]string{
			"smoke":    "smoke.md",
			"bash":     "trusted-bash.md",
			"approval": "approval-request.md",
		}
		return filepath.Join(workspaceRoot, "runtimes", string(options.Runtime), "prompts", codexPrompts[options.Scenario]), nil
	}
	return filepath.Join(workspaceRoot, "runtimes", string(options.Runtime), "prompts", options.Scenario+".md"), nil
}

func ensureRuntimeBundle(workspaceRoot string, runtime Runtime) error {
	switch runtime {
	case RuntimeClaude:
		repoSettings := filepath.Join(workspaceRoot, ".agentic-control", "claude", "settings.json")
		legacyRepoSettings := filepath.Join(workspaceRoot, ".agent-harnesses", "claude", "settings.json")
		globalSettings := filepath.Join(firstNonEmptyEnv("CLAUDE_HOME", filepath.Join(mustUserHomeDir(), ".claude")), "agentic-control", "settings.json")
		legacyGlobalSettings := filepath.Join(firstNonEmptyEnv("CLAUDE_HOME", filepath.Join(mustUserHomeDir(), ".claude")), "agent-harnesses", "settings.json")
		if fileExists(repoSettings) || fileExists(globalSettings) || fileExists(legacyRepoSettings) || fileExists(legacyGlobalSettings) {
			return nil
		}
		return errors.New("missing Claude bundle; run `.artifacts/bin/agent_harness install --runtime claude --scope repo` first")
	case RuntimeOpenCode:
		repoPlugin := filepath.Join(workspaceRoot, ".opencode", "plugins", "agentic-control.js")
		legacyRepoPlugin := filepath.Join(workspaceRoot, ".opencode", "plugins", "agent-harness.js")
		globalPlugin := filepath.Join(firstNonEmptyEnv("OPENCODE_CONFIG_DIR", filepath.Join(mustUserHomeDir(), ".config", "opencode")), "plugins", "agentic-control.js")
		legacyGlobalPlugin := filepath.Join(firstNonEmptyEnv("OPENCODE_CONFIG_DIR", filepath.Join(mustUserHomeDir(), ".config", "opencode")), "plugins", "agent-harness.js")
		if fileExists(repoPlugin) || fileExists(globalPlugin) || fileExists(legacyRepoPlugin) || fileExists(legacyGlobalPlugin) {
			return nil
		}
		return errors.New("missing OpenCode bundle; run `.artifacts/bin/agent_harness install --runtime opencode` first")
	case RuntimePi:
		repoExtension := filepath.Join(workspaceRoot, ".pi", "extensions", "agentic-control.ts")
		globalExtension := filepath.Join(firstNonEmptyEnv("PI_CODING_AGENT_DIR", filepath.Join(mustUserHomeDir(), ".pi", "agent")), "extensions", "agentic-control.ts")
		if fileExists(repoExtension) || fileExists(globalExtension) {
			return nil
		}
		return errors.New("missing pi bundle; run `.artifacts/bin/agent_harness install --runtime pi --scope repo` first")
	default:
		return nil
	}
}

func runtimeEnv(options RunOptions) []string {
	if options.Runtime != RuntimeOpenCode {
		return nil
	}
	if options.Scenario == "approval" {
		return []string{`OPENCODE_CONFIG_CONTENT={"$schema":"https://opencode.ai/config.json","permission":{"bash":{"*":"ask","pwd*":"allow","ls *":"allow"}}}`}
	}
	if options.Scenario == "bash" {
		return []string{`OPENCODE_CONFIG_CONTENT={"$schema":"https://opencode.ai/config.json","permission":{"bash":"allow"}}`}
	}
	return nil
}

func runtimeCommand(workspaceRoot, promptText string, options RunOptions) ([]string, error) {
	switch options.Runtime {
	case RuntimeCodex:
		return append([]string{"codex", "--enable", "codex_hooks", "--no-alt-screen", "--ask-for-approval", options.ApprovalPolicy, "--sandbox", options.Sandbox}, append(options.ExtraArgs, promptText)...), nil
	case RuntimeGemini:
		command := []string{"gemini", "--approval-mode", options.GeminiApprovalMode, "--prompt-interactive", promptText}
		if strings.ToLower(options.Sandbox) != "false" && strings.ToLower(options.Sandbox) != "off" && options.Sandbox != "0" {
			command = append([]string{"gemini", "--sandbox", "--approval-mode", options.GeminiApprovalMode, "--prompt-interactive", promptText}, options.ExtraArgs...)
			return command, nil
		}
		command = append(command, options.ExtraArgs...)
		return command, nil
	case RuntimeClaude:
		settingsPath := filepath.Join(workspaceRoot, ".agentic-control", "claude", "settings.json")
		if !fileExists(settingsPath) {
			legacySettingsPath := filepath.Join(workspaceRoot, ".agent-harnesses", "claude", "settings.json")
			if fileExists(legacySettingsPath) {
				settingsPath = legacySettingsPath
			} else {
				settingsPath = filepath.Join(firstNonEmptyEnv("CLAUDE_HOME", filepath.Join(mustUserHomeDir(), ".claude")), "agentic-control", "settings.json")
				if !fileExists(settingsPath) {
					settingsPath = filepath.Join(firstNonEmptyEnv("CLAUDE_HOME", filepath.Join(mustUserHomeDir(), ".claude")), "agent-harnesses", "settings.json")
				}
			}
		}
		return append([]string{"claude", "--settings", settingsPath, "--permission-mode", options.ClaudePermissionMode}, append(options.ExtraArgs, promptText)...), nil
	case RuntimeOpenCode:
		return append([]string{"opencode", "run"}, append(options.ExtraArgs, promptText)...), nil
	case RuntimePi:
		return append([]string{"pi", "-p"}, append(options.ExtraArgs, promptText)...), nil
	default:
		return nil, fmt.Errorf("unsupported runtime: %s", options.Runtime)
	}
}

func ensureInteractiveRunEnvironment(runtime Runtime) error {
	if runtime == RuntimePi {
		return nil
	}
	stdinInfo, err := os.Stdin.Stat()
	if err != nil {
		return err
	}
	stdoutInfo, err := os.Stdout.Stat()
	if err != nil {
		return err
	}
	if stdinInfo.Mode()&os.ModeCharDevice == 0 || stdoutInfo.Mode()&os.ModeCharDevice == 0 {
		return fmt.Errorf("%s live diagnostics require an interactive TTY; run this command in a real terminal, or use `mise run diag:fixtures:%s` for non-interactive inspection", runtime, runtime)
	}
	return nil
}

func startDebugListener(socketPath string, stdout io.Writer) (func(), error) {
	_ = os.Remove(socketPath)
	if err := os.MkdirAll(filepath.Dir(socketPath), 0o755); err != nil {
		return nil, err
	}
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, err
	}
	var once sync.Once
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				scanner := bufio.NewScanner(conn)
				for scanner.Scan() {
					text := strings.TrimSpace(scanner.Text())
					if text == "" {
						continue
					}
					_, _ = fmt.Fprintf(stdout, "[hook] %s\n", text)
				}
			}()
		}
	}()
	return func() {
		once.Do(func() {
			_ = listener.Close()
			_ = os.Remove(socketPath)
		})
	}, nil
}

func mustUserHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}
