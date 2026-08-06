// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package token

import (
	"time"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
)

// DefaultTTL is the safe default token lifetime when none is specified.
const DefaultTTL = 24 * time.Hour

// IssueParams describes a token to issue.
type IssueParams struct {
	AppID     string
	AppKey    string
	RoomID    string
	UserID    string
	TTL       time.Duration // global expiry; DefaultTTL when zero
	Publish   bool          // grant publish privilege (default true)
	Subscribe bool          // grant subscribe privilege (default true)
}

// Info is a decoded, human/Agent-friendly view of a token.
type Info struct {
	AppID     string    `json:"app_id"`
	RoomID    string    `json:"room_id"`
	UserID    string    `json:"user_id"`
	IssuedAt  time.Time `json:"issued_at"`
	ExpireAt  time.Time `json:"expire_at,omitempty"`
	Publish   bool      `json:"publish"`
	Subscribe bool      `json:"subscribe"`
	Expired   bool      `json:"expired"`
}

// IssueResult is returned by Issue.
type IssueResult struct {
	Token     string    `json:"token"`
	AppID     string    `json:"app_id"`
	RoomID    string    `json:"room_id"`
	UserID    string    `json:"user_id"`
	ExpireAt  time.Time `json:"expire_at"`
	Publish   bool      `json:"publish"`
	Subscribe bool      `json:"subscribe"`
}

// Issue signs a new RTC join token locally.
func Issue(p IssueParams) (*IssueResult, error) {
	if p.AppKey == "" {
		return nil, errs.New("vertc.token.missing_credential", errs.TypeAuth,
			"AppKey is required to sign a token").
			WithHint("set RTC_APP_KEY in the process environment or local .env.local — never pass it in command arguments or store it in config")
	}
	if p.AppID == "" || p.RoomID == "" || p.UserID == "" {
		return nil, errs.New("vertc.token.missing_config", errs.TypeValidation,
			"app_id, room_id and user_id are all required").
			WithHint("fill rtc.app_id/room_id/user_id in vertc.config.yaml")
	}
	// The VRTC AccessToken wire format encodes a fixed 24-char AppID; a wrong
	// length would sign a token that cannot be decoded later. Reject it here
	// rather than emit a token that fails `token check` (design: 生成即可运行).
	if len(p.AppID) != appIDLength {
		return nil, errs.New("vertc.token.invalid_appid", errs.TypeValidation,
			"AppID must be %d characters, got %d", appIDLength, len(p.AppID)).
			WithParam("rtc.app_id").
			WithHint("use the 24-char VRTC AppID from the console (RTC_APP_ID)")
	}
	ttl := p.TTL
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	// Default to granting both when neither is explicitly set.
	if !p.Publish && !p.Subscribe {
		p.Publish, p.Subscribe = true, true
	}

	expireAt := time.Now().Add(ttl)
	at := NewAccessToken(p.AppID, p.AppKey, p.RoomID, p.UserID)
	at.ExpireTime(expireAt)
	if p.Publish {
		at.AddPrivilege(PrivPublishStream, expireAt)
	}
	if p.Subscribe {
		at.AddPrivilege(PrivSubscribeStream, expireAt)
	}
	raw, err := at.Serialize()
	if err != nil {
		return nil, errs.Wrap(err, "vertc.cli.internal", "serialize token: %s", err)
	}
	return &IssueResult{
		Token:     raw,
		AppID:     p.AppID,
		RoomID:    p.RoomID,
		UserID:    p.UserID,
		ExpireAt:  expireAt,
		Publish:   p.Publish,
		Subscribe: p.Subscribe,
	}, nil
}

// Inspect decodes a token without verifying its signature.
func Inspect(raw string) (*Info, error) {
	at, err := ParseAccessToken(raw)
	if err != nil {
		return nil, errs.New("vertc.token.invalid", errs.TypeValidation,
			"cannot decode token: %s", err).
			WithHint("ensure you passed a full VRTC AccessToken string")
	}
	return infoFrom(at), nil
}

// Check decodes AND verifies the token against appKey, reporting validity and
// expiry. Returns a typed error for expired/invalid tokens (non-zero exit).
func Check(raw, appKey string) (*Info, error) {
	at, err := ParseAccessToken(raw)
	if err != nil {
		return nil, errs.New("vertc.token.invalid", errs.TypeValidation,
			"cannot decode token: %s", err)
	}
	info := infoFrom(at)
	if info.Expired {
		return info, errs.New("vertc.token.expired", errs.TypeValidation,
			"token expired at %s", info.ExpireAt.Format(time.RFC3339)).
			WithHint("run `vertc token issue` to mint a fresh token")
	}
	if appKey != "" && !at.Verify(appKey) {
		return info, errs.New("vertc.token.invalid", errs.TypeValidation,
			"token signature does not match the provided AppKey").
			WithHint("confirm RTC_APP_KEY matches the AppID that issued this token")
	}
	return info, nil
}

func infoFrom(at *AccessToken) *Info {
	info := &Info{
		AppID:    at.AppID,
		RoomID:   at.RoomID,
		UserID:   at.UserID,
		IssuedAt: time.Unix(int64(at.IssuedAt), 0),
	}
	if at.ExpireAt > 0 {
		info.ExpireAt = time.Unix(int64(at.ExpireAt), 0)
		info.Expired = uint32(time.Now().Unix()) > at.ExpireAt
	}
	if _, ok := at.Privileges[uint16(PrivPublishStream)]; ok {
		info.Publish = true
	}
	if _, ok := at.Privileges[uint16(PrivSubscribeStream)]; ok {
		info.Subscribe = true
	}
	return info
}
