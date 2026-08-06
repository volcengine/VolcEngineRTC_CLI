// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package openapi

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// fixedReq builds a deterministic signed request for the tests.
func fixedReq(t *testing.T, secret string) *http.Request {
	t.Helper()
	body := []byte(`{"AppId":"app","RoomId":"room","TaskId":"task"}`)
	req, err := http.NewRequest(http.MethodPost,
		"https://rtc.volcengineapi.com/?Action=StartVoiceChat&Version=2024-12-01",
		strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "rtc.volcengineapi.com"
	if err := SignRequest(req, Credential{AccessKeyID: "AKLTtest", SecretAccessKey: secret}, SignOptions{
		Region:  DefaultRegion,
		Service: DefaultService,
		Now:     time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC),
		Body:    body,
	}); err != nil {
		t.Fatal(err)
	}
	return req
}

func TestSignSetsHeaders(t *testing.T) {
	req := fixedReq(t, "topsecretkey")
	if got := req.Header.Get("X-Date"); got != "20241201T000000Z" {
		t.Fatalf("X-Date = %q", got)
	}
	if req.Header.Get("X-Content-Sha256") == "" {
		t.Fatal("X-Content-Sha256 not set")
	}
	auth := req.Header.Get("Authorization")
	for _, want := range []string{"HMAC-SHA256", "Credential=AKLTtest/20241201/cn-north-1/rtc/request",
		"SignedHeaders=content-type;host;x-content-sha256;x-date", "Signature="} {
		if !strings.Contains(auth, want) {
			t.Fatalf("Authorization missing %q: %s", want, auth)
		}
	}
}

func TestSignIncludesSessionToken(t *testing.T) {
	body := []byte(`{"Limit":1}`)
	req, err := http.NewRequest(http.MethodPost, "https://rtc.volcengineapi.com/?Action=Ping&Version=2020-01-01", strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	if err := SignRequest(req, Credential{AccessKeyID: "AKID", SecretAccessKey: "SECRET", SessionToken: "SESSION"}, SignOptions{
		Service: "rtc",
		Region:  "cn-north-1",
		Now:     time.Date(2026, 7, 7, 10, 0, 0, 0, time.UTC),
		Body:    body,
	}); err != nil {
		t.Fatal(err)
	}
	if req.Header.Get("X-Security-Token") != "SESSION" {
		t.Fatalf("X-Security-Token = %q", req.Header.Get("X-Security-Token"))
	}
	if !strings.Contains(req.Header.Get("Authorization"), "x-security-token") {
		t.Fatalf("Authorization missing x-security-token: %s", req.Header.Get("Authorization"))
	}
}

// TestSignDeterministicVector locks the algorithm output for a fixed input, so a
// regression in canonicalization/key-derivation is caught. (The cross-check
// against the live Volcengine API happens in end-to-end acceptance.)
func TestSignDeterministicVector(t *testing.T) {
	req := fixedReq(t, "topsecretkey")
	const wantSig = "43c65ebca204b0bd8fba66f7af061705e2856067638aa8cf3aa74025b0362606"
	got := signatureOf(req.Header.Get("Authorization"))
	if got != wantSig {
		t.Fatalf("signature drift:\n got  %s\n want %s", got, wantSig)
	}
}

func TestSignDiffersBySecret(t *testing.T) {
	a := signatureOf(fixedReq(t, "keyA").Header.Get("Authorization"))
	b := signatureOf(fixedReq(t, "keyB").Header.Get("Authorization"))
	if a == b {
		t.Fatal("different secrets produced the same signature")
	}
}

func signatureOf(auth string) string {
	i := strings.Index(auth, "Signature=")
	if i < 0 {
		return ""
	}
	return auth[i+len("Signature="):]
}
