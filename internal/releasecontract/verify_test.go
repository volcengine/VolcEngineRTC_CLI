// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package releasecontract

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func fixtureBinary(t *testing.T, root, version string) []byte {
	return fixtureBinaryWithMarker(t, root, version, version)
}

func fixtureBinaryWithMarker(t *testing.T, root, version, markerVersion string) []byte {
	t.Helper()
	program := `package main
import (
  "embed"
  "encoding/json"
  "fmt"
  "os"
  "sort"
)
var Version = "dev"
var ReleaseMarker = "VERTC_RELEASE_VERSION=dev;"
//go:embed skills/*/SKILL.md
var content embed.FS
func main() {
  if ReleaseMarker == "" { os.Exit(3) }
  args := os.Args[1:]
  if len(args) > 0 && args[0] == "version" {
    _ = json.NewEncoder(os.Stdout).Encode(map[string]any{"ok":true,"data":map[string]string{"version":Version}}); return
  }
  if len(args) > 1 && args[0] == "skills" && args[1] == "list" {
    entries, _ := content.ReadDir("skills"); names := []string{}; for _, e := range entries { names = append(names, e.Name()) }; sort.Strings(names)
    skills := []map[string]string{}; for _, name := range names { skills = append(skills, map[string]string{"name":name}) }
    _ = json.NewEncoder(os.Stdout).Encode(map[string]any{"ok":true,"data":map[string]any{"skills":skills}}); return
  }
  if len(args) > 2 && args[0] == "skills" && args[1] == "read" { data, err := content.ReadFile("skills/"+args[2]+"/SKILL.md"); if err != nil { os.Exit(2) }; _, _ = os.Stdout.Write(data); return }
  fmt.Fprintln(os.Stderr, "unexpected args"); os.Exit(2)
}`
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte(program), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module fixture\n\ngo 1.23\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	binaryPath := filepath.Join(t.TempDir(), "vertc")
	command := exec.Command("go", "build", "-trimpath", "-ldflags", "-X main.Version="+version+" -X main.ReleaseMarker=VERTC_RELEASE_VERSION="+markerVersion+";", "-o", binaryPath, ".")
	command.Dir = root
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build fixture: %v\n%s", err, output)
	}
	data, err := os.ReadFile(binaryPath)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func archiveFixture(t *testing.T, target Target, binary []byte, skills map[string][]byte) []byte {
	t.Helper()
	var output bytes.Buffer
	if strings.HasSuffix(target.Archive, ".zip") {
		writer := zip.NewWriter(&output)
		files := map[string][]byte{target.Binary: binary}
		for name, data := range skills {
			files["skills/"+name+"/SKILL.md"] = data
		}
		for name, data := range files {
			entry, err := writer.Create(name)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = entry.Write(data)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		return output.Bytes()
	}
	gzipWriter := gzip.NewWriter(&output)
	tarWriter := tar.NewWriter(gzipWriter)
	files := map[string][]byte{target.Binary: binary}
	for name, data := range skills {
		files["skills/"+name+"/SKILL.md"] = data
	}
	for name, data := range files {
		if err := tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(data))}); err != nil {
			t.Fatal(err)
		}
		_, _ = tarWriter.Write(data)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func verifiedFixture(t *testing.T) (Manifest, string, string, string) {
	t.Helper()
	root := validSkills(t, "1.2.3")
	skills, _, err := DiscoverSkills(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := StampSkills(root, "1.2.3-rc.1", skills); err != nil {
		t.Fatal(err)
	}
	for index := range skills {
		skills[index].Version = "1.2.3-rc.1"
	}
	manifest := Manifest{
		Identity: Identity{Stability: StabilityPrerelease, Destination: DestinationPublic, Version: "1.2.3-rc.1", Baseline: "1.2.3"},
		Source:   "worktree", SourceRoot: root, SourceCommit: "fixture", Skills: skills,
		Targets: releaseTargets("1.2.3-rc.1"), PackageName: "@volcengine/rtc-cli", ChecksumFile: "checksums.txt",
	}
	binary := fixtureBinary(t, root, manifest.Version)
	artifacts := t.TempDir()
	expectedSkills := map[string][]byte{}
	for _, skill := range skills {
		expectedSkills[skill.Name], _ = os.ReadFile(filepath.Join(root, filepath.FromSlash(skill.Path)))
	}
	var checksumLines []string
	for _, target := range manifest.Targets {
		data := archiveFixture(t, target, binary, expectedSkills)
		if err := os.WriteFile(filepath.Join(artifacts, target.Archive), data, 0o644); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		checksumLines = append(checksumLines, fmt.Sprintf("%x  %s", sum, target.Archive))
	}
	checksums := filepath.Join(artifacts, manifest.ChecksumFile)
	if err := os.WriteFile(checksums, []byte(strings.Join(checksumLines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	packageJSON := filepath.Join(artifacts, "package.json")
	if err := os.WriteFile(packageJSON, []byte(`{"name":"@volcengine/rtc-cli","version":"1.2.3-rc.1"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	return manifest, artifacts, checksums, packageJSON
}

func TestVerifyAllTargetsAndSkills(t *testing.T) {
	manifest, artifacts, checksums, packageJSON := verifiedFixture(t)
	if err := Verify(VerifyOptions{Manifest: manifest, ArtifactsDir: artifacts, Checksums: checksums, PackageJSON: packageJSON}); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyRejectsOneMismatchingTarget(t *testing.T) {
	manifest, artifacts, checksums, packageJSON := verifiedFixture(t)
	target := manifest.Targets[0]
	archivePath := filepath.Join(artifacts, target.Archive)
	data, err := os.ReadFile(filepath.Join(manifest.SourceRoot, "skills", "byted-sample-alpha", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	badSkills := map[string][]byte{
		"byted-sample-alpha": bytes.Replace(data, []byte(manifest.Version), []byte("1.2.3-rc.9"), 1),
	}
	for _, skill := range manifest.Skills[1:] {
		badSkills[skill.Name], _ = os.ReadFile(filepath.Join(manifest.SourceRoot, filepath.FromSlash(skill.Path)))
	}
	binary := fixtureBinary(t, manifest.SourceRoot, manifest.Version)
	badArchive := archiveFixture(t, target, binary, badSkills)
	if err := os.WriteFile(archivePath, badArchive, 0o644); err != nil {
		t.Fatal(err)
	}
	// Keep checksums honest so postflight reaches the Skill mismatch.
	sum := sha256.Sum256(badArchive)
	checksumData, _ := os.ReadFile(checksums)
	lines := strings.Split(strings.TrimSpace(string(checksumData)), "\n")
	for index, line := range lines {
		if strings.HasSuffix(line, target.Archive) {
			lines[index] = fmt.Sprintf("%x  %s", sum, target.Archive)
		}
	}
	_ = os.WriteFile(checksums, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
	err = Verify(VerifyOptions{Manifest: manifest, ArtifactsDir: artifacts, Checksums: checksums, PackageJSON: packageJSON})
	if err == nil || !strings.Contains(err.Error(), "version mismatch") {
		t.Fatalf("error=%v", err)
	}
}

func TestVerifyRejectsPackageAndChecksumMismatch(t *testing.T) {
	manifest, artifacts, checksums, packageJSON := verifiedFixture(t)
	if err := os.WriteFile(packageJSON, []byte(`{"name":"@volcengine/rtc-cli","version":"9.9.9"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	err := Verify(VerifyOptions{Manifest: manifest, ArtifactsDir: artifacts, Checksums: checksums, PackageJSON: packageJSON})
	if err == nil || !strings.Contains(err.Error(), "package metadata") {
		t.Fatalf("error=%v", err)
	}
	if err := os.WriteFile(filepath.Join(artifacts, manifest.Targets[0].Archive), []byte("corrupt"), 0o644); err != nil {
		t.Fatal(err)
	}
	err = Verify(VerifyOptions{Manifest: manifest, ArtifactsDir: artifacts, Checksums: checksums})
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("error=%v", err)
	}
}

func TestVerifyRejectsCrossTargetReleaseMarkerMismatch(t *testing.T) {
	manifest, artifacts, checksums, _ := verifiedFixture(t)
	var target Target
	for _, candidate := range manifest.Targets {
		if candidate.GOOS != runtime.GOOS {
			target = candidate
			break
		}
	}
	if target.Archive == "" {
		t.Fatal("no cross target in manifest")
	}
	wrongBinary := fixtureBinaryWithMarker(t, manifest.SourceRoot, manifest.Version, "wrong-9.9.9")
	expectedSkills := map[string][]byte{}
	for _, skill := range manifest.Skills {
		expectedSkills[skill.Name], _ = os.ReadFile(filepath.Join(manifest.SourceRoot, filepath.FromSlash(skill.Path)))
	}
	badArchive := archiveFixture(t, target, wrongBinary, expectedSkills)
	if err := os.WriteFile(filepath.Join(artifacts, target.Archive), badArchive, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(badArchive)
	checksumData, _ := os.ReadFile(checksums)
	lines := strings.Split(strings.TrimSpace(string(checksumData)), "\n")
	for index, line := range lines {
		if strings.HasSuffix(line, target.Archive) {
			lines[index] = fmt.Sprintf("%x  %s", sum, target.Archive)
		}
	}
	_ = os.WriteFile(checksums, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
	err := Verify(VerifyOptions{Manifest: manifest, ArtifactsDir: artifacts, Checksums: checksums})
	if err == nil || !strings.Contains(err.Error(), "release marker") {
		t.Fatalf("error=%v", err)
	}
}

func TestVerifyBuildInfoRejectsPrefixVersionMarkers(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "skills", "byted-sample-alpha")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	const expected = "1.2.3"
	for _, actual := range []string{"1.2.3-dev", "1.2.3-snapshot", "1.2.3-rc.1"} {
		t.Run(actual, func(t *testing.T) {
			binary := fixtureBinaryWithMarker(t, root, expected, actual)
			if err := verifyBuildInfo(binary, expected); err == nil {
				t.Fatalf("verifyBuildInfo accepted prefix marker %q for %q", actual, expected)
			}
		})
	}
}

func TestFixtureCoversNativeTarget(t *testing.T) {
	manifest, _, _, _ := verifiedFixture(t)
	for _, target := range manifest.Targets {
		if target.GOOS == runtime.GOOS && target.GOARCH == runtime.GOARCH {
			return
		}
	}
	t.Fatalf("manifest does not cover native target %s/%s", runtime.GOOS, runtime.GOARCH)
}

func TestNativeBinaryFilename(t *testing.T) {
	for _, test := range []struct {
		goos string
		want string
	}{
		{goos: "linux", want: "vertc"},
		{goos: "darwin", want: "vertc"},
		{goos: "windows", want: "vertc.exe"},
	} {
		t.Run(test.goos, func(t *testing.T) {
			if got := nativeBinaryFilename(test.goos); got != test.want {
				t.Fatalf("nativeBinaryFilename(%q)=%q, want %q", test.goos, got, test.want)
			}
		})
	}
}

func TestPublicCommitPrepareBuildAndVerify(t *testing.T) {
	repo := releaseRepo(t)
	// Add the fixture CLI source to the committed public ref.
	_ = fixtureBinary(t, repo, "1.2.3")
	git(t, repo, "add", "main.go", "go.mod")
	git(t, repo, "commit", "-qm", "add fixture cli")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("dirty ignored\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	prepared := filepath.Join(t.TempDir(), "prepared")
	manifest, err := Prepare(PrepareOptions{
		RepoRoot: repo, Destination: prepared, Stability: StabilityStable, Publication: DestinationPublic,
		Version: "v1.2.3", Source: SourceCommit, Ref: "HEAD",
	})
	if err != nil {
		t.Fatal(err)
	}
	binary := fixtureBinary(t, prepared, manifest.Version)
	expectedSkills := map[string][]byte{}
	for _, skill := range manifest.Skills {
		expectedSkills[skill.Name], _ = os.ReadFile(filepath.Join(prepared, filepath.FromSlash(skill.Path)))
	}
	artifacts := t.TempDir()
	var checksumLines []string
	for _, target := range manifest.Targets {
		data := archiveFixture(t, target, binary, expectedSkills)
		if err := os.WriteFile(filepath.Join(artifacts, target.Archive), data, 0o644); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		checksumLines = append(checksumLines, fmt.Sprintf("%x  %s", sum, target.Archive))
	}
	checksums := filepath.Join(artifacts, manifest.ChecksumFile)
	if err := os.WriteFile(checksums, []byte(strings.Join(checksumLines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Verify(VerifyOptions{Manifest: manifest, ArtifactsDir: artifacts, Checksums: checksums}); err != nil {
		t.Fatal(err)
	}
}
