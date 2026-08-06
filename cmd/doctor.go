// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/affordance"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/config"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/doctor"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/meta"
)

// doctorReport wraps a doctor.Report so it can render human output.
type doctorReport struct{ doctor.Report }

func (r doctorReport) Pretty(w io.Writer) {
	for _, c := range r.Checks {
		mark := "✓"
		if c.Status == doctor.WARN {
			mark = "!"
		} else if c.Status == doctor.FAIL {
			mark = "✗"
		} else if c.Status == doctor.SKIP {
			mark = "-"
		} else if c.Status == doctor.UNKNOWN {
			mark = "?"
		}
		fmt.Fprintf(w, "%s [%s] %s — %s\n", mark, c.Status, c.Title, c.Detail)
		if c.Hint != "" && c.Status != doctor.PASS {
			fmt.Fprintf(w, "    hint: %s\n", c.Hint)
		}
	}
	fmt.Fprintf(w, "%d passed, %d warned, %d skipped, %d unknown, %d failed → runnable=%v\n",
		r.Passed, r.Warned, r.Skipped, r.Unknown, r.Failed, r.Runnable)
}

func newDoctorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Run two-level readiness diagnostics (CLI self-check + project)",
		RunE: func(_ *cobra.Command, args []string) error {
			cfg, path, _ := config.LoadNearest(".") // nil cfg handled by RunProject
			dir := projectDir(path)
			restoreEnv, err := loadProjectEnv(dir)
			if err != nil {
				return err
			}
			defer restoreEnv()
			return emitDoctor(doctor.RunAll(cfg, path, dir))
		},
	}
	cmd.AddCommand(newDoctorProjectCmd(), newDoctorCLICmd())
	affordance.Attach(cmd, affordance.Affordance{
		When:   []string{"Before dev, or whenever a step fails, to locate the blocker without changing project state"},
		Prereq: []string{"Run inside a scaffolded project for the project tier"},
		Examples: []string{
			meta.BinName + " doctor",
			meta.BinName + " doctor project",
			meta.BinName + " doctor --format json",
		},
	})
	return cmd
}

func newDoctorProjectCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "project",
		Short: "Run only project-readiness checks",
		RunE: func(_ *cobra.Command, args []string) error {
			cfg, path, _ := config.LoadNearest(".")
			dir := projectDir(path)
			restoreEnv, err := loadProjectEnv(dir)
			if err != nil {
				return err
			}
			defer restoreEnv()
			return emitDoctor(doctor.RunAllProject(cfg, path, dir))
		},
	}
}

func newDoctorCLICmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cli",
		Short: "Run only CLI self-check",
		RunE: func(c *cobra.Command, args []string) error {
			return emitDoctor(doctor.RunAllCLI())
		},
	}
}

// emitDoctor prints the report and fails (non-zero) when any check FAILed.
func emitDoctor(rep doctor.Report) error {
	if err := out().Data(doctorReport{rep}); err != nil {
		return err
	}
	if rep.Failed > 0 {
		return errs.New("vertc.doctor.failed", errs.TypePrecondition,
			"%d check(s) failed", rep.Failed).
			WithHint("fix the FAIL items above, then re-run `%s doctor`", meta.BinName).
			WithExit(1).Reported()
	}
	return nil
}
