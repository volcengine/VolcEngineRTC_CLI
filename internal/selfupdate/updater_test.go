// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package selfupdate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/skillscheck"
)

func resetOverrides() {
	ExecutableOverride = nil
	NpmInstallOverride = nil
	VerifyOverride = nil
	SkillsSyncOverride = nil
	SkillsAddOverride = nil
	SkillsRemoveOverride = nil
}

func TestDetectInstallMethod(t *testing.T) {
	t.Cleanup(resetOverrides)
	ExecutableOverride = func() (string, error) { return "/usr/local/lib/node_modules/@volcengine/rtc-cli/scripts/vertc", nil }
	method, _, err := DetectInstallMethod()
	if err != nil || method != InstallNPM {
		t.Fatalf("method=%s err=%v", method, err)
	}
	ExecutableOverride = func() (string, error) { return "/usr/local/bin/vertc", nil }
	method, _, err = DetectInstallMethod()
	if err != nil || method != InstallManual {
		t.Fatalf("method=%s err=%v", method, err)
	}
}

func TestOverrides(t *testing.T) {
	t.Cleanup(resetOverrides)
	called := 0
	NpmInstallOverride = func(context.Context, string, string) error { called++; return nil }
	VerifyOverride = func(context.Context, string, string) error { called++; return nil }
	SkillsSyncOverride = func(context.Context, string) (SkillsSyncResult, error) {
		called++
		return SkillsSyncResult{}, errors.New("offline")
	}
	if err := RunNpmInstall(context.Background(), "1.2.3", "/prefix/lib/node_modules/@volcengine/rtc-cli/scripts/bin/vertc"); err != nil {
		t.Fatal(err)
	}
	if err := VerifyBinary(context.Background(), "1.2.3", "/prefix/lib/node_modules/@volcengine/rtc-cli/scripts/bin/vertc"); err != nil {
		t.Fatal(err)
	}
	if _, err := SyncSkills(context.Background(), "1.2.3"); err == nil || called != 3 {
		t.Fatalf("called=%d err=%v", called, err)
	}
}

func TestNpmPrefix(t *testing.T) {
	for input, want := range map[string]string{
		"/usr/local/lib/node_modules/@volcengine/rtc-cli/scripts/bin/vertc":                       "/usr/local",
		"/Users/me/.nvm/versions/node/v22/lib/node_modules/@volcengine/rtc-cli/scripts/bin/vertc": "/Users/me/.nvm/versions/node/v22", // public-scan: allow — synthetic test path
	} {
		got, err := npmPrefix(input)
		if err != nil || got != want {
			t.Errorf("npmPrefix(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
}

func TestRollbackBinaryAndCleanupBackup(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "vertc.exe")
	if err := os.WriteFile(binary, []byte("new"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binary+".old", []byte("old"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := RollbackBinary(binary); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(binary)
	if err != nil || string(data) != "old" {
		t.Fatalf("rollback data=%q err=%v", data, err)
	}
	if err := os.WriteFile(binary+".old", []byte("stale"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := CleanupBackup(binary); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(binary + ".old"); !os.IsNotExist(err) {
		t.Fatalf("backup remains: %v", err)
	}
	if err := CleanupBackup(binary); err != nil {
		t.Fatalf("cleanup must be idempotent: %v", err)
	}
}

func TestMaterializeSkillsUsesEmbeddedReleaseContent(t *testing.T) {
	root := t.TempDir()
	if err := materializeSkills(root); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"byted-interactai-guide"} {
		path := filepath.Join(root, "skills", name, "SKILL.md")
		if data, err := os.ReadFile(path); err != nil || len(data) == 0 {
			t.Fatalf("materialized %s: bytes=%d err=%v", path, len(data), err)
		}
	}
}

func TestSyncSkillsCopiesEmbeddedContentAndMarksVersion(t *testing.T) {
	t.Cleanup(resetOverrides)
	t.Setenv("VERTC_STATE_DIR", t.TempDir())

	var source string
	SkillsAddOverride = func(_ context.Context, gotSource string, args ...string) error {
		source = gotSource
		want := []string{"-g", "--copy", "-y"}
		if !slices.Equal(args, want) {
			t.Fatalf("skills add args=%q, want %q", args, want)
		}
		if _, err := os.Stat(filepath.Join(gotSource, "skills", "byted-interactai-guide", "SKILL.md")); err != nil {
			t.Fatalf("materialized skill unavailable during install: %v", err)
		}
		return nil
	}
	var removed []string
	SkillsRemoveOverride = func(_ context.Context, name string, args ...string) error {
		removed = append(removed, name)
		if !slices.Equal(args, []string{"-g", "-y"}) {
			t.Fatalf("skills remove name=%q args=%q", name, args)
		}
		return nil
	}

	if _, err := SyncSkills(context.Background(), "v1.2.3"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("temporary source still exists after sync: %v", err)
	}
	wantRemoved := []string{"byted-interactai-voice-agent", "vertc-voice-agent"}
	if !slices.Equal(removed, wantRemoved) {
		t.Fatalf("removed skills = %q, want %q", removed, wantRemoved)
	}
	state, err := os.ReadFile(filepath.Join(os.Getenv("VERTC_STATE_DIR"), "skills-state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(state), `"version":"1.2.3"`) {
		t.Fatalf("unexpected skills state: %s", state)
	}
}

func TestSyncSkillsLegacyRemovalFailureDoesNotAdvanceVersion(t *testing.T) {
	t.Cleanup(resetOverrides)
	stateDir := t.TempDir()
	t.Setenv("VERTC_STATE_DIR", stateDir)
	SkillsAddOverride = func(context.Context, string, ...string) error { return nil }
	var removed []string
	SkillsRemoveOverride = func(_ context.Context, name string, _ ...string) error {
		removed = append(removed, name)
		if name == "vertc-voice-agent" {
			return errors.New("remove failed")
		}
		return nil
	}

	if _, err := SyncSkills(context.Background(), "2.0.0"); err == nil || !strings.Contains(err.Error(), "remove retired skill vertc-voice-agent") {
		t.Fatalf("unexpected error: %v", err)
	}
	wantRemoved := []string{"byted-interactai-voice-agent", "vertc-voice-agent"}
	if !slices.Equal(removed, wantRemoved) {
		t.Fatalf("removed skills = %q, want %q", removed, wantRemoved)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "skills-state.json")); !os.IsNotExist(err) {
		t.Fatalf("failed migration wrote state: %v", err)
	}
}

func TestSkillsRemoveOutputError(t *testing.T) {
	for _, output := range []string{
		"Failed to remove 1 skill(s)",
		"Could not remove skill from Codex: permission denied",
		"Could not scan directory /skills: permission denied",
	} {
		if skillsRemoveOutputError(output) == "" {
			t.Fatalf("failure output accepted: %q", output)
		}
	}
	for _, output := range []string{
		"Successfully removed 1 skill(s)",
		"No matching skills found for: byted-interactai-voice-agent",
		"No matching skills found for: vertc-voice-agent",
	} {
		if reason := skillsRemoveOutputError(output); reason != "" {
			t.Fatalf("success output rejected: %q (%s)", output, reason)
		}
	}
}

func TestSkillsAddOnlyUnsupportedPromptScript(t *testing.T) {
	output := "■ Failed to install 1\n│\n│ ✗ byted-interactai-guide → PromptScript: PromptScript does not support global skill installation\n"
	if !skillsAddOnlyUnsupportedPromptScript(output) {
		t.Fatal("expected the single unsupported PromptScript target to be recoverable")
	}
	for _, output := range []string{
		"■ Failed to install 1\n│ ✗ byted-interactai-guide → Codex: permission denied\n",
		"■ Failed to install 2\n│ ✗ byted-interactai-guide → PromptScript: PromptScript does not support global skill installation\n│ ✗ byted-interactai-guide → Codex: permission denied\n",
	} {
		if skillsAddOnlyUnsupportedPromptScript(output) {
			t.Fatalf("unexpected recoverable installer output: %q", output)
		}
	}
}

func TestSyncSkillsFailureDoesNotAdvanceVersion(t *testing.T) {
	t.Cleanup(resetOverrides)
	stateDir := t.TempDir()
	t.Setenv("VERTC_STATE_DIR", stateDir)
	if err := skillscheck.MarkSynced("1.0.0"); err != nil {
		t.Fatal(err)
	}
	SkillsAddOverride = func(context.Context, string, ...string) error {
		return errors.New("installer failed")
	}

	if _, err := SyncSkills(context.Background(), "2.0.0"); err == nil {
		t.Fatal("expected sync failure")
	}
	state, err := os.ReadFile(filepath.Join(stateDir, "skills-state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(state), `"version":"1.0.0"`) {
		t.Fatalf("sync failure advanced state: %s", state)
	}
}
