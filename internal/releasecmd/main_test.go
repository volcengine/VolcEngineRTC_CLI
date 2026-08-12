// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
