// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/meta"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/selfupdate"
)

func withUpdateOverrides(t *testing.T, latest string) {
	t.Helper()
	oldFetch, oldVersion, oldFormat, oldDryRun := fetchLatest, meta.Version, flagFormat, flagDryRun
	fetchLatest = func(context.Context) (string, error) { return latest, nil }
	meta.Version = "1.0.0"
	flagFormat = "json"
	flagDryRun = false
	t.Cleanup(func() {
		fetchLatest, meta.Version, flagFormat, flagDryRun = oldFetch, oldVersion, oldFormat, oldDryRun
		selfupdate.ExecutableOverride = nil
		selfupdate.NpmInstallOverride = nil
		selfupdate.VerifyOverride = nil
		selfupdate.SkillsSyncOverride = nil
		selfupdate.SkillsAddOverride = nil
	})
}

func TestRunUpdateReportsManualRequired(t *testing.T) {
	withUpdateOverrides(t, "2.0.0")
	selfupdate.ExecutableOverride = func() (string, error) { return "/usr/local/bin/vertc", nil }
	result := captureUpdateResult(t, func() error { return runUpdate(context.Background(), false, false) })
	if result.Status != "manual_required" || result.Install != selfupdate.InstallManual || result.ManualCommand == "" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func captureUpdateResult(t *testing.T, run func() error) updateResult {
	t.Helper()
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStdout := os.Stdout
	os.Stdout = write
	defer func() { os.Stdout = oldStdout }()
	if err := run(); err != nil {
		t.Fatal(err)
	}
	_ = write.Close()
	body, err := io.ReadAll(read)
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Data updateResult `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("invalid output %q: %v", body, err)
	}
	return envelope.Data
}

func TestRunUpdateDryRunHasNoSideEffects(t *testing.T) {
	withUpdateOverrides(t, "2.0.0")
	flagDryRun = true
	selfupdate.ExecutableOverride = func() (string, error) {
		return "/prefix/lib/node_modules/@volcengine/rtc-cli/scripts/bin/vertc", nil
	}
	selfupdate.NpmInstallOverride = func(context.Context, string, string) error { t.Fatal("npm called during dry-run"); return nil }
	selfupdate.VerifyOverride = func(context.Context, string, string) error { t.Fatal("verify called during dry-run"); return nil }
	selfupdate.SkillsSyncOverride = func(context.Context, string) (selfupdate.SkillsSyncResult, error) {
		t.Fatal("skills sync called during dry-run")
		return selfupdate.SkillsSyncResult{}, nil
	}
	result := captureUpdateResult(t, func() error { return runUpdate(context.Background(), false, false) })
	if result.Status != "would_update" || result.Install != selfupdate.InstallNPM {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestRunUpdateDryRunCurrentDoesNotSyncSkills(t *testing.T) {
	withUpdateOverrides(t, "1.0.0")
	flagDryRun = true
	selfupdate.ExecutableOverride = func() (string, error) { return "/usr/local/bin/vertc", nil }
	selfupdate.SkillsSyncOverride = func(context.Context, string) (selfupdate.SkillsSyncResult, error) {
		t.Fatal("skills sync called during dry-run")
		return selfupdate.SkillsSyncResult{}, nil
	}
	result := captureUpdateResult(t, func() error { return runUpdate(context.Background(), false, false) })
	if result.Status != "up_to_date" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestRunUpdateCheckReportsSkillsStatus(t *testing.T) {
	withUpdateOverrides(t, "2.0.0")
	t.Setenv("VERTC_STATE_DIR", t.TempDir())
	for _, key := range []string{"CI", "CONTINUOUS_INTEGRATION", "GITHUB_ACTIONS", "GITLAB_CI", "BUILDKITE", "JENKINS_URL", "TF_BUILD", "CIRCLECI", "TRAVIS", "TEAMCITY_VERSION", "CODEBUILD_BUILD_ID", "VERTC_NO_SKILLS_NOTIFIER"} {
		t.Setenv(key, "")
	}
	selfupdate.ExecutableOverride = func() (string, error) { return "/usr/local/bin/vertc", nil }
	result := captureUpdateResult(t, func() error { return runUpdate(context.Background(), true, false) })
	if result.Status != "manual_required" || result.SkillsStatus != "out_of_sync" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestRunUpdateRollsBackAfterVerifyFailure(t *testing.T) {
	withUpdateOverrides(t, "2.0.0")
	dir := filepath.Join(t.TempDir(), "lib", "node_modules", "@volcengine", "rtc-cli", "scripts", "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(dir, "vertc")
	selfupdate.ExecutableOverride = func() (string, error) { return binary, nil }
	selfupdate.NpmInstallOverride = func(context.Context, string, string) error {
		if err := os.WriteFile(binary, []byte("new"), 0o700); err != nil {
			return err
		}
		return os.WriteFile(binary+".old", []byte("old"), 0o700)
	}
	selfupdate.VerifyOverride = func(context.Context, string, string) error { return os.ErrInvalid }
	err := runUpdate(context.Background(), false, false)
	typed, ok := errs.As(err)
	if !ok || typed.Code != "vertc.update.verify_failed" {
		t.Fatalf("unexpected error: %v", err)
	}
	data, readErr := os.ReadFile(binary)
	if readErr != nil || string(data) != "old" {
		t.Fatalf("binary was not rolled back: data=%q err=%v", data, readErr)
	}
}

func TestRunUpdateReportsPartialSkillsFailure(t *testing.T) {
	withUpdateOverrides(t, "2.0.0")
	executable := "/usr/local/lib/node_modules/@volcengine/rtc-cli/scripts/bin/vertc"
	selfupdate.ExecutableOverride = func() (string, error) {
		return executable, nil
	}
	selfupdate.NpmInstallOverride = func(_ context.Context, _ string, path string) error {
		if path != executable {
			t.Fatalf("install path=%q, want %q", path, executable)
		}
		return nil
	}
	selfupdate.VerifyOverride = func(_ context.Context, _ string, path string) error {
		if path != executable {
			t.Fatalf("verify path=%q, want %q", path, executable)
		}
		return nil
	}
	selfupdate.SkillsSyncOverride = func(context.Context, string) (selfupdate.SkillsSyncResult, error) {
		return selfupdate.SkillsSyncResult{}, os.ErrNotExist
	}

	result := captureUpdateResult(t, func() error { return runUpdate(context.Background(), false, false) })
	if result.Status != "updated" || result.SkillsError == "" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestRunUpdateSynchronizesSkillsWhenBinaryIsCurrent(t *testing.T) {
	withUpdateOverrides(t, "1.0.0")
	called := false
	selfupdate.SkillsSyncOverride = func(_ context.Context, version string) (selfupdate.SkillsSyncResult, error) {
		called = true
		if version != "1.0.0" {
			t.Fatalf("version=%q", version)
		}
		return selfupdate.SkillsSyncResult{}, nil
	}

	result := captureUpdateResult(t, func() error { return runUpdate(context.Background(), false, false) })
	if !called {
		t.Fatal("expected skills synchronization")
	}
	if result.Status != "up_to_date" || !result.SkillsSynced {
		t.Fatalf("unexpected result: %+v", result)
	}
}
