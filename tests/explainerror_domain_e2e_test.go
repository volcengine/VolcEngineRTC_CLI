// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package tests

import (
	"strings"
	"testing"
)

// explain-error must surface domain/source/verified for a verified voice-agent
// runtime code, in the JSON envelope.
func TestExplainErrorVoiceAgentDomainMetadata(t *testing.T) {
	dir := t.TempDir()
	r := run(t, dir, nil, "explain-error", "1004005")
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
	data, _ := r.envelope(t)["data"].(map[string]any)
	if data["found"] != true {
		t.Fatalf("expected found: %v", data)
	}
	if data["domain"] != "voice-agent" {
		t.Errorf("domain = %v, want voice-agent", data["domain"])
	}
	if data["verified"] != "verified" {
		t.Errorf("verified = %v, want verified", data["verified"])
	}
	if src, _ := data["source"].(string); !strings.Contains(src, "1928198") {
		t.Errorf("source = %v, want official page URL", data["source"])
	}
}

// A web-sdk code must be tagged with the web-sdk domain in the JSON envelope.
func TestExplainErrorWebSDKDomain(t *testing.T) {
	dir := t.TempDir()
	r := run(t, dir, nil, "explain-error", "INVALID_TOKEN")
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
	data, _ := r.envelope(t)["data"].(map[string]any)
	if data["domain"] != "web-sdk" {
		t.Errorf("domain = %v, want web-sdk", data["domain"])
	}
	if data["verified"] != "verified" {
		t.Errorf("verified = %v, want verified", data["verified"])
	}
}

// A curated-seed (unverified) entry must be flagged, not shown as fact, and must
// not expose an internal source. Checked in both JSON and Pretty output.
func TestExplainErrorCuratedSeedFlagged(t *testing.T) {
	dir := t.TempDir()

	// JSON: verified == curated-seed, no source leaked.
	r := run(t, dir, nil, "explain-error", "SignatureDoesNotMatch")
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
	data, _ := r.envelope(t)["data"].(map[string]any)
	if data["verified"] != "curated-seed" {
		t.Errorf("verified = %v, want curated-seed", data["verified"])
	}
	if src, ok := data["source"]; ok && src != "" {
		t.Errorf("curated-seed entry leaked a source: %v", src)
	}

	// Pretty: explicit unverified warning is present.
	r = run(t, dir, nil, "explain-error", "SignatureDoesNotMatch", "--format", "pretty")
	if r.code != 0 {
		t.Fatalf("pretty exit %d: %s", r.code, r.stderr)
	}
	if !strings.Contains(r.stdout, "未验证") || !strings.Contains(r.stdout, "curated-seed") {
		t.Errorf("pretty output missing unverified warning: %q", r.stdout)
	}
}
