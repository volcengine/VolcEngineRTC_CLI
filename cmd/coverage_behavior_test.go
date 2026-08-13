// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/config"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/meta"
)

func TestConfigCommandsReadWriteDryRunAndValidate(t *testing.T) {
	oldDryRun, oldFormat := flagDryRun, flagFormat
	t.Cleanup(func() { flagDryRun, flagFormat = oldDryRun, oldFormat })
	flagFormat = "json"
	t.Setenv("RTC_APP_ID", "app123456789012345678901")
	dir := t.TempDir()
	t.Chdir(dir)
	cfg := config.Default("demo", "voice-agent", "web")
	if err := config.Save(cfg, meta.ConfigFileName); err != nil {
		t.Fatal(err)
	}

	output := captureCommandStdout(t, func() error { return newConfigShowCmd().Execute() })
	if !strings.Contains(output, `"Name": "demo"`) {
		t.Fatalf("config show output missing project name: %s", output)
	}

	get := newConfigGetCmd()
	get.SetArgs([]string{"rtc.room_id"})
	output = captureCommandStdout(t, get.Execute)
	if !strings.Contains(output, `"value": "room-01"`) {
		t.Fatalf("config get output missing room id: %s", output)
	}

	flagDryRun = true
	dryRun := newConfigSetCmd()
	dryRun.SetArgs([]string{"rtc.room_id", "room-preview"})
	output = captureCommandStdout(t, dryRun.Execute)
	if !strings.Contains(output, `"written": "false"`) {
		t.Fatalf("config dry-run output=%s", output)
	}
	loaded, _, err := config.LoadNearest(".")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.RTC.RoomID != "room-01" {
		t.Fatalf("dry-run persisted room id %q", loaded.RTC.RoomID)
	}

	flagDryRun = false
	set := newConfigSetCmd()
	set.SetArgs([]string{"rtc.room_id", "room-real"})
	output = captureCommandStdout(t, set.Execute)
	if !strings.Contains(output, `"written": "true"`) {
		t.Fatalf("config set output=%s", output)
	}
	loaded, _, err = config.LoadNearest(".")
	if err != nil || loaded.RTC.RoomID != "room-real" {
		t.Fatalf("persisted room id=%q err=%v", loaded.RTC.RoomID, err)
	}

	output = captureCommandStdout(t, func() error { return newConfigValidateCmd().Execute() })
	if !strings.Contains(output, `"ok": true`) {
		t.Fatalf("valid config report=%s", output)
	}

	loaded.Project.Name = ""
	if err := config.Save(loaded, meta.ConfigFileName); err != nil {
		t.Fatal(err)
	}
	var validateErr error
	output = captureCommandStdout(t, func() error {
		validateErr = newConfigValidateCmd().Execute()
		return nil
	})
	typed, ok := errs.As(validateErr)
	if !ok || typed.Code != "vertc.config.missing_field" || !typed.IsReported() {
		t.Fatalf("validate error=%v", validateErr)
	}
	if !strings.Contains(output, `"field": "project.name"`) {
		t.Fatalf("invalid config report missing finding: %s", output)
	}
}

func TestExplainErrorCommandCoversKnownAndUnknownCodes(t *testing.T) {
	oldFormat := flagFormat
	t.Cleanup(func() { flagFormat = oldFormat })
	flagFormat = "json"

	known := newExplainErrorCmd()
	known.SetArgs([]string{"INVALID_TOKEN"})
	output := captureCommandStdout(t, known.Execute)
	if !strings.Contains(output, `"found": true`) || !strings.Contains(output, `"doctor_check": "token.valid"`) {
		t.Fatalf("known code output=%s", output)
	}

	unknown := newExplainErrorCmd()
	unknown.SetArgs([]string{"NOT_A_REAL_CODE"})
	var commandErr error
	output = captureCommandStdout(t, func() error {
		commandErr = unknown.Execute()
		return nil
	})
	typed, ok := errs.As(commandErr)
	if !ok || typed.Code != "vertc.explain.unknown_code" || !typed.IsReported() {
		t.Fatalf("unknown code error=%v", commandErr)
	}
	if !strings.Contains(output, `"found": false`) || !strings.Contains(output, `"query": "NOT_A_REAL_CODE"`) {
		t.Fatalf("unknown code output=%s", output)
	}
}

func TestEmitTemplateListUsesRegistry(t *testing.T) {
	oldFormat := flagFormat
	t.Cleanup(func() { flagFormat = oldFormat })
	flagFormat = "json"
	output := captureCommandStdout(t, emitTemplateList)
	for _, want := range []string{`"scene": "voice-agent"`, `"platform": "web"`, `"available": true`, `"sdk": "@volcengine/rtc@4.68.1"`} {
		if !strings.Contains(output, want) {
			t.Fatalf("template list output missing %q: %s", want, output)
		}
	}
}

func TestInitListRoutesToTemplateRegistryWithoutProjectWrites(t *testing.T) {
	oldFormat := flagFormat
	t.Cleanup(func() { flagFormat = oldFormat })
	flagFormat = "json"
	dir := t.TempDir()
	t.Chdir(dir)
	command := newInitCmd()
	command.SetArgs([]string{"--list"})
	output := captureCommandStdout(t, command.Execute)
	if !strings.Contains(output, `"scene": "voice-agent"`) {
		t.Fatalf("init --list output=%s", output)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("init --list wrote project files: %v", entries)
	}
}

func captureCommandStdout(t *testing.T, run func() error) string {
	t.Helper()
	previous := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	defer func() { os.Stdout = previous }()
	if err := run(); err != nil {
		_ = writer.Close()
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdout = previous
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	return string(data)
}
