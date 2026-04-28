//go:build !windows

package providerprobe

import (
	"os/exec"
	"syscall"
)

func configureCommandGroup(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func terminateCommandGroup(command *exec.Cmd) {
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
