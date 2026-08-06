// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/affordance"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/meta"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/selfupdate"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/skillscheck"
	updatecheck "github.com/volcengine/VolcEngineRTC_CLI/internal/update"
)

var fetchLatest = updatecheck.FetchLatest

type updateResult struct {
	Current       string                   `json:"current"`
	Latest        string                   `json:"latest"`
	Status        string                   `json:"status"`
	Install       selfupdate.InstallMethod `json:"install_method,omitempty"`
	SkillsSynced  bool                     `json:"skills_synced,omitempty"`
	SkillsError   string                   `json:"skills_error,omitempty"`
	SkillsStatus  string                   `json:"skills_status"`
	ManualCommand string                   `json:"manual_command,omitempty"`
}

func (r updateResult) Pretty(w io.Writer) {
	fmt.Fprintf(w, "%s: %s → %s (%s)\n", meta.BinName, r.Current, r.Latest, r.Status)
	if r.SkillsError != "" {
		fmt.Fprintf(w, "skills: sync failed: %s\n", r.SkillsError)
	} else if r.SkillsSynced {
		fmt.Fprintln(w, "skills: synchronized")
	} else if r.SkillsStatus != "" {
		fmt.Fprintf(w, "skills: %s\n", r.SkillsStatus)
	}
	if r.ManualCommand != "" {
		fmt.Fprintf(w, "manual update: %s\n", r.ManualCommand)
	}
}

func newUpdateCmd() *cobra.Command {
	var check, force, jsonOutput bool
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Check for or install a newer CLI and synchronize official skills",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			if jsonOutput {
				flagFormat = "json"
			}
			return runUpdate(c.Context(), check, force)
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "check for an update without installing it")
	cmd.Flags().BoolVar(&force, "force", false, "reinstall the latest version even when already current")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "force JSON output")
	affordance.Attach(cmd, affordance.Affordance{
		When:     []string{"A lifecycle notice reports a newer CLI or stale official skills"},
		Avoid:    []string{"Replacing a manually installed binary from an unknown package manager"},
		Prereq:   []string{"npm-managed installs require npm and network access"},
		Examples: []string{meta.BinName + " update --check", meta.BinName + " update", meta.BinName + " update --force --json"},
	})
	return cmd
}

func runUpdate(parent context.Context, check, force bool) error {
	ctx, cancel := context.WithTimeout(parent, 10*time.Minute)
	defer cancel()
	latest, err := fetchLatest(ctx)
	if err != nil {
		return errs.New("vertc.update.network", errs.TypeIO, "cannot query the npm registry: %v", err).
			WithCause(err).WithHint("check network access, then re-run `%s update`", meta.BinName)
	}
	current := strings.TrimPrefix(meta.Version, "v")
	newer := updatecheck.IsNewer(current, latest)
	method, path, detectErr := selfupdate.DetectInstallMethod()
	if detectErr != nil {
		return errs.New("vertc.update.detect_failed", errs.TypeIO, "cannot detect install method: %v", detectErr).WithCause(detectErr)
	}
	result := updateResult{Current: current, Latest: latest, Install: method, SkillsStatus: skillscheck.Status(current)}
	if check || flagDryRun {
		status := "up_to_date"
		if newer || force {
			status = "available"
			if flagDryRun {
				status = "would_update"
			}
			if method != selfupdate.InstallNPM {
				status = "manual_required"
				result.ManualCommand = manualUpdateCommand(latest)
			}
		}
		result.Status = status
		return out().Data(result)
	}
	if !force && !newer {
		updatecheck.SetPending(nil)
		result.Status = "up_to_date"
		return emitSkillsSync(ctx, result, current)
	}
	if method != selfupdate.InstallNPM {
		result.Status = "manual_required"
		result.ManualCommand = manualUpdateCommand(latest)
		return out().Data(result)
	}
	out().Progress("updating %s to %s with npm...", meta.BinName, latest)
	if err := selfupdate.RunNpmInstall(ctx, latest, path); err != nil {
		_ = selfupdate.RollbackBinary(path)
		return errs.New("vertc.update.failed", errs.TypeIO, "npm update failed: %v", err).
			WithCause(err).WithHint("if npm reports EACCES, fix npm global permissions and retry")
	}
	if err := selfupdate.VerifyBinary(ctx, latest, path); err != nil {
		hadBackup := selfupdate.HasBackup(path)
		rollbackErr := selfupdate.RollbackBinary(path)
		hint := fmt.Sprintf("reinstall with `npm install -g %s@%s`", selfupdate.NpmPackage, latest)
		if hadBackup && rollbackErr == nil {
			hint = "the previous binary was restored; retry the update"
		}
		return errs.New("vertc.update.verify_failed", errs.TypeIO, "updated binary verification failed: %v", err).
			WithCause(err).WithHint("%s", hint)
	}
	_ = selfupdate.CleanupBackup(path)
	result.Status = "updated"
	updatecheck.SetPending(nil)
	return emitSkillsSync(ctx, result, latest)
}

func manualUpdateCommand(version string) string {
	return fmt.Sprintf("npm install -g %s@%s", selfupdate.NpmPackage, strings.TrimPrefix(version, "v"))
}

func emitSkillsSync(ctx context.Context, result updateResult, version string) error {
	syncResult, err := selfupdate.SyncSkills(ctx, version)
	for _, warning := range syncResult.Warnings {
		out().Warn("%s", warning)
	}
	if err != nil {
		result.SkillsError = err.Error()
		out().Warn("CLI updated, but official skills could not be synchronized: %v", err)
		out().Warn("retry with: %s skills sync", meta.BinName)
	} else {
		result.SkillsSynced = true
		result.SkillsStatus = "synchronized"
	}
	return out().Data(result)
}
