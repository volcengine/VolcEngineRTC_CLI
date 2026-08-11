// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVoiceAgentInitDryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	r := run(t, dir, nil, "init", "--scene", "voice-agent", "--platform", "web", "--name", "va", "--dry-run")
	if r.code != 0 {
		t.Fatalf("dry-run exit %d: %s", r.code, r.stderr)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("dry-run wrote files: %v", entries)
	}
}

func TestVoiceAgentInitSeedsFullStackProject(t *testing.T) {
	dir := t.TempDir()
	if r := run(t, dir, nil, "init", ".", "--scene", "voice-agent", "--platform", "web", "--name", "va"); r.code != 0 {
		t.Fatalf("init: stderr=%s stdout=%s", r.stderr, r.stdout)
	}
	cfg, err := os.ReadFile(filepath.Join(dir, "vertc.config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"scene: voice-agent", "config_file: server/scenes/default.json", "user_id: voice_agent", "target_user_id:"} {
		if !strings.Contains(string(cfg), want) {
			t.Fatalf("config missing %q:\n%s", want, cfg)
		}
	}
	for _, want := range []string{
		".env.example",
		"package.json",
		"web/package.json",
		"web/src/App.tsx",
		"server/package.json",
		"server/app.js",
		"server/scenes/default.json",
		"vertc.template.yaml",
	} {
		if _, err := os.Stat(filepath.Join(dir, want)); err != nil {
			t.Fatalf("generated full-stack file %s missing: %v", want, err)
		}
	}
	serverApp := readFile(t, filepath.Join(dir, "server", "app.js"))
	webCommon := readFile(t, filepath.Join(dir, "web", "src", "lib", "useCommon.ts"))
	if !strings.Contains(serverApp, "BusinessId: config.businessId") ||
		!strings.Contains(serverApp, "VERTC_OPENAPI_USER_AGENT") ||
		!strings.Contains(serverApp, `requestData.headers["User-Agent"] = config.openApiUserAgent`) ||
		!strings.Contains(webCommon, "RtcClient.setBusinessId(rtc.BusinessId)") {
		t.Fatalf("generated project is missing BusinessId or OpenAPI User-Agent attribution")
	}
	taskfile, err := os.ReadFile(filepath.Join(dir, "vertc.taskfile.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"version: 2", "topology: web-server", "agent_control: server", "web: 3000", "server: 3001", "yarn dev"} {
		if !strings.Contains(string(taskfile), want) {
			t.Fatalf("taskfile missing %q:\n%s", want, taskfile)
		}
	}
}

func TestServerManagedAgentCommandsRouteToWeb(t *testing.T) {
	dir := t.TempDir()
	if r := run(t, dir, nil, "init", ".", "--scene", "voice-agent", "--platform", "web", "--name", "va"); r.code != 0 {
		t.Fatalf("init: %s", r.stderr)
	}
	for _, action := range []string{"start", "update", "stop", "status"} {
		r := run(t, dir, nil, "agent", action, "--dry-run")
		if r.code == 0 {
			t.Fatalf("agent %s should be server-managed", action)
		}
		e := r.envelope(t)
		errObj, _ := e["error"].(map[string]any)
		if errObj["code"] != "vertc.agent.server_managed" {
			t.Fatalf("agent %s returned unexpected error: %v", action, e)
		}
	}
}

func TestServerManagedDoctorLoadsDotEnvLocal(t *testing.T) {
	dir := t.TempDir()
	if r := run(t, dir, nil, "init", ".", "--scene", "voice-agent", "--platform", "web", "--name", "va"); r.code != 0 {
		t.Fatalf("init: %s", r.stderr)
	}
	if r := run(t, dir, nil, "config", "set", "rtc.app_id", "${VERTC_E2E_LOCAL_APP_ID}"); r.code != 0 {
		t.Fatalf("config set: %s", r.stderr)
	}
	dotenv := "VERTC_E2E_LOCAL_APP_ID=appid-from-dotenv\nRTC_APP_KEY=app-key-from-dotenv\n" // public-scan: allow; gitleaks:allow — synthetic test credentials
	if err := os.WriteFile(filepath.Join(dir, ".env.local"), []byte(dotenv), 0o600); err != nil {
		t.Fatal(err)
	}
	// doctor project as a whole FAILs here (no agent credentials — see
	// TestVoiceAgentDoctorAndExplain); this test only asserts .env.local is
	// loaded, i.e. the credential check picked up RTC_APP_KEY from that file.
	r := run(t, dir, nil, "doctor", "project")
	e := r.envelope(t)
	data, _ := e["data"].(map[string]any)
	checks, _ := data["checks"].([]any)
	credOK := false
	for _, c := range checks {
		m, _ := c.(map[string]any)
		if m["id"] == "project.credential" && m["status"] == "PASS" {
			credOK = true
		}
	}
	if !credOK {
		t.Fatalf("expected project.credential PASS from .env.local RTC_APP_KEY: %v", data)
	}
}

func TestVoiceAgentDoctorAndExplain(t *testing.T) {
	dir := t.TempDir()
	if r := run(t, dir, nil, "init", ".", "--scene", "voice-agent", "--platform", "web", "--name", "va"); r.code != 0 {
		t.Fatalf("init: %s", r.stderr)
	}
	// doctor without Signin auth → non-zero.
	if r := run(t, dir, nil, "doctor", "project"); r.code == 0 {
		t.Fatal("expected doctor FAIL without agent credentials")
	}
	// explain-error covers a conversational-AI code.
	r := run(t, dir, nil, "explain-error", "SignatureDoesNotMatch")
	if r.code != 0 {
		t.Fatalf("explain-error conversational code exit %d", r.code)
	}
	e := r.envelope(t)
	data, _ := e["data"].(map[string]any)
	if data["found"] != true {
		t.Fatalf("expected found=true for SignatureDoesNotMatch: %v", data)
	}
}
