// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

// Package telemetry builds the process-scoped invocation User-Agent used by
// first-party RTC OpenAPI requests.
package telemetry

import (
	"crypto/rand"
	"fmt"
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
	InvocationID   string
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
	callerNamePattern        = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	majorMinorVersionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
	semverPattern            = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-(?:(?:0|[1-9][0-9]*)|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:(?:0|[1-9][0-9]*)|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*))*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)
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

func hasKeyPrefix(env map[string]string, prefix string) bool {
	for key := range env {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

func hasValueIgnoreCase(env map[string]string, key, expected string) bool {
	return strings.EqualFold(strings.TrimSpace(env[key]), expected)
}

func hasValueContainingIgnoreCase(env map[string]string, key, expected string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(env[key])), strings.ToLower(expected))
}

func normalizeStableName(value string) string {
	if value == "" || len(value) > maxNameLength || !stableNamePattern.MatchString(value) {
		return ""
	}
	return value
}

func normalizeCallerName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > maxNameLength || !callerNamePattern.MatchString(value) {
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

func detectOuterRuntime(env map[string]string) string {
	if hasNonEmptyPrefix(env, "OPENCLAW_") {
		return "openclaw"
	}
	if hasNonEmptyPrefix(env, "ARKCLAW_") || hasValueContainingIgnoreCase(env, "IDENTITY_NAME", "arkclaw") {
		return "arkclaw"
	}
	if hasNonEmptyPrefix(env, "HERMES_") {
		return "hermes"
	}
	if _, ok := env["COZE_CLAW_AGENT_ID"]; ok {
		return "coze-claw"
	}
	return ""
}

func detectInnerAIClient(env map[string]string) string {
	if hasValueIgnoreCase(env, "AI_AGENT", "trae") {
		return "trae"
	}
	if custom := normalizeCallerName(env["AI_AGENT"]); custom != "" {
		return custom
	}
	if hasKeyPrefix(env, "DOUBAO_OFFICE_") {
		return "doubao-office"
	}

	hasClaude := hasAnyNonEmptyValue(env, "CLAUDECODE", "CLAUDE_CODE")
	if hasClaude && hasNonEmptyValue(env, "CLAUDE_CODE_IS_COWORK") {
		return "claude-cowork"
	}
	if hasClaude {
		return "claude-code"
	}
	if hasNonEmptyPrefix(env, "CODEX_") {
		return "codex"
	}
	if hasNonEmptyPrefix(env, "CURSOR_") {
		return "cursor"
	}
	if hasNonEmptyPrefix(env, "TRAE_") || hasNonEmptyValue(env, "COCO_PLUGIN_ROOT") || hasValueIgnoreCase(env, "ICUBE_PRODUCT_BRAND_NAME", "trae") {
		return "trae"
	}
	if hasNonEmptyValue(env, "GEMINI_CLI") {
		return "gemini-cli"
	}
	if hasAnyNonEmptyValue(env, "KIRO_SESSION_ID", "KIRO_AGENT_PATH") {
		return "kiro"
	}
	if hasAnyNonEmptyValue(env, "OPENCODE", "OPENCODE_CLIENT") {
		return "opencode"
	}
	if hasNonEmptyValue(env, "ANTIGRAVITY_AGENT") {
		return "antigravity"
	}
	if hasAnyNonEmptyValue(env, "COPILOT_CLI", "COPILOT_MODEL", "COPILOT_ALLOW_ALL") {
		return "github-copilot"
	}
	if hasNonEmptyValue(env, "CLINE_ACTIVE") {
		return "cline"
	}
	if hasNonEmptyValue(env, "AMP_CURRENT_THREAD_ID") {
		return "amp"
	}
	if env["PI_CODING_AGENT"] == "true" {
		return "pi"
	}
	if hasNonEmptyValue(env, "REPL_ID") {
		return "replit"
	}
	if hasNonEmptyValue(env, "AUGMENT_AGENT") {
		return "augment"
	}
	if hasNonEmptyValue(env, "QWEN_CODE") {
		return "qwen-code"
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
	if caller := strings.Join(removeEmpty(outer, inner), ","); caller != "" {
		return caller
	}
	return normalizeStableName(strings.TrimSpace(env["IDENTITY_NAME"]))
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

func newInvocationID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return UnknownValue
	}
	value[6] = value[6]&0x0f | 0x40
	value[8] = value[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", value[:4], value[4:6], value[6:8], value[8:10], value[10:])
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
		CLIName: options.CLIName, CLIVersion: version, InvocationID: newInvocationID(), InvocationType: invocationType,
		CallerName: callerName, SkillName: skillName, SkillVersion: skillVersion,
	}
}
