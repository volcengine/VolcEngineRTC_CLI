// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package errorcodes

import "testing"

func TestLoadWebSeedCount(t *testing.T) {
	kb, err := LoadWeb()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	// Web seed (88) plus the voice-agent conversational-AI seed.
	if kb.Count() < 88 {
		t.Fatalf("expected at least 88 entries, got %d", kb.Count())
	}
	if kb.Platform != "web" {
		t.Fatalf("expected platform web, got %q", kb.Platform)
	}
	// A conversational-AI code must resolve (merged KB).
	if _, ok := kb.Lookup("SignatureDoesNotMatch"); !ok {
		t.Fatal("voice-agent code SignatureDoesNotMatch not merged")
	}
}

func TestLookupStringCode(t *testing.T) {
	kb, _ := LoadWeb()
	e, ok := kb.Lookup("INVALID_TOKEN")
	if !ok {
		t.Fatal("INVALID_TOKEN not found")
	}
	if e.Enum != "ErrorCode" || e.DoctorCheck != "token.valid" {
		t.Fatalf("unexpected entry: %+v", e)
	}
	// Case-insensitive.
	if _, ok := kb.Lookup("invalid_token"); !ok {
		t.Fatal("lookup should be case-insensitive")
	}
}

func TestLookupNumericCode(t *testing.T) {
	kb, _ := LoadWeb()
	e, ok := kb.Lookup("1202")
	if !ok {
		t.Fatal("1202 not found")
	}
	if e.Enum != "ForwardStreamError" {
		t.Fatalf("unexpected entry: %+v", e)
	}
}

func TestLookupByName(t *testing.T) {
	kb, _ := LoadWeb()
	// Numeric entries also index by symbolic name.
	if _, ok := kb.Lookup("FORWARD_STREAM_ERROR_INVALID_TOKEN"); !ok {
		t.Fatal("name lookup failed")
	}
}

func TestLookupUnknown(t *testing.T) {
	kb, _ := LoadWeb()
	if _, ok := kb.Lookup("NOPE_NOT_A_CODE"); ok {
		t.Fatal("expected unknown code to miss")
	}
}
