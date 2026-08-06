// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

// Package skillscan is the Skill quality gate. It harvests every `vertc …`
// invocation from Skill/template markdown (fenced blocks and inline code spans),
// then validates each against the live cobra command tree so a Skill can never
// teach a command/subcommand/flag the binary does not have. It also runs
// metadata/link/secret/banned-command checks. Everything is a plain Go check so
// the command tree itself is the source of truth (design: reuse NewRootCmd()).
package skillscan

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Command is one harvested `vertc …` invocation (Args excludes the leading verb).
type Command struct {
	Raw        string
	SourceFile string
	Line       int
	Args       []string
}

var (
	// inlineVertc matches an inline-code span whose content is a vertc command,
	// optionally prefixed by ENV=val assignments (e.g. `VERTC_NO_SKILLS_NOTICE=1 vertc …`).
	inlineVertc = regexp.MustCompile("`((?:[A-Z_][A-Z0-9_]*=[^`\\s]+\\s+)*vertc\\s+[^`]*)`")
	envAssign   = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*=[^\s]+$`)
)

// HarvestDir walks root for *.md and returns every harvested vertc command.
func HarvestDir(root string) ([]Command, error) {
	var out []Command
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		cmds, err := harvestFile(path)
		if err != nil {
			return err
		}
		out = append(out, cmds...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SourceFile != out[j].SourceFile {
			return out[i].SourceFile < out[j].SourceFile
		}
		return out[i].Line < out[j].Line
	})
	return out, nil
}

func harvestFile(path string) ([]Command, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	var out []Command
	inFence := false
	for i := 0; i < len(lines); i++ {
		raw := strings.TrimSpace(lines[i])
		if strings.HasPrefix(raw, "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			// A fenced command line: join `\`-continuations, strip ENV= prefixes.
			if args, ok := fencedCommand(lines, &i); ok {
				out = append(out, Command{Raw: strings.Join(append([]string{"vertc"}, args...), " "),
					SourceFile: path, Line: i + 1, Args: args})
			}
			continue
		}
		// Outside fences: harvest inline-code vertc spans.
		for _, m := range inlineVertc.FindAllStringSubmatch(lines[i], -1) {
			if args, ok := parseVertc(m[1]); ok {
				out = append(out, Command{Raw: "vertc " + strings.Join(args, " "),
					SourceFile: path, Line: i + 1, Args: args})
			}
		}
	}
	return out, nil
}

// fencedCommand joins a possibly `\`-continued command starting at *i and returns
// its args if it is a vertc command.
func fencedCommand(lines []string, i *int) ([]string, bool) {
	line := strings.TrimSpace(lines[*i])
	joined := trimContinuation(line)
	for continues(line) && *i+1 < len(lines) {
		*i++
		line = strings.TrimSpace(lines[*i])
		joined += " " + trimContinuation(line)
	}
	return parseVertc(joined)
}

// parseVertc strips leading ENV=val assignments and a trailing `# …` shell
// comment, and, if the command is `vertc`, returns its remaining args (fields).
func parseVertc(s string) ([]string, bool) {
	fields := strings.Fields(s)
	for len(fields) > 0 && envAssign.MatchString(fields[0]) {
		fields = fields[1:]
	}
	if len(fields) == 0 || fields[0] != "vertc" {
		return nil, false
	}
	args := fields[1:]
	// Drop a trailing shell comment ("… # note").
	for i, f := range args {
		if f == "#" {
			args = args[:i]
			break
		}
	}
	return args, true
}

func continues(line string) bool { return strings.HasSuffix(strings.TrimRight(line, " \t"), `\`) }

func trimContinuation(line string) string {
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimRight(line, " \t"), `\`))
}
