//go:build windows

package cmd

import (
	"os/exec"
	"syscall"
)

// buildTaskCmd bypasses Go's CommandLineToArgvW-compatible quoting because
// cmd.exe uses different parsing rules. The outer quotes delimit the command
// string consumed by /C; cmdline itself remains unchanged.
func buildTaskCmd(shell, cmdline string) *exec.Cmd {
	cmd := exec.Command(shell)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CmdLine: shell + ` /D /C "` + cmdline + `"`,
	}
	return cmd
}
