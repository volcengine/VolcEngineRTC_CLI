// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package errorcodes

import "testing"

// LoadWebSDK must not leak voice-agent codes, and vice versa (no domain mixing).
func TestDomainScopedLoadersDoNotMix(t *testing.T) {
	web, err := LoadWebSDK()
	if err != nil {
		t.Fatalf("LoadWebSDK: %v", err)
	}
	if _, ok := web.Lookup("1004005"); ok {
		t.Error("LoadWebSDK leaked a voice-agent code (1004005)")
	}
	if web.CountByDomain(DomainVoiceAgent) != 0 {
		t.Errorf("LoadWebSDK has voice-agent entries: %d", web.CountByDomain(DomainVoiceAgent))
	}
	if _, ok := web.Lookup("INVALID_TOKEN"); !ok {
		t.Error("LoadWebSDK missing its own web-sdk code (INVALID_TOKEN)")
	}

	agent, err := LoadVoiceAgent()
	if err != nil {
		t.Fatalf("LoadVoiceAgent: %v", err)
	}
	if _, ok := agent.Lookup("INVALID_TOKEN"); ok {
		t.Error("LoadVoiceAgent leaked a web-sdk code (INVALID_TOKEN)")
	}
	if agent.CountByDomain(DomainWebSDK) != 0 {
		t.Errorf("LoadVoiceAgent has web-sdk entries: %d", agent.CountByDomain(DomainWebSDK))
	}
}

// LoadAll merges both domains but every entry keeps its own domain tag.
func TestLoadAllPreservesPerEntryDomain(t *testing.T) {
	kb, err := LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if kb.CountByDomain(DomainWebSDK) == 0 || kb.CountByDomain(DomainVoiceAgent) == 0 {
		t.Fatalf("expected both domains present, got web-sdk=%d voice-agent=%d",
			kb.CountByDomain(DomainWebSDK), kb.CountByDomain(DomainVoiceAgent))
	}
	if kb.CountByDomain(DomainWebSDK)+kb.CountByDomain(DomainVoiceAgent) != kb.Count() {
		t.Error("domain counts do not sum to total; an entry lost its domain")
	}
	if e, ok := kb.Lookup("INVALID_TOKEN"); !ok || e.Domain != DomainWebSDK {
		t.Errorf("INVALID_TOKEN domain = %q, want %q", e.Domain, DomainWebSDK)
	}
	if e, ok := kb.Lookup("1004005"); !ok || e.Domain != DomainVoiceAgent {
		t.Errorf("1004005 domain = %q, want %q", e.Domain, DomainVoiceAgent)
	}
}

// Entries inherit domain/source/verified from the file meta when unset.
func TestEntryInheritsFileDefaults(t *testing.T) {
	kb, _ := LoadVoiceAgent()
	e, ok := kb.Lookup("1004005")
	if !ok {
		t.Fatal("1004005 not found")
	}
	if e.Domain != DomainVoiceAgent {
		t.Errorf("domain = %q, want inherited %q", e.Domain, DomainVoiceAgent)
	}
	if e.Verified != VerifiedYes {
		t.Errorf("verified = %q, want inherited %q", e.Verified, VerifiedYes)
	}
	if e.VerifiedAt == "" {
		t.Error("verified_at should be inherited from file meta")
	}
	if e.Source == "" {
		t.Error("1004005 should carry its official source URL")
	}
}

// Entry-level values win over the file default (curated-seed overrides verified).
func TestEntryOverridesFileDefault(t *testing.T) {
	kb, _ := LoadVoiceAgent()
	e, ok := kb.Lookup("SignatureDoesNotMatch")
	if !ok {
		t.Fatal("SignatureDoesNotMatch not found")
	}
	if e.Verified != VerifiedSeed {
		t.Errorf("verified = %q, want entry-level override %q", e.Verified, VerifiedSeed)
	}
	// Auth/signature placeholders must not expose an (internal) source URL.
	if e.Source != "" {
		t.Errorf("curated-seed auth entry leaked a source: %q", e.Source)
	}
}
