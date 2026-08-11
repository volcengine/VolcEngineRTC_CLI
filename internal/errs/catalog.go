// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package errs

import (
	"regexp"
	"sort"
	"strings"
)

// CatalogEntry documents one stable error.code.
type CatalogEntry struct {
	Code    string `json:"code"`
	Type    Type   `json:"type"`
	Summary string `json:"summary"`
}

// codePattern enforces the `vertc.<domain>.<subtype>` shape. The CI check
// (scripts/snapshot-error-codes) asserts every code emitted by the binary is
// registered here and matches this pattern.
var codePattern = regexp.MustCompile(`^vertc\.[a-z][a-z0-9_]*\.[a-z][a-z0-9_]*$`)

// Catalog is the converged registry of every CLI-owned error.code.
// Adding or renaming a code REQUIRES a matching entry here; the CI snapshot
// check fails otherwise.
var Catalog = []CatalogEntry{
	// --- cli / foundation ---
	{"vertc.cli.unknown_command", TypeNotFound, "Subcommand does not exist; hint suggests nearest candidate"},
	{"vertc.cli.invalid_flag", TypeValidation, "A flag received an invalid value"},
	{"vertc.cli.internal", TypeInternal, "Unexpected internal CLI failure"},
	{"vertc.cli.not_implemented", TypePrecondition, "Feature is not available"},

	// --- update lifecycle ---
	{"vertc.update.network", TypeIO, "Failed to query npm for the latest CLI release"},
	{"vertc.update.detect_failed", TypeIO, "Failed to detect how the running CLI was installed"},
	{"vertc.update.manual_install", TypePrecondition, "Self-update refused to overwrite a manually installed binary"},
	{"vertc.update.failed", TypeIO, "npm failed to install the requested CLI release"},
	{"vertc.update.verify_failed", TypeIO, "The updated CLI binary did not report the expected version"},

	// --- auth ---
	{"vertc.auth.not_authenticated", TypeAuth, "Command requires authentication; run auth login"},
	{"vertc.auth.authorization_failed", TypeAuth, "Signin authorization failed or callback was invalid"},
	{"vertc.auth.interaction_required", TypePrecondition, "Signin requires an explicit browser choice in a non-interactive terminal"},
	{"vertc.auth.invalid_callback", TypeValidation, "OAuth loopback callback configuration is invalid"},
	{"vertc.auth.invalid_token", TypeAuth, "Stored auth token is missing or cannot be used"},
	{"vertc.auth.storage_unavailable", TypeAuth, "Selected credential storage is unavailable or unsafe"},
	{"vertc.auth.token_exchange_failed", TypeAuth, "OAuth token endpoint exchange or refresh failed"},

	// --- config ---
	{"vertc.config.not_found", TypeNotFound, "vertc.config.yaml not found in the current project"},
	{"vertc.config.parse_error", TypeValidation, "vertc.config.yaml could not be parsed"},
	{"vertc.config.missing_field", TypeValidation, "A required config field is missing"},
	{"vertc.config.invalid_field", TypeValidation, "A config field has an invalid type or value"},
	{"vertc.config.unresolved_env", TypeValidation, "A ${ENV} placeholder cannot be resolved"},
	{"vertc.config.write_failed", TypeIO, "Failed to persist vertc.config.yaml"},
	{"vertc.config.invalid_identity", TypeValidation, "room_id/user_id is empty, too long, or has disallowed characters"},

	// --- token ---
	{"vertc.token.missing_credential", TypeAuth, "AppKey (or other credential) is not available"},
	{"vertc.token.missing_config", TypeValidation, "app_id/room_id/user_id missing for token issue"},
	{"vertc.token.invalid_appid", TypeValidation, "AppID is not the required 24-character length"},
	{"vertc.token.invalid", TypeValidation, "Token failed to parse or verify"},
	{"vertc.token.expired", TypeValidation, "Token is expired"},

	// --- scaffolding / template ---
	{"vertc.template.not_found", TypeNotFound, "No template for the requested scene x platform"},
	{"vertc.template.render_failed", TypeIO, "Template rendering failed"},
	{"vertc.template.download_failed", TypeIO, "Remote template archive could not be downloaded or cached"},
	{"vertc.template.checksum_mismatch", TypeValidation, "Remote template archive did not match its pinned SHA-256"},
	{"vertc.template.contract_invalid", TypeValidation, "Remote template archive or manifest violates the template contract"},
	{"vertc.init.target_exists", TypePrecondition, "Target directory already exists and is not empty"},

	// --- env ---
	{"vertc.env.write_failed", TypeIO, "Failed to write environment file"},

	// --- doctor ---
	{"vertc.doctor.failed", TypePrecondition, "One or more doctor checks reported FAIL"},

	// --- explain-error ---
	{"vertc.explain.unknown_code", TypeNotFound, "Error code is not in the offline knowledge base"},

	// --- OpenAPI ---
	{"vertc.openapi.http_error", TypePrecondition, "OpenAPI endpoint returned a non-success HTTP status"},
	{"vertc.openapi.invalid_request", TypeValidation, "OpenAPI request flags or payload are invalid"},
	{"vertc.openapi.request_failed", TypeIO, "OpenAPI transport/HTTP call failed"},
	{"vertc.openapi.sign_failed", TypeValidation, "OpenAPI request signing failed"},

	// --- dev ---
	{"vertc.dev.config_incomplete", TypePrecondition, "Config incomplete; run doctor before dev"},
	{"vertc.dev.app_setup_failed", TypePrecondition, "RTC application discovery or credential setup failed"},
	{"vertc.dev.port_in_use", TypePrecondition, "A template development port is already occupied"},
	{"vertc.dev.task_failed", TypeIO, "Template dev task exited non-zero"},

	// --- conversational agent (StartVoiceChat / UpdateVoiceChat / StopVoiceChat) ---
	{"vertc.agent.missing_config", TypeValidation, "Required agent (asr/tts/llm) config is missing"},
	{"vertc.agent.no_task", TypePrecondition, "No running agent task for stop/update/status"},
	{"vertc.agent.server_managed", TypePrecondition, "Agent lifecycle is owned by the generated companion server"},

	{"vertc.openapi.api_error", TypePrecondition, "Volcengine OpenAPI returned an error code"},

	// --- embedded skills (skills list/read) ---
	{"vertc.skills.unavailable", TypeInternal, "Skill content is not embedded in this build"},
	{"vertc.skills.unknown_skill", TypeNotFound, "Requested skill is not an official embedded skill"},
	{"vertc.skills.invalid_path", TypeValidation, "Skill reference path is absolute or escapes the skill root"},
	{"vertc.skills.not_found", TypeNotFound, "Requested skill reference does not exist"},
	{"vertc.skills.sync_failed", TypeIO, "Official embedded Skills could not be synchronized to Agent runtimes"},

	// --- interactive dev setup ---
	{"vertc.dev.interactive_required", TypePrecondition, "Deprecated: interactive dev setup requires a public resource selection; use vertc.dev.selection_required"},
	{"vertc.dev.selection_required", TypePrecondition, "Dev setup requires a public resource selection"},
	{"vertc.provision.no_app", TypeNotFound, "No active RTC app is available for interactive setup"},
	{"vertc.provision.no_bot", TypeNotFound, "No usable conversational-AI bot is available for scene setup"},
	{"vertc.provision.not_selected", TypeValidation, "Interactive bot selection was not completed"},
	{"vertc.provision.write_failed", TypeIO, "Failed to validate or write a generated bot scene"},
}

var catalogIndex = func() map[string]CatalogEntry {
	m := make(map[string]CatalogEntry, len(Catalog))
	for _, e := range Catalog {
		m[e.Code] = e
	}
	return m
}()

// IsRegistered reports whether code exists in the catalog.
func IsRegistered(code string) bool {
	_, ok := catalogIndex[code]
	return ok
}

// ValidateCatalog checks every entry matches the code pattern and is unique.
// Returns a sorted list of problems (empty when healthy). Used by the CI check.
func ValidateCatalog() []string {
	var problems []string
	seen := map[string]bool{}
	for _, e := range Catalog {
		if !codePattern.MatchString(e.Code) {
			problems = append(problems, "invalid code shape: "+e.Code)
		}
		if seen[e.Code] {
			problems = append(problems, "duplicate code: "+e.Code)
		}
		seen[e.Code] = true
		if strings.TrimSpace(e.Summary) == "" {
			problems = append(problems, "missing summary: "+e.Code)
		}
	}
	sort.Strings(problems)
	return problems
}

func subtypeOf(code string) string {
	parts := strings.Split(code, ".")
	if len(parts) >= 3 {
		return parts[len(parts)-1]
	}
	return ""
}
