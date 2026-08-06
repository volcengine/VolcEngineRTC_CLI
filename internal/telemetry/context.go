// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

// Package telemetry builds the process-scoped invocation User-Agent used by
// first-party RTC OpenAPI requests.
package telemetry

import (
	"os"
	"regexp"
	"strings"

	"golang.org/x/term"
)

const UnknownValue = "unknown"

type InvocationType string

const (
	InvocationTypeSkill     InvocationType = "skill"
	InvocationTypeDirect    InvocationType = "direct"
	InvocationTypeIDEPlugin InvocationType = "ide-plugin"
	InvocationTypeManual    InvocationType = "manual"
	InvocationTypeUnknown   InvocationType = "unknown"
)

type InvocationContext struct {
	CLIName        string
	CLIVersion     string
	InvocationType InvocationType
	CallerName     string
	SkillName      string
	SkillVersion   string
}

type ResolveInvocationContextOptions struct {
	CLIName     string
	CLIVersion  string
	Env         map[string]string
	StdinIsTTY  *bool
	StdoutIsTTY *bool
}

const (
	maxNameLength    = 128
	maxVersionLength = 32
	maxSkillIDLength = maxNameLength + 1 + maxVersionLength
)

var (
	stableNamePattern        = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	majorMinorVersionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
	semverPattern            = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-(?:(?:0|[1-9][0-9]*)|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:(?:0|[1-9][0-9]*)|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*))*)?$`)
)

type skillIdentity struct {
	name    string
	version string
}

func processEnvironment() map[string]string {
	env := make(map[string]string)
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			env[key] = value
		}
	}
	return env
}

func resolveTTY(value *bool, file *os.File) bool {
	if value != nil {
		return *value
	}
	return term.IsTerminal(int(file.Fd()))
}

func hasNonEmptyValue(env map[string]string, key string) bool {
	value, ok := env[key]
	return ok && strings.TrimSpace(value) != ""
}

func hasAnyNonEmptyValue(env map[string]string, keys ...string) bool {
	for _, key := range keys {
		if hasNonEmptyValue(env, key) {
			return true
		}
	}
	return false
}

func hasNonEmptyPrefix(env map[string]string, prefix string) bool {
	for key, value := range env {
		if strings.HasPrefix(key, prefix) && strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

func hasValueIgnoreCase(env map[string]string, key, expected string) bool {
	return strings.EqualFold(strings.TrimSpace(env[key]), expected)
}

func normalizeStableName(value string) string {
	if value == "" || len(value) > maxNameLength || !stableNamePattern.MatchString(value) {
		return ""
	}
	return value
}

func normalizeSkillID(value string) skillIdentity {
	if value == "" || len(value) > maxSkillIDLength {
		return skillIdentity{}
	}
	parts := strings.Split(value, "/")
	if len(parts) > 2 {
		return skillIdentity{}
	}
	name := normalizeStableName(parts[0])
	if name == "" {
		return skillIdentity{}
	}
	result := skillIdentity{name: name}
	if len(parts) == 2 && isValidSkillVersion(parts[1]) {
		result.version = parts[1]
	}
	return result
}

func unambiguous(candidates []string) string {
	unique := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		unique[candidate] = struct{}{}
	}
	if len(unique) != 1 {
		return ""
	}
	for candidate := range unique {
		return candidate
	}
	return ""
}

func detectOuterRuntime(env map[string]string) string {
	var candidates []string
	if hasNonEmptyPrefix(env, "OPENCLAW_") {
		candidates = append(candidates, "openclaw")
	}
	if hasNonEmptyPrefix(env, "ARKCLAW_") {
		candidates = append(candidates, "arkclaw")
	}
	if hasNonEmptyPrefix(env, "HERMES_") {
		candidates = append(candidates, "hermes")
	}
	if _, ok := env["COZE_CLAW_AGENT_ID"]; ok {
		candidates = append(candidates, "coze-claw")
	}
	return unambiguous(candidates)
}

func detectInnerAIClient(env map[string]string) string {
	if custom := normalizeStableName(strings.TrimSpace(env["AI_AGENT"])); custom != "" {
		return custom
	}
	if hasValueIgnoreCase(env, "AI_AGENT", "trae") {
		return "trae"
	}

	var candidates []string
	if hasAnyNonEmptyValue(env, "TRAE_CLI_PLUGIN_ROOT", "COCO_PLUGIN_ROOT") || hasValueIgnoreCase(env, "ICUBE_PRODUCT_BRAND_NAME", "trae") {
		candidates = append(candidates, "trae")
	}
	hasClaude := hasAnyNonEmptyValue(env, "CLAUDECODE", "CLAUDE_CODE")
	if hasClaude && hasNonEmptyValue(env, "CLAUDE_CODE_IS_COWORK") {
		candidates = append(candidates, "claude-cowork")
	} else if hasClaude {
		candidates = append(candidates, "claude-code")
	}
	if hasAnyNonEmptyValue(env, "CODEX_THREAD_ID", "CODEX_SANDBOX", "CODEX_CI") {
		candidates = append(candidates, "codex")
	}
	if hasNonEmptyValue(env, "CURSOR_TRACE_ID") || env["CURSOR_AGENT"] == "1" || env["CURSOR_EXTENSION_HOST_ROLE"] == "agent-exec" {
		candidates = append(candidates, "cursor")
	}
	if hasNonEmptyValue(env, "GEMINI_CLI") {
		candidates = append(candidates, "gemini-cli")
	}
	if hasAnyNonEmptyValue(env, "KIRO_SESSION_ID", "KIRO_AGENT_PATH") {
		candidates = append(candidates, "kiro")
	}
	if hasAnyNonEmptyValue(env, "OPENCODE", "OPENCODE_CLIENT") {
		candidates = append(candidates, "opencode")
	}
	if hasNonEmptyValue(env, "ANTIGRAVITY_AGENT") {
		candidates = append(candidates, "antigravity")
	}
	if hasAnyNonEmptyValue(env, "COPILOT_CLI", "COPILOT_MODEL", "COPILOT_ALLOW_ALL") {
		candidates = append(candidates, "github-copilot")
	}
	if hasNonEmptyValue(env, "CLINE_ACTIVE") {
		candidates = append(candidates, "cline")
	}
	if hasNonEmptyValue(env, "AMP_CURRENT_THREAD_ID") {
		candidates = append(candidates, "amp")
	}
	if env["PI_CODING_AGENT"] == "true" {
		candidates = append(candidates, "pi")
	}
	if hasNonEmptyValue(env, "REPL_ID") {
		candidates = append(candidates, "replit")
	}
	if hasNonEmptyValue(env, "AUGMENT_AGENT") {
		candidates = append(candidates, "augment")
	}
	if hasNonEmptyValue(env, "QWEN_CODE") {
		candidates = append(candidates, "qwen-code")
	}
	if len(candidates) > 0 {
		return unambiguous(candidates)
	}
	if hasNonEmptyValue(env, "COPILOT_GITHUB_TOKEN") {
		return "github-copilot"
	}
	return ""
}

func resolveAICaller(env map[string]string) string {
	outer, inner := detectOuterRuntime(env), detectInnerAIClient(env)
	if outer == inner {
		inner = ""
	}
	return strings.Join(removeEmpty(outer, inner), ",")
}

func removeEmpty(values ...string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}

func ResolveInvocationContext(options ResolveInvocationContextOptions) InvocationContext {
	env := options.Env
	if env == nil {
		env = processEnvironment()
	}
	skill := normalizeSkillID(env["VE_SKILL_ID"])
	ide := normalizeStableName(strings.TrimSpace(env["IDE_CLIENT_NAME"]))
	caller := resolveAICaller(env)

	invocationType := InvocationTypeUnknown
	callerName := UnknownValue
	switch {
	case skill.name != "":
		invocationType = InvocationTypeSkill
		if caller != "" {
			callerName = caller
		}
	case ide != "":
		invocationType = InvocationTypeIDEPlugin
		callerName = ide
	case caller != "":
		invocationType = InvocationTypeDirect
		callerName = caller
	case resolveTTY(options.StdinIsTTY, os.Stdin) && resolveTTY(options.StdoutIsTTY, os.Stdout):
		invocationType = InvocationTypeManual
	}

	version := UnknownValue
	if isValidVersion(options.CLIVersion) {
		version = options.CLIVersion
	}
	skillName, skillVersion := UnknownValue, UnknownValue
	if skill.name != "" {
		skillName = skill.name
	}
	if skill.version != "" {
		skillVersion = skill.version
	}
	return InvocationContext{
		CLIName: options.CLIName, CLIVersion: version, InvocationType: invocationType,
		CallerName: callerName, SkillName: skillName, SkillVersion: skillVersion,
	}
}
