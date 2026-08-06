// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

// Package releasecontract implements the version and Skill invariants shared by
// stable, prerelease, and snapshot release entry points.
package releasecontract

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/skillscan"
)

// Stability identifies the version policy independently from publication.
type Stability string

const (
	StabilityStable     Stability = "stable"
	StabilityPrerelease Stability = "prerelease"
	StabilitySnapshot   Stability = "snapshot"
)

// Destination identifies where a release may be published.
type Destination string

const (
	DestinationPublic Destination = "public"
	DestinationNone   Destination = "none"
)

var (
	stableVersion        = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
	prereleaseIdentifier = regexp.MustCompile(`^[0-9A-Za-z-]+$`)
)

// Identity is the one resolved version used by all phases of a release.
type Identity struct {
	Stability   Stability   `json:"stability"`
	Destination Destination `json:"destination"`
	Version     string      `json:"version"`
	Baseline    string      `json:"baseline"`
}

// Skill is one official Skill discovered from a direct child of skills/.
type Skill struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Path    string `json:"path"`
}

// Target names one platform build and its archive postcondition.
type Target struct {
	GOOS    string `json:"goos"`
	GOARCH  string `json:"goarch"`
	Binary  string `json:"binary"`
	Archive string `json:"archive"`
}

// Manifest records the immutable inputs that postflight must prove.
type Manifest struct {
	Identity
	Source       string   `json:"source"`
	SourceRoot   string   `json:"source_root"`
	SourceCommit string   `json:"source_commit,omitempty"`
	SourceDate   string   `json:"source_date,omitempty"`
	Skills       []Skill  `json:"skills"`
	Targets      []Target `json:"targets"`
	PackageName  string   `json:"package_name"`
	ChecksumFile string   `json:"checksum_file"`
}

// Resolve derives the canonical release identity from stability, destination,
// requested version, and the checked-in stable Skill baseline.
func Resolve(stability Stability, destination Destination, requested, baseline string) (Identity, error) {
	baseline = strings.TrimSpace(strings.TrimPrefix(baseline, "v"))
	if !stableVersion.MatchString(baseline) {
		return Identity{}, fmt.Errorf("Skill baseline %q must be stable X.Y.Z SemVer", baseline)
	}
	requested = strings.TrimSpace(strings.TrimPrefix(requested, "v"))
	identity := Identity{Stability: stability, Destination: destination, Baseline: baseline}
	switch destination {
	case DestinationPublic, DestinationNone:
	default:
		return Identity{}, fmt.Errorf("unknown publication destination %q", destination)
	}
	switch stability {
	case StabilityStable:
		if destination != DestinationPublic {
			return Identity{}, fmt.Errorf("stable stability is incompatible with %q destination", destination)
		}
		if !stableVersion.MatchString(requested) {
			return Identity{}, fmt.Errorf("public stable version %q must be a vX.Y.Z tag or X.Y.Z", requested)
		}
		if requested != baseline {
			return Identity{}, fmt.Errorf("public stable version %q does not match Skill baseline %q", requested, baseline)
		}
		identity.Version = requested
	case StabilityPrerelease:
		if destination != DestinationPublic && destination != DestinationNone {
			return Identity{}, fmt.Errorf("prerelease stability is incompatible with %q destination", destination)
		}
		if !validPrereleaseVersion(requested) {
			return Identity{}, fmt.Errorf("prerelease version %q must be valid X.Y.Z-prerelease SemVer", requested)
		}
		if coreVersion(requested) != baseline {
			return Identity{}, fmt.Errorf("prerelease version %q is incompatible with Skill baseline %q", requested, baseline)
		}
		identity.Version = requested
	case StabilitySnapshot:
		if destination != DestinationNone {
			return Identity{}, fmt.Errorf("snapshot stability is incompatible with %q destination", destination)
		}
		if requested != "" {
			return Identity{}, errors.New("snapshot version is derived from the Skill baseline and must not be supplied")
		}
		parts := stableVersion.FindStringSubmatch(baseline)
		patch, err := strconv.ParseUint(parts[3], 10, 64)
		if err != nil || patch == ^uint64(0) {
			return Identity{}, fmt.Errorf("cannot increment snapshot baseline %q", baseline)
		}
		identity.Version = fmt.Sprintf("%s.%s.%d-snapshot", parts[1], parts[2], patch+1)
	default:
		return Identity{}, fmt.Errorf("unknown release stability %q", stability)
	}
	return identity, nil
}

func validPrereleaseVersion(version string) bool {
	separator := strings.IndexByte(version, '-')
	if separator < 0 || !stableVersion.MatchString(version[:separator]) {
		return false
	}
	for _, identifier := range strings.Split(version[separator+1:], ".") {
		if !prereleaseIdentifier.MatchString(identifier) {
			return false
		}
		if len(identifier) > 1 && identifier[0] == '0' {
			allDigits := true
			for _, char := range identifier {
				if char < '0' || char > '9' {
					allDigits = false
					break
				}
			}
			if allDigits {
				return false
			}
		}
	}
	return true
}

func coreVersion(version string) string {
	if index := strings.IndexByte(version, '-'); index >= 0 {
		return version[:index]
	}
	return version
}

// DiscoverSkills validates and returns every official Skill under root/skills.
// All Skills must share one stable checked-in version.
func DiscoverSkills(root string) ([]Skill, string, error) {
	skillsDir := filepath.Join(root, "skills")
	problems, err := skillscan.CheckSkills(skillsDir)
	if err != nil {
		return nil, "", fmt.Errorf("validate official Skills: %w", err)
	}
	if len(problems) > 0 {
		messages := make([]string, 0, len(problems))
		for _, problem := range problems {
			messages = append(messages, fmt.Sprintf("%s: %s", problem.SourceFile, problem.Reason))
		}
		return nil, "", fmt.Errorf("official Skill validation failed: %s", strings.Join(messages, "; "))
	}

	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil, "", fmt.Errorf("read official Skills: %w", err)
	}
	var skills []Skill
	seen := map[string]string{}
	baseline := ""
	for _, entry := range entries {
		entryPath := filepath.Join(skillsDir, entry.Name())
		info, err := os.Lstat(entryPath)
		if err != nil {
			return nil, "", fmt.Errorf("inspect Skill %q: %w", entry.Name(), err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, "", fmt.Errorf("Skill path %q must not be a symlink", entry.Name())
		}
		if !entry.IsDir() {
			continue
		}
		main := filepath.Join(entryPath, "SKILL.md")
		mainInfo, err := os.Lstat(main)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil || !mainInfo.Mode().IsRegular() {
			return nil, "", fmt.Errorf("Skill %q SKILL.md must be a regular file", entry.Name())
		}
		data, err := os.ReadFile(main)
		if err != nil {
			return nil, "", fmt.Errorf("read Skill %q: %w", entry.Name(), err)
		}
		fm, err := parseFrontmatter(data)
		if err != nil {
			return nil, "", fmt.Errorf("parse Skill %q frontmatter: %w", entry.Name(), err)
		}
		if previous, ok := seen[fm.Name]; ok {
			return nil, "", fmt.Errorf("duplicate Skill name %q in %s and %s", fm.Name, previous, main)
		}
		seen[fm.Name] = main
		if !stableVersion.MatchString(fm.Version) {
			return nil, "", fmt.Errorf("Skill %q checked-in version %q must be stable X.Y.Z SemVer", fm.Name, fm.Version)
		}
		if baseline == "" {
			baseline = fm.Version
		} else if fm.Version != baseline {
			return nil, "", fmt.Errorf("Skill %q version %q does not match baseline %q", fm.Name, fm.Version, baseline)
		}
		skills = append(skills, Skill{Name: fm.Name, Version: fm.Version, Path: filepath.ToSlash(filepath.Join("skills", entry.Name(), "SKILL.md"))})
	}
	if len(skills) == 0 {
		return nil, "", errors.New("no official Skills containing SKILL.md were found")
	}
	sort.Slice(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name })
	return skills, baseline, nil
}

type frontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Version     string `yaml:"version"`
}

func parseFrontmatter(data []byte) (frontmatter, error) {
	block, _, _, err := frontmatterBlock(data)
	if err != nil {
		return frontmatter{}, err
	}
	var fm frontmatter
	if err := yaml.Unmarshal(block, &fm); err != nil {
		return frontmatter{}, err
	}
	return fm, nil
}

func frontmatterBlock(data []byte) (block []byte, start, end int, err error) {
	if !bytes.HasPrefix(data, []byte("---\n")) && !bytes.HasPrefix(data, []byte("---\r\n")) {
		return nil, 0, 0, errors.New("missing opening frontmatter delimiter")
	}
	firstNewline := bytes.IndexByte(data, '\n')
	if firstNewline < 0 {
		return nil, 0, 0, errors.New("unterminated frontmatter")
	}
	contentStart := firstNewline + 1
	remaining := data[contentStart:]
	for offset := 0; offset <= len(remaining); {
		next := bytes.IndexByte(remaining[offset:], '\n')
		lineEnd := len(remaining)
		if next >= 0 {
			lineEnd = offset + next
		}
		line := bytes.TrimSuffix(remaining[offset:lineEnd], []byte("\r"))
		if bytes.Equal(line, []byte("---")) {
			return remaining[:offset], contentStart, contentStart + lineEnd, nil
		}
		if next < 0 {
			break
		}
		offset = lineEnd + 1
	}
	return nil, 0, 0, errors.New("missing closing frontmatter delimiter")
}

// StampSkills replaces exactly the frontmatter version line in every manifest
// Skill while preserving all other Markdown bytes.
func StampSkills(root, version string, skills []Skill) error {
	if strings.TrimSpace(version) == "" {
		return errors.New("stamp version is empty")
	}
	for _, skill := range skills {
		path := filepath.Join(root, filepath.FromSlash(skill.Path))
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s for stamping: %w", skill.Name, err)
		}
		block, start, _, err := frontmatterBlock(data)
		if err != nil {
			return fmt.Errorf("parse %s for stamping: %w", skill.Name, err)
		}
		lines := bytes.Split(block, []byte("\n"))
		found := 0
		for index, line := range lines {
			trimmed := bytes.TrimSpace(bytes.TrimSuffix(line, []byte("\r")))
			if bytes.HasPrefix(trimmed, []byte("version:")) {
				found++
				indent := line[:len(line)-len(bytes.TrimLeft(line, " \t"))]
				ending := []byte{}
				if bytes.HasSuffix(line, []byte("\r")) {
					ending = []byte("\r")
				}
				lines[index] = append(append(append([]byte{}, indent...), []byte("version: \"")...), append([]byte(version), append([]byte("\""), ending...)...)...)
			}
		}
		if found != 1 {
			return fmt.Errorf("Skill %q must contain exactly one frontmatter version field, found %d", skill.Name, found)
		}
		updatedBlock := bytes.Join(lines, []byte("\n"))
		updated := append([]byte{}, data[:start]...)
		updated = append(updated, updatedBlock...)
		updated = append(updated, data[start+len(block):]...)
		if err := os.WriteFile(path, updated, 0o644); err != nil {
			return fmt.Errorf("stamp Skill %q: %w", skill.Name, err)
		}
		fm, err := parseFrontmatter(updated)
		if err != nil || fm.Version != version {
			return fmt.Errorf("validate stamped Skill %q version: got %q: %v", skill.Name, fm.Version, err)
		}
	}
	return nil
}

// WriteManifest atomically writes a release manifest.
func WriteManifest(path string, manifest Manifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// ReadManifest reads a previously emitted release manifest.
func ReadManifest(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, err
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, err
	}
	if manifest.Version == "" || len(manifest.Skills) == 0 || len(manifest.Targets) == 0 || manifest.SourceRoot == "" || manifest.PackageName == "" || manifest.ChecksumFile == "" {
		return Manifest{}, errors.New("release manifest is incomplete")
	}
	return manifest, nil
}
