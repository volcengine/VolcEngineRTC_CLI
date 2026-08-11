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

	syntheticDevVersion = "0.0.0-dev"
	snapshotVersion     = "0.0.0-snapshot"
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
// and the requested version.
func Resolve(stability Stability, destination Destination, requested string) (Identity, error) {
	requested = strings.TrimSpace(strings.TrimPrefix(requested, "v"))
	identity := Identity{Stability: stability, Destination: destination}
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
		identity.Version = requested
	case StabilityPrerelease:
		if destination != DestinationPublic && destination != DestinationNone {
			return Identity{}, fmt.Errorf("prerelease stability is incompatible with %q destination", destination)
		}
		if !validPrereleaseVersion(requested) {
			return Identity{}, fmt.Errorf("prerelease version %q must be valid X.Y.Z-prerelease SemVer", requested)
		}
		if requested == syntheticDevVersion || requested == snapshotVersion {
			return Identity{}, fmt.Errorf("prerelease version %q is reserved for non-release use", requested)
		}
		identity.Version = requested
	case StabilitySnapshot:
		if destination != DestinationNone {
			return Identity{}, fmt.Errorf("snapshot stability is incompatible with %q destination", destination)
		}
		if requested != "" {
			return Identity{}, errors.New("snapshot version is fixed by the release contract and must not be supplied")
		}
		identity.Version = snapshotVersion
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

func validReleasedVersion(version string) bool {
	if version == syntheticDevVersion || version == snapshotVersion {
		return false
	}
	return stableVersion.MatchString(version) || validPrereleaseVersion(version)
}

// DiscoverSkills validates and returns every official Skill under root/skills.
// All Skills must share one real checked-in stable or prerelease version.
func DiscoverSkills(root string) ([]Skill, error) {
	skillsDir := filepath.Join(root, "skills")
	problems, err := skillscan.CheckSkills(skillsDir)
	if err != nil {
		return nil, fmt.Errorf("validate official Skills: %w", err)
	}
	if len(problems) > 0 {
		messages := make([]string, 0, len(problems))
		for _, problem := range problems {
			messages = append(messages, fmt.Sprintf("%s: %s", problem.SourceFile, problem.Reason))
		}
		return nil, fmt.Errorf("official Skill validation failed: %s", strings.Join(messages, "; "))
	}

	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil, fmt.Errorf("read official Skills: %w", err)
	}
	var skills []Skill
	seen := map[string]string{}
	baseline := ""
	for _, entry := range entries {
		entryPath := filepath.Join(skillsDir, entry.Name())
		info, err := os.Lstat(entryPath)
		if err != nil {
			return nil, fmt.Errorf("inspect Skill %q: %w", entry.Name(), err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("Skill path %q must not be a symlink", entry.Name())
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
			return nil, fmt.Errorf("Skill %q SKILL.md must be a regular file", entry.Name())
		}
		data, err := os.ReadFile(main)
		if err != nil {
			return nil, fmt.Errorf("read Skill %q: %w", entry.Name(), err)
		}
		fm, err := parseFrontmatter(data)
		if err != nil {
			return nil, fmt.Errorf("parse Skill %q frontmatter: %w", entry.Name(), err)
		}
		if previous, ok := seen[fm.Name]; ok {
			return nil, fmt.Errorf("duplicate Skill name %q in %s and %s", fm.Name, previous, main)
		}
		seen[fm.Name] = main
		if !validReleasedVersion(fm.Version) {
			return nil, fmt.Errorf("Skill %q checked-in version %q must be released X.Y.Z or X.Y.Z-ID SemVer", fm.Name, fm.Version)
		}
		if baseline == "" {
			baseline = fm.Version
		} else if fm.Version != baseline {
			return nil, fmt.Errorf("Skill %q version %q does not match baseline %q", fm.Name, fm.Version, baseline)
		}
		skills = append(skills, Skill{Name: fm.Name, Version: fm.Version, Path: filepath.ToSlash(filepath.Join("skills", entry.Name(), "SKILL.md"))})
	}
	if len(skills) == 0 {
		return nil, errors.New("no official Skills containing SKILL.md were found")
	}
	sort.Slice(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name })
	return skills, nil
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
		updated, err := renderSkillVersion(data, skill.Name, version)
		if err != nil {
			return fmt.Errorf("parse %s for stamping: %w", skill.Name, err)
		}
		if err := os.WriteFile(path, updated, 0o644); err != nil {
			return fmt.Errorf("stamp Skill %q: %w", skill.Name, err)
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
