// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package skillsfs

import (
	"testing"
	"testing/fstest"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/meta"
)

func testFS() fstest.MapFS {
	return fstest.MapFS{
		"byted-interactai-guide/SKILL.md":                      {Data: []byte("---\nname: byted-interactai-guide\ndescription: voice agent\n---\n\n# body\n")},
		"byted-interactai-guide/references/quickstart.md":      {Data: []byte("# quickstart\n")},
		"byted-interactai-guide/references/troubleshooting.md": {Data: []byte("# trouble\n")},
		"vertc-example-skill/SKILL.md":                         {Data: []byte("---\nname: vertc-example-skill\ndescription: example skill\n---\n")},
		"vertc-example-skill/references/lifecycle.md":          {Data: []byte("# lifecycle\n")},
		// A stray dir with no SKILL.md must be ignored by List/ensureSkill.
		"not-a-skill/README.md": {Data: []byte("nope")},
	}
}

func TestListReportsNameDescriptionVersionReferences(t *testing.T) {
	r := New(testFS())
	skills, err := r.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 2 {
		t.Fatalf("expected 2 skills, got %d: %+v", len(skills), skills)
	}
	if skills[0].Name != "byted-interactai-guide" || skills[1].Name != "vertc-example-skill" {
		t.Fatalf("unexpected/unsorted names: %+v", skills)
	}
	voiceAgent := skills[0]
	if voiceAgent.Description != "voice agent" {
		t.Fatalf("description = %q", voiceAgent.Description)
	}
	if voiceAgent.Version != meta.Version {
		t.Fatalf("version = %q, want %q", voiceAgent.Version, meta.Version)
	}
	if len(voiceAgent.References) != 2 || voiceAgent.References[0] != "byted-interactai-guide/references/quickstart.md" {
		t.Fatalf("references = %v", voiceAgent.References)
	}
}

func TestReadSkillAndReferenceByteIdentical(t *testing.T) {
	fsys := testFS()
	r := New(fsys)

	got, err := r.ReadSkill("byted-interactai-guide")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(fsys["byted-interactai-guide/SKILL.md"].Data) {
		t.Fatal("SKILL.md bytes differ from source")
	}

	ref, cleaned, err := r.ReadReference("byted-interactai-guide", "references/quickstart.md")
	if err != nil {
		t.Fatal(err)
	}
	if cleaned != "references/quickstart.md" {
		t.Fatalf("cleaned = %q", cleaned)
	}
	if string(ref) != string(fsys["byted-interactai-guide/references/quickstart.md"].Data) {
		t.Fatal("reference bytes differ from source")
	}

	// Slash-form via SplitArg.
	name, rest := SplitArg("byted-interactai-guide/references/quickstart.md")
	if name != "byted-interactai-guide" || rest != "references/quickstart.md" {
		t.Fatalf("SplitArg = %q,%q", name, rest)
	}
}

func TestRejectsUnsafeAndUnknown(t *testing.T) {
	r := New(testFS())
	cases := []struct {
		name, ref, wantCode string
	}{
		{"byted-interactai-guide", "../../etc/passwd", "vertc.skills.invalid_path"},
		{"byted-interactai-guide", "/etc/passwd", "vertc.skills.invalid_path"},
		{"byted-interactai-guide", `..\evil`, "vertc.skills.invalid_path"},
		// Windows-style "..\" embedded mid-path (not just as a prefix) must also be rejected.
		{"byted-interactai-guide", `references\..\..\etc\passwd`, "vertc.skills.invalid_path"},
		{"byted-interactai-guide", "references/missing.md", "vertc.skills.not_found"},
		{"no-such-skill", "references/x.md", "vertc.skills.unknown_skill"},
		{"not-a-skill", "README.md", "vertc.skills.unknown_skill"},
	}
	for _, c := range cases {
		_, _, err := r.ReadReference(c.name, c.ref)
		typed, ok := errs.As(err)
		if !ok || typed.Code != c.wantCode {
			t.Fatalf("ReadReference(%q,%q): err=%v, want %s", c.name, c.ref, err, c.wantCode)
		}
	}

	// A bare unknown skill name and traversal in the name are rejected too.
	if _, err := r.ReadSkill(".."); err == nil {
		t.Fatal("ReadSkill(\"..\") should fail")
	}
	if _, err := r.ReadSkill("not-a-skill"); err == nil {
		t.Fatal("ReadSkill(\"not-a-skill\") should fail (no SKILL.md)")
	}
}

func TestListPathOneLayer(t *testing.T) {
	r := New(testFS())
	entries, listed, err := r.ListPath("byted-interactai-guide/references")
	if err != nil {
		t.Fatal(err)
	}
	if listed != "byted-interactai-guide/references" || len(entries) != 2 {
		t.Fatalf("listed=%q entries=%v", listed, entries)
	}
}
