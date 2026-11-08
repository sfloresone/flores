//go:build !unix

package tool

import "os/exec"

// Without process groups only the shell itself can be stopped.

func ownGroup(*exec.Cmd) {}

func terminateGroup(cmd *exec.Cmd) { killGroup(cmd) }

func killGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
