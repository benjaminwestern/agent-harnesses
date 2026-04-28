//go:build !windows

package opencode

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func configureServerCommand(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func terminateServerCommand(command *exec.Cmd) {
	if command == nil || command.Process == nil {
		return
	}
	pid := command.Process.Pid
	if pid <= 0 {
		return
	}
	if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil {
		_ = command.Process.Signal(syscall.SIGTERM)
	}
}

func serverPIDAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func terminateOwnedServerPID(pid int) {
	if pid <= 0 {
		return
	}
	if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil {
		if process, findErr := os.FindProcess(pid); findErr == nil {
			_ = process.Signal(syscall.SIGTERM)
		}
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !serverPIDAlive(pid) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}

	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil {
		if process, findErr := os.FindProcess(pid); findErr == nil {
			_ = process.Kill()
		}
	}
}

func serverCommandMatches(pid int, baseURL string) bool {
	port := serverOwnerPort(baseURL)
	if port == "" {
		return false
	}
	output, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "command=").Output()
	if err != nil {
		return false
	}
	command := string(output)
	return strings.Contains(command, "opencode") &&
		strings.Contains(command, "serve") &&
		strings.Contains(command, "--port") &&
		strings.Contains(command, port)
}
