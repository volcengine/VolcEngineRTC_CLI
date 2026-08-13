// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/releasecontract"
)

func commandFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	skillDir := filepath.Join(root, "skills", "byted-sample-guide")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	skill := "---\nname: byted-sample-guide\ndescription: command fixture\nversion: \"1.2.3\"\n---\n\n# Fixture\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skill), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "LICENSE"), []byte("fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte("{\"name\":\"fixture\",\"version\":\"1.2.3\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	changelog := "# Changelog\n\n## 1.2.4\n\n- Fixture release.\n"
	if err := os.WriteFile(filepath.Join(root, "CHANGELOG.md"), []byte(changelog), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestSourceVersionModes(t *testing.T) {
	root := commandFixture(t)
	packagePath := filepath.Join(root, "package.json")
	before, err := os.ReadFile(packagePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := sourceVersion([]string{"--repo", root, "--version", "1.2.4", "--dry-run"}); err != nil {
		t.Fatal(err)
	}
	afterDryRun, _ := os.ReadFile(packagePath)
	if string(afterDryRun) != string(before) {
		t.Fatal("dry-run modified package.json")
	}
	if err := sourceVersion([]string{"--repo", root, "--version", "1.2.4", "--check"}); err == nil || !strings.Contains(err.Error(), "not 1.2.4") {
		t.Fatalf("check error=%v", err)
	}
	if err := sourceVersion([]string{"--repo", root, "--version", "1.2.4"}); err != nil {
		t.Fatal(err)
	}
	if err := sourceVersion([]string{"--repo", root, "--version", "1.2.4", "--check"}); err != nil {
		t.Fatal(err)
	}
}

func TestSourceVersionRejectsConflictingModes(t *testing.T) {
	root := commandFixture(t)
	err := sourceVersion([]string{"--repo", root, "--version", "1.2.4", "--check", "--dry-run"})
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("error=%v", err)
	}
}

func TestSourceVersionCheckRequiresMatchingChangelogEntry(t *testing.T) {
	root := commandFixture(t)
	err := sourceVersion([]string{"--repo", root, "--version", "1.2.3", "--check"})
	if err == nil || !strings.Contains(err.Error(), "CHANGELOG.md") || !strings.Contains(err.Error(), "## 1.2.3") {
		t.Fatalf("error=%v", err)
	}
}

func TestSourceVersionCheckRejectsPlaceholderChangelogEntry(t *testing.T) {
	root := commandFixture(t)
	changelog := "# Changelog\n\n## 1.2.3\n\nInitial public release preparation.\n"
	if err := os.WriteFile(filepath.Join(root, "CHANGELOG.md"), []byte(changelog), 0o644); err != nil {
		t.Fatal(err)
	}
	err := sourceVersion([]string{"--repo", root, "--version", "1.2.3", "--check"})
	if err == nil || !strings.Contains(err.Error(), "placeholder") {
		t.Fatalf("error=%v", err)
	}
}

func TestReleaseCommandArgumentValidation(t *testing.T) {
	tests := []struct {
		name string
		run  func() error
		want string
	}{
		{name: "source required", run: func() error { return sourceVersion(nil) }, want: "requires --repo and --version"},
		{name: "source positional", run: func() error {
			return sourceVersion([]string{"--repo", commandFixture(t), "--version", "1.2.3", "extra"})
		}, want: "no positional"},
		{name: "resolve invalid", run: func() error { return resolve([]string{"--stability", "invalid", "--publication", "none"}) }, want: "stability"},
		{name: "prepare required", run: func() error { return prepare(nil) }, want: "prepare requires"},
		{name: "verify required", run: func() error { return verify(nil) }, want: "verify requires"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.run()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error=%v, want containing %q", err, tc.want)
			}
		})
	}
	missingManifest := filepath.Join(t.TempDir(), "missing.json")
	if err := manifestVersion([]string{"--manifest", missingManifest}); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing manifest error=%v, want os.ErrNotExist", err)
	}
}

func TestResolveAndManifestVersionPrintCanonicalVersion(t *testing.T) {
	got := captureStdout(t, func() error {
		return resolve([]string{"--stability", "stable", "--publication", "public", "--version", "v1.2.3"})
	})
	if got != "1.2.3\n" {
		t.Fatalf("resolve output=%q", got)
	}

	manifestPath := filepath.Join(t.TempDir(), "manifest.json")
	manifest := releasecontract.Manifest{
		Identity:     releasecontract.Identity{Version: "2.3.4"},
		SourceRoot:   t.TempDir(),
		Skills:       []releasecontract.Skill{{Name: "sample", Path: "skills/sample/SKILL.md"}},
		Targets:      []releasecontract.Target{{GOOS: "linux", GOARCH: "amd64"}},
		PackageName:  "fixture",
		ChecksumFile: "checksums.txt",
	}
	if err := releasecontract.WriteManifest(manifestPath, manifest); err != nil {
		t.Fatal(err)
	}
	got = captureStdout(t, func() error { return manifestVersion([]string{"--manifest", manifestPath}) })
	if got != "2.3.4\n" {
		t.Fatalf("manifest-version output=%q", got)
	}
}

func TestVerifyReadsManifestBeforeArtifactValidation(t *testing.T) {
	manifestPath := filepath.Join(t.TempDir(), "malformed.json")
	if err := os.WriteFile(manifestPath, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := verify([]string{
		"--manifest", manifestPath,
		"--artifacts", filepath.Join(t.TempDir(), "missing-artifacts"),
		"--checksums", filepath.Join(t.TempDir(), "missing-checksums.txt"),
	})
	if err == nil || !strings.Contains(err.Error(), "read release manifest") {
		t.Fatalf("error=%v, want manifest read failure before artifact validation", err)
	}
}

func captureStdout(t *testing.T, run func() error) string {
	t.Helper()
	previous := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	t.Cleanup(func() { os.Stdout = previous })
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
