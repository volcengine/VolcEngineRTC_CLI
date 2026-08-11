//go:build !windows

package cmd

import "os/exec"

func buildTaskCmd(shell, cmdline string) *exec.Cmd {
	return exec.Command(shell, "-c", cmdline)
}
