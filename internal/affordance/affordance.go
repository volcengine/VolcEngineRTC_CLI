// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

// Package affordance attaches machine- and human-readable usage guidance to
// each command: when to use it, when to avoid it, prerequisites, and examples.
//
// Affordances are surfaced in `--help` so both humans and Agents can
// judge *when* to call a command, not just how. Agent scene workflows reference
// the same data when choosing and ordering commands.
package affordance

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// Affordance is the usage guidance for a single command.
type Affordance struct {
	// When lists situations where this command is the right choice.
	When []string
	// Avoid lists situations where it should not be used.
	Avoid []string
	// Prereq lists prerequisites that must hold before running.
	Prereq []string
	// Examples are concrete invocations.
	Examples []string
}

// annotationKey namespaces the affordance stored on cobra command annotations.
const annotationKey = "vertc.affordance"

// Attach wires an affordance onto a cobra command: it registers a structured
// annotation (for the `affordance` introspection command) and appends a
// rendered "AFFORDANCE" section to the command's help template.
func Attach(cmd *cobra.Command, a Affordance) {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[annotationKey] = a.encode()

	orig := cmd.HelpFunc()
	cmd.SetHelpFunc(func(c *cobra.Command, args []string) {
		orig(c, args)
		if c == cmd {
			fmt.Fprint(c.OutOrStdout(), a.Render())
		}
	})
}

// Get returns the affordance attached to a command, if any.
func Get(cmd *cobra.Command) (Affordance, bool) {
	if cmd.Annotations == nil {
		return Affordance{}, false
	}
	raw, ok := cmd.Annotations[annotationKey]
	if !ok {
		return Affordance{}, false
	}
	return decode(raw), true
}

// Render produces the human-readable "AFFORDANCE" help section.
func (a Affordance) Render() string {
	var b strings.Builder
	b.WriteString("\nAffordance:\n")
	section := func(title string, items []string) {
		if len(items) == 0 {
			return
		}
		fmt.Fprintf(&b, "  %s:\n", title)
		for _, it := range items {
			fmt.Fprintf(&b, "    - %s\n", it)
		}
	}
	section("When to use", a.When)
	section("Avoid when", a.Avoid)
	section("Prerequisites", a.Prereq)
	section("Examples", a.Examples)
	return b.String()
}

// --- annotation (de)serialization: a compact line-based encoding ---

func (a Affordance) encode() string {
	var b strings.Builder
	enc := func(tag string, items []string) {
		for _, it := range items {
			fmt.Fprintf(&b, "%s\t%s\n", tag, it)
		}
	}
	enc("when", a.When)
	enc("avoid", a.Avoid)
	enc("prereq", a.Prereq)
	enc("example", a.Examples)
	return b.String()
}

func decode(raw string) Affordance {
	var a Affordance
	for _, line := range strings.Split(raw, "\n") {
		tag, val, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		switch tag {
		case "when":
			a.When = append(a.When, val)
		case "avoid":
			a.Avoid = append(a.Avoid, val)
		case "prereq":
			a.Prereq = append(a.Prereq, val)
		case "example":
			a.Examples = append(a.Examples, val)
		}
	}
	return a
}
