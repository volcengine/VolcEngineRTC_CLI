// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"path/filepath"
	"testing"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
)

func TestProjectFilePathAcceptsInside(t *testing.T) {
	dir := "/tmp/proj"
	for _, name := range []string{"scene.json", "./scene.json", "server/scenes/default.json"} {
		got, err := ProjectFilePath(dir, name)
		if err != nil {
			t.Fatalf("ProjectFilePath(%q) unexpected error: %v", name, err)
		}
		want := filepath.Join(dir, name)
		if got != want {
			t.Fatalf("ProjectFilePath(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestProjectFilePathRejectsEscape(t *testing.T) {
	dir := "/tmp/proj"
	for _, name := range []string{
		"/etc/passwd",            // absolute
		`C:\Windows\win.ini`,     // Windows drive absolute
		"../outside.json",        // parent escape
		"../../etc/passwd",       // deeper escape
		"sub/../../outside.json", // escape after cleaning
		"",                       // empty
	} {
		_, err := ProjectFilePath(dir, name)
		typed, ok := errs.As(err)
		if !ok || typed.Code != "vertc.config.invalid_field" {
			t.Fatalf("ProjectFilePath(%q) = %v, want vertc.config.invalid_field error", name, err)
		}
	}
}
