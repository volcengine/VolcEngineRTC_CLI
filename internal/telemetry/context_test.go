// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package telemetry

import "testing"

func boolPtr(value bool) *bool { return &value }

func baseOptions(env map[string]string) ResolveInvocationContextOptions {
	return ResolveInvocationContextOptions{
		CLIName: "vertc", CLIVersion: "1.2.3", Env: env,
		StdinIsTTY: boolPtr(false), StdoutIsTTY: boolPtr(false),
	}
}

func TestResolveInvocationContextPriority(t *testing.T) {
	context := ResolveInvocationContext(baseOptions(map[string]string{
		"VE_SKILL_ID":         "byted-interactai-guide/1.0",
		"IDE_CLIENT_NAME":     "vscode",
		"OPENCLAW_SESSION_ID": "1",
		"CLAUDECODE":          "1",
	}))
	if context.InvocationType != InvocationTypeSkill || context.CallerName != "openclaw,claude-code" ||
		context.SkillName != "byted-interactai-guide" || context.SkillVersion != "1.0" {
		t.Fatalf("ResolveInvocationContext() = %#v", context)
	}
	if !uuidPattern.MatchString(context.InvocationID) {
		t.Fatalf("invocation ID = %q", context.InvocationID)
	}
}

func TestResolveInvocationContextNativeCallers(t *testing.T) {
	tests := []struct {
		env  map[string]string
		want string
	}{
		{map[string]string{"OPENCLAW_SESSION_ID": "1"}, "openclaw"},
		{map[string]string{"ARKCLAW_RUN_ID": "1"}, "arkclaw"},
		{map[string]string{"HERMES_AGENT_ID": "1"}, "hermes"},
		{map[string]string{"COZE_CLAW_AGENT_ID": ""}, "coze-claw"},
		{map[string]string{"TRAE_CLI_PLUGIN_ROOT": "/tmp/plugin"}, "trae"},
		{map[string]string{"DOUBAO_OFFICE_PRESENT": ""}, "doubao-office"},
		{map[string]string{"AI_AGENT": " TRAE "}, "trae"},
		{map[string]string{"CLAUDECODE": "1"}, "claude-code"},
		{map[string]string{"CLAUDE_CODE": "1", "CLAUDE_CODE_IS_COWORK": "1"}, "claude-cowork"},
		{map[string]string{"CODEX_THREAD_ID": "thread"}, "codex"},
		{map[string]string{"CURSOR_AGENT": "1"}, "cursor"},
		{map[string]string{"GEMINI_CLI": "1"}, "gemini-cli"},
		{map[string]string{"KIRO_AGENT_PATH": "/tmp/kiro"}, "kiro"},
		{map[string]string{"OPENCODE": "1"}, "opencode"},
		{map[string]string{"ANTIGRAVITY_AGENT": "1"}, "antigravity"},
		{map[string]string{"COPILOT_CLI": "1"}, "github-copilot"},
		{map[string]string{"CLINE_ACTIVE": "1"}, "cline"},
		{map[string]string{"AMP_CURRENT_THREAD_ID": "thread"}, "amp"},
		{map[string]string{"PI_CODING_AGENT": "true"}, "pi"},
		{map[string]string{"REPL_ID": "repl"}, "replit"},
		{map[string]string{"AUGMENT_AGENT": "1"}, "augment"},
		{map[string]string{"QWEN_CODE": "1"}, "qwen-code"},
	}
	for _, test := range tests {
		context := ResolveInvocationContext(baseOptions(test.env))
		if context.InvocationType != InvocationTypeDirect || context.CallerName != test.want {
			t.Errorf("env %#v: got %#v, want caller %q", test.env, context, test.want)
		}
	}
}

func TestResolveInvocationContextCustomAndPrioritizedCallers(t *testing.T) {
	custom := ResolveInvocationContext(baseOptions(map[string]string{
		"AI_AGENT": " Custom.Agent ", "OPENCLAW_SESSION_ID": "1", "CLAUDECODE": "1",
	}))
	if custom.CallerName != "openclaw,Custom.Agent" {
		t.Fatalf("custom caller = %q", custom.CallerName)
	}

	prioritizedOuter := ResolveInvocationContext(baseOptions(map[string]string{
		"OPENCLAW_SESSION_ID": "1", "ARKCLAW_RUN_ID": "1", "CLAUDECODE": "1",
	}))
	if prioritizedOuter.CallerName != "openclaw,claude-code" {
		t.Fatalf("prioritized outer caller = %q", prioritizedOuter.CallerName)
	}

	prioritizedInner := ResolveInvocationContext(baseOptions(map[string]string{
		"CLAUDECODE": "1", "CODEX_THREAD_ID": "thread",
	}))
	if prioritizedInner.InvocationType != InvocationTypeDirect || prioritizedInner.CallerName != "claude-code" {
		t.Fatalf("prioritized inner context = %#v", prioritizedInner)
	}

	identity := ResolveInvocationContext(baseOptions(map[string]string{"IDENTITY_NAME": "custom-runtime"}))
	if identity.CallerName != "custom-runtime" {
		t.Fatalf("identity caller = %q", identity.CallerName)
	}
}

func TestResolveInvocationContextSkillAndIDEValidation(t *testing.T) {
	invalidSkill := ResolveInvocationContext(baseOptions(map[string]string{
		"VE_SKILL_ID": "bad@skill/1.0.0", "CODEX_THREAD_ID": "thread",
	}))
	if invalidSkill.InvocationType != InvocationTypeDirect || invalidSkill.SkillName != UnknownValue {
		t.Fatalf("invalid skill context = %#v", invalidSkill)
	}

	invalidVersion := ResolveInvocationContext(baseOptions(map[string]string{"VE_SKILL_ID": "valid-skill/latest"}))
	if invalidVersion.InvocationType != InvocationTypeSkill || invalidVersion.SkillName != "valid-skill" || invalidVersion.SkillVersion != UnknownValue {
		t.Fatalf("invalid skill version context = %#v", invalidVersion)
	}

	ide := ResolveInvocationContext(baseOptions(map[string]string{"IDE_CLIENT_NAME": " windsurf ", "TERM_PROGRAM": "vscode"}))
	if ide.InvocationType != InvocationTypeIDEPlugin || ide.CallerName != "windsurf" {
		t.Fatalf("IDE context = %#v", ide)
	}

	terminalOnly := ResolveInvocationContext(baseOptions(map[string]string{"TERM_PROGRAM": "vscode"}))
	if terminalOnly.InvocationType != InvocationTypeUnknown {
		t.Fatalf("terminal-only context = %#v", terminalOnly)
	}
}

func TestResolveInvocationContextManualRequiresBothTTYs(t *testing.T) {
	options := baseOptions(map[string]string{})
	options.StdinIsTTY, options.StdoutIsTTY = boolPtr(true), boolPtr(true)
	if got := ResolveInvocationContext(options).InvocationType; got != InvocationTypeManual {
		t.Fatalf("both TTY invocation = %q", got)
	}
	options.StdoutIsTTY = boolPtr(false)
	if got := ResolveInvocationContext(options).InvocationType; got != InvocationTypeUnknown {
		t.Fatalf("partial TTY invocation = %q", got)
	}
}
