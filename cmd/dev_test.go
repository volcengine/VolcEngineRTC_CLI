// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/config"
	projectenv "github.com/volcengine/VolcEngineRTC_CLI/internal/env"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/meta"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/openapi"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/provision"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/telemetry"
)

type fakeRTCAppClient struct {
	apps         []openapi.RtcApp
	listErr      error
	appKey       string
	appKeyErr    error
	requestedID  string
	listRequests int
	bots         []openapi.Bot
	botErr       error
	botCalls     int
}

const validVoiceAgentScene = `{"SceneConfig":{"name":"Default"},"VoiceChat":{"Config":{"ASRConfig":{"Provider":"volcano","ProviderParams":{}},"LLMConfig":{"Mode":"ArkV3","EndPointId":"ep-test"},"TTSConfig":{"Provider":"volcano","ProviderParams":{}}},"AgentConfig":{"UserId":"default-agent"}}}`

const validCustomLLMVoiceAgentScene = `{"SceneConfig":{"name":"Custom"},"VoiceChat":{"Config":{"ASRConfig":{"Provider":"volcano","ProviderParams":{}},"LLMConfig":{"Mode":"CustomLLM","Url":"https://api.deepseek.com/chat/completions","ModelName":"deepseek-v4-flash","Custom":"{\"thinking\":{\"type\":\"disabled\"}}"},"TTSConfig":{"Provider":"volcano_bidirection","ProviderParams":{}}},"AgentConfig":{"UserId":"custom-agent"}}}`

func (f *fakeRTCAppClient) DescribeNewRtcApps(context.Context, int, int) ([]openapi.RtcApp, error) {
	f.listRequests++
	return f.apps, f.listErr
}

func (f *fakeRTCAppClient) DescribeAppKeys(_ context.Context, appID string) (openapi.AppKeys, error) {
	f.requestedID = appID
	return openapi.AppKeys{AppID: appID, AppKey: f.appKey}, f.appKeyErr
}

func (f *fakeRTCAppClient) AibotxQuery(context.Context, int, int) ([]openapi.Bot, error) {
	f.botCalls++
	return f.bots, f.botErr
}

func TestDevSetupFlags(t *testing.T) {
	cmd := newDevCmd()
	for _, name := range []string{"app-id", "bot-id", "reconfigure"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Fatalf("missing dev flag %q", name)
		}
	}
	if cmd.Flags().Lookup("list-resources") != nil {
		t.Fatal("dev --list-resources should not be exposed")
	}

	err := validateDevSetupFlags(config.Default("demo", "test-scene", "web"), []string{"bot"})
	typed, ok := errs.As(err)
	if !ok || typed.Code != "vertc.cli.invalid_flag" {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateDevSceneAcceptsCustomLLM(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, provision.DefaultConfigFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(validCustomLLMVoiceAgentScene), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateDevScene(dir, provision.DefaultConfigFile); err != nil {
		t.Fatalf("dev rejected valid CustomLLM scene: %v", err)
	}
}

func TestValidateDevSceneRejectsCustomLLMWithoutURLWithoutExposingAPIKey(t *testing.T) {
	sentinel := strings.Repeat("x", 37)
	dir := t.TempDir()
	path := filepath.Join(dir, provision.DefaultConfigFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	raw := strings.Replace(validCustomLLMVoiceAgentScene, `"Url":"https://api.deepseek.com/chat/completions",`, "", 1)
	extraField := `"API` + `Key":"` + sentinel + `",`
	raw = strings.Replace(raw, `"Custom":`, extraField+`"Custom":`, 1)
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	err := validateDevScene(dir, provision.DefaultConfigFile)
	if err == nil || !strings.Contains(err.Error(), "VoiceChat.Config.LLMConfig.Url") || strings.Contains(err.Error(), sentinel) {
		t.Fatalf("dev invalid CustomLLM error = %v, want Url path without secret", err)
	}
}

func TestSelectionRequiredDetailsExposeOnlyPublicMetadata(t *testing.T) {
	err := devBotSelectionRequiredError([]openapi.Bot{{
		ID: "bot-one", Name: "Bot One",
		Config:      json.RawMessage(`{"secret-looking-config":"must-not-appear"}`),
		AgentConfig: map[string]any{"Token": "must-not-appear"},
	}}, "choose")
	raw, marshalErr := json.Marshal(err)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	if !strings.Contains(string(raw), `"bot_id":"bot-one"`) || !strings.Contains(string(raw), `"details"`) {
		t.Fatalf("missing public candidates: %s", raw)
	}
	for _, forbidden := range []string{"must-not-appear", "AppKey", "Config", "AgentConfig"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("selection details leaked %q: %s", forbidden, raw)
		}
	}
}

func TestExplicitResourceSelectorsValidateIDsAndOrder(t *testing.T) {
	apps := []openapi.RtcApp{{AppID: "active", Status: "1"}}
	if selected, err := selectRTCAppByID(apps, " active "); err != nil || selected.AppID != "active" {
		t.Fatalf("selected app = %+v, err = %v", selected, err)
	}
	if _, err := selectRTCAppByID(apps, "missing"); err == nil {
		t.Fatal("unknown app should fail")
	}

	bots := []openapi.Bot{{ID: "one"}, {ID: "two"}}
	selected, err := selectRTCBotsByID(bots, []string{"two", "one"})
	if err != nil || len(selected) != 2 || selected[0].ID != "two" || selected[1].ID != "one" {
		t.Fatalf("selected bots = %+v, err = %v", selected, err)
	}
	for _, ids := range [][]string{{"missing"}, {"one", "one"}, {""}} {
		if _, err := selectRTCBotsByID(bots, ids); err == nil {
			t.Fatalf("bot IDs %v should fail", ids)
		}
	}
}

func TestBootstrapNonInteractiveReturnsBotCandidatesBeforeAppKey(t *testing.T) {
	oldFactory, oldTerminal := newDevConsoleClient, rtcInputIsTerminal
	fake := &fakeRTCAppClient{
		apps: []openapi.RtcApp{{AppID: "app", Status: "1"}},
		bots: []openapi.Bot{{ID: "bot", Name: "Bot"}},
	}
	newDevConsoleClient = func(*config.Config) devConsoleClient {
		return fake
	}
	rtcInputIsTerminal = func(io.Reader) bool { return false }
	t.Cleanup(func() {
		newDevConsoleClient = oldFactory
		rtcInputIsTerminal = oldTerminal
	})

	_, err := bootstrapDevRTCAppCredentialsWithOptions(
		context.Background(), config.Default("demo", "voice-agent", "web"), t.TempDir(),
		strings.NewReader(""), &bytes.Buffer{}, devSetupOptions{AppID: "app"},
	)
	typed, ok := errs.As(err)
	if !ok || typed.Code != "vertc.dev.selection_required" || typed.Param != "--bot-id" || typed.Details["bots"] == nil {
		t.Fatalf("error = %v", err)
	}
	if fake.requestedID != "" {
		t.Fatal("selection ambiguity fetched AppKey")
	}
	if strings.Contains(typed.Hint, "--app-key") {
		t.Fatalf("hint exposed AppKey argument: %q", typed.Hint)
	}
}

func TestBootstrapZeroBotsUsesAndPersistsDefaultScene(t *testing.T) {
	dir := t.TempDir()
	defaultPath := filepath.Join(dir, provision.DefaultConfigFile)
	if err := os.MkdirAll(filepath.Dir(defaultPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(defaultPath, []byte(validVoiceAgentScene), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default("demo", "voice-agent", "web")
	if err := config.Save(cfg, config.Path(dir)); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RTC_APP_ID", "")
	t.Setenv("RTC_APP_KEY", "")
	t.Setenv("VITE_RTC_APP_ID", "${RTC_APP_ID}")
	fake := &fakeRTCAppClient{
		apps:   []openapi.RtcApp{{AppID: "only-app", Name: "Only", Status: "1"}},
		appKey: "secret",
	}
	oldFactory, oldTerminal := newDevConsoleClient, rtcInputIsTerminal
	newDevConsoleClient = func(*config.Config) devConsoleClient { return fake }
	rtcInputIsTerminal = func(io.Reader) bool { return false }
	t.Cleanup(func() {
		newDevConsoleClient = oldFactory
		rtcInputIsTerminal = oldTerminal
	})

	var prompt bytes.Buffer
	restore, err := bootstrapDevRTCAppCredentials(context.Background(), cfg, dir, strings.NewReader(""), &prompt)
	if err != nil {
		t.Fatal(err)
	}
	defer restore()
	if cfg.Agent.ConfigSource != config.AgentConfigSourceDefault || cfg.Agent.ConfigFile != provision.DefaultConfigFile {
		t.Fatalf("default fallback not persisted in memory: %+v", cfg.Agent)
	}
	reloaded, err := config.Load(config.Path(dir))
	if err != nil || reloaded.Agent.ConfigSource != config.AgentConfigSourceDefault {
		t.Fatalf("default fallback not persisted on disk: %+v, err=%v", reloaded, err)
	}
	if fake.botCalls != 1 || fake.requestedID != "only-app" {
		t.Fatalf("OpenAPI calls: bot=%d appKey=%q", fake.botCalls, fake.requestedID)
	}
	if !strings.Contains(prompt.String(), conversationalAIConsoleURL) {
		t.Fatalf("missing console guidance: %q", prompt.String())
	}
}

func TestBootstrapReusesPersistedBotSceneWithoutQuery(t *testing.T) {
	dir := t.TempDir()
	scene := filepath.Join(dir, "server", "scenes", "bot-existing.json")
	if err := os.MkdirAll(filepath.Dir(scene), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(scene, []byte(validVoiceAgentScene), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default("demo", "voice-agent", "web")
	cfg.Agent.ConfigFile = "server/scenes/bot-existing.json"
	cfg.Agent.ConfigSource = config.AgentConfigSourceBot
	t.Setenv("RTC_APP_ID", "existing-app")
	t.Setenv("RTC_APP_KEY", "existing-key")
	fake := &fakeRTCAppClient{}
	oldFactory := newDevConsoleClient
	newDevConsoleClient = func(*config.Config) devConsoleClient { return fake }
	t.Cleanup(func() { newDevConsoleClient = oldFactory })

	restore, err := bootstrapDevRTCAppCredentials(context.Background(), cfg, dir, strings.NewReader(""), &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	restore()
	if fake.listRequests != 0 || fake.botCalls != 0 || fake.requestedID != "" {
		t.Fatalf("persisted selection unexpectedly queried OpenAPI: %+v", fake)
	}
}

func TestBootstrapBotQueryFailureDoesNotUseDefault(t *testing.T) {
	dir := t.TempDir()
	defaultPath := filepath.Join(dir, provision.DefaultConfigFile)
	if err := os.MkdirAll(filepath.Dir(defaultPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(defaultPath, []byte(validVoiceAgentScene), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default("demo", "voice-agent", "web")
	t.Setenv("RTC_APP_ID", "existing-app")
	t.Setenv("RTC_APP_KEY", "existing-key")
	fake := &fakeRTCAppClient{botErr: errs.New("vertc.openapi.request_failed", errs.TypeIO, "query failed")}
	oldFactory := newDevConsoleClient
	newDevConsoleClient = func(*config.Config) devConsoleClient { return fake }
	t.Cleanup(func() { newDevConsoleClient = oldFactory })

	_, err := bootstrapDevRTCAppCredentials(context.Background(), cfg, dir, strings.NewReader(""), &bytes.Buffer{})
	if err == nil || cfg.Agent.ConfigSource != "" {
		t.Fatalf("query failure silently used default: err=%v agent=%+v", err, cfg.Agent)
	}
}

func TestDevCredentialBootstrapRunsWhenEitherCredentialIsMissing(t *testing.T) {
	cfg := config.Default("demo", "test-scene", "web")
	t.Setenv("RTC_APP_ID", "")
	t.Setenv("RTC_APP_KEY", "")
	if !config.CanBootstrapRTCAppCredentials(cfg, config.Validate(cfg), false) {
		t.Fatal("both missing credentials should trigger bootstrap")
	}

	t.Setenv("RTC_APP_ID", "aabbccddeeff001122334455")
	if !config.CanBootstrapRTCAppCredentials(cfg, config.Validate(cfg), false) {
		t.Fatal("missing AppKey should trigger bootstrap")
	}

	t.Setenv("RTC_APP_ID", "")
	t.Setenv("RTC_APP_KEY", "existing-key")
	if !config.CanBootstrapRTCAppCredentials(cfg, config.Validate(cfg), false) {
		t.Fatal("missing AppID should trigger bootstrap")
	}

	t.Setenv("RTC_APP_ID", "aabbccddeeff001122334455")
	t.Setenv("RTC_APP_KEY", "existing-key")
	if config.CanBootstrapRTCAppCredentials(cfg, config.Validate(cfg), false) {
		t.Fatal("complete credentials should not trigger bootstrap")
	}
	if !config.CanBootstrapRTCAppCredentials(cfg, config.Validate(cfg), true) {
		t.Fatal("forced reconfiguration should trigger bootstrap")
	}
}

func TestDevCredentialBootstrapRejectsOtherConfigErrors(t *testing.T) {
	cfg := config.Default("demo", "test-scene", "web")
	cfg.RTC.RoomID = ""
	t.Setenv("RTC_APP_ID", "")
	t.Setenv("RTC_APP_KEY", "")
	if config.CanBootstrapRTCAppCredentials(cfg, config.Validate(cfg), false) {
		t.Fatal("bootstrap must not hide unrelated config errors")
	}
}

func TestBootstrapRTCAppCredentialsSelectsFetchesAndWritesEnv(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(projectenv.Path(dir), []byte("OTHER=value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RTC_APP_ID", "")
	t.Setenv("RTC_APP_KEY", "")
	t.Setenv("VITE_RTC_APP_ID", "${RTC_APP_ID}")
	fake := &fakeRTCAppClient{
		apps: []openapi.RtcApp{
			{AppID: "111111111111111111111111", Name: "One", Status: "1"},
			{AppID: "222222222222222222222222", Name: "Two", Status: "1"},
		},
		appKey: "selected-secret",
	}
	oldFactory, oldTerminal := newDevConsoleClient, rtcInputIsTerminal
	newDevConsoleClient = func(*config.Config) devConsoleClient { return fake }
	rtcInputIsTerminal = func(io.Reader) bool { return true }
	t.Cleanup(func() {
		newDevConsoleClient = oldFactory
		rtcInputIsTerminal = oldTerminal
	})

	var prompt bytes.Buffer
	restore, err := bootstrapDevRTCAppCredentials(
		context.Background(), config.Default("demo", "test-scene", "web"), dir,
		strings.NewReader("\x1b[B \r"), &prompt,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer restore()
	if fake.requestedID != "222222222222222222222222" {
		t.Fatalf("DescribeAppKeys AppId = %q", fake.requestedID)
	}
	values, err := projectenv.Load(projectenv.Path(dir))
	if err != nil {
		t.Fatal(err)
	}
	if values["RTC_APP_ID"] != fake.requestedID || values["VITE_RTC_APP_ID"] != fake.requestedID {
		t.Fatalf("AppID updates = %+v", values)
	}
	if values["RTC_APP_KEY"] != "selected-secret" || values["OTHER"] != "value" {
		t.Fatalf("env values = %+v", values)
	}
	if os.Getenv("RTC_APP_ID") != fake.requestedID || os.Getenv("RTC_APP_KEY") != "selected-secret" {
		t.Fatal("selected credentials were not applied to the dev process")
	}
	if strings.Contains(prompt.String(), "selected-secret") {
		t.Fatal("prompt leaked AppKey")
	}
	if !strings.Contains(prompt.String(), "↑/↓ move · space select · enter confirm") {
		t.Fatal("selector did not render keyboard instructions")
	}
}

func TestBootstrapRTCAppCredentialsOnlyOffersActiveApps(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RTC_APP_ID", "")
	t.Setenv("RTC_APP_KEY", "")
	t.Setenv("VITE_RTC_APP_ID", "${RTC_APP_ID}")
	fake := &fakeRTCAppClient{
		apps: []openapi.RtcApp{
			{AppID: "disabled-app", Name: "Disabled", Status: "0"},
			{AppID: "active-app", Name: "Active", Status: "1"},
			{AppID: "other-app", Name: "Other", Status: "2"},
		},
		appKey: "selected-secret",
	}
	oldFactory, oldTerminal := newDevConsoleClient, rtcInputIsTerminal
	newDevConsoleClient = func(*config.Config) devConsoleClient { return fake }
	rtcInputIsTerminal = func(io.Reader) bool { return true }
	t.Cleanup(func() {
		newDevConsoleClient = oldFactory
		rtcInputIsTerminal = oldTerminal
	})

	var prompt bytes.Buffer
	restore, err := bootstrapDevRTCAppCredentials(
		context.Background(), config.Default("demo", "test-scene", "web"), dir,
		strings.NewReader(" \r"), &prompt,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer restore()
	if fake.requestedID != "active-app" {
		t.Fatalf("DescribeAppKeys AppId = %q, want active-app", fake.requestedID)
	}
	if strings.Contains(prompt.String(), "Disabled") || strings.Contains(prompt.String(), "Other") {
		t.Fatalf("selector rendered a non-active app: %q", prompt.String())
	}
}

func TestSelectRTCAppKeyboardInteraction(t *testing.T) {
	apps := []openapi.RtcApp{
		{AppID: "one", Name: "One", Status: "1"},
		{AppID: "two", Name: "Two", Status: "1"},
		{AppID: "three", Name: "Three", Status: "1"},
	}

	t.Run("arrow up wraps then space selects and enter confirms", func(t *testing.T) {
		var output bytes.Buffer
		selected, err := selectRTCApp(strings.NewReader("\x1b[A \r"), &output, apps)
		if err != nil {
			t.Fatal(err)
		}
		if selected.AppID != "three" {
			t.Fatalf("selected AppID = %q", selected.AppID)
		}
	})

	t.Run("enter requires a space selection", func(t *testing.T) {
		var output bytes.Buffer
		selected, err := selectRTCApp(strings.NewReader("\r\x1b[B \r"), &output, apps)
		if err != nil {
			t.Fatal(err)
		}
		if selected.AppID != "two" {
			t.Fatalf("selected AppID = %q", selected.AppID)
		}
		if !strings.Contains(output.String(), "select an application with space before confirming") {
			t.Fatal("selector did not explain that space is required")
		}
	})
}

func TestSelectRTCBotsAllowsMultipleSelections(t *testing.T) {
	bots := []openapi.Bot{
		{ID: "one", Name: "One"},
		{ID: "two", Name: "Two"},
		{ID: "three", Name: "Three"},
	}
	var output bytes.Buffer
	selected, err := selectRTCBots(strings.NewReader(" \x1b[B \r"), &output, bots)
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 2 || selected[0].ID != "one" || selected[1].ID != "two" {
		t.Fatalf("selected bots = %+v", selected)
	}
	if !strings.Contains(output.String(), "space toggle") {
		t.Fatal("selector did not render multi-select instructions")
	}
}

func TestBootstrapDevRTCAppCredentialsWritesBotScenesWithoutChangingDefault(t *testing.T) {
	dir := t.TempDir()
	defaultPath := filepath.Join(dir, "server", "scenes", "default.json")
	if err := os.MkdirAll(filepath.Dir(defaultPath), 0o755); err != nil {
		t.Fatal(err)
	}
	defaultScene := []byte(`{"SceneConfig":{"name":"Default","icon":"keep"},"VoiceChat":{"Config":{"ASRConfig":{"Provider":"volcano","ProviderParams":{}},"LLMConfig":{"Mode":"ArkV3","EndPointId":"ep-test"},"TTSConfig":{"Provider":"volcano","ProviderParams":{}}},"AgentConfig":{"UserId":"default-agent"}}}`)
	if err := os.WriteFile(defaultPath, defaultScene, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RTC_APP_ID", "")
	t.Setenv("RTC_APP_KEY", "")
	t.Setenv("VITE_RTC_APP_ID", "${RTC_APP_ID}")
	appClient := &fakeRTCAppClient{
		apps:   []openapi.RtcApp{{AppID: "selected-app", Name: "Selected", Status: "1"}},
		appKey: "selected-secret",
		bots: []openapi.Bot{
			{ID: "one", Name: "First bot", Config: json.RawMessage(`{"ASRConfig":{"Provider":"volcano","ProviderParams":{}},"LLMConfig":{"Mode":"ArkV3","EndPointId":"ep-test"},"TTSConfig":{"Provider":"volcano","ProviderParams":{}}}`), AgentConfig: map[string]any{"WelcomeMessage": "hello one"}},
			{ID: "two", Name: "Second bot", Config: json.RawMessage(`{"ASRConfig":{"Provider":"volcano","ProviderParams":{}},"LLMConfig":{"Mode":"ArkV3","EndPointId":"ep-test"},"TTSConfig":{"Provider":"volcano","ProviderParams":{}}}`), AgentConfig: map[string]any{"WelcomeMessage": "hello two"}},
		}}
	oldAppFactory, oldTerminal := newDevConsoleClient, rtcInputIsTerminal
	newDevConsoleClient = func(*config.Config) devConsoleClient { return appClient }
	rtcInputIsTerminal = func(io.Reader) bool { return true }
	t.Cleanup(func() {
		newDevConsoleClient = oldAppFactory
		rtcInputIsTerminal = oldTerminal
	})

	cfg := config.Default("demo", "voice-agent", "web")
	var prompt bytes.Buffer
	restore, err := bootstrapDevRTCAppCredentials(
		context.Background(), cfg, dir, strings.NewReader(" \x1b[B \r"), &prompt,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer restore()
	if appClient.botCalls != 1 {
		t.Fatalf("AibotxQuery calls = %d", appClient.botCalls)
	}
	after, err := os.ReadFile(defaultPath)
	if err != nil || !bytes.Equal(defaultScene, after) {
		t.Fatalf("default scene changed: %v", err)
	}
	for index, name := range []string{"bot-one.json", "bot-two.json"} {
		raw, err := os.ReadFile(filepath.Join(filepath.Dir(defaultPath), name))
		if err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
		scene, err := config.ParseServerSceneJSON(raw)
		if err != nil {
			t.Fatalf("invalid %s: %v", name, err)
		}
		if scene.SceneConfig["name"] != appClient.bots[index].Name || scene.VoiceChat.AgentConfig["WelcomeMessage"] != appClient.bots[index].AgentConfig["WelcomeMessage"] || scene.VoiceChat.AgentConfig["UserId"] != "default-agent" {
			t.Fatalf("%s does not match selected bot: %+v", name, scene)
		}
	}
	if cfg.Agent.ConfigSource != config.AgentConfigSourceBot || cfg.Agent.ConfigFile != "server/scenes/bot-one.json" {
		t.Fatalf("selected bot scene was not persisted: %+v", cfg.Agent)
	}
	if !strings.Contains(prompt.String(), "default scene unchanged") {
		t.Fatalf("prompt did not state default behavior: %q", prompt.String())
	}
}

func TestBootstrapDevRTCAppCredentialsRollsBackHandledWriteFailures(t *testing.T) {
	newFixture := func(t *testing.T) (string, *config.Config, *fakeRTCAppClient, string, []byte) {
		t.Helper()
		dir := t.TempDir()
		defaultPath := filepath.Join(dir, "server", "scenes", "default.json")
		if err := os.MkdirAll(filepath.Dir(defaultPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(defaultPath, []byte(validVoiceAgentScene), 0o644); err != nil {
			t.Fatal(err)
		}
		envPath := projectenv.Path(dir)
		envBefore := []byte("OTHER=before\n")
		if err := os.WriteFile(envPath, envBefore, 0o600); err != nil {
			t.Fatal(err)
		}
		client := &fakeRTCAppClient{
			apps:   []openapi.RtcApp{{AppID: "selected-app", Name: "Selected", Status: "1"}},
			appKey: "selected-secret",
			bots: []openapi.Bot{
				{ID: "one", Name: "One", Config: json.RawMessage(`{"ASRConfig":{"Provider":"volcano","ProviderParams":{}},"LLMConfig":{"Mode":"ArkV3","EndPointId":"ep-test"},"TTSConfig":{"Provider":"volcano","ProviderParams":{}}}`)},
				{ID: "two", Name: "Two", Config: json.RawMessage(`{"ASRConfig":{"Provider":"volcano","ProviderParams":{}},"LLMConfig":{"Mode":"ArkV3","EndPointId":"ep-test"},"TTSConfig":{"Provider":"volcano","ProviderParams":{}}}`)},
			},
		}
		return dir, config.Default("demo", "voice-agent", "web"), client, envPath, envBefore
	}

	for _, test := range []struct {
		name        string
		injectWrite func(t *testing.T, realEnv func(string, map[string]string) ([]string, error), realScenes func(string, string, []openapi.Bot, bool) ([]provision.SceneResult, error))
	}{
		{
			name: "env writer corrupts then fails",
			injectWrite: func(t *testing.T, _ func(string, map[string]string) ([]string, error), _ func(string, string, []openapi.Bot, bool) ([]provision.SceneResult, error)) {
				writeDevProjectEnv = func(path string, _ map[string]string) ([]string, error) {
					if err := os.WriteFile(path, []byte("corrupt\n"), 0o600); err != nil {
						t.Fatal(err)
					}
					return nil, errs.New("vertc.env.write_failed", errs.TypeIO, "injected env failure")
				}
			},
		},
		{
			name: "scene writer commits one file then fails",
			injectWrite: func(t *testing.T, _ func(string, map[string]string) ([]string, error), realScenes func(string, string, []openapi.Bot, bool) ([]provision.SceneResult, error)) {
				writeDevBotScenes = func(dir, configFile string, bots []openapi.Bot, dryRun bool) ([]provision.SceneResult, error) {
					if dryRun {
						return realScenes(dir, configFile, bots, true)
					}
					if _, err := realScenes(dir, configFile, bots[:1], false); err != nil {
						t.Fatal(err)
					}
					return nil, errs.New("vertc.provision.write_failed", errs.TypeIO, "injected scene failure")
				}
			},
		},
		{
			name: "config writer corrupts after scenes then fails",
			injectWrite: func(t *testing.T, _ func(string, map[string]string) ([]string, error), _ func(string, string, []openapi.Bot, bool) ([]provision.SceneResult, error)) {
				writeDevConfig = func(_ *config.Config, path string) error {
					if err := os.WriteFile(path, []byte("corrupt\n"), 0o644); err != nil {
						t.Fatal(err)
					}
					return errs.New("vertc.config.write_failed", errs.TypeIO, "injected config failure")
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir, cfg, client, envPath, envBefore := newFixture(t)
			t.Setenv("RTC_APP_ID", "")
			t.Setenv("RTC_APP_KEY", "")
			t.Setenv("VITE_RTC_APP_ID", "${RTC_APP_ID}")
			oldFactory, oldTerminal := newDevConsoleClient, rtcInputIsTerminal
			oldEnvWriter, oldSceneWriter, oldConfigWriter := writeDevProjectEnv, writeDevBotScenes, writeDevConfig
			newDevConsoleClient = func(*config.Config) devConsoleClient { return client }
			rtcInputIsTerminal = func(io.Reader) bool { return false }
			t.Cleanup(func() {
				newDevConsoleClient = oldFactory
				rtcInputIsTerminal = oldTerminal
				writeDevProjectEnv = oldEnvWriter
				writeDevBotScenes = oldSceneWriter
				writeDevConfig = oldConfigWriter
			})
			test.injectWrite(t, oldEnvWriter, oldSceneWriter)

			_, err := bootstrapDevRTCAppCredentialsWithOptions(
				context.Background(), cfg, dir, strings.NewReader(""), &bytes.Buffer{},
				devSetupOptions{AppID: "selected-app", BotIDs: []string{"one", "two"}},
			)
			if err == nil {
				t.Fatal("injected write failure should fail setup")
			}
			after, readErr := os.ReadFile(envPath)
			if readErr != nil || !bytes.Equal(after, envBefore) {
				t.Fatalf("env was not rolled back: %q, err=%v", after, readErr)
			}
			for _, name := range []string{"bot-one.json", "bot-two.json"} {
				if _, statErr := os.Stat(filepath.Join(dir, "server", "scenes", name)); !os.IsNotExist(statErr) {
					t.Fatalf("partial scene %s remains: %v", name, statErr)
				}
			}
			if _, statErr := os.Stat(config.Path(dir)); !os.IsNotExist(statErr) {
				t.Fatalf("partial config remains: %v", statErr)
			}
			if os.Getenv("RTC_APP_ID") != "" || os.Getenv("RTC_APP_KEY") != "" {
				t.Fatal("process credentials were not restored")
			}
		})
	}
}

func TestBootstrapDevRTCAppCredentialsReturnsAppCandidates(t *testing.T) {
	oldFactory, oldTerminal := newDevConsoleClient, rtcInputIsTerminal
	fake := &fakeRTCAppClient{apps: []openapi.RtcApp{
		{AppID: "one", Name: "One", Status: "1"},
		{AppID: "two", Name: "Two", Status: "1"},
	}}
	newDevConsoleClient = func(*config.Config) devConsoleClient { return fake }
	rtcInputIsTerminal = func(io.Reader) bool { return false }
	t.Cleanup(func() {
		newDevConsoleClient = oldFactory
		rtcInputIsTerminal = oldTerminal
	})

	_, err := bootstrapDevRTCAppCredentials(
		context.Background(), config.Default("demo", "test-scene", "web"), t.TempDir(),
		strings.NewReader("1\n"), &bytes.Buffer{},
	)
	typed, ok := errs.As(err)
	if !ok || typed.Code != "vertc.dev.selection_required" || typed.Details["apps"] == nil {
		t.Fatalf("error = %v", err)
	}
}

func TestDevDryRunReportsBootstrapWithoutOpenAPIOrWrite(t *testing.T) {
	previousWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previousWD) })
	if err := config.Save(config.Default("demo", "test-scene", "web"), config.Path(dir)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "vertc.taskfile.yaml"), []byte("version: 1\nscene: test-scene\nplatform: web\ntasks:\n  dev:\n    - 'false'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RTC_APP_ID", "")
	t.Setenv("RTC_APP_KEY", "")
	oldFactory, oldDryRun := newDevConsoleClient, flagDryRun
	called := false
	newDevConsoleClient = func(*config.Config) devConsoleClient {
		called = true
		return &fakeRTCAppClient{}
	}
	flagDryRun = true
	t.Cleanup(func() {
		newDevConsoleClient = oldFactory
		flagDryRun = oldDryRun
	})

	if err := newDevCmd().Execute(); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("dry-run called OpenAPI")
	}
	if _, err := os.Stat(projectenv.Path(dir)); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote .env.local: %v", err)
	}
}

func TestDevBootstrapsCredentialsThenRunsTask(t *testing.T) {
	previousWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previousWD) })
	if err := config.Save(config.Default("demo", "test-scene", "web"), config.Path(dir)); err != nil {
		t.Fatal(err)
	}
	selectedAppID := "aabbccddeeff001122334455"
	taskfile := "version: 1\nscene: test-scene\nplatform: web\ntasks:\n  dev:\n    - 'test \"$RTC_APP_ID\" = \"" + selectedAppID + "\" && test \"$RTC_APP_KEY\" = \"secret\" && touch dev-ran'\n"
	if err := os.WriteFile(filepath.Join(dir, "vertc.taskfile.yaml"), []byte(taskfile), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(projectenv.Path(dir), []byte("RTC_APP_ID=\nRTC_APP_KEY=\nVITE_RTC_APP_ID=${RTC_APP_ID}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RTC_APP_ID", "")
	t.Setenv("RTC_APP_KEY", "")
	t.Setenv("VITE_RTC_APP_ID", "${RTC_APP_ID}")
	fake := &fakeRTCAppClient{
		apps:   []openapi.RtcApp{{AppID: selectedAppID, Name: "Demo", Status: "1"}},
		appKey: "secret",
	}
	oldFactory, oldTerminal, oldDryRun := newDevConsoleClient, rtcInputIsTerminal, flagDryRun
	newDevConsoleClient = func(*config.Config) devConsoleClient { return fake }
	rtcInputIsTerminal = func(io.Reader) bool { return true }
	flagDryRun = false
	t.Cleanup(func() {
		newDevConsoleClient = oldFactory
		rtcInputIsTerminal = oldTerminal
		flagDryRun = oldDryRun
	})

	cmd := newDevCmd()
	cmd.SetIn(strings.NewReader(" \r"))
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "dev-ran")); err != nil {
		t.Fatalf("dev task did not run with selected credentials: %v", err)
	}
	values, err := projectenv.Load(projectenv.Path(dir))
	if err != nil {
		t.Fatal(err)
	}
	if values["RTC_APP_ID"] != selectedAppID || values["RTC_APP_KEY"] != "secret" {
		t.Fatalf("persisted env = %+v", values)
	}
}

func TestLoadProjectEnvUsesProcessPrecedenceAndRestores(t *testing.T) {
	dir := t.TempDir()
	processKey := "VERTC_TEST_PROCESS_PRECEDENCE"
	fileOnlyKey := "VERTC_TEST_FILE_ONLY"
	t.Setenv(processKey, "from-process")
	_ = os.Unsetenv(fileOnlyKey)
	t.Cleanup(func() { _ = os.Unsetenv(fileOnlyKey) })
	contents := processKey + "=from-file\n" + fileOnlyKey + "=loaded\n"
	if err := os.WriteFile(filepath.Join(dir, ".env.local"), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	restore, err := loadProjectEnv(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv(processKey); got != "from-process" {
		t.Fatalf("process env was replaced: %q", got)
	}
	if got := os.Getenv(fileOnlyKey); got != "loaded" {
		t.Fatalf("file env was not loaded: %q", got)
	}
	restore()
	if got := os.Getenv(processKey); got != "from-process" {
		t.Fatalf("process env changed after restore: %q", got)
	}
	if _, ok := os.LookupEnv(fileOnlyKey); ok {
		t.Fatal("file-only env remained set after restore")
	}
}

func TestServerManagedTaskEnvUsesCLIWithoutLongTermCredentials(t *testing.T) {
	stdinIsTTY, stdoutIsTTY := false, false
	telemetry.InitializeInvocation(telemetry.ResolveInvocationContextOptions{
		CLIName:    "vertc",
		CLIVersion: "1.2.3",
		Env: map[string]string{
			"VE_SKILL_ID": "byted-interactai-guide/0.0.1",
		},
		StdinIsTTY:  &stdinIsTTY,
		StdoutIsTTY: &stdoutIsTTY,
	})
	for _, key := range []string{
		"VOLCENGINE_ACCESS_KEY_ID",
		"VOLCENGINE_SECRET_ACCESS_KEY",
	} {
		_ = os.Unsetenv(key)
		key := key
		t.Cleanup(func() { _ = os.Unsetenv(key) })
	}
	dir := t.TempDir()
	cfg := config.Default("demo", "voice-agent", "web")
	cfg.Agent.ConfigFile = "custom/agent.json"
	taskEnv, err := serverManagedTaskEnv(cfg, dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := taskEnvValue(taskEnv, "AGENT_CONFIG_PATH"); got != filepath.Join(dir, "custom/agent.json") {
		t.Fatalf("AGENT_CONFIG_PATH = %q", got)
	}
	if got := taskEnvValue(taskEnv, "VERTC_PROJECT_DIR"); got != dir {
		t.Fatalf("VERTC_PROJECT_DIR = %q", got)
	}
	if got := taskEnvValue(taskEnv, "VERTC_BUSINESS_ID"); got != meta.BusinessID {
		t.Fatalf("VERTC_BUSINESS_ID = %q", got)
	}
	wantUserAgent := "vertc/1.2.3 invocation/skill skill/byted-interactai-guide#0.0.1"
	if got := taskEnvValue(taskEnv, "VERTC_OPENAPI_USER_AGENT"); got != wantUserAgent {
		t.Fatalf("VERTC_OPENAPI_USER_AGENT = %q, want %q", got, wantUserAgent)
	}
	if taskEnvValue(taskEnv, "VERTC_CLI_PATH") == "" {
		t.Fatalf("CLI agent mode environment = %v", taskEnv)
	}
	for _, key := range []string{"VERTC_STS_BROKER_URL", "VERTC_STS_BROKER_TOKEN", "VOLCENGINE_SESSION_TOKEN"} {
		if taskEnvValue(taskEnv, key) != "" {
			t.Fatalf("CLI mode must not expose %s", key)
		}
	}
}

func TestServerManagedTaskEnvPreservesExplicitCredentials(t *testing.T) {
	t.Setenv("VOLCENGINE_ACCESS_KEY_ID", "EXPLICIT_AK")
	t.Setenv("VOLCENGINE_SECRET_ACCESS_KEY", "EXPLICIT_SK")
	dir := t.TempDir()
	taskEnv, err := serverManagedTaskEnv(config.Default("demo", "voice-agent", "web"), dir)
	if err != nil {
		t.Fatal(err)
	}
	if taskEnvValue(taskEnv, "VERTC_CLI_PATH") != "" {
		t.Fatalf("explicit credentials should select direct OpenAPI mode: %v", taskEnv)
	}
	if taskEnvValue(taskEnv, "AGENT_CONFIG_PATH") != filepath.Join(dir, "server/scenes/default.json") {
		t.Fatalf("task environment = %v", taskEnv)
	}
	if os.Getenv("VOLCENGINE_ACCESS_KEY_ID") != "EXPLICIT_AK" || os.Getenv("VOLCENGINE_SECRET_ACCESS_KEY") != "EXPLICIT_SK" {
		t.Fatal("explicit server credentials were changed")
	}
}

func TestServerManagedTaskEnvRejectsIncompleteLongTermCredentials(t *testing.T) {
	t.Setenv("VOLCENGINE_ACCESS_KEY_ID", "EXPLICIT_AK")
	t.Setenv("VOLCENGINE_SECRET_ACCESS_KEY", "")
	_, err := serverManagedTaskEnv(config.Default("demo", "voice-agent", "web"), t.TempDir())
	typed, ok := errs.As(err)
	if !ok || typed.Code != "vertc.auth.invalid_token" {
		t.Fatalf("error = %v", err)
	}
}

func taskEnvValue(values []string, key string) string {
	prefix := key + "="
	for _, value := range values {
		if strings.HasPrefix(value, prefix) {
			return strings.TrimPrefix(value, prefix)
		}
	}
	return ""
}

func TestRunTasksWithEnvInjectsOnlyIntoChild(t *testing.T) {
	const key = "VERTC_TEST_CHILD_ONLY"
	_ = os.Unsetenv(key)
	t.Cleanup(func() { _ = os.Unsetenv(key) })
	if err := runTasksWithEnv(t.TempDir(), []string{`test "$VERTC_TEST_CHILD_ONLY" = "available"`}, []string{key + "=available"}); err != nil {
		t.Fatal(err)
	}
	if _, exists := os.LookupEnv(key); exists {
		t.Fatal("task environment leaked into the CLI process")
	}
}

func TestTaskShellCommandUsesCmdOnWindows(t *testing.T) {
	t.Setenv("ComSpec", `C:\Windows\System32\cmd.exe`)
	name, args := taskShellCommand("windows", "yarn dev")
	if name != `C:\Windows\System32\cmd.exe` {
		t.Fatalf("shell = %q, want ComSpec", name)
	}
	wantArgs := []string{"/D", "/S", "/C", "yarn dev"}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("args = %q, want %q", args, wantArgs)
	}
}

func TestTaskShellCommandFallsBackToCmdExeOnWindows(t *testing.T) {
	t.Setenv("ComSpec", "")
	name, _ := taskShellCommand("windows", "yarn dev")
	if name != "cmd.exe" {
		t.Fatalf("shell = %q, want cmd.exe", name)
	}
}

func TestServerManagedProjectRoutesAllAgentCommandsToWeb(t *testing.T) {
	previousWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previousWD) })
	if err := config.Save(config.Default("demo", "voice-agent", "web"), config.Path(dir)); err != nil {
		t.Fatal(err)
	}
	taskfile := "version: 2\nscene: voice-agent\nplatform: web\nruntime:\n  topology: web-server\n  agent_control: server\n"
	if err := os.WriteFile(filepath.Join(dir, "vertc.taskfile.yaml"), []byte(taskfile), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, action := range []string{"start", "update", "stop", "status"} {
		cmd := NewRootCmd()
		cmd.SetArgs([]string{"agent", action})
		err := cmd.Execute()
		typed, ok := errs.As(err)
		if !ok || typed.Code != "vertc.agent.server_managed" {
			t.Fatalf("agent %s error = %v, want vertc.agent.server_managed", action, err)
		}
		if typed.Hint == "" {
			t.Fatalf("agent %s should guide the user to the web app", action)
		}
	}
}

func TestV1ProjectKeepsCLIAgentControl(t *testing.T) {
	dir := t.TempDir()
	taskfile := "version: 1\nscene: voice-agent\nplatform: web\n"
	if err := os.WriteFile(filepath.Join(dir, "vertc.taskfile.yaml"), []byte(taskfile), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := requireCLIAgentControl(config.Default("demo", "voice-agent", "web"), dir); err != nil {
		t.Fatalf("v1 project should retain CLI agent control: %v", err)
	}
}

func TestDoctorProjectLoadsDotEnvForServerManagedProject(t *testing.T) {
	previousWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previousWD) })
	if err := config.Save(config.Default("demo", "voice-agent", "web"), config.Path(dir)); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "server", "scenes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "server", "scenes", "default.json"), []byte(`{"SceneConfig":{"Name":"Default"},"VoiceChat":{"Config":{"ASRConfig":{"Provider":"volcano","ProviderParams":{}},"LLMConfig":{"Mode":"ArkV3","EndPointId":"ep-test"},"TTSConfig":{"Provider":"volcano","ProviderParams":{}}},"AgentConfig":{"UserId":"voice_agent"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	taskfile := "version: 2\nscene: voice-agent\nplatform: web\nsdk:\n  name: '@volcengine/rtc'\n  version: '4.68.1'\nruntime:\n  topology: web-server\n  agent_control: server\n"
	if err := os.WriteFile(filepath.Join(dir, "vertc.taskfile.yaml"), []byte(taskfile), 0o644); err != nil {
		t.Fatal(err)
	}
	dotenv := "RTC_APP_ID=appid-from-file\nRTC_APP_KEY=appkey-from-file\nVOLCENGINE_ACCESS_KEY_ID=AKID\nVOLCENGINE_SECRET_ACCESS_KEY=SECRET\n" // public-scan: allow; gitleaks:allow — synthetic test credentials
	if err := os.WriteFile(filepath.Join(dir, ".env.local"), []byte(dotenv), 0o600); err != nil {
		t.Fatal(err)
	}
	// Minimal scaffold entrypoints so doctor's file-presence check passes.
	for _, f := range []struct{ path, body string }{
		{"package.json", "{}"},
		{"web/package.json", "{}"},
		{"web/src/App.tsx", "export default function App(){return null}"},
		{"server/package.json", "{}"},
		{"server/app.js", "// server"},
	} {
		p := filepath.Join(dir, f.path)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(f.body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	cmd := newDoctorProjectCmd()
	if err := cmd.Execute(); err != nil {
		t.Fatalf("doctor project should use .env.local: %v", err)
	}
}

func TestDoctorDoesNotCompleteMissingRTCAppCredentials(t *testing.T) {
	previousWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previousWD) })
	if err := config.Save(config.Default("demo", "test-scene", "web"), config.Path(dir)); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RTC_APP_ID", "")
	t.Setenv("RTC_APP_KEY", "")
	t.Setenv("VITE_RTC_APP_ID", "${RTC_APP_ID}")
	fake := &fakeRTCAppClient{}
	oldAppFactory, oldDryRun := newDevConsoleClient, flagDryRun
	newDevConsoleClient = func(*config.Config) devConsoleClient { return fake }
	flagDryRun = false
	t.Cleanup(func() {
		newDevConsoleClient = oldAppFactory
		flagDryRun = oldDryRun
	})

	cmd := newDoctorCmd()
	cmd.SetErr(&bytes.Buffer{})
	err = cmd.Execute()
	if typed, ok := errs.As(err); !ok || typed.Code != "vertc.doctor.failed" {
		t.Fatalf("doctor error = %v, want missing-credential diagnostics", err)
	}
	if fake.listRequests != 0 || fake.requestedID != "" || fake.botCalls != 0 {
		t.Fatalf("doctor performed completion: app=%d key=%q bots=%d", fake.listRequests, fake.requestedID, fake.botCalls)
	}
	if _, err := os.Stat(projectenv.Path(dir)); !os.IsNotExist(err) {
		t.Fatalf("doctor wrote .env.local: %v", err)
	}
}
