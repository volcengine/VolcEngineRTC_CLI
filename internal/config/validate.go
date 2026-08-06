// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
)

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// Severity classifies a validation finding.
type Severity string

const (
	SevError Severity = "error"
	SevWarn  Severity = "warn"
)

// Finding is a single parseable validation result keyed by field path.
type Finding struct {
	Field    string   `json:"field"`
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
	Hint     string   `json:"hint,omitempty"`
}

// Report is the outcome of validating a config.
type Report struct {
	OK       bool      `json:"ok"`
	Findings []Finding `json:"findings"`
}

// HasErrors reports whether any finding is an error.
func (r Report) HasErrors() bool {
	for _, f := range r.Findings {
		if f.Severity == SevError {
			return true
		}
	}
	return false
}

// CanBootstrapRTCAppCredentials reports whether dev can resolve the current
// config errors by interactively selecting an RTC application. The generated
// config deliberately keeps AppID behind RTC_APP_ID so AppID and AppKey can be
// written together on first run.
func CanBootstrapRTCAppCredentials(c *Config, report Report, force bool) bool {
	if c == nil || strings.TrimSpace(c.RTC.AppID) != "${RTC_APP_ID}" {
		return false
	}
	resolvedAppID, missing := ResolveEnv(c.RTC.AppID)
	appIDMissing := strings.TrimSpace(resolvedAppID) == "" || contains(missing, "RTC_APP_ID")
	appKeyMissing := strings.TrimSpace(os.Getenv("RTC_APP_KEY")) == ""
	if !force && !appIDMissing && !appKeyMissing {
		return false
	}
	for _, finding := range report.Findings {
		if finding.Severity == SevError && finding.Field != "rtc.app_id" {
			return false
		}
	}
	return true
}

// Err converts an errored report into a typed CLI error (first error finding).
func (r Report) Err() error {
	for _, f := range r.Findings {
		if f.Severity == SevError {
			e := errs.New("vertc.config.missing_field", errs.TypeValidation,
				"%s", f.Message).WithParam(f.Field)
			if strings.Contains(f.Message, "invalid") {
				e.Code = "vertc.config.invalid_field"
			} else if strings.Contains(f.Message, "environment variable") {
				e.Code = "vertc.config.unresolved_env"
			}
			if f.Hint != "" {
				e = e.WithHint("%s", f.Hint)
			}
			return e
		}
	}
	return nil
}

// requiredField declares a required config field and how to read its value.
type requiredField struct {
	path string
	get  func(*Config) string
}

var requiredFields = []requiredField{
	{"project.name", func(c *Config) string { return c.Project.Name }},
	{"project.scene", func(c *Config) string { return c.Project.Scene }},
	{"project.platform", func(c *Config) string { return c.Project.Platform }},
	{"rtc.app_id", func(c *Config) string { return c.RTC.AppID }},
	{"rtc.room_id", func(c *Config) string { return c.RTC.RoomID }},
	{"rtc.user_id", func(c *Config) string { return c.RTC.UserID }},
}

// Validate checks required fields, types, and `${ENV}` resolvability.
func Validate(c *Config) Report {
	var findings []Finding

	if c.Version == 0 {
		findings = append(findings, Finding{
			Field: "version", Severity: SevWarn,
			Message: "version is unset", Hint: fmt.Sprintf("set version: %d", SchemaVersion),
		})
	} else if c.Version > SchemaVersion {
		findings = append(findings, Finding{
			Field: "version", Severity: SevWarn,
			Message: fmt.Sprintf("config version %d is newer than supported %d", c.Version, SchemaVersion),
			Hint:    "upgrade the CLI",
		})
	}

	for _, rf := range requiredFields {
		val := strings.TrimSpace(rf.get(c))
		if val == "" {
			findings = append(findings, Finding{
				Field: rf.path, Severity: SevError,
				Message: fmt.Sprintf("required field %s is missing", rf.path),
				Hint:    fmt.Sprintf("set %s in the config", rf.path),
			})
			continue
		}
		// `${ENV}` resolvability + non-empty resolution.
		resolved, missing := ResolveEnv(val)
		if len(missing) > 0 {
			findings = append(findings, Finding{
				Field: rf.path, Severity: SevError,
				Message: fmt.Sprintf("%s references unset environment variable %s", rf.path, strings.Join(missing, ", ")),
				Hint:    fmt.Sprintf("export %s or set them in local .env.local; never pass secrets in command arguments", strings.Join(missing, ", ")),
			})
		} else if strings.TrimSpace(resolved) == "" {
			findings = append(findings, Finding{
				Field: rf.path, Severity: SevError,
				Message: fmt.Sprintf("%s resolves to an empty value", rf.path),
				Hint:    fmt.Sprintf("set a non-empty value for %s (or its ${ENV})", rf.path),
			})
		}
	}

	if c.Project.Scene == "voice-agent" {
		findings = append(findings, validateAgent(c)...)
	}

	return Report{OK: len(findings) == 0 || !anyError(findings), Findings: findings}
}

// validateAgent checks that a voice-agent project references its external AI
// configuration and that legacy CLI-managed identity fields remain coherent.
// The config file's existence and JSON validity are doctor checks because the
// effective default depends on the taskfile runtime version.
func validateAgent(c *Config) []Finding {
	var f []Finding
	if source := strings.TrimSpace(c.Agent.ConfigSource); source != "" &&
		source != AgentConfigSourceDefault && source != AgentConfigSourceBot {
		f = append(f, Finding{Field: "agent.config_source", Severity: SevError,
			Message: fmt.Sprintf("agent.config_source %q is not supported", source),
			Hint:    "use template-default or console-bot"})
	} else if source == "" {
		f = append(f, Finding{Field: "agent.config_source", Severity: SevWarn,
			Message: "agent scene selection has not been resolved by dev",
			Hint:    "run dev to select an existing bot or confirm the built-in default scene"})
	}
	if strings.TrimSpace(c.Agent.ConfigFile) == "" {
		f = append(f, Finding{Field: "agent.config_file", Severity: SevWarn,
			Message: "agent.config_file is unset; the runtime-specific default will be used",
			Hint:    "set agent.config_file explicitly; generated projects use server/scenes/default.json"})
	}
	// Consistency: the agent's target must match the web user, else it talks to no one.
	if c.Agent.TargetUserID != "" && c.RTC.UserID != "" && c.Agent.TargetUserID != c.RTC.UserID {
		f = append(f, Finding{Field: "agent.target_user_id", Severity: SevWarn,
			Message: fmt.Sprintf("agent.target_user_id (%s) != rtc.user_id (%s)", c.Agent.TargetUserID, c.RTC.UserID),
			Hint:    "align agent.target_user_id with the web user's rtc.user_id"})
	}
	return f
}

func anyError(fs []Finding) bool {
	for _, f := range fs {
		if f.Severity == SevError {
			return true
		}
	}
	return false
}
