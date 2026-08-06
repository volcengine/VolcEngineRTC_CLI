// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
)

func TestSaveLoadRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vertc.config.yaml")
	c := Default("demo", "test-scene", "web")
	c.RTC.RoomID = "room-42"
	if err := Save(c, path); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Project.Name != "demo" || got.RTC.RoomID != "room-42" || got.Version != SchemaVersion {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
}

func TestValidateMissingField(t *testing.T) {
	t.Setenv("RTC_APP_ID", "resolved") // isolate the missing-field case
	c := Default("demo", "test-scene", "web")
	c.RTC.RoomID = "" // required
	rep := Validate(c)
	if !rep.HasErrors() {
		t.Fatalf("expected errors for missing room_id")
	}
	e, ok := errs.As(rep.Err())
	if !ok || e.Param != "rtc.room_id" {
		t.Fatalf("expected rtc.room_id param, got %v", rep.Err())
	}
	if e.Code != "vertc.config.missing_field" {
		t.Fatalf("expected missing_field code, got %s", e.Code)
	}
}

func TestValidateUnresolvedEnv(t *testing.T) {
	t.Setenv("SOME_UNSET_VAR_XYZ", "")        // ensure unset semantics via lookup
	c := Default("demo", "test-scene", "web") // app_id defaults to ${RTC_APP_ID}
	rep := Validate(c)
	if !rep.HasErrors() {
		t.Fatalf("expected unresolved ${RTC_APP_ID} error")
	}
	found := false
	for _, f := range rep.Findings {
		if f.Field == "rtc.app_id" && f.Severity == SevError {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected rtc.app_id error, got %+v", rep.Findings)
	}
}

func TestValidatePassesWhenEnvSet(t *testing.T) {
	t.Setenv("RTC_APP_ID", "resolved-app-id")
	c := Default("demo", "test-scene", "web")
	rep := Validate(c)
	if rep.HasErrors() {
		t.Fatalf("expected no errors, got %+v", rep.Findings)
	}
}

func TestCanBootstrapRTCAppCredentialsOnlyForCredentialErrors(t *testing.T) {
	t.Setenv("RTC_APP_ID", "")
	t.Setenv("RTC_APP_KEY", "")
	c := Default("demo", "test-scene", "web")
	if !CanBootstrapRTCAppCredentials(c, Validate(c), false) {
		t.Fatal("generated config with missing RTC credentials should allow dev bootstrap")
	}

	c.RTC.RoomID = ""
	if CanBootstrapRTCAppCredentials(c, Validate(c), false) {
		t.Fatal("dev bootstrap must not hide unrelated config errors")
	}

	c.RTC.RoomID = "room-01"
	c.RTC.AppID = "fixed-app-id"
	if CanBootstrapRTCAppCredentials(c, Validate(c), false) {
		t.Fatal("fixed AppID configs do not support interactive credential bootstrap")
	}
}

func TestResolveEnv(t *testing.T) {
	t.Setenv("FOO_BAR", "hello")
	got, missing := ResolveEnv("x-${FOO_BAR}-y")
	if got != "x-hello-y" || len(missing) != 0 {
		t.Fatalf("resolve got %q missing %v", got, missing)
	}
	_, missing = ResolveEnv("${DEFINITELY_UNSET_VAR}")
	if len(missing) != 1 || missing[0] != "DEFINITELY_UNSET_VAR" {
		t.Fatalf("expected missing var, got %v", missing)
	}
}

func TestGetSet(t *testing.T) {
	c := Default("demo", "test-scene", "web")
	if err := c.Set("rtc.room_id", "r9"); err != nil {
		t.Fatal(err)
	}
	v, err := c.Get("rtc.room_id")
	if err != nil || v != "r9" {
		t.Fatalf("get/set mismatch: %q %v", v, err)
	}
	if err := c.Set("bogus.field", "x"); err == nil {
		t.Fatalf("expected error for unknown field")
	}
}

func TestFindWalksUp(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "a", "b")
	c := Default("demo", "test-scene", "web")
	if err := Save(c, filepath.Join(dir, "vertc.config.yaml")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	found, err := Find(sub)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if filepath.Dir(found) != dir {
		t.Fatalf("expected find at %s, got %s", dir, found)
	}
}
