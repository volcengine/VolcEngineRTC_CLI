// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

// Package skillsfs reads binary-embedded official Skill content from an injected
// fs.FS rooted at the skill list (entries like "byted-interactai-guide/SKILL.md").
// It is offline and auth-free by construction. Content is byte-identical to the
// on-disk sources it was embedded from, and its version is bound to the CLI
// build (meta.Version) — embedded Skills always ship in lockstep with the binary.
//
// The path guards (reject "..", absolute, "..\") are adapted from the audited
// a small embedded content reader; the envelope/errors are vertc's own.
package skillsfs

import (
	"io/fs"
	"path"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/meta"
)

// Reader serves embedded skill content over an injected fs.FS.
type Reader struct {
	fsys fs.FS
}

// New returns a Reader over fsys (rooted at the skill list).
func New(fsys fs.FS) *Reader { return &Reader{fsys: fsys} }

// SkillInfo is one skill's list entry.
type SkillInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	// Version is the CLI build version the content shipped with (meta.Version).
	Version string `json:"version"`
	// References are the skill-prefixed reference paths (feed back into `read`).
	References []string `json:"references,omitempty"`
}

// DirEntry.Path is skill-prefixed (e.g. "byted-interactai-guide/references/x.md") so
// it can be fed straight back into `read`.
type DirEntry struct {
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
}

// List returns every embedded skill (a dir containing SKILL.md), sorted by name.
func (r *Reader) List() ([]SkillInfo, error) {
	entries, err := fs.ReadDir(r.fsys, ".")
	if err != nil {
		return nil, errs.New("vertc.skills.unavailable", errs.TypeInternal,
			"failed to read embedded skills: %s", err)
	}
	out := make([]SkillInfo, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if info, ok := r.skillInfo(e.Name()); ok {
			out = append(out, info)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (r *Reader) skillInfo(name string) (SkillInfo, bool) {
	data, err := fs.ReadFile(r.fsys, name+"/SKILL.md")
	if err != nil {
		return SkillInfo{}, false
	}
	desc := parseDescription(data)
	return SkillInfo{
		Name:        name,
		Description: desc,
		Version:     meta.Version,
		References:  r.referencePaths(name),
	}, true
}

// referencePaths returns the skill-prefixed paths of the skill's reference files
// (one layer under references/), best-effort (nil when there are none).
func (r *Reader) referencePaths(name string) []string {
	entries, err := fs.ReadDir(r.fsys, name+"/references")
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		out = append(out, name+"/references/"+e.Name())
	}
	sort.Strings(out)
	return out
}

// ListPath lists one directory layer (no recursion) under "<name>" or
// "<name>/<sub>", returning the entries and the cleaned path listed.
func (r *Reader) ListPath(arg string) ([]DirEntry, string, error) {
	name, sub := SplitArg(arg)
	if err := r.ensureSkill(name); err != nil {
		return nil, "", err
	}
	dir := name
	if sub != "" {
		cleaned, err := cleanSubPath(sub)
		if err != nil {
			return nil, "", err
		}
		dir = name + "/" + cleaned
		info, err := fs.Stat(r.fsys, dir)
		if err != nil {
			return nil, "", notFound(sub, name)
		}
		if !info.IsDir() {
			return nil, "", errs.New("vertc.skills.invalid_path", errs.TypeValidation,
				"path %q is a file, not a directory; use `%s skills read %s/%s`", sub, meta.BinName, name, cleaned)
		}
	}
	entries, err := fs.ReadDir(r.fsys, dir)
	if err != nil {
		return nil, "", errs.New("vertc.skills.unavailable", errs.TypeInternal,
			"failed to read embedded skill content: %s", err)
	}
	out := make([]DirEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, DirEntry{Path: dir + "/" + e.Name(), IsDir: e.IsDir()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, dir, nil
}

// SplitArg splits "<name>/<rest>" at the first separator; a bare name yields "".
func SplitArg(arg string) (name, rest string) {
	name, rest, _ = strings.Cut(arg, "/")
	return name, rest
}

// ReadSkill returns the bytes of <name>/SKILL.md.
func (r *Reader) ReadSkill(name string) ([]byte, error) {
	if err := r.ensureSkill(name); err != nil {
		return nil, err
	}
	data, err := fs.ReadFile(r.fsys, name+"/SKILL.md")
	if err != nil {
		return nil, errs.New("vertc.skills.unavailable", errs.TypeInternal,
			"failed to read embedded skill content: %s", err)
	}
	return data, nil
}

// ReadReference returns the bytes of <name>/<relpath> and the cleaned path.
func (r *Reader) ReadReference(name, relpath string) ([]byte, string, error) {
	if err := r.ensureSkill(name); err != nil {
		return nil, "", err
	}
	cleaned, err := cleanSubPath(relpath)
	if err != nil {
		return nil, "", err
	}
	full := name + "/" + cleaned
	info, err := fs.Stat(r.fsys, full)
	if err != nil {
		return nil, "", notFound(relpath, name)
	}
	if info.IsDir() {
		return nil, "", errs.New("vertc.skills.invalid_path", errs.TypeValidation,
			"reference %q is a directory, not a file", relpath)
	}
	data, err := fs.ReadFile(r.fsys, full)
	if err != nil {
		return nil, "", errs.New("vertc.skills.unavailable", errs.TypeInternal,
			"failed to read embedded skill content: %s", err)
	}
	return data, cleaned, nil
}

// Version returns the embedded-content version (bound to the binary build).
func (r *Reader) Version() string { return meta.Version }

func (r *Reader) ensureSkill(name string) error {
	if name == "" || strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
		return unknownSkill(name)
	}
	info, err := fs.Stat(r.fsys, name)
	if err != nil || !info.IsDir() {
		return unknownSkill(name)
	}
	// A directory without a SKILL.md is not an official skill.
	if _, err := fs.Stat(r.fsys, name+"/SKILL.md"); err != nil {
		return unknownSkill(name)
	}
	return nil
}

func unknownSkill(name string) error {
	return errs.New("vertc.skills.unknown_skill", errs.TypeNotFound,
		"unknown skill %q", name).WithParam(name).
		WithHint("run `%s skills list` to see available skills", meta.BinName)
}

func notFound(relpath, name string) error {
	return errs.New("vertc.skills.not_found", errs.TypeNotFound,
		"reference %q not found in skill %q", relpath, name).
		WithHint("run `%s skills list %s` to see files in this skill", meta.BinName, name)
}

// cleanSubPath returns the cleaned form of relpath, rejecting absolute paths and
// ".." escapes. relpath must be non-empty (callers handle the skill-root case).
func cleanSubPath(relpath string) (string, error) {
	// fs.FS uses '/' exclusively; fold any '\' to '/' before cleaning so a
	// Windows-style "..\" — anywhere in the path, not just as a leading prefix —
	// collapses to "../" and is caught below, rather than surviving path.Clean
	// (which treats '\' as an ordinary filename character).
	normalized := strings.ReplaceAll(relpath, `\`, "/")
	cleaned := path.Clean(normalized)
	if relpath == "" || path.IsAbs(normalized) ||
		cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", errs.New("vertc.skills.invalid_path", errs.TypeValidation,
			"invalid path %q: must be a relative path without '..'", relpath).WithParam(relpath).
			WithHint("read a listed reference, e.g. `%s skills read <skill> references/<file>`", meta.BinName)
	}
	return cleaned, nil
}

// parseDescription best-effort-extracts the frontmatter `description`; missing or
// unparseable frontmatter yields "".
func parseDescription(skillMD []byte) string {
	lines := strings.Split(string(skillMD), "\n")
	if len(lines) == 0 || strings.TrimRight(lines[0], "\r") != "---" {
		return ""
	}
	block := make([]string, 0, len(lines))
	closed := false
	for _, ln := range lines[1:] {
		if strings.TrimRight(ln, "\r") == "---" {
			closed = true
			break
		}
		block = append(block, ln)
	}
	if !closed {
		return ""
	}
	var fm struct {
		Description string `yaml:"description"`
	}
	if err := yaml.Unmarshal([]byte(strings.Join(block, "\n")), &fm); err != nil {
		return ""
	}
	return fm.Description
}
