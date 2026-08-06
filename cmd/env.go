// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/affordance"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/config"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/env"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/meta"
)

func newEnvCmd() *cobra.Command {
	// Hidden advanced fallback: the happy path configures credentials via
	// interactive `dev`. `env write` stays registered and runnable for
	// non-interactive / manual local-file workflows, but it is kept out of
	// the browsable `--help` command list.
	cmd := &cobra.Command{
		Use:    "env",
		Short:  "Manage local environment variables and base config",
		Hidden: true,
	}
	cmd.AddCommand(newEnvWriteCmd())
	affordance.Attach(cmd, affordance.Affordance{
		When:     []string{"Interactive dev setup is unavailable — non-secret environment/region fallback"},
		Examples: []string{meta.BinName + " env write --app-id abc123"},
	})
	return cmd
}

func newEnvWriteCmd() *cobra.Command {
	var (
		appID  string
		appKey string
		roomID string
		userID string
		region string
		sets   []string
	)
	cmd := &cobra.Command{
		Use:   "write",
		Short: "Collect and write env vars/base config (secrets not echoed)",
		RunE: func(c *cobra.Command, args []string) error {
			updates := map[string]string{}
			put := func(k, v string) {
				if v != "" {
					updates[k] = v
				}
			}
			for key, value := range rtcAppCredentialUpdates(appID, appKey) {
				put(key, value)
			}
			put("VITE_RTC_ROOM_ID", roomID)
			put("VITE_RTC_USER_ID", userID)
			put("RTC_REGION", region)
			for _, kv := range sets {
				k, v, ok := strings.Cut(kv, "=")
				if !ok {
					return errs.New("vertc.cli.invalid_flag", errs.TypeValidation,
						"--set expects KEY=VALUE, got %q", kv).WithParam("--set")
				}
				put(strings.TrimSpace(k), strings.TrimSpace(v))
			}
			if len(updates) == 0 {
				return errs.New("vertc.cli.invalid_flag", errs.TypeValidation,
					"nothing to write").WithHint("pass --app-id/--room-id/… or --set a non-secret KEY=VALUE; edit .env.local locally for manual secret setup")
			}

			path := projectEnvWritePath(".")
			if flagDryRun {
				out().Progress("dry-run: would write %d key(s) to %s", len(updates), path)
				return out().Data(map[string]any{"dry_run": true, "keys": maskedKeys(updates)})
			}
			written, err := env.Write(path, updates)
			if err != nil {
				return err
			}
			out().Progress("wrote %d key(s) to %s", len(written), path)
			// Report keys with masked values so secrets never echo.
			return out().Data(map[string]any{"path": path, "written": maskedKeys(updates)})
		},
	}
	cmd.Flags().StringVar(&appID, "app-id", "", "RTC AppID")
	cmd.Flags().StringVar(&appKey, "app-key", "", "RTC AppKey (secret; masked in output)")
	_ = cmd.Flags().MarkDeprecated("app-key", "use auth login + dev; for manual recovery edit .env.local with local secure tooling")
	cmd.Flags().StringVar(&roomID, "room-id", "", "RTC RoomID")
	cmd.Flags().StringVar(&userID, "user-id", "", "RTC UserID")
	cmd.Flags().StringVar(&region, "region", "", "region, e.g. cn-north-1")
	cmd.Flags().StringArrayVar(&sets, "set", nil, "extra KEY=VALUE (repeatable)")
	affordance.Attach(cmd, affordance.Affordance{
		When:     []string{"Manual non-secret setup, before token issue / dev"},
		Prereq:   []string{"A scaffolded project directory"},
		Examples: []string{meta.BinName + " env write --app-id abc"},
	})
	return cmd
}

// projectEnvWritePath resolves where `env write` writes .env.local: next to the
// nearest vertc.config.yaml (the project root) when one exists, else startDir.
// Without this, running `env write` from a project subdirectory created a stray
// .env.local there that shadowed the real one (env.findUp resolves the nearest),
// diverging from token/dev, which operate on the project root.
func projectEnvWritePath(startDir string) string {
	if cfgPath, err := config.Find(startDir); err == nil {
		return env.Path(projectDir(cfgPath))
	}
	return env.Path(startDir)
}

// rtcAppCredentialUpdates is shared by the explicit env write command and the
// interactive dev bootstrap. AppKey remains server-side only.
func rtcAppCredentialUpdates(appID, appKey string) map[string]string {
	updates := map[string]string{}
	if strings.TrimSpace(appID) != "" {
		updates["RTC_APP_ID"] = appID
		updates["VITE_RTC_APP_ID"] = appID
	}
	if strings.TrimSpace(appKey) != "" {
		updates["RTC_APP_KEY"] = appKey
	}
	return updates
}

// maskedKeys returns key→masked-value, hiding secret values.
func maskedKeys(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		if env.IsSensitive(k) {
			out[k] = env.Mask(v)
		} else {
			out[k] = v
		}
	}
	return out
}
