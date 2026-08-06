// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package skillscan_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/volcengine/VolcEngineRTC_CLI/cmd"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/skillscan"
)

// moduleRoot returns the repo root (two levels up from internal/skillscan).
func moduleRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(filepath.Join(wd, "..", ".."))
}

// allowedUnverifiable is the explicit, documented allowlist of harvested commands
// that cannot be statically resolved because a placeholder stands in the
// subcommand position. Each entry is a deliberate GENERIC reference, not a real
// invocation, and is reported (never silently skipped). A new entry MUST carry a
// reason here in review.
var allowedUnverifiable = map[string]bool{
	// Generic "run --help for any subcommand" guidance in vertc-shared.
	"vertc <command> --help": true,
	// Generic "any vertc command" reference in the skill-template prose.
	"vertc …": true,
}

// TestOfficialSkillCommandsAreAuthentic harvests every `vertc …` command from the
// official Skills and the template and validates it against the live command tree.
func TestOfficialSkillCommandsAreAuthentic(t *testing.T) {
	root := cmd.NewRootCmd()
	var all []skillscan.Command
	for _, dir := range []string{"skills", "skill-template"} {
		cmds, err := skillscan.HarvestDir(filepath.Join(moduleRoot(t), dir))
		if err != nil {
			t.Fatalf("harvest %s: %v", dir, err)
		}
		all = append(all, cmds...)
	}
	if len(all) == 0 {
		t.Fatal("harvested zero commands — the harvester or content is wrong")
	}
	problems, unverifiable := skillscan.Validate(root, all)
	for _, p := range problems {
		t.Errorf("command authenticity: %s", p)
	}
	for _, u := range unverifiable {
		if !allowedUnverifiable[u.Raw] {
			t.Errorf("unverifiable command not in allowlist: %s", u)
		}
	}
}

// TestOfficialSkillsPassMetadataAndSafetyChecks runs the metadata/link/secret/
// banned-command gate over the official Skills and the template.
func TestOfficialSkillsPassMetadataAndSafetyChecks(t *testing.T) {
	root := moduleRoot(t)
	problems, err := skillscan.CheckSkills(filepath.Join(root, "skills"))
	if err != nil {
		t.Fatal(err)
	}
	tpl, err := skillscan.CheckContentFiles(filepath.Join(root, "skill-template"), filepath.Join(root, "skills"))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range append(problems, tpl...) {
		t.Errorf("skill check: %s", p)
	}
}
