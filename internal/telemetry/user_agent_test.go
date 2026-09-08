// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package telemetry

import (
	"strings"
	"testing"
)

func testContext() InvocationContext {
	return InvocationContext{
		CLIName: "vertc", CLIVersion: "1.2.3", InvocationID: "123e4567-e89b-42d3-a456-426614174000", InvocationType: InvocationTypeManual,
		CallerName: UnknownValue, SkillName: UnknownValue, SkillVersion: UnknownValue,
	}
}

func TestBuildInvocationUserAgent(t *testing.T) {
	context := testContext()
	if got, ok := BuildInvocationUserAgent(&context); !ok || got != "vertc/1.2.3 invocation/manual invocation-id/123e4567-e89b-42d3-a456-426614174000" {
		t.Fatalf("BuildInvocationUserAgent() = %q, %v", got, ok)
	}

	context.InvocationType = InvocationTypeSkill
	context.CallerName = "openclaw,claude-code"
	context.SkillName = "byted-interactai-guide"
	context.SkillVersion = "1.0"
	want := "vertc/1.2.3 invocation/skill caller/openclaw+claude-code skill/byted-interactai-guide#1.0 invocation-id/123e4567-e89b-42d3-a456-426614174000"
	if got, ok := BuildInvocationUserAgent(&context); !ok || got != want {
		t.Fatalf("BuildInvocationUserAgent() = %q, %v, want %q", got, ok, want)
	}

	context.CLIVersion = "1.2.3+build.1"
	if _, ok := BuildInvocationUserAgent(&context); !ok {
		t.Fatal("SemVer build metadata was rejected")
	}
}

func TestBuildInvocationUserAgentDropsInvalidOptionalValues(t *testing.T) {
	context := testContext()
	context.InvocationType = InvocationTypeDirect
	context.CallerName = "codex\r\nInjected"
	context.SkillName = "invalid skill"
	if got, ok := BuildInvocationUserAgent(&context); !ok || got != "vertc/1.2.3 invocation/direct invocation-id/123e4567-e89b-42d3-a456-426614174000" {
		t.Fatalf("BuildInvocationUserAgent() = %q, %v", got, ok)
	}

	context.CallerName = "openclaw,claude-code,codex"
	if got, _ := BuildInvocationUserAgent(&context); strings.Contains(got, "caller/") {
		t.Fatalf("long caller chain was retained: %q", got)
	}
}

func TestBuildInvocationUserAgentRejectsInvalidRequiredValues(t *testing.T) {
	tests := []InvocationContext{testContext(), testContext(), testContext(), testContext()}
	tests[0].CLIName = "vertc injected"
	tests[1].CLIVersion = "latest"
	tests[2].InvocationID = "not-a-uuid"
	tests[3].InvocationType = "invalid"
	for _, context := range tests {
		if got, ok := BuildInvocationUserAgent(&context); ok || got != "" {
			t.Errorf("BuildInvocationUserAgent(%#v) = %q, %v", context, got, ok)
		}
	}
}

func TestBuildInvocationUserAgentLengthFallback(t *testing.T) {
	context := testContext()
	context.CLIName = "a" + strings.Repeat("b", 199)
	context.CallerName = strings.Repeat("a", 128)
	context.SkillName = strings.Repeat("a", 128)
	context.SkillVersion = "12345678901234567890123456789012"
	want := context.CLIName + "/1.2.3 invocation/manual skill/" + context.SkillName + " invocation-id/" + context.InvocationID
	if got, ok := BuildInvocationUserAgent(&context); !ok || got != want {
		t.Fatalf("BuildInvocationUserAgent() = %q, %v", got, ok)
	}
	context.CLIName = "a" + strings.Repeat("b", 459)
	if got, ok := BuildInvocationUserAgent(&context); ok || got != "" {
		t.Fatalf("oversized required UA = %q, %v", got, ok)
	}
}

func TestInvocationStoreFirstCallWins(t *testing.T) {
	resetInvocationForTest()
	t.Cleanup(resetInvocationForTest)
	InitializeInvocation(baseOptions(map[string]string{"AI_AGENT": "codex"}))
	InitializeInvocation(ResolveInvocationContextOptions{
		CLIName: "other-cli", CLIVersion: "9.9.9", Env: map[string]string{},
		StdinIsTTY: boolPtr(false), StdoutIsTTY: boolPtr(false),
	})
	if got, ok := GetInvocationUserAgent(); !ok || !strings.HasPrefix(got, "vertc/1.2.3 invocation/direct caller/codex invocation-id/") {
		t.Fatalf("GetInvocationUserAgent() = %q, %v", got, ok)
	}
}
