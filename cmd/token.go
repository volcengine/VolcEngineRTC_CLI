// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/affordance"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/config"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/env"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/meta"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/token"
)

func newTokenCmd() *cobra.Command {
	// Hidden advanced tool: the full-stack voice-agent template issues a
	// join token server-side for each session, so the happy path never mints one
	// by hand. `token issue/check/inspect` stays registered and runnable for
	// manual / legacy CLI-managed / debugging use, but is kept off the browsable
	// `--help` command list.
	cmd := &cobra.Command{
		Use:    "token",
		Short:  "Issue, check and inspect RTC join tokens (local VRTC AccessToken)",
		Hidden: true,
	}
	cmd.AddCommand(newTokenIssueCmd(), newTokenCheckCmd(), newTokenInspectCmd())
	affordance.Attach(cmd, affordance.Affordance{
		When:   []string{"Manual/legacy: minting or validating an RTC join Token by hand (the full-stack template issues it server-side)"},
		Prereq: []string{"rtc.app_id/room_id/user_id in config; RTC_APP_KEY in env"},
		Examples: []string{
			meta.BinName + " token issue --write",
			meta.BinName + " token check",
		},
	})
	return cmd
}

func newTokenIssueCmd() *cobra.Command {
	var (
		ttl       time.Duration
		write     bool
		noPublish bool
		noSub     bool
		roomFlag  string
		userFlag  string
	)
	cmd := &cobra.Command{
		Use:   "issue",
		Short: "Issue an RTC join token from config + RTC_APP_KEY",
		RunE: func(c *cobra.Command, args []string) error {
			cfg, path, err := config.LoadNearest(".")
			if err != nil {
				return err
			}
			appID, missing := config.ResolveEnv(cfg.RTC.AppID)
			if len(missing) > 0 {
				return errs.New("vertc.config.unresolved_env", errs.TypeValidation,
					"rtc.app_id references unset env %v", missing).WithParam("rtc.app_id").
					WithHint("set missing values in the process environment or local .env.local; never pass AppKey in command arguments")
			}
			// Resolve the room/user this token is minted for: an explicit
			// --room-id / --user-id overrides config for THIS issuance; absent
			// them we read config (unchanged behavior).
			roomChanged := c.Flags().Changed("room-id")
			userChanged := c.Flags().Changed("user-id")
			roomID, roomParam := cfg.RTC.RoomID, "rtc.room_id"
			userID, userParam := cfg.RTC.UserID, "rtc.user_id"
			if roomChanged {
				roomID, roomParam = roomFlag, "--room-id"
			} else if roomID, err = resolveConfigIdentity(roomParam, roomID); err != nil {
				return err
			}
			if userChanged {
				userID, userParam = userFlag, "--user-id"
			} else if userID, err = resolveConfigIdentity(userParam, userID); err != nil {
				return err
			}
			// Reject an identity RTC can't accept before signing, so a bad value
			// never produces an unrunnable token (design: 生成即可运行).
			if err := config.ValidateIdentity(roomParam, roomID); err != nil {
				return err
			}
			if err := config.ValidateIdentity(userParam, userID); err != nil {
				return err
			}
			res, err := token.Issue(token.IssueParams{
				AppID:     appID,
				AppKey:    os.Getenv("RTC_APP_KEY"),
				RoomID:    roomID,
				UserID:    userID,
				TTL:       ttl,
				Publish:   !noPublish,
				Subscribe: !noSub,
			})
			if err != nil {
				return err
			}
			if write && !flagDryRun {
				// config is the single source of truth …
				cfg.RTC.Token = res.Token
				// Write-through: persist the room/user this token was minted for
				// so `agent start` (which reads config) and the web runtime
				// follow the SAME room/user — no "真人在 room-X、agent 在 room-01"
				// drift. Keep target_user_id tracking rtc.user_id.
				if userChanged {
					followTargetUser(cfg, userID)
					cfg.RTC.UserID = userID
				}
				if roomChanged {
					cfg.RTC.RoomID = roomID
				}
				// … and the web runtime consumes VITE_-prefixed vars from
				// .env.local, so sync the (written-through) config-derived values
				// there too. This closes the "Token: MISSING" gap in the frontend.
				envPath := env.Path(filepath.Dir(path))
				if err := persistTokenWrite(cfg, path, envPath, map[string]string{
					"VITE_RTC_APP_ID":  appID,
					"VITE_RTC_ROOM_ID": roomID,
					"VITE_RTC_USER_ID": userID,
					"VITE_RTC_TOKEN":   res.Token,
				}, env.Write); err != nil {
					return err
				}
				if roomChanged || userChanged {
					out().Progress("wrote through rtc.room_id=%s / rtc.user_id=%s into %s (agent start & web now follow)", cfg.RTC.RoomID, cfg.RTC.UserID, path)
				} else {
					out().Progress("backfilled rtc.token into %s", path)
				}
				out().Progress("synced VITE_RTC_* into %s (restart/refresh `dev` to pick up)", envPath)
			} else if write && flagDryRun {
				out().Progress("dry-run: not backfilling config or .env.local")
				if roomChanged || userChanged {
					out().Progress("dry-run: would write through rtc.room_id=%s / rtc.user_id=%s", roomID, userID)
				}
			}
			return out().Data(res)
		},
	}
	cmd.Flags().DurationVar(&ttl, "ttl", token.DefaultTTL, "token lifetime (e.g. 24h)")
	cmd.Flags().BoolVar(&write, "write", false, "backfill the issued token into "+meta.ConfigFileName)
	cmd.Flags().BoolVar(&noPublish, "no-publish", false, "do not grant publish privilege")
	cmd.Flags().BoolVar(&noSub, "no-subscribe", false, "do not grant subscribe privilege")
	cmd.Flags().StringVar(&roomFlag, "room-id", "", "room to issue for; with --write, writes through to rtc.room_id (default: config)")
	cmd.Flags().StringVar(&userFlag, "user-id", "", "user to issue for; with --write, writes through to rtc.user_id (default: config)")
	affordance.Attach(cmd, affordance.Affordance{
		When:   []string{"Right after filling config, to unblock room join"},
		Prereq: []string{"rtc.app_id/room_id/user_id set; RTC_APP_KEY exported"},
		Examples: []string{
			meta.BinName + " token issue --write",
			meta.BinName + " token issue --room-id room-2 --user-id alice --write",
			meta.BinName + " token issue --ttl 1h --no-publish",
		},
	})
	return cmd
}

// resolveConfigIdentity expands config placeholders and rejects missing env
// references before validation/signing. A literal ${ENV} is syntactically a
// valid RTC identity, so ValidateIdentity alone cannot distinguish this error.
func resolveConfigIdentity(param, value string) (string, error) {
	resolved, missing := config.ResolveEnv(value)
	if len(missing) > 0 {
		return "", errs.New("vertc.config.unresolved_env", errs.TypeValidation,
			"%s references unset env %v", param, missing).WithParam(param).
			WithHint("set the environment variable or replace the placeholder in %s", meta.ConfigFileName)
	}
	return resolved, nil
}

// followTargetUser keeps agent.target_user_id tracking rtc.user_id when it was
// following it: empty (auto-follows via agent start's fallback) stays empty, and
// a target equal to the current rtc.user_id is advanced to newUserID. A target
// pointing at a different, intentional user is left untouched — doctor surfaces
// that mismatch. Call BEFORE overwriting cfg.RTC.UserID.
func followTargetUser(cfg *config.Config, newUserID string) {
	currentUserID, currentMissing := config.ResolveEnv(cfg.RTC.UserID)
	targetUserID, targetMissing := config.ResolveEnv(cfg.Agent.TargetUserID)
	if cfg.Agent.TargetUserID != "" && len(currentMissing) == 0 && len(targetMissing) == 0 && targetUserID == currentUserID {
		cfg.Agent.TargetUserID = newUserID
	}
}

type envWriteFunc func(string, map[string]string) ([]string, error)

type fileSnapshot struct {
	path    string
	data    []byte
	mode    os.FileMode
	existed bool
}

// persistTokenWrite keeps config and .env.local consistent. It snapshots both
// files before writing and restores them if either write fails, including when
// an env writer partially modifies its target before returning an error.
func persistTokenWrite(cfg *config.Config, configPath, envPath string, updates map[string]string, writeEnv envWriteFunc) error {
	configBefore, err := snapshotFile(configPath, "vertc.config.write_failed")
	if err != nil {
		return err
	}
	envBefore, err := snapshotFile(envPath, "vertc.env.write_failed")
	if err != nil {
		return err
	}
	if err := config.Save(cfg, configPath); err != nil {
		if rollbackErr := configBefore.restore(); rollbackErr != nil {
			return errs.New("vertc.config.write_failed", errs.TypeIO,
				"write config: %s; rollback failed: %s", err, rollbackErr)
		}
		return err
	}
	if _, err := writeEnv(envPath, updates); err != nil {
		writeErr := err
		if _, ok := errs.As(err); !ok {
			writeErr = errs.Wrap(err, "vertc.env.write_failed", "sync %q: %s", envPath, err)
		}
		configRollbackErr := configBefore.restore()
		envRollbackErr := envBefore.restore()
		if configRollbackErr != nil || envRollbackErr != nil {
			return errs.New("vertc.env.write_failed", errs.TypeIO,
				"sync env: %s; rollback config: %v; rollback env: %v", writeErr, configRollbackErr, envRollbackErr)
		}
		return writeErr
	}
	return nil
}

func snapshotFile(path, errorCode string) (fileSnapshot, error) {
	s := fileSnapshot{path: path}
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, errs.New(errorCode, errs.TypeIO, "inspect %q: %s", path, err)
	}
	if !info.Mode().IsRegular() {
		return s, errs.New(errorCode, errs.TypeIO, "%q is not a regular file", path)
	}
	s.data, err = os.ReadFile(path)
	if err != nil {
		return s, errs.New(errorCode, errs.TypeIO, "read %q: %s", path, err)
	}
	s.mode = info.Mode().Perm()
	s.existed = true
	return s, nil
}

func (s fileSnapshot) restore() error {
	if s.existed {
		return os.WriteFile(s.path, s.data, s.mode)
	}
	err := os.Remove(s.path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func newTokenCheckCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check [token]",
		Short: "Verify a token's signature and expiry (defaults to config token)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			raw, err := tokenArgOrConfig(args)
			if err != nil {
				return err
			}
			info, err := token.Check(raw, os.Getenv("RTC_APP_KEY"))
			if err != nil {
				// Emit the decoded info too, then fail with the typed error.
				if info != nil {
					_ = out().Data(info)
					if te, ok := errs.As(err); ok {
						return te.Reported()
					}
				}
				return err
			}
			return out().Data(info)
		},
	}
	affordance.Attach(cmd, affordance.Affordance{
		When:     []string{"Diagnosing INVALID_TOKEN / TOKEN_EXPIRED before or during a call"},
		Prereq:   []string{"RTC_APP_KEY exported to verify the signature"},
		Examples: []string{meta.BinName + " token check", meta.BinName + " token check 001abc..."},
	})
	return cmd
}

func newTokenInspectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inspect [token]",
		Short: "Decode a token's contents without verifying it",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			raw, err := tokenArgOrConfig(args)
			if err != nil {
				return err
			}
			info, err := token.Inspect(raw)
			if err != nil {
				return err
			}
			return out().Data(info)
		},
	}
	affordance.Attach(cmd, affordance.Affordance{
		When:     []string{"Reading room/user/privilege/expiry from a token string"},
		Avoid:    []string{"Judging validity — use `token check` for that"},
		Examples: []string{meta.BinName + " token inspect 001abc..."},
	})
	return cmd
}

// tokenArgOrConfig returns the token from args[0] or the config's rtc.token.
func tokenArgOrConfig(args []string) (string, error) {
	if len(args) == 1 && args[0] != "" {
		return args[0], nil
	}
	cfg, _, err := config.LoadNearest(".")
	if err != nil {
		return "", err
	}
	if cfg.RTC.Token == "" {
		return "", errs.New("vertc.token.missing_config", errs.TypeValidation,
			"no token provided and rtc.token is empty").
			WithHint("pass a token argument or run `%s token issue --write`", meta.BinName)
	}
	return mustResolve(cfg.RTC.Token), nil
}

// mustResolve expands ${ENV} placeholders, leaving unresolved ones as-is.
func mustResolve(s string) string {
	v, _ := config.ResolveEnv(s)
	return v
}
