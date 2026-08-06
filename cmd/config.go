// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"github.com/spf13/cobra"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/affordance"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/config"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/meta"
)

func newConfigCmd() *cobra.Command {
	// Hidden advanced tool: `init` generates vertc.config.yaml and `doctor`
	// validates it, so the happy path needs no manual config editing. `config
	// show/get/set/validate` stays registered and runnable for manual / legacy
	// CLI-managed / debugging use, but is kept off the browsable `--help` list.
	cmd := &cobra.Command{
		Use:    "config",
		Short:  "Read, write and validate " + meta.ConfigFileName,
		Hidden: true,
		// Reject unknown subcommands (e.g. the removed `diff`/`push`/`pull`) with a
		// typed non-zero error instead of silently falling through to this group's
		// help; bare `config` still prints help (exit 0).
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) == 0 {
				return c.Help()
			}
			return errs.New("vertc.cli.unknown_command", errs.TypeNotFound,
				"unknown command %q for %q", args[0], c.CommandPath()).
				WithHint("run `%s config --help` for available config commands", meta.BinName)
		},
	}
	cmd.AddCommand(
		newConfigShowCmd(),
		newConfigGetCmd(),
		newConfigSetCmd(),
		newConfigValidateCmd(),
	)
	affordance.Attach(cmd, affordance.Affordance{
		When:   []string{"Inspecting or editing the single source of truth " + meta.ConfigFileName},
		Prereq: []string{"A scaffolded project (run `" + meta.BinName + " init` first)"},
		Examples: []string{
			meta.BinName + " config show",
			meta.BinName + " config set rtc.room_id room-42",
			meta.BinName + " config validate",
		},
	})
	return cmd
}

func newConfigShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show the current config",
		RunE: func(c *cobra.Command, args []string) error {
			cfg, path, err := config.LoadNearest(".")
			if err != nil {
				return err
			}
			out().Progress("loaded %s", path)
			return out().Data(cfg)
		},
	}
}

func newConfigGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <field>",
		Short: "Get a config field by dotted path",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cfg, _, err := config.LoadNearest(".")
			if err != nil {
				return err
			}
			val, err := cfg.Get(args[0])
			if err != nil {
				return err
			}
			return out().Data(map[string]string{"field": args[0], "value": val})
		},
	}
}

func newConfigSetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set <field> <value>",
		Short: "Set a config field by dotted path",
		Args:  cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			cfg, path, err := config.LoadNearest(".")
			if err != nil {
				return err
			}
			if err := cfg.Set(args[0], args[1]); err != nil {
				return err
			}
			if flagDryRun {
				out().Progress("dry-run: not writing")
				return out().Data(map[string]string{"field": args[0], "value": args[1], "written": "false"})
			}
			if err := config.Save(cfg, path); err != nil {
				return err
			}
			out().Progress("updated %s", path)
			return out().Data(map[string]string{"field": args[0], "value": args[1], "written": "true"})
		},
	}
	affordance.Attach(cmd, affordance.Affordance{
		When:     []string{"Filling in required identifiers after init"},
		Prereq:   []string{meta.ConfigFileName + " exists"},
		Examples: []string{meta.BinName + " config set rtc.app_id ${RTC_APP_ID}"},
	})
	return cmd
}

func newConfigValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Validate required fields, types and ${ENV} resolvability",
		RunE: func(c *cobra.Command, args []string) error {
			cfg, path, err := config.LoadNearest(".")
			if err != nil {
				return err
			}
			out().Progress("validating %s", path)
			rep := config.Validate(cfg)
			if rep.HasErrors() {
				// Emit the structured report AND fail with a typed error so the
				// exit code is non-zero and Agents can branch.
				o := out()
				_ = o.Data(rep)
				return errs.New("vertc.config.missing_field", errs.TypeValidation,
					"config validation failed with %d error(s)", countErrors(rep)).
					WithHint("see findings above; run `%s doctor project` for guided fixes", meta.BinName).
					Reported()
			}
			return out().Data(rep)
		},
	}
}

func countErrors(r config.Report) int {
	n := 0
	for _, f := range r.Findings {
		if f.Severity == config.SevError {
			n++
		}
	}
	return n
}
