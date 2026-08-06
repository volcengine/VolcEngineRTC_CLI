// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/config"
)

func TestFollowTargetUserResolvesPlaceholders(t *testing.T) {
	t.Setenv("RTC_REAL_USER", "user-01")
	cfg := config.Default("demo", "voice-agent", "web")
	cfg.RTC.UserID = "${RTC_REAL_USER}"
	cfg.Agent.TargetUserID = "user-01"

	followTargetUser(cfg, "bob")

	if cfg.Agent.TargetUserID != "bob" {
		t.Fatalf("target should follow resolved rtc.user_id, got %q", cfg.Agent.TargetUserID)
	}
}

func TestPersistTokenWriteRollsBackBothFiles(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "vertc.config.yaml")
	envPath := filepath.Join(dir, ".env.local")
	cfg := config.Default("demo", "test-scene", "web")
	if err := config.Save(cfg, configPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(envPath, []byte("KEEP=original\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	configBefore, _ := os.ReadFile(configPath)
	envBefore, _ := os.ReadFile(envPath)

	cfg.RTC.RoomID = "room-new"
	cfg.RTC.Token = "token-new"
	err := persistTokenWrite(cfg, configPath, envPath, map[string]string{"VITE_RTC_ROOM_ID": "room-new"},
		func(path string, _ map[string]string) ([]string, error) {
			if writeErr := os.WriteFile(path, []byte("partial"), 0o600); writeErr != nil {
				return nil, writeErr
			}
			return nil, errors.New("injected env failure")
		})
	if err == nil {
		t.Fatal("expected env write failure")
	}
	configAfter, _ := os.ReadFile(configPath)
	envAfter, _ := os.ReadFile(envPath)
	if string(configAfter) != string(configBefore) {
		t.Fatalf("config was not rolled back:\n%s", configAfter)
	}
	if string(envAfter) != string(envBefore) {
		t.Fatalf("env was not rolled back: %q", envAfter)
	}
}
