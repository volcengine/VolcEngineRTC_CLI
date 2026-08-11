//go:build windows

package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsTaskShellExecutesQuotedBinary(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	spaceDir := filepath.Join(root, "path with spaces")
	if err := os.MkdirAll(spaceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	helper := filepath.Join(spaceDir, "task helper.exe")
	if err := copyWindowsTestExecutable(self, helper); err != nil {
		t.Fatal(err)
	}

	sentinel := filepath.Join(root, "helper-ran.txt")
	argsFile := filepath.Join(root, "helper-args.txt")
	cmdline := fmt.Sprintf(
		`"%s" -test.run=TestWindowsTaskShellHelper$ "first simple" "second with spaces"`,
		helper,
	)
	shell, _ := taskShellCommand("windows", cmdline)
	cmd := buildTaskCmd(shell, cmdline)
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"GO_WANT_WINDOWS_TASK_HELPER=1",
		"WINDOWS_TASK_SENTINEL="+sentinel,
		"WINDOWS_TASK_ARGS="+argsFile,
	)

	err = cmd.Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 42 {
		t.Fatalf("task error = %v, want helper exit code 42 (raw command line %q)", err, cmd.SysProcAttr.CmdLine)
	}
	if _, err := os.Stat(sentinel); err != nil {
		t.Fatalf("quoted helper did not execute: %v (raw command line %q)", err, cmd.SysProcAttr.CmdLine)
	}
	rawArgs, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"first simple", "second with spaces"} {
		if !strings.Contains(string(rawArgs), want) {
			t.Errorf("helper args %q do not contain %q", rawArgs, want)
		}
	}
	if strings.Contains(cmd.SysProcAttr.CmdLine, " /S ") {
		t.Fatalf("raw command line unexpectedly uses /S: %q", cmd.SysProcAttr.CmdLine)
	}
}

func TestWindowsTaskShellHelper(t *testing.T) {
	if os.Getenv("GO_WANT_WINDOWS_TASK_HELPER") != "1" {
		return
	}
	if err := os.WriteFile(os.Getenv("WINDOWS_TASK_SENTINEL"), []byte("ran"), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(98)
	}
	if err := os.WriteFile(os.Getenv("WINDOWS_TASK_ARGS"), []byte(strings.Join(os.Args, "\n")), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(97)
	}
	os.Exit(42)
}

func copyWindowsTestExecutable(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
