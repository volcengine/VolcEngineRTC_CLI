// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package env

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadIntoProcessSetsUnsetAndWalksUp(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(Path(root), []byte("VERTC_TEST_APPID=app-24-chars-000000000000\nVERTC_TEST_KEY=secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Run from a nested subdir to exercise the walk-up.
	sub := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	os.Unsetenv("VERTC_TEST_APPID")
	os.Unsetenv("VERTC_TEST_KEY")
	t.Cleanup(func() { os.Unsetenv("VERTC_TEST_APPID"); os.Unsetenv("VERTC_TEST_KEY") })

	set, err := LoadIntoProcess(sub)
	if err != nil {
		t.Fatalf("LoadIntoProcess: %v", err)
	}
	if len(set) != 2 {
		t.Fatalf("set = %v, want both keys", set)
	}
	if os.Getenv("VERTC_TEST_APPID") != "app-24-chars-000000000000" || os.Getenv("VERTC_TEST_KEY") != "secret" {
		t.Fatalf("env not loaded: appid=%q key=%q", os.Getenv("VERTC_TEST_APPID"), os.Getenv("VERTC_TEST_KEY"))
	}
}

func TestLoadIntoProcessRealEnvWins(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(Path(root), []byte("VERTC_TEST_WINS=from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VERTC_TEST_WINS", "from-shell") // real env already set
	if _, err := LoadIntoProcess(root); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("VERTC_TEST_WINS"); got != "from-shell" {
		t.Fatalf("real env must win, got %q", got)
	}
}

func TestLoadIntoProcessMissingFileIsNoError(t *testing.T) {
	set, err := LoadIntoProcess(t.TempDir())
	if err != nil || set != nil {
		t.Fatalf("missing .env.local: set=%v err=%v", set, err)
	}
}

// TestLoadDotenvSemantics pins parity with Node dotenv / Vite (the same file the
// web runtime reads): quotes are stripped, `export ` is dropped, and inline
// comments on unquoted values are removed — so a quoted RTC_APP_KEY signs tokens
// with the same value the frontend uses, not a quote-wrapped mismatch.
func TestLoadDotenvSemantics(t *testing.T) {
	root := t.TempDir()
	content := "RTC_APP_KEY=\"abc secret\"\n" + // matched double quotes stripped, inner spaces kept
		"RTC_APP_ID='app-24-chars-000000000000'\n" + // single quotes stripped
		"export RTC_ROOM_ID=room-1\n" + // export prefix dropped
		"RTC_USER_ID=user-1 # trailing comment\n" + // inline comment removed
		"RTC_TAB_COMMENT=user-2\t# tab comment\n" + // tab-delimited inline comment removed
		"RTC_QUOTED_COMMENT=\"value\" # note\n" + // comment after closing quote ignored
		"RTC_HASH_LITERAL=ab#cd\n" // bare '#' (no leading whitespace) kept verbatim
	if err := os.WriteFile(Path(root), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	values, err := Load(Path(root))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := map[string]string{
		"RTC_APP_KEY":        "abc secret",
		"RTC_APP_ID":         "app-24-chars-000000000000",
		"RTC_ROOM_ID":        "room-1",
		"RTC_USER_ID":        "user-1",
		"RTC_TAB_COMMENT":    "user-2",
		"RTC_QUOTED_COMMENT": "value",
		"RTC_HASH_LITERAL":   "ab#cd",
	}
	for k, w := range want {
		if got := values[k]; got != w {
			t.Errorf("%s = %q, want %q", k, got, w)
		}
	}
}
