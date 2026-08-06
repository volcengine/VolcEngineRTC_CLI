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
		if normalizeStableName(part) == "" {
			return false
		}
	}
	return true
}

func BuildInvocationUserAgent(context *InvocationContext) (string, bool) {
	if context == nil || !productPattern.MatchString(context.CLIName) || !isValidVersion(context.CLIVersion) || !validInvocationType(context.InvocationType) {
		return "", false
	}
	required := context.CLIName + "/" + context.CLIVersion + " invocation/" + string(context.InvocationType)
	optional := make([]string, 0, 2)
	if context.CallerName != UnknownValue && validCallerChain(context.CallerName) {
		optional = append(optional, "caller/"+strings.ReplaceAll(context.CallerName, ",", "+"))
	}
	if context.SkillName != UnknownValue && normalizeStableName(context.SkillName) != "" {
		skillSegment := "skill/" + context.SkillName
		if context.SkillVersion != UnknownValue && isValidSkillVersion(context.SkillVersion) {
			skillSegment += "#" + context.SkillVersion
		}
		optional = append(optional, skillSegment)
	}
	userAgent := strings.Join(append([]string{required}, optional...), " ")
	if len(userAgent) <= maxUserAgentLength {
		return userAgent, true
	}
	if len(required) <= maxUserAgentLength {
		return required, true
	}
	return "", false
}
