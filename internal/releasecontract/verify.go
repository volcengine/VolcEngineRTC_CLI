// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package releasecontract

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// VerifyOptions identifies the completed, still-temporary release outputs.
type VerifyOptions struct {
	Manifest     Manifest
	ArtifactsDir string
	Checksums    string
	PackageJSON  string
}

// Verify proves version and Skill equality for every manifest target before
// callers are allowed to publish or expose final outputs.
func Verify(options VerifyOptions) error {
	manifest := options.Manifest
	if manifest.Version == "" || len(manifest.Skills) == 0 || len(manifest.Targets) == 0 {
		return errors.New("cannot verify an incomplete release manifest")
	}
	expectedSkills := make(map[string][]byte, len(manifest.Skills))
	for _, skill := range manifest.Skills {
		data, err := os.ReadFile(filepath.Join(manifest.SourceRoot, filepath.FromSlash(skill.Path)))
		if err != nil {
			return fmt.Errorf("read prepared Skill %q: %w", skill.Name, err)
		}
		fm, err := parseFrontmatter(data)
		if err != nil || fm.Version != manifest.Version {
			return fmt.Errorf("prepared Skill %q version is not %q", skill.Name, manifest.Version)
		}
		expectedSkills[skill.Name] = data
	}

	checksums, err := readChecksums(options.Checksums)
	if err != nil {
		return err
	}
	seenArchives := map[string]bool{}
	for _, target := range manifest.Targets {
		archivePath := filepath.Join(options.ArtifactsDir, target.Archive)
		archiveData, err := os.ReadFile(archivePath)
		if err != nil {
			return fmt.Errorf("read target archive %s: %w", target.Archive, err)
		}
		if !strings.Contains(target.Archive, "_"+manifest.Version+"_") {
			return fmt.Errorf("archive %q does not contain canonical version %q", target.Archive, manifest.Version)
		}
		wantChecksum, ok := checksums[target.Archive]
		if !ok {
			return fmt.Errorf("checksum missing for %s", target.Archive)
		}
		actualChecksum := sha256.Sum256(archiveData)
		if hex.EncodeToString(actualChecksum[:]) != wantChecksum {
			return fmt.Errorf("checksum mismatch for %s", target.Archive)
		}
		files, err := readArchive(target.Archive, archiveData)
		if err != nil {
			return err
		}
		if err := verifyArchiveTarget(manifest, target, files, expectedSkills); err != nil {
			return fmt.Errorf("verify %s: %w", target.Archive, err)
		}
		seenArchives[target.Archive] = true
	}
	for name := range checksums {
		if strings.HasSuffix(name, ".tar.gz") || strings.HasSuffix(name, ".zip") {
			if !seenArchives[name] {
				return fmt.Errorf("checksums contains unexpected release archive %s", name)
			}
		}
	}
	if options.PackageJSON != "" {
		if err := verifyPackageJSON(options.PackageJSON, manifest); err != nil {
			return err
		}
	}
	return nil
}

func verifyArchiveTarget(manifest Manifest, target Target, files map[string][]byte, expectedSkills map[string][]byte) error {
	binary, ok := files[target.Binary]
	if !ok {
		return fmt.Errorf("binary %s is missing", target.Binary)
	}
	if err := verifyBuildInfo(binary, manifest.Version); err != nil {
		return err
	}
	actualSkills := map[string][]byte{}
	for name, data := range files {
		parts := strings.Split(name, "/")
		if len(parts) == 3 && parts[0] == "skills" && parts[2] == "SKILL.md" {
			actualSkills[parts[1]] = data
		}
	}
	if len(actualSkills) != len(expectedSkills) {
		return fmt.Errorf("archive Skill count=%d, want %d", len(actualSkills), len(expectedSkills))
	}
	for name, expected := range expectedSkills {
		actual, ok := actualSkills[name]
		if !ok {
			return fmt.Errorf("archived Skill %q is missing", name)
		}
		fm, err := parseFrontmatter(actual)
		if err != nil || fm.Version != manifest.Version {
			return fmt.Errorf("archived Skill %q version mismatch", name)
		}
		if !bytes.Equal(actual, expected) {
			return fmt.Errorf("archived Skill %q differs from prepared source", name)
		}
		if !bytes.Contains(binary, expected) {
			return fmt.Errorf("binary does not contain prepared Skill %q bytes", name)
		}
	}
	for name := range actualSkills {
		if _, ok := expectedSkills[name]; !ok {
			return fmt.Errorf("archive contains unexpected Skill %q", name)
		}
	}
	if target.GOOS == runtime.GOOS && target.GOARCH == runtime.GOARCH {
		if err := verifyNativeCLI(binary, manifest, expectedSkills); err != nil {
			return err
		}
	}
	return nil
}

func verifyBuildInfo(binary []byte, version string) error {
	marker := "VERTC_RELEASE_VERSION=" + version + ";"
	info, err := buildinfo.Read(bytes.NewReader(binary))
	if err != nil {
		return fmt.Errorf("read binary build info: %w", err)
	}
	var ldflags string
	for _, setting := range info.Settings {
		if setting.Key == "-ldflags" && strings.Contains(setting.Value, ".ReleaseMarker="+marker) {
			return nil
		}
		if setting.Key == "-ldflags" {
			ldflags = setting.Value
		}
	}
	// The Go build info format does not guarantee that -ldflags is retained. For
	// cross targets, require the exact canonical value in the binary image; the
	// native target is additionally executed below and proves the runtime field.
	if bytes.Contains(binary, []byte(marker)) {
		return nil
	}
	return fmt.Errorf("binary linker metadata %q and image do not contain canonical release marker %q", ldflags, marker)
}

func verifyNativeCLI(binary []byte, manifest Manifest, expectedSkills map[string][]byte) error {
	dir, err := os.MkdirTemp("", "vertc-release-verify-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, nativeBinaryFilename(runtime.GOOS))
	if err := os.WriteFile(path, binary, 0o700); err != nil {
		return err
	}
	env := append(os.Environ(), "VERTC_NO_UPDATE_NOTIFIER=1", "VERTC_NO_SKILLS_NOTIFIER=1")
	versionOutput, err := runCLI(path, env, "version", "--format", "json")
	if err != nil {
		return fmt.Errorf("run native version verification: %w", err)
	}
	var versionEnvelope struct {
		Data struct {
			Version   string `json:"version"`
			Commit    string `json:"commit"`
			BuildDate string `json:"build_date"`
		} `json:"data"`
	}
	if err := json.Unmarshal(versionOutput, &versionEnvelope); err != nil || versionEnvelope.Data.Version != manifest.Version {
		return fmt.Errorf("native CLI version=%q, want %q", versionEnvelope.Data.Version, manifest.Version)
	}
	if manifest.Destination == DestinationPublic && (versionEnvelope.Data.Commit != manifest.SourceCommit || versionEnvelope.Data.BuildDate != manifest.SourceDate) {
		return fmt.Errorf("native CLI source identity=%q/%q, want %q/%q", versionEnvelope.Data.Commit, versionEnvelope.Data.BuildDate, manifest.SourceCommit, manifest.SourceDate)
	}
	listOutput, err := runCLI(path, env, "skills", "list", "--format", "json")
	if err != nil {
		return fmt.Errorf("run native Skill list verification: %w", err)
	}
	var listEnvelope struct {
		Data struct {
			Skills []struct {
				Name string `json:"name"`
			} `json:"skills"`
		} `json:"data"`
	}
	if err := json.Unmarshal(listOutput, &listEnvelope); err != nil {
		return fmt.Errorf("decode native Skill list: %w", err)
	}
	names := make([]string, 0, len(listEnvelope.Data.Skills))
	for _, skill := range listEnvelope.Data.Skills {
		names = append(names, skill.Name)
	}
	sort.Strings(names)
	expectedNames := make([]string, 0, len(expectedSkills))
	for name := range expectedSkills {
		expectedNames = append(expectedNames, name)
	}
	sort.Strings(expectedNames)
	if strings.Join(names, "\x00") != strings.Join(expectedNames, "\x00") {
		return fmt.Errorf("native embedded Skill set=%v, want %v", names, expectedNames)
	}
	for _, name := range expectedNames {
		actual, err := runCLI(path, env, "skills", "read", name)
		if err != nil {
			return fmt.Errorf("read native embedded Skill %q: %w", name, err)
		}
		if !bytes.Equal(actual, expectedSkills[name]) {
			return fmt.Errorf("native embedded Skill %q differs from prepared source", name)
		}
	}
	return nil
}

func nativeBinaryFilename(goos string) string {
	if goos == "windows" {
		return "vertc.exe"
	}
	return "vertc"
}

func runCLI(path string, env []string, args ...string) ([]byte, error) {
	command := exec.Command(path, args...)
	command.Env = env
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", strings.TrimSpace(stderr.String()), err)
	}
	return output, nil
}

func readChecksums(file string) (map[string]string, error) {
	input, err := os.Open(file)
	if err != nil {
		return nil, fmt.Errorf("read checksums: %w", err)
	}
	defer input.Close()
	checksums := map[string]string{}
	scanner := bufio.NewScanner(input)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 || len(fields[0]) != 64 {
			return nil, fmt.Errorf("malformed checksum line %q", scanner.Text())
		}
		name := strings.TrimPrefix(fields[1], "*")
		if _, exists := checksums[name]; exists {
			return nil, fmt.Errorf("duplicate checksum for %s", name)
		}
		if _, err := hex.DecodeString(fields[0]); err != nil {
			return nil, fmt.Errorf("invalid checksum for %s", name)
		}
		checksums[name] = strings.ToLower(fields[0])
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return checksums, nil
}

func readArchive(name string, data []byte) (map[string][]byte, error) {
	files := map[string][]byte{}
	if strings.HasSuffix(name, ".zip") {
		reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", name, err)
		}
		for _, file := range reader.File {
			if file.FileInfo().IsDir() {
				continue
			}
			if !file.Mode().IsRegular() {
				return nil, fmt.Errorf("archive %s contains non-regular path %q", name, file.Name)
			}
			if file.UncompressedSize64 > 256<<20 {
				return nil, fmt.Errorf("archive %s path %q exceeds 256 MiB", name, file.Name)
			}
			clean, err := cleanArchivePath(file.Name)
			if err != nil {
				return nil, err
			}
			input, err := file.Open()
			if err != nil {
				return nil, err
			}
			content, readErr := io.ReadAll(io.LimitReader(input, 256<<20))
			closeErr := input.Close()
			if readErr != nil {
				return nil, readErr
			}
			if closeErr != nil {
				return nil, closeErr
			}
			if _, duplicate := files[clean]; duplicate {
				return nil, fmt.Errorf("archive %s contains duplicate path %q", name, clean)
			}
			files[clean] = content
		}
		return files, nil
	}
	if !strings.HasSuffix(name, ".tar.gz") {
		return nil, fmt.Errorf("unsupported release archive %s", name)
	}
	gzipReader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if header.Typeflag == tar.TypeDir || header.Typeflag == tar.TypeXGlobalHeader {
			continue
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != byte(0) {
			return nil, fmt.Errorf("archive %s contains non-regular path %q", name, header.Name)
		}
		if header.Size > 256<<20 {
			return nil, fmt.Errorf("archive %s path %q exceeds 256 MiB", name, header.Name)
		}
		clean, err := cleanArchivePath(header.Name)
		if err != nil {
			return nil, err
		}
		content, err := io.ReadAll(io.LimitReader(tarReader, 256<<20))
		if err != nil {
			return nil, err
		}
		if _, duplicate := files[clean]; duplicate {
			return nil, fmt.Errorf("archive %s contains duplicate path %q", name, clean)
		}
		files[clean] = content
	}
	return files, nil
}

func cleanArchivePath(name string) (string, error) {
	clean := path.Clean(strings.TrimPrefix(name, "./"))
	if clean == "." || path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("unsafe archive path %q", name)
	}
	return clean, nil
}

func verifyPackageJSON(file string, manifest Manifest) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("read package metadata: %w", err)
	}
	var metadata struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &metadata); err != nil {
		return fmt.Errorf("decode package metadata: %w", err)
	}
	if metadata.Name != manifest.PackageName || metadata.Version != manifest.Version {
		return fmt.Errorf("package metadata is %s@%s, want %s@%s", metadata.Name, metadata.Version, manifest.PackageName, manifest.Version)
	}
	return nil
}
