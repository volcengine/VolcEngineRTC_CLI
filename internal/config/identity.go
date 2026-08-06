// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"strings"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
)

// maxIdentityLen is RTC's length limit for room_id / user_id.
const maxIdentityLen = 128

// identitySymbols are the non-alphanumeric characters RTC allows in
// room_id / user_id (in addition to letters, digits and space). Anything
// outside [A-Za-z0-9] ∪ space ∪ this set is rejected.
const identitySymbols = "!#$%&()+-:;<=.>?@[]^_{}|~,"

// ValidateIdentity checks that a room_id / user_id value is acceptable to RTC:
// non-empty, at most maxIdentityLen characters, and limited to RTC's allowed
// character set. param names the offending flag/field ("--room-id",
// "rtc.user_id", …) for the error envelope. Rejecting bad values before signing
// a token or writing config keeps a bad identity from producing an unrunnable
// token/project — the same guard the token layer applies to the AppID length
// (design: 生成即可运行). Returns a typed *errs.Error (vertc.config.invalid_identity)
// on failure, nil when valid.
func ValidateIdentity(param, value string) error {
	if strings.TrimSpace(value) == "" {
		return errs.New("vertc.config.invalid_identity", errs.TypeValidation,
			"%s must not be empty", param).WithParam(param).
			WithHint("provide a non-empty value (letters, digits, space or %s)", identitySymbols)
	}
	if n := len([]rune(value)); n > maxIdentityLen {
		return errs.New("vertc.config.invalid_identity", errs.TypeValidation,
			"%s is %d characters, exceeds RTC limit of %d", param, n, maxIdentityLen).
			WithParam(param).
			WithHint("shorten it to at most %d characters", maxIdentityLen)
	}
	for _, r := range value {
		if isAllowedIdentityRune(r) {
			continue
		}
		return errs.New("vertc.config.invalid_identity", errs.TypeValidation,
			"%s contains disallowed character %q", param, string(r)).
			WithParam(param).
			WithHint("allowed: letters, digits, space and %s", identitySymbols)
	}
	return nil
}

// isAllowedIdentityRune reports whether r is in RTC's room_id/user_id charset.
func isAllowedIdentityRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	case r == ' ':
		return true
	default:
		return strings.ContainsRune(identitySymbols, r)
	}
}
