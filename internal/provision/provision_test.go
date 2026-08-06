// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package provision

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/config"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/openapi"
)

const testScene = `{"SceneConfig":{"name":"Default","icon":"keep"},"VoiceChat":{"Config":{"old":true},"AgentConfig":{"UserId":"default-agent"}}}`

func writeDefaultScene(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, DefaultConfigFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(testScene), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func testBots() []openapi.Bot {
	configRaw := json.RawMessage(`{"ASRConfig":{"Provider":"volcano","ProviderParams":{}},"LLMConfig":{"Mode":"ArkV3","EndPointId":"ep-test"},"TTSConfig":{"Provider":"volcano","ProviderParams":{}}}`)
	return []openapi.Bot{
		{ID: "one", Name: "First", Config: configRaw, AgentConfig: map[string]any{"WelcomeMessage": "one"}},
		{ID: "two", Name: "Second", Config: configRaw, AgentConfig: map[string]any{"UserId": "bot-user"}},
	}
}

func TestWriteBotScenesPreservesDefaultAndWritesEverySelection(t *testing.T) {
	dir := t.TempDir()
	defaultPath := writeDefaultScene(t, dir)
	before, err := os.ReadFile(defaultPath)
	if err != nil {
		t.Fatal(err)
	}

	results, err := WriteBotScenes(dir, DefaultConfigFile, testBots(), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %+v", results)
	}
	after, err := os.ReadFile(defaultPath)
	if err != nil || string(after) != string(before) {
		t.Fatalf("default scene changed: %v", err)
	}

	firstRaw, err := os.ReadFile(filepath.Join(filepath.Dir(defaultPath), "bot-one.json"))
	if err != nil {
		t.Fatal(err)
	}
	first, err := config.ParseServerSceneJSON(firstRaw)
	if err != nil {
		t.Fatal(err)
	}
	if first.SceneConfig["name"] != "First" || first.SceneConfig["icon"] != "keep" {
		t.Fatalf("first SceneConfig = %+v", first.SceneConfig)
	}
	if first.VoiceChat.AgentConfig["UserId"] != "default-agent" {
		t.Fatalf("missing fallback UserId: %+v", first.VoiceChat.AgentConfig)
	}
}

func TestWriteBotScenesDryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	defaultPath := writeDefaultScene(t, dir)
	results, err := WriteBotScenes(dir, DefaultConfigFile, testBots(), true)
	if err != nil || len(results) != 2 {
		t.Fatalf("dry-run = %+v, %v", results, err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(defaultPath), "bot-one.json")); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote bot scene: %v", err)
	}
}

func TestWriteBotScenesKeepsExistingUnselectedBot(t *testing.T) {
	dir := t.TempDir()
	defaultPath := writeDefaultScene(t, dir)
	oldPath := filepath.Join(filepath.Dir(defaultPath), "bot-old.json")
	if err := os.WriteFile(oldPath, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteBotScenes(dir, DefaultConfigFile, testBots()[:1], false); err != nil {
		t.Fatal(err)
	}
	if raw, err := os.ReadFile(oldPath); err != nil || string(raw) != "keep" {
		t.Fatalf("unselected bot changed: %q, %v", raw, err)
	}
}

type fakeBotLister struct {
	bots []openapi.Bot
	err  error
}

func (f fakeBotLister) AibotxQuery(context.Context, int, int) ([]openapi.Bot, error) {
	return f.bots, f.err
}

func TestListBotsRejectsEmptyResult(t *testing.T) {
	if _, err := ListBots(context.Background(), fakeBotLister{}); err == nil {
		t.Fatal("expected empty bot list to fail")
	}
}
