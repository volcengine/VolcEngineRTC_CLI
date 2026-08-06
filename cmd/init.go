// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/affordance"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/config"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/meta"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/template"
)

// initResult is the structured output of `init`.
type initResult struct {
	Scene    string   `json:"scene"`
	Platform string   `json:"platform"`
	Name     string   `json:"name"`
	Dir      string   `json:"dir"`
	RoomID   string   `json:"room_id"`
	UserID   string   `json:"user_id"`
	AppID    string   `json:"app_id"`
	DryRun   bool     `json:"dry_run"`
	Files    []string `json:"files"`
	Next     []string `json:"next_steps"`
}

func (r initResult) Pretty(w io.Writer) {
	verb := "generated"
	if r.DryRun {
		verb = "would generate (dry-run)"
	}
	fmt.Fprintf(w, "%s %s × %s → %s\n", verb, r.Scene, r.Platform, r.Dir)
	fmt.Fprintf(w, "identity: room=%s user=%s app=%s\n", r.RoomID, r.UserID, r.AppID)
	for _, f := range r.Files {
		fmt.Fprintf(w, "  %s/%s\n", r.Dir, f)
	}
	fmt.Fprintln(w, "next:")
	for _, n := range r.Next {
		fmt.Fprintf(w, "  - %s\n", n)
	}
}

func newInitCmd() *cobra.Command {
	var (
		scene    string
		platform string
		name     string
		roomFlag string
		userFlag string
		list     bool
	)
	cmd := &cobra.Command{
		Use:   "init [dir]",
		Short: "Scaffold a minimal runnable project for a scene × platform",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if list {
				return emitTemplateList()
			}
			if scene == "" || platform == "" {
				return errs.New("vertc.cli.invalid_flag", errs.TypeValidation,
					"both --scene and --platform are required").
					WithHint("run `%s init --list` to see available combinations", meta.BinName)
			}
			tmpl, ok := template.Find(scene, platform)
			if !ok || !tmpl.Available {
				return errs.New("vertc.template.not_found", errs.TypeNotFound,
					"no available template for %s × %s", scene, platform).
					WithParam("--scene/--platform").
					WithHint("run `%s init --list`; available: voice-agent × web", meta.BinName)
			}

			if name == "" {
				name = scene + "-" + platform
			}
			dir := "./" + name
			if len(args) == 1 && args[0] != "" {
				dir = args[0]
			}

			cfg := config.Default(name, scene, platform)
			// --room-id / --user-id override the room-01 / user-01 defaults and
			// land in the config (single source of truth), so token issue /
			// agent start / web all read the same identity. Validate before
			// writing so a bad value never scaffolds an unrunnable project.
			if c.Flags().Changed("room-id") {
				cfg.RTC.RoomID = roomFlag
			}
			if c.Flags().Changed("user-id") {
				cfg.RTC.UserID = userFlag
			}
			if err := config.ValidateIdentity("--room-id", cfg.RTC.RoomID); err != nil {
				return err
			}
			if err := config.ValidateIdentity("--user-id", cfg.RTC.UserID); err != nil {
				return err
			}
			out().Progress("using room=%s user=%s app=%s — change with `%s token issue --room-id <id> --user-id <id> --write` or edit %s",
				cfg.RTC.RoomID, cfg.RTC.UserID, cfg.RTC.AppID, meta.BinName, meta.ConfigFileName)

			files, err := template.Resolve(c.Context(), tmpl, cfg, template.RemoteOptions{DryRun: flagDryRun})
			if err != nil {
				return err
			}

			// Assemble the file list (rendered files + the generated config).
			paths := make([]string, 0, len(files)+1)
			paths = append(paths, meta.ConfigFileName)
			for _, f := range files {
				paths = append(paths, f.Path)
			}
			sort.Strings(paths)

			result := initResult{
				Scene: scene, Platform: platform, Name: name, Dir: dir,
				RoomID: cfg.RTC.RoomID, UserID: cfg.RTC.UserID, AppID: cfg.RTC.AppID,
				DryRun: flagDryRun, Files: paths,
			}

			if flagDryRun {
				out().Progress("dry-run: previewing %d file(s), nothing written", len(paths))
				result.Next = voiceAgentNextSteps(scene)
				return out().Data(result)
			}

			// Refuse to clobber a non-empty target directory.
			if nonEmptyDir(dir) {
				return errs.New("vertc.init.target_exists", errs.TypePrecondition,
					"target directory %q exists and is not empty", dir).
					WithHint("choose an empty dir: `%s init ./new-dir --scene %s --platform %s`", meta.BinName, scene, platform)
			}
			if tmpl.Remote != nil {
				materializeDir, err := filepath.Abs(dir)
				if err != nil {
					return errs.Wrap(err, "vertc.template.render_failed", "resolve target %q: %s", dir, err)
				}
				workingDir, err := os.Getwd()
				if err != nil {
					return errs.Wrap(err, "vertc.template.render_failed", "resolve current directory: %s", err)
				}
				replacesWorkingDir := filepath.Clean(materializeDir) == filepath.Clean(workingDir)
				configRaw, err := config.Marshal(cfg)
				if err != nil {
					return err
				}
				files = append(files, template.RenderedFile{
					Path: meta.ConfigFileName, Content: string(configRaw), Bytes: len(configRaw),
				})
				if err := template.MaterializeRemote(materializeDir, files, false); err != nil {
					return err
				}
				// Replacing the process working directory by its absolute path leaves
				// the process attached to the removed inode. Re-enter the installed
				// directory so subsequent commands continue to resolve ".".
				if replacesWorkingDir {
					if err := os.Chdir(materializeDir); err != nil {
						return errs.Wrap(err, "vertc.template.render_failed", "enter installed target: %s", err)
					}
				}
			} else {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					return errs.New("vertc.template.render_failed", errs.TypeIO, "create %q: %s", dir, err)
				}
				if err := template.Write(dir, files); err != nil {
					return err
				}
				if err := config.Save(cfg, config.Path(dir)); err != nil {
					return err
				}
			}
			out().Progress("scaffolded %s × %s into %s", scene, platform, dir)

			result.Next = voiceAgentNextSteps(scene)
			return out().Data(result)
		},
	}
	cmd.Flags().StringVar(&scene, "scene", "", "scene, e.g. voice-agent")
	cmd.Flags().StringVar(&platform, "platform", "", "platform, e.g. web")
	cmd.Flags().StringVar(&name, "name", "", "project name (default scene-platform)")
	cmd.Flags().StringVar(&roomFlag, "room-id", "", "room_id to scaffold into config (default room-01)")
	cmd.Flags().StringVar(&userFlag, "user-id", "", "rtc.user_id (real user) to scaffold into config (default user-01)")
	cmd.Flags().BoolVar(&list, "list", false, "list available scene × platform combinations")

	affordance.Attach(cmd, affordance.Affordance{
		When:   []string{"Starting a new project for a supported scene × platform"},
		Avoid:  []string{"Adding capability to an existing project (use config/token instead)"},
		Prereq: []string{"An empty target directory"},
		Examples: []string{
			meta.BinName + " init --scene voice-agent --platform web",
			meta.BinName + " init ./my-agent --scene voice-agent --platform web",
			meta.BinName + " init --scene voice-agent --platform web --room-id room-2 --user-id alice",
			meta.BinName + " init --scene voice-agent --platform web --dry-run",
			meta.BinName + " init --list",
		},
	})
	return cmd
}

// voiceAgentNextSteps keeps scaffolding offline: login establishes Signin
// credentials, then the first dev run selects the RTC app and bot scenes.
func voiceAgentNextSteps(scene string) []string {
	if scene != "voice-agent" {
		return []string{
			meta.BinName + " env write   # configure non-secret local values; keep secrets in local secure tooling",
			meta.BinName + " token issue --write",
			meta.BinName + " doctor",
			meta.BinName + " dev",
		}
	}
	return []string{
		meta.BinName + " auth login   # 登录并保存 Signin 凭证",
		meta.BinName + " dev          # 首次运行选择 App/Bot，然后启动 web + server",
	}
}

// nonEmptyDir reports whether dir exists and contains entries.
func nonEmptyDir(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false // does not exist → safe to create
	}
	return len(entries) > 0
}
