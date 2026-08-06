// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// issueEnv is a resolvable AppID (24 chars) + AppKey for local token signing.
var issueEnv = []string{"RTC_APP_ID=app123456789012345678901", "RTC_APP_KEY=secretkeyvalue12345"} // public-scan: allow; gitleaks:allow — synthetic test credentials

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// TestInitScaffoldsRoomUser: --room-id/--user-id land in config, target_user_id
// is left empty (auto-follows rtc.user_id), and the summary is emitted.
func TestInitScaffoldsRoomUser(t *testing.T) {
	dir := t.TempDir()
	r := run(t, dir, nil, "init", ".", "--scene", "voice-agent", "--platform", "web", "--name", "va",
		"--room-id", "room-2", "--user-id", "alice")
	if r.code != 0 {
		t.Fatalf("init exit %d: %s", r.code, r.stderr)
	}
	cfg := readFile(t, filepath.Join(dir, "vertc.config.yaml"))
	if !strings.Contains(cfg, "room_id: room-2") || !strings.Contains(cfg, "user_id: alice") {
		t.Fatalf("config missing scaffolded room/user:\n%s", cfg)
	}
	// target_user_id must stay empty so it auto-follows rtc.user_id.
	if !strings.Contains(cfg, `target_user_id: ""`) {
		t.Fatalf("expected empty target_user_id (auto-follow):\n%s", cfg)
	}
	// Structured summary carries room/user/app.
	data, _ := r.envelope(t)["data"].(map[string]any)
	if data["room_id"] != "room-2" || data["user_id"] != "alice" || data["app_id"] != "${RTC_APP_ID}" {
		t.Fatalf("unexpected init summary: %v", data)
	}
}

// TestInitDefaultRoomUser: without flags the room-01/user-01 defaults persist.
func TestInitDefaultRoomUser(t *testing.T) {
	dir := t.TempDir()
	r := run(t, dir, nil, "init", ".", "--scene", "voice-agent", "--platform", "web", "--name", "demo")
	if r.code != 0 {
		t.Fatalf("init exit %d: %s", r.code, r.stderr)
	}
	data, _ := r.envelope(t)["data"].(map[string]any)
	if data["room_id"] != "room-01" || data["user_id"] != "user-01" {
		t.Fatalf("expected default room-01/user-01, got %v", data)
	}
}

// TestInitInvalidIdentityRejected: a disallowed identity is caught before any
// file is written.
func TestInitInvalidIdentityRejected(t *testing.T) {
	dir := t.TempDir()
	r := run(t, dir, nil, "init", "./demo", "--scene", "voice-agent", "--platform", "web", "--user-id", "bad/slash")
	if r.code == 0 {
		t.Fatal("expected non-zero exit for invalid user-id")
	}
	errObj, _ := r.envelope(t)["error"].(map[string]any)
	if errObj["code"] != "vertc.config.invalid_identity" {
		t.Fatalf("expected vertc.config.invalid_identity, got %v", errObj)
	}
	if _, err := os.Stat(filepath.Join(dir, "demo")); err == nil {
		t.Fatal("invalid identity must not scaffold a project")
	}
}

// TestTokenIssueWriteThrough: --room-id/--user-id --write persists to config and
// re-syncs the VITE_* consumers; without --write it only affects the signing.
func TestTokenIssueWriteThrough(t *testing.T) {
	dir := setupProject(t)
	if r := run(t, dir, issueEnv, "token", "issue", "--room-id", "room-x", "--user-id", "user-x", "--write"); r.code != 0 {
		t.Fatalf("token issue --write: %s", r.stderr)
	}
	cfg := readFile(t, filepath.Join(dir, "vertc.config.yaml"))
	if !strings.Contains(cfg, "room_id: room-x") || !strings.Contains(cfg, "user_id: user-x") {
		t.Fatalf("write-through did not update config:\n%s", cfg)
	}
	env := readFile(t, filepath.Join(dir, ".env.local"))
	if !strings.Contains(env, "VITE_RTC_ROOM_ID=room-x") || !strings.Contains(env, "VITE_RTC_USER_ID=user-x") {
		t.Fatalf(".env.local not re-synced to new room/user:\n%s", env)
	}
	// Without --write, config must NOT change (only this issuance uses room-y).
	if r := run(t, dir, issueEnv, "token", "issue", "--room-id", "room-y"); r.code != 0 {
		t.Fatalf("token issue (no write): %s", r.stderr)
	}
	if cfg2 := readFile(t, filepath.Join(dir, "vertc.config.yaml")); !strings.Contains(cfg2, "room_id: room-x") {
		t.Fatalf("no-write issuance leaked into config:\n%s", cfg2)
	}
}

// TestTokenWriteThroughFollowsTarget: writing through rtc.user_id keeps
// agent.target_user_id following it (empty stays empty; a target equal to the
// old user_id advances to the new one).
func TestTokenWriteThroughFollowsTarget(t *testing.T) {
	// Case 1: empty target auto-follows — stays empty after write-through.
	dir := t.TempDir()
	if r := run(t, dir, nil, "init", ".", "--scene", "voice-agent", "--platform", "web", "--name", "va"); r.code != 0 {
		t.Fatalf("init: %s", r.stderr)
	}
	if r := run(t, dir, issueEnv, "token", "issue", "--user-id", "carol", "--write"); r.code != 0 {
		t.Fatalf("token issue: %s", r.stderr)
	}
	cfg := readFile(t, filepath.Join(dir, "vertc.config.yaml"))
	if !strings.Contains(cfg, "user_id: carol") || !strings.Contains(cfg, `target_user_id: ""`) {
		t.Fatalf("empty target should stay empty after write-through:\n%s", cfg)
	}

	// Case 2: a target explicitly equal to the old rtc.user_id advances with it.
	dir2 := t.TempDir()
	if r := run(t, dir2, nil, "init", ".", "--scene", "voice-agent", "--platform", "web", "--name", "va"); r.code != 0 {
		t.Fatalf("init: %s", r.stderr)
	}
	if r := run(t, dir2, nil, "config", "set", "agent.target_user_id", "user-01"); r.code != 0 {
		t.Fatalf("config set target: %s", r.stderr)
	}
	if r := run(t, dir2, issueEnv, "token", "issue", "--user-id", "bob", "--write"); r.code != 0 {
		t.Fatalf("token issue: %s", r.stderr)
	}
	cfg2 := readFile(t, filepath.Join(dir2, "vertc.config.yaml"))
	if !strings.Contains(cfg2, "user_id: bob") || !strings.Contains(cfg2, "target_user_id: bob") {
		t.Fatalf("target should follow rtc.user_id to bob:\n%s", cfg2)
	}

	// Case 3: config placeholders are compared by their resolved identity.
	dir3 := t.TempDir()
	if r := run(t, dir3, nil, "init", ".", "--scene", "voice-agent", "--platform", "web", "--name", "va"); r.code != 0 {
		t.Fatalf("init: %s", r.stderr)
	}
	if r := run(t, dir3, nil, "config", "set", "rtc.user_id", "${RTC_REAL_USER}"); r.code != 0 {
		t.Fatalf("config set rtc user: %s", r.stderr)
	}
	if r := run(t, dir3, nil, "config", "set", "agent.target_user_id", "user-01"); r.code != 0 {
		t.Fatalf("config set target: %s", r.stderr)
	}
	placeholderEnv := append(append([]string{}, issueEnv...), "RTC_REAL_USER=user-01")
	if r := run(t, dir3, placeholderEnv, "token", "issue", "--user-id", "dave", "--write"); r.code != 0 {
		t.Fatalf("token issue with resolved old user: %s", r.stderr)
	}
	cfg3 := readFile(t, filepath.Join(dir3, "vertc.config.yaml"))
	if !strings.Contains(cfg3, "user_id: dave") || !strings.Contains(cfg3, "target_user_id: dave") {
		t.Fatalf("resolved target should follow rtc.user_id to dave:\n%s", cfg3)
	}
}

func TestTokenIssueRejectsUnresolvedIdentityEnv(t *testing.T) {
	dir := setupProject(t)
	if r := run(t, dir, nil, "config", "set", "rtc.room_id", "${RTC_MISSING_ROOM_FOR_TEST}"); r.code != 0 {
		t.Fatalf("config set room: %s", r.stderr)
	}
	r := run(t, dir, issueEnv, "token", "issue")
	if r.code == 0 {
		t.Fatal("expected unresolved room env to fail")
	}
	errObj, _ := r.envelope(t)["error"].(map[string]any)
	if errObj["code"] != "vertc.config.unresolved_env" || errObj["param"] != "rtc.room_id" {
		t.Fatalf("unexpected error envelope: %v", errObj)
	}
}

// TestTokenIssueDryRunNoWriteThrough: --write --dry-run signs but writes nothing.
func TestTokenIssueDryRunNoWriteThrough(t *testing.T) {
	dir := setupProject(t)
	if r := run(t, dir, issueEnv, "token", "issue", "--room-id", "room-z", "--user-id", "user-z", "--write", "--dry-run"); r.code != 0 {
		t.Fatalf("token issue dry-run: %s", r.stderr)
	}
	cfg := readFile(t, filepath.Join(dir, "vertc.config.yaml"))
	if strings.Contains(cfg, "room_id: room-z") {
		t.Fatalf("dry-run wrote through to config:\n%s", cfg)
	}
	if _, err := os.Stat(filepath.Join(dir, ".env.local")); err == nil {
		t.Fatal("dry-run must not write .env.local")
	}
}
