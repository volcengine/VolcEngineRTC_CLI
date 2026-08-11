// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package releasecontract

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sourceVersionRepo(t *testing.T, version string) string {
	t.Helper()
	root := validSkills(t, version)
	packageJSON := "{\n  \"name\": \"@volcengine/rtc-cli\",\n  \"version\": \"" + version + "\",\n  \"private\": false\n}\n"
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(packageJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestPlanAndApplySourceVersionUpdatesEveryTarget(t *testing.T) {
	root := sourceVersionRepo(t, "1.2.3")
	plan, err := PlanSourceVersion(root, "v1.2.4-rc.1")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Version != "1.2.4-rc.1" || len(plan.Targets) != 3 || len(plan.Changes) != 3 {
		t.Fatalf("plan=%+v", plan)
	}
	if err := ApplySourceVersion(plan); err != nil {
		t.Fatal(err)
	}
	verified, err := PlanSourceVersion(root, "1.2.4-rc.1")
	if err != nil {
		t.Fatal(err)
	}
	if len(verified.Changes) != 0 {
		t.Fatalf("verified changes=%v", verified.Changes)
	}
	packageData, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(packageData), `"version": "1.2.4-rc.1"`) || !strings.Contains(string(packageData), `"private": false`) {
		t.Fatalf("package formatting/content changed unexpectedly:\n%s", packageData)
	}
}

func TestPlanSourceVersionRejectsSyntheticDevVersion(t *testing.T) {
	root := sourceVersionRepo(t, "1.2.3")
	_, err := PlanSourceVersion(root, "0.0.0-dev")
	if err == nil || !strings.Contains(err.Error(), "cannot be 0.0.0-dev") {
		t.Fatalf("error=%v", err)
	}
}

func TestApplySourceVersionRollsBackAfterReplaceFailure(t *testing.T) {
	root := sourceVersionRepo(t, "1.2.3")
	plan, err := PlanSourceVersion(root, "1.2.4")
	if err != nil {
		t.Fatal(err)
	}
	originals := map[string][]byte{}
	for _, change := range plan.Changes {
		originals[change.Path] = append([]byte{}, change.Old...)
	}
	calls := 0
	err = applySourceVersion(plan, func(source, target string) error {
		calls++
		if calls == 2 {
			return errors.New("injected rename failure")
		}
		return os.Rename(source, target)
	})
	if err == nil || !strings.Contains(err.Error(), "injected rename failure") {
		t.Fatalf("error=%v", err)
	}
	for path, want := range originals {
		got, readErr := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if string(got) != string(want) {
			t.Fatalf("%s was not rolled back", path)
		}
	}
}
