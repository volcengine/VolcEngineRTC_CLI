// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package releasecontract

import (
	"crypto/sha256"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func git(t *testing.T, root string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-c", "core.hooksPath=/dev/null", "-C", root}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return string(output)
}

func releaseRepo(t *testing.T) string {
	t.Helper()
	root, err := os.MkdirTemp("", "release-contract-repo-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	writeSkill(t, root, "byted-sample-alpha", SourceSkillVersion)
	writeSkill(t, root, "byted-sample-beta", SourceSkillVersion)
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("committed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "deleted.txt"), []byte("delete me\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte("{\"name\":\"@volcengine/rtc-cli\",\"version\":\"1.2.3\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, root, "init", "-q")
	git(t, root, "config", "user.name", "Release Test")
	git(t, root, "config", "user.email", "release@example.com")
	git(t, root, "add", ".")
	git(t, root, "commit", "-qm", "fixture")
	return root
}

func fileHash(t *testing.T, path string) [32]byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(data)
}

func TestPrepareDirtyWorktreeIsIsolated(t *testing.T) {
	root := releaseRepo(t)
	skillPath := filepath.Join(root, "skills", "byted-sample-alpha", "SKILL.md")
	beforeSkill := fileHash(t, skillPath)
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("staged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, root, "add", "README.md")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("unstaged after staged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "untracked.txt"), []byte("untracked\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "deleted.txt")); err != nil {
		t.Fatal(err)
	}
	beforeStatus := git(t, root, "status", "--porcelain=v1", "--untracked-files=all")
	destination := filepath.Join(t.TempDir(), "prepared")
	manifest, err := Prepare(PrepareOptions{
		RepoRoot: root, Destination: destination, Stability: StabilityPrerelease, Publication: DestinationPublic,
		Version: "1.2.3-rc.7", Source: SourceWorktree, AllowDirty: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Version != "1.2.3-rc.7" || len(manifest.Skills) != 2 {
		t.Fatalf("manifest=%+v", manifest)
	}
	if got := git(t, root, "status", "--porcelain=v1", "--untracked-files=all"); got != beforeStatus {
		t.Fatalf("worktree status changed:\nbefore=%q\nafter=%q", beforeStatus, got)
	}
	if got := fileHash(t, skillPath); got != beforeSkill {
		t.Fatal("invoking Skill was modified")
	}
	data, _ := os.ReadFile(filepath.Join(destination, "README.md"))
	if string(data) != "unstaged after staged\n" {
		t.Fatalf("prepared README=%q", data)
	}
	if _, err := os.Stat(filepath.Join(destination, "untracked.txt")); err != nil {
		t.Fatal("untracked file was not captured")
	}
	if _, err := os.Stat(filepath.Join(destination, "deleted.txt")); !os.IsNotExist(err) {
		t.Fatal("working-tree deletion was not captured")
	}
	stamped, _ := os.ReadFile(filepath.Join(destination, "skills", "byted-sample-alpha", "SKILL.md"))
	if !strings.Contains(string(stamped), `version: "1.2.3-rc.7"`) {
		t.Fatalf("Skill not stamped:\n%s", stamped)
	}
}

func TestPrepareDirtyRequiresOptIn(t *testing.T) {
	root := releaseRepo(t)
	if err := os.WriteFile(filepath.Join(root, "dirty.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "prepared")
	_, err := Prepare(PrepareOptions{RepoRoot: root, Destination: destination, Stability: StabilityPrerelease, Publication: DestinationPublic, Version: "1.2.3-rc.1", Source: SourceWorktree})
	if err == nil || !strings.Contains(err.Error(), "refusing dirty") {
		t.Fatalf("error=%v", err)
	}
	if _, statErr := os.Stat(destination); !os.IsNotExist(statErr) {
		t.Fatal("destination exists after preflight failure")
	}
}

func TestPrepareCommitIgnoresDirtyWorktree(t *testing.T) {
	root := releaseRepo(t)
	committedBlob := strings.TrimSpace(git(t, root, "rev-parse", "HEAD:README.md"))
	git(t, root, "config", "core.autocrlf", "true")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "prepared")
	manifest, err := Prepare(PrepareOptions{RepoRoot: root, Destination: destination, Stability: StabilityStable, Publication: DestinationPublic, Version: "v9.8.7", Source: SourceCommit, Ref: "HEAD"})
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Source != "commit" || manifest.Version != "9.8.7" {
		t.Fatalf("manifest=%+v", manifest)
	}
	wantCommit := strings.TrimSpace(git(t, root, "rev-parse", "HEAD"))
	wantDate := strings.TrimSpace(git(t, root, "show", "-s", "--format=%cI", "HEAD"))
	if manifest.SourceCommit != wantCommit || manifest.SourceDate != wantDate {
		t.Fatalf("source identity=%s/%s, want %s/%s", manifest.SourceCommit, manifest.SourceDate, wantCommit, wantDate)
	}
	preparedBlob := strings.TrimSpace(git(t, root, "hash-object", "--no-filters", filepath.Join(destination, "README.md")))
	if preparedBlob != committedBlob {
		t.Fatalf("commit export blob=%s, want %s", preparedBlob, committedBlob)
	}
}

func TestExportCommitReapsProcessAfterFilesystemFailure(t *testing.T) {
	root := releaseRepo(t)
	destination := t.TempDir()
	if err := os.WriteFile(filepath.Join(destination, "skills"), []byte("blocks archive directory\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() { done <- exportCommit(root, "HEAD", destination) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("exportCommit unexpectedly succeeded with a file blocking the skills directory")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("exportCommit did not reap git archive after a filesystem failure")
	}
}

func TestPrepareFailureCleansOwnedDestination(t *testing.T) {
	root := releaseRepo(t)
	destination := filepath.Join(t.TempDir(), "prepared")
	_, err := Prepare(PrepareOptions{
		RepoRoot: root, Destination: destination, Stability: StabilityPrerelease, Publication: DestinationPublic,
		Version: "1.2.3-rc.1", Source: SourceWorktree,
		AfterCopyFor: func(string) error { return errors.New("injected after-copy failure") },
	})
	if err == nil || !strings.Contains(err.Error(), "injected") {
		t.Fatalf("error=%v", err)
	}
	if _, statErr := os.Stat(destination); !os.IsNotExist(statErr) {
		t.Fatal("owned destination remains after failure")
	}
}

func TestPrepareRefusesDestinationInsideRepository(t *testing.T) {
	root := releaseRepo(t)
	_, err := Prepare(PrepareOptions{RepoRoot: root, Destination: filepath.Join(root, "prepared"), Stability: StabilityStable, Publication: DestinationPublic, Version: "1.2.3", Source: SourceCommit})
	if err == nil || !strings.Contains(err.Error(), "outside") {
		t.Fatalf("error=%v", err)
	}
}

func TestPrepareMissingPackageMetadataCleansDestination(t *testing.T) {
	root := releaseRepo(t)
	if err := os.Remove(filepath.Join(root, "package.json")); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "prepared")
	_, err := Prepare(PrepareOptions{
		RepoRoot: root, Destination: destination, Stability: StabilityPrerelease, Publication: DestinationPublic,
		Version: "1.2.3-rc.1", Source: SourceWorktree, AllowDirty: true,
	})
	if err == nil || !strings.Contains(err.Error(), "package.json is missing") {
		t.Fatalf("error=%v", err)
	}
	if _, statErr := os.Stat(destination); !os.IsNotExist(statErr) {
		t.Fatal("destination remains after package metadata failure")
	}
}
