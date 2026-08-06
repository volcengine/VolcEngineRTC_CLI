// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/config"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
)

// A malicious/untrusted agent.config_file must not let `agent start` read files
// outside the project directory (path-traversal hardening).
func TestBuildStartRequestRejectsConfigPathTraversal(t *testing.T) {
	for _, cfgFile := range []string{"../evil.json", "/etc/passwd"} {
		cfg := &config.Config{
			Project: config.Project{Scene: "voice-agent"},
			RTC:     config.RTC{AppID: "app123456789012345678901", RoomID: "room-01", UserID: "user-01"},
			Agent:   config.Agent{ConfigFile: cfgFile},
		}
		_, err := buildStartRequest(cfg, t.TempDir())
		typed, ok := errs.As(err)
		if !ok || typed.Code != "vertc.config.invalid_field" {
			t.Fatalf("config_file %q: err = %v, want vertc.config.invalid_field", cfgFile, err)
		}
	}
}

// An empty agent.user_id must not clobber the scene's validated AgentConfig.UserId,
// otherwise `agent start` would launch a task the AI cannot join under.
func TestBuildStartRequestKeepsSceneUserIDWhenConfigEmpty(t *testing.T) {
	dir := t.TempDir()
	scene := `{
      "SceneConfig": {"Name": "demo"},
      "VoiceChat": {
        "Config": {
          "ASRConfig": {"Provider": "volcano", "ProviderParams": {}},
          "LLMConfig": {"Mode": "ArkV3", "EndPointId": "ep-test"},
          "TTSConfig": {"Provider": "volcano", "ProviderParams": {}}
        },
        "AgentConfig": {"UserId": "scene-agent"}
      }
    }`
	sceneDir := filepath.Join(dir, "server", "scenes")
	if err := os.MkdirAll(sceneDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sceneDir, "default.json"), []byte(scene), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Project: config.Project{Scene: "voice-agent"},
		RTC:     config.RTC{AppID: "app123456789012345678901", RoomID: "room-01", UserID: "user-01"},
		Agent:   config.Agent{ConfigFile: "server/scenes/default.json", UserID: ""}, // empty on purpose
	}
	req, err := buildStartRequest(cfg, dir)
	if err != nil {
		t.Fatalf("buildStartRequest: %v", err)
	}
	if got, _ := req.AgentConfig["UserId"].(string); got != "scene-agent" {
		t.Fatalf("AgentConfig.UserId = %q, want scene-agent (not clobbered to empty)", got)
	}
}
