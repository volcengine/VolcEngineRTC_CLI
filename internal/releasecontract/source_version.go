// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

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
)

var packageVersionField = regexp.MustCompile(`("version"\s*:\s*)"[^"]*"`)

// SourceVersionChange is one tracked release metadata file that would change.
type SourceVersionChange struct {
	Path string
	Mode os.FileMode
	Old  []byte
	New  []byte
}

// SourceVersionPlan is a fully rendered, validated source-version transaction.
type SourceVersionPlan struct {
	Root    string
	Version string
	Targets []string
	Changes []SourceVersionChange
}

// PlanSourceVersion renders every package and Skill metadata target without
// writing. The returned plan can be checked, previewed, or atomically applied.
func PlanSourceVersion(root, requested string) (SourceVersionPlan, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return SourceVersionPlan{}, err
	}
	version := strings.TrimSpace(strings.TrimPrefix(requested, "v"))
	if !validReleasedVersion(version) {
		return SourceVersionPlan{}, fmt.Errorf("release source version %q must be X.Y.Z or X.Y.Z-ID and cannot be 0.0.0-dev", version)
	}
	skills, err := DiscoverSkills(root)
	if err != nil {
		return SourceVersionPlan{}, err
	}

	plan := SourceVersionPlan{Root: root, Version: version}
	packagePath := filepath.Join(root, "package.json")
	if err := appendSourceVersionChange(&plan, packagePath, func(data []byte) ([]byte, error) {
		return renderPackageVersion(data, version)
	}); err != nil {
		return SourceVersionPlan{}, err
	}
	for _, skill := range skills {
		skill := skill
		path := filepath.Join(root, filepath.FromSlash(skill.Path))
		if err := appendSourceVersionChange(&plan, path, func(data []byte) ([]byte, error) {
			return renderSkillVersion(data, skill.Name, version)
		}); err != nil {
			return SourceVersionPlan{}, err
		}
	}
	sort.Strings(plan.Targets)
	sort.Slice(plan.Changes, func(i, j int) bool { return plan.Changes[i].Path < plan.Changes[j].Path })
	return plan, nil
}

func appendSourceVersionChange(plan *SourceVersionPlan, path string, render func([]byte) ([]byte, error)) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	updated, err := render(data)
	if err != nil {
		return fmt.Errorf("prepare %s: %w", filepath.Base(path), err)
	}
	rel, err := filepath.Rel(plan.Root, path)
	if err != nil {
		return err
	}
	rel = filepath.ToSlash(rel)
	plan.Targets = append(plan.Targets, rel)
	if !bytes.Equal(data, updated) {
		plan.Changes = append(plan.Changes, SourceVersionChange{Path: rel, Mode: info.Mode().Perm(), Old: data, New: updated})
	}
	return nil
}

func renderPackageVersion(data []byte, version string) ([]byte, error) {
	var metadata struct {
		Version *string `json:"version"`
	}
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("parse package metadata: %w", err)
	}
	if metadata.Version == nil {
		return nil, errors.New("package metadata has no version field")
	}
	matches := packageVersionField.FindAllSubmatchIndex(data, -1)
	if len(matches) != 1 {
		return nil, fmt.Errorf("package metadata must contain exactly one version field, found %d", len(matches))
	}
	match := matches[0]
	updated := append([]byte{}, data[:match[2]]...)
	updated = append(updated, data[match[2]:match[3]]...)
	updated = append(updated, '"')
	updated = append(updated, version...)
	updated = append(updated, '"')
	updated = append(updated, data[match[1]:]...)
	var check struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(updated, &check); err != nil || check.Version != version {
		return nil, fmt.Errorf("validate prepared package version: got %q: %v", check.Version, err)
	}
	return updated, nil
}

func renderSkillVersion(data []byte, name, version string) ([]byte, error) {
	block, start, _, err := frontmatterBlock(data)
	if err != nil {
		return nil, err
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
			replacement := append([]byte{}, indent...)
			replacement = append(replacement, []byte("version: \"")...)
			replacement = append(replacement, version...)
			replacement = append(replacement, '"')
			replacement = append(replacement, ending...)
			lines[index] = replacement
		}
	}
	if found != 1 {
		return nil, fmt.Errorf("Skill %q must contain exactly one frontmatter version field, found %d", name, found)
	}
	updatedBlock := bytes.Join(lines, []byte("\n"))
	updated := append([]byte{}, data[:start]...)
	updated = append(updated, updatedBlock...)
	updated = append(updated, data[start+len(block):]...)
	fm, err := parseFrontmatter(updated)
	if err != nil || fm.Name != name || fm.Version != version {
		return nil, fmt.Errorf("validate prepared Skill %q version: got %q/%q: %v", name, fm.Name, fm.Version, err)
	}
	return updated, nil
}

// ApplySourceVersion applies a rendered plan as one rollback-capable file set.
func ApplySourceVersion(plan SourceVersionPlan) error {
	return applySourceVersion(plan, os.Rename)
}

func applySourceVersion(plan SourceVersionPlan, rename func(string, string) error) error {
	type stagedFile struct {
		change SourceVersionChange
		temp   string
	}
	staged := make([]stagedFile, 0, len(plan.Changes))
	cleanup := func() {
		for _, file := range staged {
			if file.temp != "" {
				_ = os.Remove(file.temp)
			}
		}
	}
	defer cleanup()
	for _, change := range plan.Changes {
		target := filepath.Join(plan.Root, filepath.FromSlash(change.Path))
		file, err := os.CreateTemp(filepath.Dir(target), ".release-version-*")
		if err != nil {
			return err
		}
		temp := file.Name()
		if _, err = file.Write(change.New); err == nil {
			err = file.Chmod(change.Mode)
		}
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			_ = os.Remove(temp)
			return err
		}
		staged = append(staged, stagedFile{change: change, temp: temp})
	}

	applied := 0
	for index, file := range staged {
		target := filepath.Join(plan.Root, filepath.FromSlash(file.change.Path))
		if err := rename(file.temp, target); err != nil {
			for rollback := applied - 1; rollback >= 0; rollback-- {
				prior := staged[rollback].change
				priorTarget := filepath.Join(plan.Root, filepath.FromSlash(prior.Path))
				_ = os.WriteFile(priorTarget, prior.Old, prior.Mode)
			}
			return fmt.Errorf("replace %s after %d updates: %w", file.change.Path, applied, err)
		}
		staged[index].temp = ""
		applied++
	}
	return nil
}
