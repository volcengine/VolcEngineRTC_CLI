// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package paths

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestStateDirOverride(t *testing.T) {
	t.Setenv("VERTC_STATE_DIR", "/tmp/vertc-test-state")
	if got := StateDir(); got != "/tmp/vertc-test-state" {
		t.Fatalf("StateDir() = %q", got)
	}
}

func TestWriteFileAtomic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "state.json")
	if err := WriteFileAtomic(path, []byte(`{"ok":true}`)); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"ok":true}` {
		t.Fatalf("got %q", got)
	}
	if info, err := os.Stat(path); err != nil {
		t.Fatal(err)
	} else if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("state file must be private: info=%v", info)
	}
}

func TestStateDirUsesPlatformConfigWhenNotOverridden(t *testing.T) {
	t.Setenv("VERTC_STATE_DIR", "")
	got := StateDir()
	if got == "" || filepath.Base(got) != "vertc" {
		t.Fatalf("StateDir() = %q, want platform config path ending in vertc", got)
	}
}

func TestWriteFileAtomicReplacesExistingContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := WriteFileAtomic(path, []byte("old")); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(path, []byte("new")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "new" {
		t.Fatalf("replacement data=%q err=%v", data, err)
	}
}
