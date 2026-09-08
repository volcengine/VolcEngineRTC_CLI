// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package skillscan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// --- synthetic unit tests (no repo content) ---

func testTree() *cobra.Command {
	root := &cobra.Command{Use: "vertc"}
	root.PersistentFlags().String("format", "json", "")
	root.PersistentFlags().Bool("dry-run", false, "")

	agent := &cobra.Command{Use: "agent"}
	start := &cobra.Command{Use: "start", Run: func(*cobra.Command, []string) {}}
	start.Flags().Bool("yes", false, "")
	agent.AddCommand(start)

	token := &cobra.Command{Use: "token"}
	issue := &cobra.Command{Use: "issue", Run: func(*cobra.Command, []string) {}}
	issue.Flags().Bool("write", false, "")
	issue.Flags().String("room-id", "", "")
	token.AddCommand(issue)

	root.AddCommand(agent, token)
	return root
}

func TestValidateAcceptsRealCommands(t *testing.T) {
	cmds := []Command{
		{Raw: "vertc agent start --yes", Args: []string{"agent", "start", "--yes"}},
		{Raw: "vertc agent start --dry-run", Args: []string{"agent", "start", "--dry-run"}},
		{Raw: "vertc token issue --room-id room-2 --write", Args: []string{"token", "issue", "--room-id", "room-2", "--write"}},
		{Raw: "vertc token issue --format json", Args: []string{"token", "issue", "--format", "json"}},
		{Raw: "vertc agent start --yes -- -value", Args: []string{"agent", "start", "--yes", "--", "-value"}},
	}
	problems, unver := Validate(testTree(), cmds)
	if len(problems) != 0 || len(unver) != 0 {
		t.Fatalf("expected clean, got problems=%v unver=%v", problems, unver)
	}
}

func TestValidateFlagsDrift(t *testing.T) {
	cmds := []Command{
		{Raw: "vertc env write --asr-access-token X", Args: []string{"env", "write", "--asr-access-token", "X"}},
		{Raw: "vertc config set agent.llm.endpoint_id X", Args: []string{"config", "set", "agent.llm.endpoint_id", "X"}},
		{Raw: "vertc token issue --bogus", Args: []string{"token", "issue", "--bogus"}},
	}
	problems, _ := Validate(testTree(), cmds)
	if len(problems) != 3 {
		t.Fatalf("expected 3 problems, got %d: %v", len(problems), problems)
	}
}

func TestValidatePlaceholderIsUnverifiable(t *testing.T) {
	cmds := []Command{{Raw: "vertc <command>", Args: []string{"<command>"}}}
	problems, unver := Validate(testTree(), cmds)
	if len(problems) != 0 || len(unver) != 1 {
		t.Fatalf("expected 1 unverifiable, got problems=%v unver=%v", problems, unver)
	}
}

func TestHarvestFencedInlineAndEnvPrefix(t *testing.T) {
	dir := t.TempDir()
	md := "# doc\n\n```bash\nVERTC_NO_SKILLS_NOTICE=1 vertc doctor --format json\nvertc token issue \\\n  --write\n```\n\nRun `vertc agent status` then `vertc doctor`.\n"
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
	cmds, err := HarvestDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	// doctor (env-prefixed), token issue (continuation), agent status, doctor.
	if len(cmds) != 4 {
		t.Fatalf("expected 4 harvested, got %d: %+v", len(cmds), cmds)
	}
	// The continuation must be joined into one command.
	var joined bool
	for _, c := range cmds {
		if c.Args[0] == "token" && contains(c.Args, "--write") {
			joined = true
		}
	}
	if !joined {
		t.Fatalf("continuation not joined: %+v", cmds)
	}
}

// reasons collects the Reason field of every problem for easy assertion.
func reasons(ps []Problem) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Reason
	}
	return out
}

func countReason(ps []Problem, substr string) int {
	n := 0
	for _, p := range ps {
		if strings.Contains(p.Reason, substr) {
			n++
		}
	}
	return n
}

// TestScanFileSecretAndBannedAreCodeScoped locks in the three review fixes:
//   - pure-hex hashes (git SHA-1 / SHA-256) are not secrets;
//   - a real base64 blob in a code block still is;
//   - banned commands and secrets are only scanned inside fenced code blocks, so a
//     prose "don't use …" note or a hash in text never trips the gate;
//   - placeholder suppression is scoped to the matched token — a real key on an
//     "example" line is caught, only a <…>/$VAR wrapper is exempt.
func TestScanFileSecretAndBannedAreCodeScoped(t *testing.T) {
	md := strings.Join([]string{
		"# doc",
		"",
		"The legacy flow used `config set agent.llm.endpoint_id X` — do not use it.", // prose/inline → exempt
		"Commit " + strings.Repeat("a", 40) + " in text is fine.",                    // prose hash → exempt
		"",
		"```bash",
		"# pin commit " + strings.Repeat("b", 40),                                // 40-hex SHA-1 → ok
		"# sha256 " + strings.Repeat("c", 64),                                    // 64-hex checksum → ok
		"vertc token issue --token QUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVphYmNksv==", // base64 blob → secret
		"vertc config set agent.llm.endpoint_id X",                               // banned in code → flagged
		"vertc env write --token AKLTexampleKEYvalue1234567 # example",           // real key on example line → flagged
		"vertc env write --token <AKLTexampleKEYvalue1234567>",                   // placeholder-wrapped → exempt
		"```",
	}, "\n")

	problems := scanFile("doc.md", map[string]bool{}, []byte(md))

	if got := countReason(problems, "banned legacy command"); got != 1 {
		t.Fatalf("banned: want 1 (code only), got %d: %v", got, reasons(problems))
	}
	if got := countReason(problems, "possible secret literal"); got != 2 {
		t.Fatalf("secret: want 2 (base64 blob + bare AK key), got %d: %v", got, reasons(problems))
	}
}

func TestCheckSkillsEnforcesPublishingMetadataAndLicense(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "Byted-InteractAI-Voice-Agent")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	md := "---\nname: Byted-InteractAI-Voice-Agent\ndescription: voice agent\n---\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}

	problems, err := CheckSkills(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"only lowercase letters", "missing frontmatter version", "missing Skill root LICENSE"} {
		if countReason(problems, want) != 1 {
			t.Fatalf("missing problem %q: %v", want, reasons(problems))
		}
	}
}

func TestCheckSkillsRejectsInvalidVersionAndLicense(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "byted-product-example")
	if err := os.MkdirAll(filepath.Join(skillDir, "LICENSE"), 0o755); err != nil {
		t.Fatal(err)
	}
	md := "---\nname: byted-product-example\ndescription: example\nversion: latest\n---\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}

	problems, err := CheckSkills(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"valid SemVer", "not a non-empty regular file"} {
		if countReason(problems, want) != 1 {
			t.Fatalf("missing problem %q: %v", want, reasons(problems))
		}
	}
}

func TestValidSkillVersion(t *testing.T) {
	for _, version := range []string{"0.1.0", "0.1.0-dev", "1.2.3-rc.4", "1.2.3+build.7"} {
		if !validSkillVersion(version) {
			t.Errorf("validSkillVersion(%q) = false", version)
		}
	}
	for _, version := range []string{"v1.2.3", "1.2", "latest", "1.2.3-01", "1.2.3-alpha.01"} {
		if validSkillVersion(version) {
			t.Errorf("validSkillVersion(%q) = true", version)
		}
	}
}

func TestSkillNamePublishingFormat(t *testing.T) {
	for _, name := range []string{"byted-las-asr", "byted-interactai-guide"} {
		if !skillName.MatchString(name) {
			t.Errorf("skillName rejects %q", name)
		}
	}
	for _, name := range []string{"voice-agent", "byted-product", "byted-InteractAI-voice-agent"} {
		if skillName.MatchString(name) {
			t.Errorf("skillName accepts %q", name)
		}
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
