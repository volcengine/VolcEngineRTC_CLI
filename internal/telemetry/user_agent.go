// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package telemetry

import (
	"regexp"
	"strings"
)

const (
	maxUserAgentLength = 512
	maxCallerSegments  = 2
	maxSafeInteger     = "9007199254740991"
)

var productPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func isValidSemver(value string) bool {
	if !semverPattern.MatchString(value) {
		return false
	}
	core, _, _ := strings.Cut(value, "-")
	for _, part := range strings.Split(core, ".") {
		if len(part) > len(maxSafeInteger) || len(part) == len(maxSafeInteger) && part > maxSafeInteger {
			return false
		}
	}
	return true
}

func isValidVersion(value string) bool {
	return value != "" && len(value) <= maxVersionLength && isValidSemver(value)
}

func isValidSkillVersion(value string) bool {
	return value != "" && len(value) <= maxVersionLength && (isValidSemver(value) || majorMinorVersionPattern.MatchString(value))
}

func validInvocationType(value InvocationType) bool {
	switch value {
	case InvocationTypeSkill, InvocationTypeDirect, InvocationTypeIDEPlugin, InvocationTypeManual, InvocationTypeUnknown:
		return true
	default:
		return false
	}
}

func validCallerChain(value string) bool {
	parts := strings.Split(value, ",")
	if value == "" || len(parts) > maxCallerSegments {
		return false
	}
	for _, part := range parts {
		if normalizeCallerName(part) == "" {
			return false
		}
	}
	return true
}

func BuildInvocationUserAgent(context *InvocationContext) (string, bool) {
	if context == nil || !productPattern.MatchString(context.CLIName) || !isValidVersion(context.CLIVersion) || !uuidPattern.MatchString(context.InvocationID) || !validInvocationType(context.InvocationType) {
		return "", false
	}
	required := context.CLIName + "/" + context.CLIVersion + " invocation/" + string(context.InvocationType)
	invocationSegment := "invocation-id/" + context.InvocationID
	caller := ""
	if context.CallerName != UnknownValue && validCallerChain(context.CallerName) {
		caller = "caller/" + strings.ReplaceAll(context.CallerName, ",", "+")
	}
	skill, skillWithoutVersion := "", ""
	if context.SkillName != UnknownValue && normalizeStableName(context.SkillName) != "" {
		skillWithoutVersion = "skill/" + context.SkillName
		skill = skillWithoutVersion
		if context.SkillVersion != UnknownValue && isValidSkillVersion(context.SkillVersion) {
			skill += "#" + context.SkillVersion
		}
	}
	compose := func() string {
		return strings.Join(removeEmpty(required, caller, skill, invocationSegment), " ")
	}
	if userAgent := compose(); len(userAgent) <= maxUserAgentLength {
		return userAgent, true
	}
	if skill != skillWithoutVersion {
		skill = skillWithoutVersion
		if userAgent := compose(); len(userAgent) <= maxUserAgentLength {
			return userAgent, true
		}
	}
	caller = ""
	if userAgent := compose(); len(userAgent) <= maxUserAgentLength {
		return userAgent, true
	}
	skill = ""
	if userAgent := compose(); len(userAgent) <= maxUserAgentLength {
		return userAgent, true
	}
	return "", false
}
