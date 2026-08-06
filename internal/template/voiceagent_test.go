// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package template

import "testing"

func TestVoiceAgentTemplateAvailableAndDefault(t *testing.T) {
	tmpl, ok := Find("voice-agent", "web")
	if !ok || !tmpl.Available || !tmpl.Default {
		t.Fatalf("voice-agent/web should be available + default: %+v", tmpl)
	}
	if tmpl.Remote == nil {
		t.Fatal("voice-agent/web should use the pinned remote template")
	}
	if tmpl.Remote.Repository != "volcengine/rtc-aigc-demo" ||
		tmpl.Remote.Commit != voiceAgentRemoteCommit ||
		tmpl.Remote.SHA256 != voiceAgentProductionSHA256 {
		t.Fatalf("voice-agent remote source is not pinned to the published archive: %+v", tmpl.Remote)
	}
	if tmpl.dir != "" {
		t.Fatalf("voice-agent should not retain an embedded directory: %q", tmpl.dir)
	}

	if _, ok := Find("voice-call", "web"); ok {
		t.Fatal("voice-call/web must not be registered")
	}
}
