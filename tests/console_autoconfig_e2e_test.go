// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These E2E cases exercise the offline-deterministic setup surface. `init` only
// scaffolds and guides to auth login followed by the first interactive dev run;
// OpenAPI selection is covered by command and package unit tests.

func nextStepsOf(t *testing.T, r result) []string {
	t.Helper()
	data, _ := r.envelope(t)["data"].(map[string]any)
	raw, _ := data["next_steps"].([]any)
	steps := make([]string, 0, len(raw))
	for _, s := range raw {
		if str, ok := s.(string); ok {
			steps = append(steps, str)
		}
	}
	if len(steps) == 0 {
		t.Fatalf("no next_steps in envelope: %v", data)
	}
	return steps
}

func TestVoiceAgentInitGuidesToLoginThenDev(t *testing.T) {
	dir := t.TempDir()
	r := run(t, dir, nil, "init", ".", "--scene", "voice-agent", "--platform", "web", "--name", "va")
	if r.code != 0 {
		t.Fatalf("init exit %d: %s", r.code, r.stderr)
	}
	steps := strings.Join(nextStepsOf(t, r), "\n")
	if !strings.Contains(steps, "auth login") || !strings.Contains(steps, "首次运行选择 App/Bot") {
		t.Fatalf("next_steps should lead through auth login then first dev setup:\n%s", steps)
	}
	if strings.Contains(steps, "代码示例") {
		t.Fatalf("default next_steps should not ask to paste 代码示例:\n%s", steps)
	}
	if _, err := os.Stat(dir + "/.env.local"); !os.IsNotExist(err) {
		t.Fatal("init must not write .env.local")
	}
	data, _ := r.envelope(t)["data"].(map[string]any)
	if data["provision"] != nil {
		t.Fatalf("init must not expose a provision result: %v", data["provision"])
	}
}

func TestVoiceAgentInitRejectsRemovedProvisionFlags(t *testing.T) {
	dir := t.TempDir()
	r := run(t, dir, nil, "init", ".", "--scene", "voice-agent", "--platform", "web", "--name", "va", "--no-provision")
	if r.code == 0 {
		t.Fatalf("removed flag should fail, exit=%d stderr=%s", r.code, r.stderr)
	}
}

func TestNonInteractiveDevRoutesToAuthWithoutRequestingAppKey(t *testing.T) {
	dir := t.TempDir()
	if r := run(t, dir, nil, "init", ".", "--scene", "voice-agent", "--platform", "web", "--name", "va"); r.code != 0 {
		t.Fatalf("init: %s", r.stderr)
	}
	r := run(t, dir, nil, "dev")
	if r.code == 0 {
		t.Fatal("non-interactive first dev should require authentication or public resource selection")
	}
	envelope := r.envelope(t)
	errorData, _ := envelope["error"].(map[string]any)
	errorCode, _ := errorData["code"].(string)
	if errorCode != "vertc.auth.not_authenticated" && errorCode != "vertc.auth.storage_unavailable" {
		t.Fatalf("unexpected non-interactive auth error %q: %s%s", errorCode, r.stdout, r.stderr)
	}
	body := r.stdout + r.stderr
	if strings.Contains(body, "dev --list-resources") {
		t.Fatalf("unexpected non-interactive recovery: %s", body)
	}
	if strings.Contains(body, "--app-key") || strings.Contains(body, "provide AppKey") {
		t.Fatalf("non-interactive recovery requested AppKey: %s", body)
	}
	if _, err := os.Stat(filepath.Join(dir, ".env.local")); !os.IsNotExist(err) {
		t.Fatalf("failed non-interactive setup wrote .env.local: %v", err)
	}
}

func TestDevListResourcesIsNotACommandSurface(t *testing.T) {
	dir := t.TempDir()
	if r := run(t, dir, nil, "init", ".", "--scene", "voice-agent", "--platform", "web", "--name", "va"); r.code != 0 {
		t.Fatalf("init: %s", r.stderr)
	}
	r := run(t, dir, nil, "dev", "--list-resources")
	if r.code == 0 || !strings.Contains(r.stdout+r.stderr, "vertc.cli.invalid_flag") {
		t.Fatalf("removed discovery flag should fail locally: stdout=%s stderr=%s", r.stdout, r.stderr)
	}
}

// Credentials written to .env.local (as console provision does) must be visible
// to the CLI's own ${ENV} resolution without a manual `export`: `token issue`
// resolves rtc.app_id=${RTC_APP_ID} and reads RTC_APP_KEY straight from the file.
// This is the regression for the provision→token gap.
func TestEnvLocalLoadedForTokenIssue(t *testing.T) {
	dir := t.TempDir()
	if r := run(t, dir, nil, "init", ".", "--scene", "voice-agent", "--platform", "web", "--name", "va"); r.code != 0 {
		t.Fatalf("init: %s", r.stderr)
	}
	// Simulate what provision lands: AppId + AppKey in .env.local only (no export).
	envFile := filepath.Join(dir, ".env.local")
	content := "RTC_APP_ID=abcdef012345678901234567\nRTC_APP_KEY=secretkeyvalue0000000000000000000\n" // public-scan: allow; gitleaks:allow — synthetic test credentials
	if err := os.WriteFile(envFile, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	// No env passed to the process — the value must come from .env.local alone.
	r := run(t, dir, nil, "token", "issue", "--write")
	if r.code != 0 {
		t.Fatalf("token issue should resolve ${RTC_APP_ID} from .env.local, exit %d: %s", r.code, r.stderr)
	}
	envBytes, _ := os.ReadFile(envFile)
	if !strings.Contains(string(envBytes), "VITE_RTC_TOKEN=") {
		t.Fatalf(".env.local missing VITE_RTC_TOKEN after issue --write:\n%s", envBytes)
	}
}

func TestVoiceAgentSetupInitDryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	r := run(t, dir, nil, "init", "--scene", "voice-agent", "--platform", "web", "--name", "va", "--dry-run")
	if r.code != 0 {
		t.Fatalf("dry-run exit %d: %s", r.code, r.stderr)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("dry-run wrote files: %v", entries)
	}
}
