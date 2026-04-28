//go:build windows

package providerprobe

import "os/exec"

func configureCommandGroup(command *exec.Cmd) {}

func terminateCommandGroup(command *exec.Cmd) {
	if command == nil || command.Process == nil {
		return
	}
	_ = command.Process.Kill()
}
