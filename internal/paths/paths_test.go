// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package paths

import (
	"os"
	"path/filepath"
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
	if info, err := os.Stat(path); err != nil || info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("state file must be private: info=%v err=%v", info, err)
	}
}
