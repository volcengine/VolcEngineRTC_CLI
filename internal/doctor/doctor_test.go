// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package doctor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/auth"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/config"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/meta"
)

const validSceneJSON = `{"SceneConfig":{"Name":"Default"},"VoiceChat":{"Config":{"ASRConfig":{"Provider":"volcano","ProviderParams":{}},"LLMConfig":{"Mode":"ArkV3","EndPointId":"ep-test"},"TTSConfig":{"Provider":"volcano","ProviderParams":{}}},"AgentConfig":{"UserId":"voice_agent"}}}`

const validCustomLLMSceneJSON = `{"SceneConfig":{"name":"Custom"},"VoiceChat":{"Config":{"ASRConfig":{"Provider":"volcano","ProviderParams":{}},"LLMConfig":{"Mode":"CustomLLM","Url":"https://api.deepseek.com/chat/completions","ModelName":"deepseek-v4-flash","Custom":"{\"thinking\":{\"type\":\"disabled\"}}"},"TTSConfig":{"Provider":"volcano_bidirection","ProviderParams":{}}},"AgentConfig":{"UserId":"custom-agent"}}}`

func writeDefaultScene(t *testing.T, dir string, raw string) {
	t.Helper()
	path := filepath.Join(dir, "server", "scenes", "default.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
}

func findCheck(checks []Check, id string) (Check, bool) {
	for _, c := range checks {
		if c.ID == id {
			return c, true
		}
	}
	return Check{}, false
}

func TestAgentChecksAuth(t *testing.T) {
	cfg := config.Default("demo", "voice-agent", "web")
	restore := auth.SetTokenStoreForTest(auth.NewMemoryTokenStore())
	defer restore()

	// No Signin token -> FAIL.
	c, ok := findCheck(agentChecks(cfg, t.TempDir()), "project.agent.auth")
	if !ok || c.Status != FAIL {
		t.Fatalf("expected agent.auth FAIL without token, got %+v", c)
	}

	stsJSON, _ := json.Marshal(auth.STSCredential{AccessKeyID: "AKID", SecretAccessKey: "SECRET", SessionToken: "SESSION"})
	if err := auth.SaveToken(auth.TokenSet{
		AccessToken:  string(stsJSON),
		RefreshToken: "refresh",
		ClientID:     auth.DefaultLocalClientID,
		ExpiresAt:    time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatal(err)
	}
	c, _ = findCheck(agentChecks(cfg, t.TempDir()), "project.agent.auth")
	if c.Status != PASS {
		t.Fatalf("expected agent.auth PASS with STS token, got %+v", c)
	}
}

func TestAgentChecksTargetConsistency(t *testing.T) {
	cfg := config.Default("demo", "voice-agent", "web")
	cfg.Agent.TargetUserID = "someone-else" // != rtc.user_id (user-01)
	c, ok := findCheck(agentChecks(cfg, t.TempDir()), "project.agent.target")
	if !ok || c.Status != WARN {
		t.Fatalf("expected agent.target WARN on mismatch, got %+v", c)
	}
}

func TestVoiceAgentNotInCLITier(t *testing.T) {
	// agent checks belong to the project tier only.
	for _, c := range RunCLI() {
		if c.Level != LevelCLI {
			t.Fatalf("RunCLI produced a non-CLI check: %+v", c)
		}
	}
}

func TestCLIUpdateUnknownIsNotPass(t *testing.T) {
	oldVersion := meta.Version
	meta.Version = "1.0.0"
	t.Cleanup(func() { meta.Version = oldVersion })
	t.Setenv("VERTC_STATE_DIR", t.TempDir())
	t.Setenv("CI", "")
	t.Setenv("CONTINUOUS_INTEGRATION", "")
	t.Setenv("GITHUB_ACTIONS", "")
	t.Setenv("GITLAB_CI", "")
	t.Setenv("BUILDKITE", "")
	t.Setenv("JENKINS_URL", "")
	t.Setenv("TF_BUILD", "")
	t.Setenv("CIRCLECI", "")
	t.Setenv("TRAVIS", "")
	t.Setenv("TEAMCITY_VERSION", "")
	t.Setenv("CODEBUILD_BUILD_ID", "")
	t.Setenv("VERTC_NO_UPDATE_NOTIFIER", "")
	check, ok := findCheck(RunCLI(), "cli.update")
	if !ok || check.Status != UNKNOWN {
		t.Fatalf("expected UNKNOWN update evidence, got %+v", check)
	}
	report := summarize([]Check{check})
	if report.Passed != 0 || report.Unknown != 1 || !report.Runnable {
		t.Fatalf("unexpected report: %+v", report)
	}
}

func TestCLIUpdateSkippedInCI(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "true")
	check, ok := findCheck(RunCLI(), "cli.update")
	if !ok || check.Status != SKIP {
		t.Fatalf("expected SKIP update evidence, got %+v", check)
	}
}

func TestServerManagedAgentChecksServerConfigAndCredentials(t *testing.T) {
	t.Setenv("VOLCENGINE_ACCESS_KEY_ID", "AKID")
	t.Setenv("VOLCENGINE_SECRET_ACCESS_KEY", "SECRET")
	dir := t.TempDir()
	writeDefaultScene(t, dir, validSceneJSON)
	checks := serverManagedAgentChecks(config.Default("demo", "voice-agent", "web"), dir)
	if c, ok := findCheck(checks, "project.agent.auth"); !ok || c.Status != PASS {
		t.Fatalf("expected server auth PASS, got %+v", c)
	}
	if c, ok := findCheck(checks, "project.agent.config"); !ok || c.Status != PASS {
		t.Fatalf("expected server config PASS, got %+v", c)
	}
}

func TestServerManagedAgentConfigRejectsIncompleteShape(t *testing.T) {
	dir := t.TempDir()
	writeDefaultScene(t, dir, `{}`)
	check := serverAgentConfigCheck(config.Default("demo", "voice-agent", "web"), dir)
	if check.Status != FAIL || check.Hint == "" {
		t.Fatalf("incomplete server agent config should fail with guidance: %+v", check)
	}
}

func TestServerManagedAgentConfigAcceptsCustomLLM(t *testing.T) {
	dir := t.TempDir()
	writeDefaultScene(t, dir, validCustomLLMSceneJSON)
	check := serverAgentConfigCheck(config.Default("demo", "voice-agent", "web"), dir)
	if check.Status != PASS {
		t.Fatalf("doctor rejected valid CustomLLM scene: %+v", check)
	}
}

func TestServerManagedAgentConfigRejectsCustomLLMWithoutURLWithoutExposingAPIKey(t *testing.T) {
	sentinel := strings.Repeat("x", 37)
	dir := t.TempDir()
	raw := strings.Replace(validCustomLLMSceneJSON, `"Url":"https://api.deepseek.com/chat/completions",`, "", 1)
	extraField := `"API` + `Key":"` + sentinel + `",`
	raw = strings.Replace(raw, `"Custom":`, extraField+`"Custom":`, 1)
	writeDefaultScene(t, dir, raw)
	check := serverAgentConfigCheck(config.Default("demo", "voice-agent", "web"), dir)
	if check.Status != FAIL || !strings.Contains(check.Detail, "VoiceChat.Config.LLMConfig.Url") || strings.Contains(check.Detail, sentinel) {
		t.Fatalf("doctor invalid CustomLLM check = %+v, want Url path without secret", check)
	}
}

func TestServerManagedProjectIssuesTokenAtRuntime(t *testing.T) {
	t.Setenv("RTC_APP_ID", "appid")
	t.Setenv("RTC_APP_KEY", "appkey")
	t.Setenv("VOLCENGINE_ACCESS_KEY_ID", "AKID")
	t.Setenv("VOLCENGINE_SECRET_ACCESS_KEY", "SECRET")
	dir := t.TempDir()
	writeDefaultScene(t, dir, validSceneJSON)
	taskfile := "version: 2\nscene: voice-agent\nplatform: web\nruntime:\n  topology: web-server\n  agent_control: server\n"
	if err := os.WriteFile(filepath.Join(dir, "vertc.taskfile.yaml"), []byte(taskfile), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default("demo", "voice-agent", "web")
	c, ok := findCheck(RunProject(cfg, "", dir), "project.token")
	if !ok || c.Status != PASS || c.Detail != "issued by the companion server for each session" {
		t.Fatalf("server-managed token check = %+v", c)
	}
}

func TestMissingRTCAppCredentialsWarnWhenDevCanBootstrap(t *testing.T) {
	t.Setenv("RTC_APP_ID", "")
	t.Setenv("RTC_APP_KEY", "")
	cfg := config.Default("demo", "test-scene", "web")
	checks := RunProject(cfg, "", t.TempDir())

	appID, ok := findCheck(checks, "project.config.rtc.app_id")
	if !ok || appID.Status != WARN || !strings.Contains(appID.Hint, "dev") {
		t.Fatalf("bootstrap-eligible AppID check = %+v, want WARN with dev guidance", appID)
	}
	credential, ok := findCheck(checks, "project.credential")
	if !ok || credential.Status != WARN || !strings.Contains(credential.Detail, "dev setup will fill it") {
		t.Fatalf("bootstrap-eligible credential check = %+v, want WARN", credential)
	}
}

func TestMissingRTCAppKeyFailsWhenDevCannotBootstrap(t *testing.T) {
	t.Setenv("RTC_APP_KEY", "")
	cfg := config.Default("demo", "test-scene", "web")
	cfg.RTC.AppID = "fixed-app-id"
	credential, ok := findCheck(RunProject(cfg, "", t.TempDir()), "project.credential")
	if !ok || credential.Status != FAIL {
		t.Fatalf("non-bootstrap credential check = %+v, want FAIL", credential)
	}
}
