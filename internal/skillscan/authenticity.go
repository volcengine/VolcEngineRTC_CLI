// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package skillscan

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Problem is one gate finding, anchored to a source location.
type Problem struct {
	SourceFile string
	Line       int
	Raw        string
	Reason     string
}

func (p Problem) String() string {
	return fmt.Sprintf("%s:%d: %s  (%q)", p.SourceFile, p.Line, p.Reason, p.Raw)
}

// Validate checks every harvested command against the cobra tree rooted at root.
// A command whose subcommand position holds a placeholder (e.g. `<command>`)
// cannot be statically resolved; it is returned in unverifiable (never silently
// dropped) so callers can assert it against an explicit allowlist.
func Validate(root *cobra.Command, cmds []Command) (problems, unverifiable []Problem) {
	for _, c := range cmds {
		if p, ok, unver := validateOne(root, c); !ok {
			if unver {
				unverifiable = append(unverifiable, p)
			} else {
				problems = append(problems, p)
			}
		}
	}
	return problems, unverifiable
}

func validateOne(root *cobra.Command, c Command) (Problem, bool, bool) {
	cur := root
	depth := 0
	idx := 0
	for idx < len(c.Args) {
		tok := c.Args[idx]
		if isFlagToken(tok) {
			break
		}
		if isPlaceholder(tok) {
			if depth == 0 {
				return Problem{c.SourceFile, c.Line, c.Raw, "placeholder in subcommand position; cannot statically verify"}, false, true
			}
			break // positional placeholder
		}
		child := findChild(cur, tok)
		if child == nil {
			if depth == 0 {
				return Problem{c.SourceFile, c.Line, c.Raw, fmt.Sprintf("unknown command %q", tok)}, false, false
			}
			break // positional argument to cur
		}
		cur = child
		depth++
		idx++
	}
	// Validate the remaining tokens as flags (skipping their values / positionals).
	for j := idx; j < len(c.Args); j++ {
		tok := c.Args[j]
		if tok == "--" {
			break // standard end-of-flags marker; remaining tokens are positional
		}
		if strings.HasPrefix(tok, "--") {
			name, _, hasEq := cutFlag(tok)
			if name == "help" {
				continue
			}
			f := lookupFlag(cur, name)
			if f == nil {
				return Problem{c.SourceFile, c.Line, c.Raw,
					fmt.Sprintf("unknown flag --%s on `%s`", name, cmdPath(cur))}, false, false
			}
			if !hasEq && f.Value.Type() != "bool" && j+1 < len(c.Args) && !isFlagToken(c.Args[j+1]) {
				j++ // consume the flag's value
			}
		} else if strings.HasPrefix(tok, "-") && len(tok) > 1 {
			sh := strings.TrimPrefix(tok, "-")
			if sh == "h" {
				continue
			}
			if cur.Flags().ShorthandLookup(sh[:1]) == nil && cur.InheritedFlags().ShorthandLookup(sh[:1]) == nil {
				return Problem{c.SourceFile, c.Line, c.Raw,
					fmt.Sprintf("unknown shorthand -%s on `%s`", sh[:1], cmdPath(cur))}, false, false
			}
		}
		// bare positional token → accepted
	}
	return Problem{}, true, false
}

func findChild(cur *cobra.Command, name string) *cobra.Command {
	for _, c := range cur.Commands() {
		if c.Name() == name || c.HasAlias(name) {
			return c
		}
	}
	return nil
}

// lookupFlag resolves a long flag on cur, including inherited persistent flags
// (e.g. the root's --format / --dry-run).
func lookupFlag(cur *cobra.Command, name string) *pflag.Flag {
	if f := cur.Flags().Lookup(name); f != nil {
		return f
	}
	if f := cur.InheritedFlags().Lookup(name); f != nil {
		return f
	}
	if f := cur.PersistentFlags().Lookup(name); f != nil {
		return f
	}
	return nil
}

func cmdPath(cur *cobra.Command) string { return cur.CommandPath() }

func isFlagToken(tok string) bool { return strings.HasPrefix(tok, "-") && tok != "-" }

// cutFlag splits "--name" or "--name=value" → (name, value, hasEq).
func cutFlag(tok string) (name, value string, hasEq bool) {
	body := strings.TrimPrefix(tok, "--")
	if i := strings.IndexByte(body, '='); i >= 0 {
		return body[:i], body[i+1:], true
	}
	return body, "", false
}

// isPlaceholder reports whether a token is a doc placeholder / shell var rather
// than a literal subcommand or flag.
func isPlaceholder(tok string) bool {
	t := strings.Trim(tok, `"'`)
	switch {
	case t == "…" || t == "...":
		return true
	case strings.HasPrefix(t, "<") && strings.HasSuffix(t, ">"):
		return true
	case strings.Contains(t, "$"): // $VAR or ${VAR}
		return true
	default:
		return false
	}
}
