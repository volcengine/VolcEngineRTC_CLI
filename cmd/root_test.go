// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"io"
	"os"
	"sort"
	"testing"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/output"
)

func TestResolveOutputFormatDefaultsToPrettyInTerminal(t *testing.T) {
	got := resolveOutputFormat("", true)
	if got != output.FormatPretty {
		t.Fatalf("terminal default = %q, want %q", got, output.FormatPretty)
	}
}

func TestResolveOutputFormatDefaultsToJSONOutsideTerminal(t *testing.T) {
	got := resolveOutputFormat("", false)
	if got != output.FormatJSON {
		t.Fatalf("non-terminal default = %q, want %q", got, output.FormatJSON)
	}
}

func TestResolveOutputFormatHonorsExplicitFormat(t *testing.T) {
	for _, tc := range []struct {
		requested string
		stdoutTTY bool
		want      output.Format
	}{
		{requested: "json", stdoutTTY: true, want: output.FormatJSON},
		{requested: "pretty", stdoutTTY: false, want: output.FormatPretty},
		{requested: "table", stdoutTTY: false, want: output.FormatTable},
	} {
		if got := resolveOutputFormat(tc.requested, tc.stdoutTTY); got != tc.want {
			t.Errorf("resolveOutputFormat(%q, %t) = %q, want %q", tc.requested, tc.stdoutTTY, got, tc.want)
		}
	}
}

func TestFormatFlagUsesTerminalAwareDefault(t *testing.T) {
	format := NewRootCmd().PersistentFlags().Lookup("format")
	if format == nil {
		t.Fatal("--format flag is not registered")
	}
	if format.DefValue != "" {
		t.Fatalf("--format default = %q, want terminal-aware default", format.DefValue)
	}
}

// TestRootCommandSurface pins the public command surface. Visible
// commands are what `vertc --help` lists; hidden commands stay registered and
// runnable (advanced/manual/legacy fallbacks: env/config/token, the internal
// openapi escape hatch, and Cobra's completion) but are kept off the browsable
// surface. Pinning both sets means a newly added command — or a Cobra default
// like `completion` — cannot leak into the public surface without a deliberate
// update here.
func TestRootCommandSurface(t *testing.T) {
	root := NewRootCmd()
	// Materialize Cobra's lazily-added default commands so they are covered.
	root.InitDefaultHelpCmd()
	root.InitDefaultCompletionCmd()

	var visible, hidden []string
	for _, c := range root.Commands() {
		if c.Name() == "" {
			continue
		}
		if c.Hidden {
			hidden = append(hidden, c.Name())
		} else {
			visible = append(visible, c.Name())
		}
	}
	sort.Strings(visible)
	sort.Strings(hidden)

	wantVisible := []string{
		"auth", "dev", "doctor", "explain-error",
		"help", "init", "skills", "update", "version",
	}
	wantHidden := []string{"agent", "completion", "config", "env", "openapi", "token"}

	if !equalStringSlice(visible, wantVisible) {
		t.Errorf("public (visible) command surface drifted:\n got:  %v\n want: %v\n"+
			"update NewRootCmd (and this test) deliberately when the public surface changes", visible, wantVisible)
	}
	if !equalStringSlice(hidden, wantHidden) {
		t.Errorf("hidden command set drifted:\n got:  %v\n want: %v", hidden, wantHidden)
	}
}

func TestRefreshExcludedForLifecycleCommands(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	for _, args := range [][]string{{"vertc", "update", "--check"}, {"vertc", "doctor"}, {"vertc", "completion", "bash"}} {
		os.Args = args
		if shouldRefreshAfterCommand(NewRootCmd()) {
			t.Fatalf("refresh must be excluded for %v", args)
		}
	}
	os.Args = []string{"vertc", "version"}
	if !shouldRefreshAfterCommand(NewRootCmd()) {
		t.Fatal("ordinary commands should remain eligible for detached refresh")
	}
}

// TestRemovedCommandsRejected complements the surface snapshot: the snapshot
// pins which commands are registered/visible, but does not exercise exit
// behavior. Here we actually run each removed command through the root and
// assert it is rejected (non-nil error → non-zero exit), so a slimmed-away
// command cannot silently come back as a no-op.
func TestRemovedCommandsRejected(t *testing.T) {
	removed := [][]string{
		{"playground"},       // removed top-level
		{"template", "list"}, // removed command group (discovery folded into `init --list`)
		{"test", "smoke"},    // removed command group (folded into `doctor project`)
		{"config", "diff"},   // removed Console-sync subcommand
		{"config", "push"},
		{"config", "pull"},
	}
	for _, args := range removed {
		root := NewRootCmd()
		root.SetArgs(args)
		root.SetOut(io.Discard)
		root.SetErr(io.Discard)
		if err := root.Execute(); err == nil {
			t.Errorf("removed command %v should be rejected with an error, got nil", args)
		}
	}
}

func equalStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
