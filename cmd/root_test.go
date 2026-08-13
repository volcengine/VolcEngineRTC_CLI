// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"io"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/config"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/doctor"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
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
		"auth", "dev", "docs", "doctor", "explain-error",
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

func TestDocsCommandsAreProjectIndependent(t *testing.T) {
	root := NewRootCmd()
	docs, _, err := root.Find([]string{"docs", "search"})
	if err != nil {
		t.Fatal(err)
	}
	if !isProjectIndependentCommand(docs) {
		t.Fatal("docs search must not load project config or .env.local")
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

func TestCommandSuggestionAndEditDistance(t *testing.T) {
	root := NewRootCmd()
	if got := nearestCommand(root, "doctr"); got != "doctor" {
		t.Fatalf("nearestCommand(doctr) = %q, want doctor", got)
	}
	if got := nearestCommand(root, "config"); got == "config" {
		t.Fatalf("hidden command must not be suggested, got %q", got)
	}
	if got := nearestCommand(root, "completely-unrelated"); got != "" {
		t.Fatalf("distant command unexpectedly suggested: %q", got)
	}
	for _, tc := range []struct {
		a, b string
		want int
	}{
		{a: "doctor", b: "doctor", want: 0},
		{a: "doctr", b: "doctor", want: 1},
		{a: "", b: "init", want: 4},
		{a: "技能", b: "技", want: 1},
	} {
		if got := levenshtein(tc.a, tc.b); got != tc.want {
			t.Errorf("levenshtein(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestUnknownCommandErrorIsTypedAndActionable(t *testing.T) {
	typed := unknownCommandError(NewRootCmd(), &plainError{message: `unknown command "doctr" for "vertc"`})
	if typed.Code != "vertc.cli.unknown_command" || !strings.Contains(typed.Hint, "doctor") {
		t.Fatalf("unexpected typed error: %+v", typed)
	}
	typed = unknownCommandError(NewRootCmd(), &plainError{message: "unknown command"})
	if typed.Code != "vertc.cli.unknown_command" || !strings.Contains(typed.Hint, "--help") {
		t.Fatalf("unexpected fallback error: %+v", typed)
	}
}

type plainError struct{ message string }

func (e *plainError) Error() string { return e.message }

func TestVersionProbeAndDryRunRefreshPolicy(t *testing.T) {
	if !isVersionProbe([]string{"--format", "json", "--version"}) || !isVersionProbe([]string{"version"}) {
		t.Fatal("version probes were not recognized")
	}
	if isVersionProbe([]string{"doctor"}) {
		t.Fatal("doctor must not be classified as a version probe")
	}
	oldArgs, oldDryRun := os.Args, flagDryRun
	t.Cleanup(func() { os.Args, flagDryRun = oldArgs, oldDryRun })
	os.Args = []string{"vertc", "version"}
	root := NewRootCmd()
	flagDryRun = true
	if shouldRefreshAfterCommand(root) {
		t.Fatal("dry-run must suppress detached refresh")
	}
}

func TestInitCommandValidationStopsBeforeSideEffects(t *testing.T) {
	oldDryRun, oldFormat := flagDryRun, flagFormat
	t.Cleanup(func() { flagDryRun, flagFormat = oldDryRun, oldFormat })
	flagDryRun, flagFormat = false, "json"

	tests := []struct {
		args []string
		code string
	}{
		{args: nil, code: "vertc.cli.invalid_flag"},
		{args: []string{"--scene", "missing", "--platform", "web"}, code: "vertc.template.not_found"},
		{args: []string{"--scene", "voice-agent", "--platform", "web", "--room-id", "bad\nroom"}, code: "vertc.config.invalid_identity"},
	}
	for _, tc := range tests {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			command := newInitCmd()
			command.SetArgs(tc.args)
			command.SetOut(io.Discard)
			command.SetErr(io.Discard)
			err := command.Execute()
			typed, ok := errs.As(err)
			if !ok || typed.Code != tc.code {
				t.Errorf("init %q error = %v, want %s", tc.args, err, tc.code)
			}
			entries, readErr := os.ReadDir(dir)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if len(entries) != 0 {
				t.Fatalf("validation failure wrote project files: %v", entries)
			}
		})
	}
}

func TestCommandPrettyResultsExposeRecoveryContext(t *testing.T) {
	var buf bytes.Buffer
	initResult{
		Scene: "voice-agent", Platform: "web", Dir: "demo", RoomID: "room", UserID: "user", AppID: "app",
		DryRun: true, Files: []string{"vertc.config.yaml"}, Next: []string{"vertc dev"},
	}.Pretty(&buf)
	for _, want := range []string{"would generate (dry-run)", "identity: room=room user=user app=app", "demo/vertc.config.yaml", "vertc dev"} {
		if !strings.Contains(buf.String(), want) {
			t.Fatalf("init pretty output missing %q:\n%s", want, buf.String())
		}
	}

	buf.Reset()
	explainResult{Query: "999", Hint: "run doctor"}.Pretty(&buf)
	if !strings.Contains(buf.String(), "not in the offline knowledge base") || !strings.Contains(buf.String(), "run doctor") {
		t.Fatalf("unknown-code output lacks recovery context: %s", buf.String())
	}
	buf.Reset()
	explainResult{Found: true, Code: "1001", Enum: "TOKEN", Domain: "web", Meaning: "expired", Fix: "refresh", DoctorCheck: "auth", Source: "official", Verified: "no"}.Pretty(&buf)
	for _, want := range []string{"1001 (TOKEN) [web]", "fix: refresh", "related doctor check", "source: official", "未验证"} {
		if !strings.Contains(buf.String(), want) {
			t.Fatalf("known-code output missing %q:\n%s", want, buf.String())
		}
	}
}

func TestTemplateListPrettyAndInitHelpers(t *testing.T) {
	var buf bytes.Buffer
	templateListResult{Templates: []templateEntry{
		{Scene: "voice-agent", Platform: "web", Title: "Voice", Available: true, Default: true},
		{Scene: "future", Platform: "web", Title: "Future"},
	}}.Pretty(&buf)
	for _, want := range []string{"[available]", "(default)", "[reserved]"} {
		if !strings.Contains(buf.String(), want) {
			t.Fatalf("template list missing %q:\n%s", want, buf.String())
		}
	}

	if got := voiceAgentNextSteps("voice-agent"); len(got) != 2 || !strings.Contains(got[0], "auth login") {
		t.Fatalf("voice-agent next steps = %q", got)
	}
	if got := voiceAgentNextSteps("other"); len(got) != 4 || !strings.Contains(got[0], "env write") {
		t.Fatalf("generic next steps = %q", got)
	}
	dir := t.TempDir()
	if nonEmptyDir(dir) || nonEmptyDir(dir+"-missing") {
		t.Fatal("empty or missing directory reported non-empty")
	}
	if err := os.WriteFile(dir+"/entry", []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !nonEmptyDir(dir) {
		t.Fatal("directory containing a file reported empty")
	}
}

func TestCountErrorsCountsOnlyErrorSeverity(t *testing.T) {
	report := config.Report{Findings: []config.Finding{
		{Severity: config.SevError}, {Severity: config.SevWarn}, {Severity: config.SevError},
	}}
	if got := countErrors(report); got != 2 {
		t.Fatalf("countErrors = %d, want 2", got)
	}
}

func TestLifecyclePrettyResultsPreserveStatusAndActions(t *testing.T) {
	var buf bytes.Buffer
	doctorReport{doctor.Report{
		Checks: []doctor.Check{
			{Status: doctor.PASS, Title: "ready", Detail: "ok", Hint: "must stay hidden"},
			{Status: doctor.WARN, Title: "warning", Detail: "attention", Hint: "fix warning"},
			{Status: doctor.FAIL, Title: "failure", Detail: "blocked", Hint: "fix failure"},
			{Status: doctor.SKIP, Title: "skipped", Detail: "disabled"},
			{Status: doctor.UNKNOWN, Title: "unknown", Detail: "no evidence"},
		},
		Passed: 1, Warned: 1, Failed: 1, Skipped: 1, Unknown: 1,
	}}.Pretty(&buf)
	for _, tc := range []struct {
		marker string
		status doctor.Status
	}{
		{marker: "✓", status: doctor.PASS},
		{marker: "!", status: doctor.WARN},
		{marker: "✗", status: doctor.FAIL},
		{marker: "-", status: doctor.SKIP},
		{marker: "?", status: doctor.UNKNOWN},
	} {
		want := tc.marker + " [" + string(tc.status) + "]"
		if !strings.Contains(buf.String(), want) {
			t.Fatalf("doctor output missing %q:\n%s", want, buf.String())
		}
	}
	for _, want := range []string{"hint: fix warning", "1 passed, 1 warned"} {
		if !strings.Contains(buf.String(), want) {
			t.Fatalf("doctor output missing %q:\n%s", want, buf.String())
		}
	}
	if strings.Contains(buf.String(), "must stay hidden") {
		t.Fatalf("PASS hint should not be rendered: %s", buf.String())
	}

	buf.Reset()
	updateResult{Current: "1.0.0", Latest: "2.0.0", Status: "manual_required", SkillsError: "offline", ManualCommand: "npm install"}.Pretty(&buf)
	for _, want := range []string{"1.0.0 → 2.0.0", "skills: sync failed: offline", "manual update: npm install"} {
		if !strings.Contains(buf.String(), want) {
			t.Fatalf("update output missing %q:\n%s", want, buf.String())
		}
	}
	buf.Reset()
	updateResult{SkillsSynced: true}.Pretty(&buf)
	if !strings.Contains(buf.String(), "skills: synchronized") {
		t.Fatalf("sync success missing: %s", buf.String())
	}
	buf.Reset()
	updateResult{SkillsStatus: "stale"}.Pretty(&buf)
	if !strings.Contains(buf.String(), "skills: stale") {
		t.Fatalf("skills status missing: %s", buf.String())
	}

	buf.Reset()
	versionInfo{Name: "vertc", Version: "1.2.3", Commit: "abc", BuildDate: "today"}.Pretty(&buf)
	if got := buf.String(); got != "vertc 1.2.3 (commit abc, built today)\n" {
		t.Fatalf("version output=%q", got)
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
