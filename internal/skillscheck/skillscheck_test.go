// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package skillscheck

import "testing"

func clearAutomationEnvironment(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"CI", "CONTINUOUS_INTEGRATION", "GITHUB_ACTIONS", "GITLAB_CI",
		"BUILDKITE", "JENKINS_URL", "TF_BUILD", "CIRCLECI", "TRAVIS",
		"TEAMCITY_VERSION", "CODEBUILD_BUILD_ID",
	} {
		t.Setenv(key, "")
	}
}

func TestDriftAndMarkSynced(t *testing.T) {
	t.Setenv("VERTC_STATE_DIR", t.TempDir())
	t.Setenv("VERTC_NO_SKILLS_NOTIFIER", "")
	clearAutomationEnvironment(t)
	Init("1.2.0")
	if Pending() == nil {
		t.Fatal("expected missing skills notice")
	}
	if err := MarkSynced("v1.2.0"); err != nil {
		t.Fatal(err)
	}
	Init("1.2.0")
	if Pending() != nil {
		t.Fatal("expected synchronized skills")
	}
}

func TestDevVersionSkipped(t *testing.T) {
	t.Setenv("VERTC_STATE_DIR", t.TempDir())
	Init("0.1.0-dev")
	if Pending() != nil {
		t.Fatal("dev builds must not notify")
	}
}

func TestInitSkipsCommonCI(t *testing.T) {
	t.Setenv("VERTC_STATE_DIR", t.TempDir())
	t.Setenv("VERTC_NO_SKILLS_NOTIFIER", "")
	t.Setenv("GITHUB_ACTIONS", "true")
	Init("1.2.3")
	if notice := Pending(); notice != nil {
		t.Fatalf("CI should suppress notice: %+v", notice)
	}
	if got := Status("1.2.3"); got != "skipped" {
		t.Fatalf("CI status=%q", got)
	}
}
