// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package releasecontract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSkill(t *testing.T, root, name, version string) {
	t.Helper()
	dir := filepath.Join(root, "skills", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: fixture " + name + "\nversion: \"" + version + "\"\n---\n\n# " + name + "\n\nsentinel: keep-me\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "LICENSE"), []byte("fixture license\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func validSkills(t *testing.T, version string) string {
	t.Helper()
	root := t.TempDir()
	writeSkill(t, root, "byted-sample-alpha", version)
	writeSkill(t, root, "byted-sample-beta", version)
	return root
}

func TestResolve(t *testing.T) {
	tests := []struct {
		name, requested, want string
		stability             Stability
		destination           Destination
		wantErr               string
	}{
		{name: "public stable tag", stability: StabilityStable, destination: DestinationPublic, requested: "v9.8.7", want: "9.8.7"},
		{name: "public prerelease tag", stability: StabilityPrerelease, destination: DestinationPublic, requested: "v4.5.6-rc.4", want: "4.5.6-rc.4"},
		{name: "reserved source placeholder", stability: StabilityPrerelease, destination: DestinationPublic, requested: SourceSkillVersion, wantErr: "reserved"},
		{name: "reserved snapshot identity", stability: StabilityPrerelease, destination: DestinationPublic, requested: snapshotVersion, wantErr: "reserved"},
		{name: "non-publishing prerelease", stability: StabilityPrerelease, destination: DestinationNone, requested: "2.3.4-preview.4", want: "2.3.4-preview.4"},
		{name: "non-publishing snapshot", stability: StabilitySnapshot, destination: DestinationNone, want: "0.0.0-snapshot"},
		{name: "bad prerelease", stability: StabilityPrerelease, destination: DestinationPublic, requested: "1.2.3-rc.01", wantErr: "valid"},
		{name: "snapshot supplied", stability: StabilitySnapshot, destination: DestinationNone, requested: "1.2.4-snapshot", wantErr: "must not be supplied"},
		{name: "stable none conflict", stability: StabilityStable, destination: DestinationNone, requested: "1.2.3", wantErr: "incompatible"},
		{name: "snapshot public conflict", stability: StabilitySnapshot, destination: DestinationPublic, wantErr: "incompatible"},
		{name: "unknown stability", stability: Stability("nightly"), destination: DestinationNone, wantErr: "unknown release stability"},
		{name: "unknown destination", stability: StabilityStable, destination: Destination("partner"), requested: "1.2.3", wantErr: "unknown publication destination"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Resolve(tt.stability, tt.destination, tt.requested)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error=%v, want substring %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || got.Version != tt.want || got.Stability != tt.stability || got.Destination != tt.destination {
				t.Fatalf("Resolve()=%+v, %v; want version %q", got, err, tt.want)
			}
		})
	}
}

func TestDiscoverSkillsMultiple(t *testing.T) {
	root := validSkills(t, SourceSkillVersion)
	skills, err := DiscoverSkills(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 2 || skills[0].Name != "byted-sample-alpha" || skills[1].Name != "byted-sample-beta" {
		t.Fatalf("skills=%+v", skills)
	}
}

func TestDiscoverSkillsRejectsInvalidSet(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(t *testing.T, root string)
		wantErr string
	}{
		{name: "no skills", mutate: func(t *testing.T, root string) {
			_ = os.RemoveAll(filepath.Join(root, "skills"))
			_ = os.Mkdir(filepath.Join(root, "skills"), 0o755)
		}, wantErr: "no official Skills"},
		{name: "released version", mutate: func(t *testing.T, root string) { writeSkill(t, root, "byted-sample-beta", "1.2.4") }, wantErr: "source placeholder"},
		{name: "malformed", mutate: func(t *testing.T, root string) {
			_ = os.WriteFile(filepath.Join(root, "skills", "byted-sample-alpha", "SKILL.md"), []byte("---\nname: [\n---\n"), 0o644)
		}, wantErr: "validation failed"},
		{name: "duplicate", mutate: func(t *testing.T, root string) {
			path := filepath.Join(root, "skills", "byted-sample-beta", "SKILL.md")
			data, _ := os.ReadFile(path)
			_ = os.WriteFile(path, []byte(strings.Replace(string(data), "name: byted-sample-beta", "name: byted-sample-alpha", 1)), 0o644)
		}, wantErr: "duplicate skill name"},
		{name: "missing license", mutate: func(t *testing.T, root string) {
			_ = os.Remove(filepath.Join(root, "skills", "byted-sample-alpha", "LICENSE"))
		}, wantErr: "missing Skill root LICENSE"},
		{name: "directory name", mutate: func(t *testing.T, root string) {
			_ = os.Rename(filepath.Join(root, "skills", "byted-sample-alpha"), filepath.Join(root, "skills", "byted-sample-wrong"))
		}, wantErr: "directory"},
		{name: "unsafe symlink", mutate: func(t *testing.T, root string) {
			target := filepath.Join(root, "outside")
			_ = os.Mkdir(target, 0o755)
			path := filepath.Join(root, "skills", "byted-sample-alpha")
			_ = os.RemoveAll(path)
			if err := os.Symlink(target, path); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
		}, wantErr: "must not be a symlink"},
		{name: "other prerelease", mutate: func(t *testing.T, root string) {
			writeSkill(t, root, "byted-sample-alpha", "1.2.3-dev")
			writeSkill(t, root, "byted-sample-beta", "1.2.3-dev")
		}, wantErr: "source placeholder"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := validSkills(t, SourceSkillVersion)
			tt.mutate(t, root)
			_, err := DiscoverSkills(root)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error=%v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestStampSkillsPreservesOtherBytes(t *testing.T) {
	root := validSkills(t, SourceSkillVersion)
	skills, err := DiscoverSkills(root)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(skills[0].Path)))
	if err != nil {
		t.Fatal(err)
	}
	if err := StampSkills(root, "1.2.3-rc.9", skills); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(skills[0].Path)))
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(string(before), `version: "0.0.0-dev"`, `version: "1.2.3-rc.9"`, 1)
	if string(after) != want || !strings.Contains(string(after), "sentinel: keep-me") {
		t.Fatalf("unexpected stamped content:\n%s", after)
	}
}
