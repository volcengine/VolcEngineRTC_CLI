// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

// Package doctor implements two-level readiness diagnostics:
// CLI self-check (runtime/version/network/auth) and project readiness
// (permissions/resources/SDK/Token/config). Each check yields PASS/WARN/FAIL
// with an actionable hint; any FAIL makes the command exit non-zero.
package doctor

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/auth"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/config"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/meta"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/template"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/token"
	updatecheck "github.com/volcengine/VolcEngineRTC_CLI/internal/update"
)

// Status is a per-check verdict.
type Status string

const (
	PASS    Status = "PASS"
	WARN    Status = "WARN"
	FAIL    Status = "FAIL"
	SKIP    Status = "SKIP"
	UNKNOWN Status = "UNKNOWN"
)

// Level groups checks into the two diagnostic tiers.
type Level string

const (
	LevelCLI     Level = "cli"
	LevelProject Level = "project"
)

var (
	authRefreshSkew   = 15 * time.Minute
	authTokenEndpoint = auth.DefaultTokenEndpoint
)

// Check is a single structured diagnostic result.
type Check struct {
	ID     string `json:"id"`
	Level  Level  `json:"level"`
	Title  string `json:"title"`
	Status Status `json:"status"`
	Detail string `json:"detail"`
	Hint   string `json:"hint,omitempty"`
}

// Report aggregates checks and the overall verdict.
type Report struct {
	Checks   []Check `json:"checks"`
	Passed   int     `json:"passed"`
	Warned   int     `json:"warned"`
	Failed   int     `json:"failed"`
	Skipped  int     `json:"skipped"`
	Unknown  int     `json:"unknown"`
	Runnable bool    `json:"runnable"`
}

// summarize computes counts and the overall verdict.
func summarize(checks []Check) Report {
	r := Report{Checks: checks}
	for _, c := range checks {
		switch c.Status {
		case PASS:
			r.Passed++
		case WARN:
			r.Warned++
		case FAIL:
			r.Failed++
		case SKIP:
			r.Skipped++
		case UNKNOWN:
			r.Unknown++
		}
	}
	r.Runnable = r.Failed == 0
	return r
}

// RunCLI runs the CLI self-check tier.
func RunCLI() []Check {
	var checks []Check

	// Runtime (Go binary is always present when this runs).
	checks = append(checks, Check{
		ID: "cli.runtime", Level: LevelCLI, Title: "CLI runtime",
		Status: PASS, Detail: runtime.GOOS + "/" + runtime.GOARCH + " · go " + runtime.Version(),
	})

	// Version.
	checks = append(checks, Check{
		ID: "cli.version", Level: LevelCLI, Title: "CLI version",
		Status: PASS, Detail: meta.BinName + " " + meta.Version,
	})

	evidence := updatecheck.CachedEvidence(meta.Version)
	if info := evidence.Info; evidence.Status == updatecheck.CacheAvailable && info != nil {
		checks = append(checks, Check{
			ID: "cli.update", Level: LevelCLI, Title: "CLI update",
			Status: WARN, Detail: info.Message(), Hint: "run `" + meta.BinName + " update`",
		})
	} else if evidence.Status == updatecheck.CacheCurrent {
		checks = append(checks, Check{
			ID: "cli.update", Level: LevelCLI, Title: "CLI update",
			Status: PASS, Detail: "up to date (fresh registry cache)",
		})
	} else if evidence.Status == updatecheck.CacheSkipped {
		checks = append(checks, Check{ID: "cli.update", Level: LevelCLI, Title: "CLI update",
			Status: SKIP, Detail: "update check disabled for this environment"})
	} else {
		checks = append(checks, Check{ID: "cli.update", Level: LevelCLI, Title: "CLI update",
			Status: UNKNOWN, Detail: "no fresh update check is available",
			Hint: "run `" + meta.BinName + " update --check` when online"})
	}

	// Network: is a node toolchain reachable (needed by web templates)?
	if _, err := exec.LookPath("node"); err != nil {
		checks = append(checks, Check{
			ID: "cli.toolchain", Level: LevelCLI, Title: "Node toolchain",
			Status: WARN, Detail: "node not found on PATH",
			Hint: "install Node.js to run the web scaffold (`vertc dev`)",
		})
	} else {
		checks = append(checks, Check{
			ID: "cli.toolchain", Level: LevelCLI, Title: "Node toolchain",
			Status: PASS, Detail: "node found on PATH",
		})
	}

	// Auth (local auth state).
	if authed() {
		checks = append(checks, Check{
			ID: "cli.auth", Level: LevelCLI, Title: "Authentication",
			Status: PASS, Detail: "authenticated (Signin token)",
		})
	} else {
		checks = append(checks, Check{
			ID: "cli.auth", Level: LevelCLI, Title: "Authentication",
			Status: WARN, Detail: "not authenticated",
			Hint: "run `" + meta.BinName + " auth login` before Console/OpenAPI-backed commands",
		})
	}

	return checks
}

// RunProject runs the project-readiness tier against the config at path.
// dir is the project directory (for taskfile/SDK checks).
func RunProject(cfg *config.Config, path, dir string) []Check {
	var checks []Check

	// Config presence + completeness.
	if cfg == nil {
		checks = append(checks, Check{
			ID: "project.config", Level: LevelProject, Title: "Config present",
			Status: FAIL, Detail: meta.ConfigFileName + " not found",
			Hint: "run `" + meta.BinName + " init --scene voice-agent --platform web`",
		})
		return checks
	}
	rep := config.Validate(cfg)
	credentialBootstrapAvailable := config.CanBootstrapRTCAppCredentials(cfg, rep, false)
	if rep.HasErrors() {
		for _, f := range rep.Findings {
			if f.Severity != config.SevError {
				continue
			}
			status, hint := FAIL, f.Hint
			if credentialBootstrapAvailable && f.Field == "rtc.app_id" {
				status = WARN
				hint = "run `" + meta.BinName + " auth login`, then `" + meta.BinName + " dev`; first-run setup selects an RTC application and fills RTC_APP_ID"
			}
			checks = append(checks, Check{
				ID: "project.config." + f.Field, Level: LevelProject,
				Title: "Config: " + f.Field, Status: status, Detail: f.Message, Hint: hint,
			})
		}
	} else {
		checks = append(checks, Check{
			ID: "project.config", Level: LevelProject, Title: "Config valid",
			Status: PASS, Detail: "required fields present and ${ENV} resolvable",
		})
	}

	// Credential (AppKey is env-only).
	if os.Getenv("RTC_APP_KEY") == "" {
		status := FAIL
		detail := "RTC_APP_KEY is not set"
		hint := "run `" + meta.BinName + " auth login`, then `" + meta.BinName + " dev`; if selection is required, choose a public ID from error.details; edit .env.local locally for manual secret setup"
		if credentialBootstrapAvailable {
			status = WARN
			detail = "RTC_APP_KEY is not set; first-run dev setup will fill it"
			hint = "run `" + meta.BinName + " auth login`, then `" + meta.BinName + " dev`"
		}
		checks = append(checks, Check{
			ID: "project.credential", Level: LevelProject, Title: "AppKey credential",
			Status: status, Detail: detail, Hint: hint,
		})
	} else {
		checks = append(checks, Check{
			ID: "project.credential", Level: LevelProject, Title: "AppKey credential",
			Status: PASS, Detail: "RTC_APP_KEY is set",
		})
	}

	serverManaged := cfg.Project.Scene == "voice-agent" && serverManagedAgent(dir)

	// Scaffold files present (folded in from the former `test smoke`): a
	// generated project must keep its runnable entrypoints; missing files mean
	// the scaffold was deleted or never generated.
	checks = append(checks, scaffoldFilesCheck(dir, serverManaged))

	// Full-stack projects issue a user token for each server-owned session.
	// Legacy projects retain the explicit `token issue --write` readiness gate.
	if serverManaged {
		checks = append(checks, Check{
			ID: "project.token", Level: LevelProject, Title: "RTC Token",
			Status: PASS, Detail: "issued by the companion server for each session",
		})
	} else {
		checks = append(checks, tokenCheck(cfg))
	}

	// Conversational-AI (voice-agent) readiness.
	if cfg.Project.Scene == "voice-agent" {
		if serverManaged {
			checks = append(checks, serverManagedAgentChecks(cfg, dir)...)
		} else {
			checks = append(checks, agentChecks(cfg, dir)...)
		}
	}

	// SDK version pinned in the taskfile.
	if tf, err := loadSDK(dir); err == nil && tf != "" {
		checks = append(checks, Check{
			ID: "project.sdk", Level: LevelProject, Title: "SDK pinned",
			Status: PASS, Detail: "SDK pinned at " + tf,
		})
	} else {
		checks = append(checks, Check{
			ID: "project.sdk", Level: LevelProject, Title: "SDK pinned",
			Status: WARN, Detail: "no taskfile SDK pin found",
			Hint: "generate the project with `" + meta.BinName + " init`",
		})
	}

	return checks
}

func serverManagedAgent(dir string) bool {
	tf, err := template.LoadTaskfile(dir)
	return err == nil && tf.ServerManagedAgent()
}

// scaffoldFilesCheck verifies the core runnable files a generated project ships.
// The set mirrors the full-stack (server-managed) and legacy layouts.
func scaffoldFilesCheck(dir string, serverManaged bool) Check {
	coreFiles := []string{"package.json", "src/main.js"}
	if serverManaged {
		coreFiles = []string{
			"package.json",
			"web/package.json",
			"web/src/App.tsx",
			"server/package.json",
			"server/app.js",
		}
	}
	var missing []string
	for _, f := range coreFiles {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			missing = append(missing, f)
		}
	}
	if len(missing) > 0 {
		return Check{
			ID: "project.files", Level: LevelProject, Title: "Scaffold files",
			Status: FAIL, Detail: "missing scaffold files: " + strings.Join(missing, ", "),
			Hint: "regenerate with `" + meta.BinName + " init`",
		}
	}
	return Check{
		ID: "project.files", Level: LevelProject, Title: "Scaffold files",
		Status: PASS, Detail: "core runnable files present",
	}
}

// serverManagedAgentChecks validate the inputs consumed by the generated
// companion server rather than the legacy console-template/agent CLI flow.
func serverManagedAgentChecks(cfg *config.Config, dir string) []Check {
	return []Check{
		serverAgentAuthCheck(),
		serverAgentConfigCheck(cfg, dir),
	}
}

func serverAgentAuthCheck() Check {
	accessKey := os.Getenv("VOLCENGINE_ACCESS_KEY_ID")
	secretKey := os.Getenv("VOLCENGINE_SECRET_ACCESS_KEY")
	if accessKey != "" && secretKey != "" {
		return Check{
			ID: "project.agent.auth", Level: LevelProject, Title: "Server OpenAPI auth",
			Status: PASS, Detail: "server AK/SK credential available",
		}
	}
	if accessKey != "" || secretKey != "" {
		return Check{
			ID: "project.agent.auth", Level: LevelProject, Title: "Server OpenAPI auth",
			Status: FAIL, Detail: "server AK/SK credential is incomplete",
			Hint: "set VOLCENGINE_ACCESS_KEY_ID and VOLCENGINE_SECRET_ACCESS_KEY together",
		}
	}
	check := agentAuthCheck()
	check.Title = "Server OpenAPI auth"
	if check.Status == PASS {
		check.Detail = "Signin credential available for CLI-backed agent start/stop"
	} else {
		check.Hint = "run `" + meta.BinName + " auth login`, or set server AK/SK environment credentials"
	}
	return check
}

func serverAgentConfigCheck(cfg *config.Config, dir string) Check {
	cfgFile := cfg.Agent.ConfigFile
	if cfgFile == "" {
		cfgFile = "server/scenes/default.json"
	}
	base := Check{ID: "project.agent.config", Level: LevelProject, Title: "Server agent config"}
	raw, err := os.ReadFile(filepath.Join(dir, cfgFile))
	if err != nil {
		base.Status, base.Detail = FAIL, cfgFile+" not found"
		base.Hint = "restore the generated server AI configuration"
		return base
	}
	if err := config.ValidateServerAgentConfigJSON(raw); err != nil {
		base.Status, base.Detail = FAIL, cfgFile+": "+err.Error()
		base.Hint = "fix the server AI configuration before starting the demo"
		return base
	}
	base.Status, base.Detail = PASS, cfgFile+" valid"
	return base
}

// agentChecks are the voice-agent (conversational-AI) readiness checks: Signin
// auth, the console template file, and user/target consistency.
func agentChecks(cfg *config.Config, dir string) []Check {
	var checks []Check

	checks = append(checks, agentAuthCheck())

	// Console template: exists, valid JSON, non-empty Config.
	checks = append(checks, agentTemplateCheck(cfg, dir))

	// The agent must target the web user, else it talks to no one.
	if cfg.Agent.TargetUserID != "" && cfg.RTC.UserID != "" && cfg.Agent.TargetUserID != cfg.RTC.UserID {
		checks = append(checks, Check{
			ID: "project.agent.target", Level: LevelProject, Title: "Agent target user",
			Status: WARN, Detail: "agent.target_user_id (" + cfg.Agent.TargetUserID + ") != rtc.user_id (" + cfg.RTC.UserID + ")",
			Hint: "align agent.target_user_id with the web user's rtc.user_id",
		})
	} else {
		checks = append(checks, Check{
			ID: "project.agent.target", Level: LevelProject, Title: "Agent target user",
			Status: PASS, Detail: "agent target matches web user",
		})
	}
	return checks
}

func agentAuthCheck() Check {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	token, _, err := auth.EnsureFreshToken(ctx, nil, authRefreshSkew, authTokenEndpoint)
	if err != nil {
		return Check{
			ID: "project.agent.auth", Level: LevelProject, Title: "Agent auth",
			Status: FAIL, Detail: err.Error(),
			Hint: "run `" + meta.BinName + " auth login` before agent start/update/stop",
		}
	}
	if _, err := auth.ParseSTSCredential(token.AccessToken); err != nil {
		return Check{
			ID: "project.agent.auth", Level: LevelProject, Title: "Agent auth",
			Status: FAIL, Detail: "stored Signin token is not a valid STS credential",
			Hint: "run `" + meta.BinName + " auth login` again",
		}
	}
	return Check{
		ID: "project.agent.auth", Level: LevelProject, Title: "Agent auth",
		Status: PASS, Detail: "Signin STS credential available",
	}
}

// agentTemplateCheck verifies the console template file exists, parses, and has
// a non-empty Config.
func agentTemplateCheck(cfg *config.Config, dir string) Check {
	cfgFile := cfg.Agent.ConfigFile
	if cfgFile == "" {
		cfgFile = "server/scenes/default.json"
	}
	base := Check{ID: "project.agent.template", Level: LevelProject, Title: "Agent template"}
	tplPath, perr := config.ProjectFilePath(dir, cfgFile)
	if perr != nil {
		base.Status, base.Detail = FAIL, cfgFile+" is not a valid project-relative path"
		base.Hint = "set agent.config_file to a path inside the project, e.g. server/scenes/default.json"
		return base
	}
	raw, err := os.ReadFile(tplPath)
	if err != nil {
		base.Status, base.Detail = FAIL, cfgFile+" not found"
		base.Hint = "run `" + meta.BinName + " auth login`, then `" + meta.BinName + " dev --reconfigure` to select bot scenes; or edit " + cfgFile + " manually"
		return base
	}
	if err := config.ValidateServerAgentConfigJSON(raw); err != nil {
		base.Status, base.Detail = FAIL, cfgFile+": "+err.Error()
		base.Hint = "fix SceneConfig and VoiceChat.Config/AgentConfig in the selected scene"
		return base
	}
	base.Status, base.Detail = PASS, cfgFile+" loaded, scene valid"
	return base
}

func tokenCheck(cfg *config.Config) Check {
	if cfg.RTC.Token == "" {
		return Check{
			ID: "project.token", Level: LevelProject, Title: "RTC Token",
			Status: FAIL, Detail: "no Token in config",
			Hint: "run `" + meta.BinName + " token issue --write`",
		}
	}
	tokenStr, _ := config.ResolveEnv(cfg.RTC.Token)
	info, err := token.Check(tokenStr, os.Getenv("RTC_APP_KEY"))
	if err != nil {
		hint := "run `" + meta.BinName + " token issue --write`"
		return Check{
			ID: "project.token", Level: LevelProject, Title: "RTC Token",
			Status: FAIL, Detail: err.Error(), Hint: hint,
		}
	}
	detail := "valid"
	if !info.ExpireAt.IsZero() {
		detail = "valid, expires " + info.ExpireAt.Format(time.RFC3339)
	}
	return Check{
		ID: "project.token", Level: LevelProject, Title: "RTC Token",
		Status: PASS, Detail: detail,
	}
}

// RunAll runs both tiers.
func RunAll(cfg *config.Config, path, dir string) Report {
	checks := RunCLI()
	checks = append(checks, RunProject(cfg, path, dir)...)
	return summarize(checks)
}

// RunAllCLI runs only the CLI tier.
func RunAllCLI() Report { return summarize(RunCLI()) }

// RunAllProject runs only the project tier.
func RunAllProject(cfg *config.Config, path, dir string) Report {
	return summarize(RunProject(cfg, path, dir))
}
