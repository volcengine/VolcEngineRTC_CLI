// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/config"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/env"
)

func TestProjectEnvWritePathUsesProjectRoot(t *testing.T) {
	root := t.TempDir()
	if err := config.Save(config.Default("demo", "voice-agent", "web"), config.Path(root)); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "web", "src")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	got := projectEnvWritePath(sub)
	want := filepath.Join(root, env.FileName)
	if got != want {
		t.Fatalf("env write path = %q, want project root %q", got, want)
	}
}

func TestProjectEnvWritePathFallsBackWithoutConfig(t *testing.T) {
	dir := t.TempDir()
	if got := projectEnvWritePath(dir); got != env.Path(dir) {
		t.Fatalf("fallback path = %q, want %q", got, env.Path(dir))
	}
}
