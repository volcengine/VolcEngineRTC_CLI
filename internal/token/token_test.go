// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package token

import (
	"testing"
	"time"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
)

func TestIssueParseVerifyRoundtrip(t *testing.T) {
	res, err := Issue(IssueParams{
		AppID: "app123456789012345678901", AppKey: "secretkey",
		RoomID: "room-1", UserID: "user-1", TTL: time.Hour,
	})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if res.Token == "" || res.Token[:3] != "001" {
		t.Fatalf("unexpected token: %q", res.Token)
	}
	if !res.Publish || !res.Subscribe {
		t.Fatalf("expected publish+subscribe default")
	}

	info, err := Check(res.Token, "secretkey")
	if err != nil {
		t.Fatalf("check valid token: %v", err)
	}
	if info.RoomID != "room-1" || info.UserID != "user-1" {
		t.Fatalf("decoded mismatch: %+v", info)
	}
	if info.Expired {
		t.Fatalf("token should not be expired")
	}
}

func TestCheckWrongKeyFails(t *testing.T) {
	res, _ := Issue(IssueParams{AppID: "app123456789012345678901", AppKey: "right", RoomID: "r", UserID: "u", TTL: time.Hour})
	_, err := Check(res.Token, "wrong")
	e, ok := errs.As(err)
	if !ok || e.Code != "vertc.token.invalid" {
		t.Fatalf("expected vertc.token.invalid, got %v", err)
	}
}

func TestCheckExpiredFails(t *testing.T) {
	// Issue with a TTL in the past by crafting a token manually.
	at := NewAccessToken("app123456789012345678901", "k", "r", "u")
	past := time.Now().Add(-time.Hour)
	at.ExpireTime(past)
	at.AddPrivilege(PrivPublishStream, past)
	raw, err := at.Serialize()
	if err != nil {
		t.Fatal(err)
	}
	info, err := Check(raw, "k")
	e, ok := errs.As(err)
	if !ok || e.Code != "vertc.token.expired" {
		t.Fatalf("expected vertc.token.expired, got %v", err)
	}
	if !info.Expired {
		t.Fatalf("info.Expired should be true")
	}
}

func TestIssueMissingCredential(t *testing.T) {
	_, err := Issue(IssueParams{AppID: "a", RoomID: "r", UserID: "u"})
	e, ok := errs.As(err)
	if !ok || e.Code != "vertc.token.missing_credential" {
		t.Fatalf("expected missing_credential, got %v", err)
	}
}

func TestIssueMissingConfig(t *testing.T) {
	_, err := Issue(IssueParams{AppKey: "k"})
	e, ok := errs.As(err)
	if !ok || e.Code != "vertc.token.missing_config" {
		t.Fatalf("expected missing_config, got %v", err)
	}
}

func TestIssueInvalidAppIDLength(t *testing.T) {
	// A non-24-char AppID would sign a token that cannot be decoded later;
	// Issue must reject it up front instead of emitting a broken token.
	_, err := Issue(IssueParams{AppID: "shortid", AppKey: "k", RoomID: "r", UserID: "u"})
	e, ok := errs.As(err)
	if !ok || e.Code != "vertc.token.invalid_appid" {
		t.Fatalf("expected vertc.token.invalid_appid, got %v", err)
	}
	if e.Param != "rtc.app_id" {
		t.Fatalf("expected param rtc.app_id, got %q", e.Param)
	}
}

func TestIssueValidAppIDLengthRoundtrips(t *testing.T) {
	// Exactly 24 chars → issue + check succeed.
	res, err := Issue(IssueParams{AppID: "aabbccddeeff001122334455", AppKey: "k", RoomID: "r", UserID: "u", TTL: time.Hour})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if _, err := Check(res.Token, "k"); err != nil {
		t.Fatalf("check: %v", err)
	}
}

func TestInspectDoesNotVerify(t *testing.T) {
	res, _ := Issue(IssueParams{AppID: "app123456789012345678901", AppKey: "k", RoomID: "r", UserID: "u", TTL: time.Hour})
	info, err := Inspect(res.Token)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if info.RoomID != "r" {
		t.Fatalf("inspect decode mismatch: %+v", info)
	}
}
