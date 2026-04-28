//go:build windows

package opencode

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

func configureServerCommand(command *exec.Cmd) {}

func terminateServerCommand(command *exec.Cmd) {
	if command == nil || command.Process == nil {
		return
	}
	_ = command.Process.Kill()
}

func serverPIDAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	output, err := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/FO", "CSV", "/NH").Output()
	if err != nil {
		return false
	}
	return bytes.Contains(output, []byte(`"`+strconv.Itoa(pid)+`"`))
}

func terminateOwnedServerPID(pid int) {
	if pid <= 0 {
		return
	}
	_ = exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F").Run()
}

func serverCommandMatches(pid int, baseURL string) bool {
	port := serverOwnerPort(baseURL)
	if port == "" {
		return false
	}
	command := fmt.Sprintf("(Get-CimInstance Win32_Process -Filter \"ProcessId = %d\").CommandLine", pid)
	output, err := exec.Command("powershell", "-NoProfile", "-Command", command).Output()
	if err != nil {
		return false
	}
	line := string(output)
	return strings.Contains(line, "opencode") &&
		strings.Contains(line, "serve") &&
		strings.Contains(line, "--port") &&
		strings.Contains(line, port)
}
