// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package tests

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// The `skills` command group serves binary-embedded content offline, with no
// auth and no network, byte-identical to the source, and rejects unsafe paths.
func TestSkillsListAndRead(t *testing.T) {
	dir := t.TempDir()

	// list → JSON envelope with ok/data.skills and a binary-bound version.
	r := run(t, dir, nil, "skills", "list")
	if r.code != 0 {
		t.Fatalf("skills list exit %d: %s", r.code, r.stderr)
	}
	env := r.envelope(t)
	if env["ok"] != true {
		t.Fatalf("skills list not ok: %v", env)
	}
	data, _ := env["data"].(map[string]any)
	skills, _ := data["skills"].([]any)
	if len(skills) != 1 {
		t.Fatalf("expected the single bundled skill, got %v", data["skills"])
	}
	if data["version"] == nil || data["version"] == "" {
		t.Fatalf("skills list missing binary-bound version: %v", data)
	}

	// read (raw markdown default) → byte-identical to the embedded source.
	r = run(t, dir, nil, "skills", "read", "byted-interactai-guide")
	if r.code != 0 {
		t.Fatalf("skills read exit %d: %s", r.code, r.stderr)
	}
	srcBytes, err := os.ReadFile(filepath.Join("..", "skills", "byted-interactai-guide", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if r.stdout != string(srcBytes) {
		t.Fatal("skills read stdout not byte-identical to source SKILL.md")
	}
	if strings.Contains(r.stdout, "tip:") {
		t.Fatal("guidance leaked into stdout")
	}
}

func TestSkillsRejectsUnsafePaths(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		args     []string
		wantCode string
	}{
		{[]string{"skills", "read", "byted-interactai-guide", "../../etc/passwd"}, "vertc.skills.invalid_path"},
		{[]string{"skills", "read", "byted-interactai-guide", "/etc/passwd"}, "vertc.skills.invalid_path"},
		{[]string{"skills", "read", "not-a-real-skill"}, "vertc.skills.unknown_skill"},
		{[]string{"skills", "read", "byted-interactai-guide", "references/does-not-exist.md"}, "vertc.skills.not_found"},
	}
	for _, c := range cases {
		r := run(t, dir, nil, c.args...)
		if r.code == 0 {
			t.Fatalf("%v: expected non-zero exit", c.args)
		}
		env := r.envelope(t)
		errObj, _ := env["error"].(map[string]any)
		if errObj["code"] != c.wantCode {
			t.Fatalf("%v: got code %v, want %s", c.args, errObj["code"], c.wantCode)
		}
	}
}

// skills commands work with no auth session and no network reachable.
func TestSkillsWorkOffline(t *testing.T) {
	dir := t.TempDir()
	env := []string{"HTTP_PROXY=http://127.0.0.1:1", "HTTPS_PROXY=http://127.0.0.1:1", "VERTC_NO_UPDATE_NOTIFIER=1"}
	r := run(t, dir, env, "skills", "read", "byted-interactai-guide")
	if r.code != 0 {
		t.Fatalf("skills read offline exit %d: %s", r.code, r.stderr)
	}
	if !strings.Contains(r.stdout, "byted-interactai-guide") {
		t.Fatal("expected the voice-agent skill content")
	}
}

func TestSkillsSyncInstallsFromEmbeddedCopyAndReportsFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake npx fixture uses a POSIX shell")
	}
	fakeBin := t.TempDir()
	npxPath := filepath.Join(fakeBin, "npx")
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$@\" >> \"$VERTC_NPX_ARGS\"\n" +
		"if [ \"$2\" = add ]; then test -f \"$3/skills/byted-interactai-guide/SKILL.md\" || exit 42; fi\n" +
		"exit \"${VERTC_NPX_EXIT:-0}\"\n"
	if err := os.WriteFile(npxPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	pathEnv := "PATH=" + fakeBin + string(os.PathListSeparator) + os.Getenv("PATH")

	t.Run("success", func(t *testing.T) {
		stateDir, argsFile := t.TempDir(), filepath.Join(t.TempDir(), "npx-args")
		r := run(t, t.TempDir(), []string{
			pathEnv,
			"VERTC_STATE_DIR=" + stateDir,
			"VERTC_NPX_ARGS=" + argsFile,
			"VERTC_NO_UPDATE_NOTIFIER=1",
		}, "skills", "sync")
		if r.code != 0 {
			t.Fatalf("skills sync exit %d: %s", r.code, r.stderr)
		}
		data, _ := r.envelope(t)["data"].(map[string]any)
		if data["status"] != "synchronized" {
			t.Fatalf("unexpected sync result: %v", data)
		}
		args, err := os.ReadFile(argsFile)
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Fields(string(args))
		if len(lines) != 16 || lines[0] != "skills" || lines[1] != "add" ||
			lines[3] != "-g" || lines[4] != "--copy" || lines[5] != "-y" {
			t.Fatalf("unexpected npx args: %q", args)
		}
		wantRemove := []string{
			"skills", "remove", "byted-interactai-voice-agent", "-g", "-y",
			"skills", "remove", "vertc-voice-agent", "-g", "-y",
		}
		if !slices.Equal(lines[6:], wantRemove) {
			t.Fatalf("unexpected retired skill removal args: %q", lines[6:])
		}
		if _, err := os.Stat(lines[2]); !os.IsNotExist(err) {
			t.Fatalf("materialized source survived sync: %v", err)
		}
		if _, err := os.Stat(filepath.Join(stateDir, "skills-state.json")); err != nil {
			t.Fatalf("sync state missing: %v", err)
		}
	})

	t.Run("failure", func(t *testing.T) {
		stateDir := t.TempDir()
		r := run(t, t.TempDir(), []string{
			pathEnv,
			"VERTC_STATE_DIR=" + stateDir,
			"VERTC_NPX_ARGS=" + filepath.Join(t.TempDir(), "npx-args"),
			"VERTC_NPX_EXIT=7",
			"VERTC_NO_UPDATE_NOTIFIER=1",
		}, "skills", "sync")
		if r.code == 0 {
			t.Fatal("expected skills sync failure")
		}
		errObj, _ := r.envelope(t)["error"].(map[string]any)
		if errObj["code"] != "vertc.skills.sync_failed" {
			t.Fatalf("unexpected error: %v", errObj)
		}
		if _, err := os.Stat(filepath.Join(stateDir, "skills-state.json")); !os.IsNotExist(err) {
			t.Fatalf("failed sync wrote state: %v", err)
		}
	})
}

func TestSkillsSyncDryRunAndArgumentValidation(t *testing.T) {
	stateDir := t.TempDir()
	r := run(t, t.TempDir(), []string{
		"VERTC_STATE_DIR=" + stateDir,
		"VERTC_NO_UPDATE_NOTIFIER=1",
	}, "skills", "sync", "--dry-run")
	if r.code != 0 {
		t.Fatalf("skills sync dry-run exit %d: %s", r.code, r.stderr)
	}
	data, _ := r.envelope(t)["data"].(map[string]any)
	if data["status"] != "would_sync" {
		t.Fatalf("unexpected dry-run result: %v", data)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "skills-state.json")); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote state: %v", err)
	}

	r = run(t, t.TempDir(), nil, "skills", "sync", "unexpected")
	if r.code == 0 {
		t.Fatal("skills sync accepted a positional argument")
	}
}
