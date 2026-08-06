// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package skillscan

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	// bannedCommands are legacy/obsolete patterns that must never appear in an
	// official Skill or the template (drift blacklist). Each has a replacement
	// documented in the Skills themselves.
	bannedCommands = []*regexp.Regexp{
		regexp.MustCompile(`env write[^\n]*--asr-access-token`),
		regexp.MustCompile(`config set\s+agent\.llm\.`),
		regexp.MustCompile(`config set\s+agent\.asr\.`),
		regexp.MustCompile(`config set\s+agent\.tts\.`),
	}
	// akKey matches Volc/AWS access-key literals (precise prefix → hard fail).
	akKey = regexp.MustCompile(`\bAK(?:LT|IA)[A-Za-z0-9/+]{12,}`)
	// base64Blob matches a long base64-ish run. It is guarded by hexOnly so a pure
	// hex string (git commit SHA, SHA-256 checksum, hex id) does NOT trip the gate.
	base64Blob = regexp.MustCompile(`\b[A-Za-z0-9+/]{40,}={0,2}\b`)
	// hexOnly recognizes a pure-hex run (a SHA / checksum, not a credential).
	hexOnly = regexp.MustCompile(`^[0-9a-fA-F]+$`)
	// referenceRef matches a references/<file>.md mention.
	referenceRef = regexp.MustCompile(`references/[A-Za-z0-9._-]+\.md`)
	skillName    = regexp.MustCompile(`^byted-[a-z0-9]+(?:-[a-z0-9]+)+$`)
	skillVersion = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)
)

// Frontmatter is a skill's parsed YAML frontmatter (subset).
type Frontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Version     string `yaml:"version"`
}

// CheckSkills validates every official skill under skillsDir against the public
// package contract: valid metadata, matching directory/name, a root LICENSE,
// resolvable reference links, no banned legacy commands, and no secret-shaped
// literals. Returns a sorted problem list.
func CheckSkills(skillsDir string) ([]Problem, error) {
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil, err
	}
	knownRefs := collectReferenceSet(skillsDir)
	var problems []Problem
	names := map[string]string{} // name -> skill dir

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		skillDir := filepath.Join(skillsDir, e.Name())
		main := filepath.Join(skillDir, "SKILL.md")
		data, err := os.ReadFile(main)
		if err != nil {
			continue // not a skill dir
		}
		fm := parseFrontmatter(data)
		rel := filepath.Join(skillsDir, e.Name(), "SKILL.md")
		if fm.Name == "" {
			problems = append(problems, Problem{rel, 1, "", "missing frontmatter name"})
		} else {
			if prev, ok := names[fm.Name]; ok {
				problems = append(problems, Problem{rel, 1, "", fmt.Sprintf("duplicate skill name %q (also %s)", fm.Name, prev)})
			}
			names[fm.Name] = rel
			if fm.Name != e.Name() {
				problems = append(problems, Problem{rel, 1, "", fmt.Sprintf("frontmatter name %q != directory %q", fm.Name, e.Name())})
			}
			if len(fm.Name) > 64 || !skillName.MatchString(fm.Name) {
				problems = append(problems, Problem{rel, 1, fm.Name, "frontmatter name must match byted-{product-code}-{skill-name}, be at most 64 characters, and use only lowercase letters, digits, and non-edge hyphens"})
			}
		}
		if strings.TrimSpace(fm.Description) == "" {
			problems = append(problems, Problem{rel, 1, "", "missing frontmatter description"})
		} else if len([]rune(fm.Description)) > 1024 {
			problems = append(problems, Problem{rel, 1, "", "frontmatter description exceeds 1024 characters"})
		}
		version := strings.TrimSpace(fm.Version)
		if version == "" {
			problems = append(problems, Problem{rel, 1, "", "missing frontmatter version"})
		} else if !validSkillVersion(version) {
			problems = append(problems, Problem{rel, 1, version, "frontmatter version must be valid SemVer without a leading v"})
		}
		licensePath := filepath.Join(skillDir, "LICENSE")
		licenseInfo, licenseErr := os.Stat(licensePath)
		if licenseErr != nil || !licenseInfo.Mode().IsRegular() || licenseInfo.Size() == 0 {
			problems = append(problems, Problem{filepath.Join(skillsDir, e.Name(), "LICENSE"), 1, "", "missing Skill root LICENSE or LICENSE is not a non-empty regular file"})
		}
		// Per-file content checks (SKILL.md + references).
		problems = append(problems, checkTree(skillDir, knownRefs)...)
	}
	sort.Slice(problems, func(i, j int) bool {
		if problems[i].SourceFile != problems[j].SourceFile {
			return problems[i].SourceFile < problems[j].SourceFile
		}
		return problems[i].Line < problems[j].Line
	})
	return problems, nil
}

func validSkillVersion(version string) bool {
	if !skillVersion.MatchString(version) {
		return false
	}
	withoutBuild := strings.SplitN(version, "+", 2)[0]
	parts := strings.SplitN(withoutBuild, "-", 2)
	if len(parts) == 1 {
		return true
	}
	for _, identifier := range strings.Split(parts[1], ".") {
		if len(identifier) <= 1 || identifier[0] != '0' {
			continue
		}
		numeric := true
		for _, r := range identifier {
			if r < '0' || r > '9' {
				numeric = false
				break
			}
		}
		if numeric {
			return false
		}
	}
	return true
}

// CheckContentFiles runs the banned-command + secret + reference-link checks over
// a single tree (e.g. skill-template/, which is not a skill dir). References are
// resolved against the official skills' reference set (skillsDir), since the
// template legitimately names the reference files an author will create.
func CheckContentFiles(dir, skillsDir string) ([]Problem, error) {
	if _, err := os.Stat(dir); err != nil {
		return nil, err
	}
	return checkTree(dir, collectReferenceSet(skillsDir)), nil
}

// collectReferenceSet returns the set of "references/<file>.md" relpaths that
// exist under any skill, so cross-skill (progressive-disclosure) references and
// template structure references resolve, while true typos are still caught.
func collectReferenceSet(skillsDir string) map[string]bool {
	set := map[string]bool{}
	_ = filepath.WalkDir(skillsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if m := referenceRef.FindString(filepath.ToSlash(path)); m != "" {
			set[m] = true
		}
		return nil
	})
	return set
}

func checkTree(dir string, knownRefs map[string]bool) []Problem {
	var problems []Problem
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		problems = append(problems, scanFile(path, knownRefs, data)...)
		return nil
	})
	return problems
}

// scanFile runs the banned-command + secret checks over the fenced code blocks of
// a markdown file — prose and inline-code references are exempt, so a "don't use
// `…`" note or a long commit hash in text never trips the gate — and validates
// every reference link found anywhere in the file.
func scanFile(path string, knownRefs map[string]bool, data []byte) []Problem {
	var problems []Problem
	lines := strings.Split(string(data), "\n")
	inFence := false
	for i, line := range lines {
		isFence := strings.HasPrefix(strings.TrimSpace(line), "```")
		if isFence {
			inFence = !inFence
		}
		if inFence && !isFence { // a code line inside a fenced block
			for _, re := range bannedCommands {
				if re.MatchString(line) {
					problems = append(problems, Problem{path, i + 1, strings.TrimSpace(line),
						"banned legacy command: " + re.String()})
				}
			}
			for _, m := range findSecrets(line) {
				problems = append(problems, Problem{path, i + 1, strings.TrimSpace(line),
					"possible secret literal: " + truncate(m)})
			}
		}
		// Reference links must resolve to a real reference file and never escape.
		for _, ref := range referenceRef.FindAllString(line, -1) {
			if strings.Contains(ref, "..") {
				problems = append(problems, Problem{path, i + 1, ref, "reference path escapes with '..'"})
				continue
			}
			if !knownRefs[ref] {
				problems = append(problems, Problem{path, i + 1, ref, "dangling reference link"})
			}
		}
	}
	return problems
}

// findSecrets returns credential-shaped substrings in a code line. AK-prefixed keys
// always count. A long base64 run counts only when it is not pure hex — a git SHA
// or checksum is hex and must never be flagged. A match enclosed in a structural
// placeholder (<APP_KEY>, $RTC_APP_KEY) is documentation and is skipped; that check
// is scoped to the surrounding token, NOT the whole line, so a real key sitting on
// an "example" line is still caught.
func findSecrets(code string) []string {
	var hits []string
	for _, loc := range akKey.FindAllStringIndex(code, -1) {
		if !enclosingPlaceholder(code, loc[0], loc[1]) {
			hits = append(hits, code[loc[0]:loc[1]])
		}
	}
	for _, loc := range base64Blob.FindAllStringIndex(code, -1) {
		m := code[loc[0]:loc[1]]
		if hexOnly.MatchString(strings.TrimRight(m, "=")) {
			continue // commit SHA / checksum / hex id, not a secret
		}
		if enclosingPlaceholder(code, loc[0], loc[1]) {
			continue
		}
		hits = append(hits, m)
	}
	return hits
}

// enclosingPlaceholder reports whether the match at [start,end) sits inside a
// structural placeholder token — wrapped in <…> or carrying a $VAR — rather than
// being a literal credential. Bounds are the surrounding non-space token only.
func enclosingPlaceholder(code string, start, end int) bool {
	lo := strings.LastIndexAny(code[:start], " \t\"'") + 1
	hi := len(code)
	if k := strings.IndexAny(code[end:], " \t\"'"); k >= 0 {
		hi = end + k
	}
	tok := code[lo:hi]
	if strings.Contains(tok, "$") {
		return true
	}
	return strings.HasPrefix(tok, "<") && strings.Contains(tok, ">")
}

func truncate(s string) string {
	if len(s) > 16 {
		return s[:16] + "…"
	}
	return s
}

func parseFrontmatter(data []byte) Frontmatter {
	s := string(data)
	if !strings.HasPrefix(s, "---") {
		return Frontmatter{}
	}
	lines := strings.Split(s, "\n")
	var block []string
	closed := false
	for _, ln := range lines[1:] {
		if strings.TrimRight(ln, "\r") == "---" {
			closed = true
			break
		}
		block = append(block, ln)
	}
	if !closed {
		return Frontmatter{}
	}
	var fm Frontmatter
	_ = yaml.Unmarshal([]byte(strings.Join(block, "\n")), &fm)
	return fm
}
