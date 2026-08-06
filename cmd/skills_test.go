// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/meta"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/selfupdate"
)

func TestParseSkillReadTarget(t *testing.T) {
	cases := []struct {
		args          []string
		name, relpath string
		wantCode      string
	}{
		{[]string{"byted-interactai-guide"}, "byted-interactai-guide", "", ""},
		{[]string{"byted-interactai-guide/references/quickstart.md"}, "byted-interactai-guide", "references/quickstart.md", ""},
		{[]string{"byted-interactai-guide", "references/quickstart.md"}, "byted-interactai-guide", "references/quickstart.md", ""},
		{[]string{"byted-interactai-guide/references", "quickstart.md"}, "", "", "vertc.cli.invalid_flag"},
		{[]string{`byted-interactai-guide\references`, "quickstart.md"}, "", "", "vertc.cli.invalid_flag"},
		{[]string{}, "", "", "vertc.cli.invalid_flag"},
		{[]string{"a", "b", "c"}, "", "", "vertc.cli.invalid_flag"},
	}
	for _, c := range cases {
		name, rel, err := parseSkillReadTarget(c.args)
		if c.wantCode != "" {
			typed, ok := errs.As(err)
			if !ok || typed.Code != c.wantCode {
				t.Fatalf("args=%v: err=%v, want code %q", c.args, err, c.wantCode)
			}
			continue
		}
		if err != nil || name != c.name || rel != c.relpath {
			t.Fatalf("args=%v: got (%q,%q,%v), want (%q,%q,nil)", c.args, name, rel, err, c.name, c.relpath)
		}
	}
}

// skillsReader returns the unavailable error when nothing is embedded.
func TestSkillsReaderUnavailable(t *testing.T) {
	orig := officialSkillsFS
	t.Cleanup(func() { officialSkillsFS = orig })
	officialSkillsFS = nil

	_, err := skillsReader()
	typed, ok := errs.As(err)
	if !ok || typed.Code != "vertc.skills.unavailable" {
		t.Fatalf("err = %v, want vertc.skills.unavailable", err)
	}
}

// With an injected FS, the reader resolves official skills.
func TestSkillsReaderWithInjectedFS(t *testing.T) {
	orig := officialSkillsFS
	t.Cleanup(func() { officialSkillsFS = orig })
	officialSkillsFS = fstest.MapFS{
		"byted-interactai-guide/SKILL.md": {Data: []byte("---\nname: byted-interactai-guide\ndescription: va\n---\n")},
	}
	r, err := skillsReader()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.ReadSkill("byted-interactai-guide"); err != nil {
		t.Fatalf("ReadSkill: %v", err)
	}
}

// The real embedded content (skills.Content) is served and byte-identical.
func TestSkillsReaderEmbeddedContent(t *testing.T) {
	r, err := skillsReader()
	if err != nil {
		t.Fatal(err)
	}
	skills, err := r.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) == 0 {
		t.Fatal("expected embedded official skills")
	}
}

func withSkillsSyncOverrides(t *testing.T) {
	t.Helper()
	oldVersion, oldFormat, oldDryRun := meta.Version, flagFormat, flagDryRun
	meta.Version = "1.2.3"
	flagFormat = "json"
	flagDryRun = false
	t.Cleanup(func() {
		meta.Version, flagFormat, flagDryRun = oldVersion, oldFormat, oldDryRun
		selfupdate.SkillsSyncOverride = nil
		selfupdate.SkillsAddOverride = nil
	})
}

func captureSkillsSyncOutput(t *testing.T, dryRun bool) ([]byte, error) {
	t.Helper()
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStdout := os.Stdout
	os.Stdout = write
	err = runSkillsSync(context.Background(), dryRun)
	_ = write.Close()
	os.Stdout = oldStdout
	body, readErr := io.ReadAll(read)
	if readErr != nil {
		t.Fatal(readErr)
	}
	return body, err
}

func TestRunSkillsSyncSuccess(t *testing.T) {
	withSkillsSyncOverrides(t)
	called := false
	selfupdate.SkillsSyncOverride = func(_ context.Context, version string) (selfupdate.SkillsSyncResult, error) {
		called = true
		if version != "1.2.3" {
			t.Fatalf("version=%q", version)
		}
		return selfupdate.SkillsSyncResult{}, nil
	}
	body, err := captureSkillsSyncOutput(t, false)
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("sync was not invoked")
	}
	var envelope struct {
		Data skillsSyncResult `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.Status != "synchronized" || envelope.Data.Version != "1.2.3" {
		t.Fatalf("unexpected result: %+v", envelope.Data)
	}
}

func TestRunSkillsSyncReportsRecoverableWarning(t *testing.T) {
	withSkillsSyncOverrides(t)
	selfupdate.SkillsSyncOverride = func(context.Context, string) (selfupdate.SkillsSyncResult, error) {
		return selfupdate.SkillsSyncResult{Warnings: []string{"PromptScript was skipped"}}, nil
	}
	readErr, writeErr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStderr := os.Stderr
	os.Stderr = writeErr
	_, syncErr := captureSkillsSyncOutput(t, false)
	_ = writeErr.Close()
	os.Stderr = oldStderr
	stderr, readErrValue := io.ReadAll(readErr)
	if readErrValue != nil {
		t.Fatal(readErrValue)
	}
	if syncErr != nil {
		t.Fatal(syncErr)
	}
	if !strings.Contains(string(stderr), "warn: PromptScript was skipped") {
		t.Fatalf("stderr=%q", stderr)
	}
}

func TestRunSkillsSyncDryRunSkipsSideEffects(t *testing.T) {
	withSkillsSyncOverrides(t)
	selfupdate.SkillsSyncOverride = func(context.Context, string) (selfupdate.SkillsSyncResult, error) {
		t.Fatal("dry-run invoked synchronization")
		return selfupdate.SkillsSyncResult{}, nil
	}
	body, err := captureSkillsSyncOutput(t, true)
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Data skillsSyncResult `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.Status != "would_sync" {
		t.Fatalf("unexpected result: %+v", envelope.Data)
	}
}

func TestRunSkillsSyncReturnsTypedFailure(t *testing.T) {
	withSkillsSyncOverrides(t)
	selfupdate.SkillsSyncOverride = func(context.Context, string) (selfupdate.SkillsSyncResult, error) {
		return selfupdate.SkillsSyncResult{}, errors.New("npx unavailable")
	}
	_, err := captureSkillsSyncOutput(t, false)
	typed, ok := errs.As(err)
	if !ok || typed.Code != "vertc.skills.sync_failed" {
		t.Fatalf("err=%v, want vertc.skills.sync_failed", err)
	}
}
