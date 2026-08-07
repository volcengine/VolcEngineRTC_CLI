// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

// Package cmd wires the vertc command tree onto cobra.
package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/env"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/meta"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/output"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/selfupdate"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/skillscheck"
	updatecheck "github.com/volcengine/VolcEngineRTC_CLI/internal/update"
)

const updateRefreshChildEnv = "_VERTC_INTERNAL_UPDATE_REFRESH"

// global flag values (bound as persistent flags on the root command).
var (
	flagFormat string
	flagDryRun bool
)

// out builds an output.Writer from the current --format flag.
func out() *output.Writer {
	f := resolveOutputFormat(flagFormat, isTerminal(os.Stdout))
	return output.New(f)
}

func resolveOutputFormat(requested string, stdoutTTY bool) output.Format {
	if requested == "" {
		if stdoutTTY {
			return output.FormatPretty
		}
		return output.FormatJSON
	}
	f, err := output.ParseFormat(requested)
	if err != nil {
		// Fall back to JSON; the bad value is reported by the command itself.
		f = output.FormatJSON
	}
	return f
}

// NewRootCmd builds the root command with all subcommands registered.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   meta.BinName,
		Short: meta.BinName + " — VolcEngine AI audio/video developer workflow CLI",
		Long: meta.BinName + " scaffolds, configures, diagnoses and runs VolcEngine " +
			"AI audio/video projects. It is a developer-workflow tool (init/doctor/dev), " +
			"not an SDK wrapper.",
		SilenceErrors:     true,
		SilenceUsage:      true,
		DisableAutoGenTag: true,
		Version:           meta.Version,
		// Load the project's .env.local before project commands resolve ${ENV}, so
		// the CLI reads the same file the web runtime does. Auth and init are
		// intentionally project-independent and never inspect the current dotenv.
		PersistentPreRunE: func(c *cobra.Command, args []string) error {
			if c.Name() == "init" || c.Name() == "auth" || (c.Parent() != nil && c.Parent().Name() == "auth") {
				return nil
			}
			_, err := env.LoadIntoProcess(".")
			return err
		},
		// Bare `vertc` prints help.
		RunE: func(c *cobra.Command, args []string) error {
			if accepted, err := offerInteractiveUpdate(); err != nil {
				return err
			} else if accepted {
				return runUpdate(c.Context(), false, false)
			}
			return c.Help()
		},
	}
	root.SetVersionTemplate(fmt.Sprintf("%s %s (commit %s, built %s)\n", meta.BinName, meta.Version, meta.Commit, meta.BuildDate))
	// Keep Cobra's default `completion` command off the public surface; it stays
	// runnable but is hidden (see TestRootCommandSurface).
	root.CompletionOptions.HiddenDefaultCmd = true

	root.PersistentFlags().StringVar(&flagFormat, "format", "",
		"output format: json|pretty|table (default: pretty in a terminal, json otherwise)")
	root.PersistentFlags().BoolVar(&flagDryRun, "dry-run", false,
		"preview side effects without applying them")

	// Register the supported subcommands.
	root.AddCommand(
		newVersionCmd(),
		newAuthCmd(),
		newInitCmd(),
		newConfigCmd(),
		newTokenCmd(),
		newAgentCmd(),
		newOpenAPICmd(),
		newEnvCmd(),
		newDoctorCmd(),
		newExplainErrorCmd(),
		newDevCmd(),
		newUpdateCmd(),
		newSkillsCmd(),
	)
	return root
}

// Execute runs the CLI and returns the process exit code.
func Execute() int {
	if os.Getenv(updateRefreshChildEnv) == "1" {
		defer updatecheck.ReleaseRefreshLease()
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = updatecheck.RefreshCache(ctx, meta.Version)
		return 0
	}
	if !isVersionProbe(os.Args[1:]) {
		selfupdate.CleanupHealthyBackup()
	}
	setupNotices()
	root := NewRootCmd()
	err := root.Execute()
	if err == nil {
		if shouldRefreshAfterCommand(root) {
			startUpdateRefreshProcess(meta.Version)
		}
		return 0
	}
	// Map cobra's unknown-command / flag errors into typed envelopes.
	if typed, ok := errs.As(err); ok {
		return out().Fail(typed)
	}
	if strings.HasPrefix(err.Error(), "unknown command") {
		return out().Fail(unknownCommandError(root, err))
	}
	if strings.Contains(err.Error(), "unknown flag") ||
		strings.Contains(err.Error(), "invalid argument") ||
		strings.Contains(err.Error(), "unknown shorthand") {
		return out().Fail(errs.New("vertc.cli.invalid_flag", errs.TypeValidation,
			"%s", err.Error()).WithHint("run `%s --help` for valid flags", meta.BinName))
	}
	// Unexpected cobra error.
	return out().Fail(errs.Wrap(err, "vertc.cli.internal", "%s", err.Error()))
}

func isVersionProbe(args []string) bool {
	for _, arg := range args {
		if arg == "--version" || arg == "version" {
			return true
		}
	}
	return false
}

func setupNotices() {
	updatecheck.SetPending(nil)
	if info := updatecheck.CheckCached(meta.Version); info != nil {
		updatecheck.SetPending(info)
	}
	skillscheck.Init(meta.Version)
	output.SetNoticeProvider(func() map[string]any {
		notices := map[string]any{}
		if info := updatecheck.Pending(); info != nil {
			notices["update"] = info.Notice()
		}
		if skillNotice := skillscheck.Pending(); skillNotice != nil {
			notices["skills"] = skillNotice
		}
		if len(notices) == 0 {
			return nil
		}
		return notices
	})
}

func startUpdateRefreshProcess(version string) {
	if !updatecheck.NeedsRefresh(version) {
		return
	}
	if !updatecheck.TryAcquireRefreshLease() {
		return
	}
	executable, err := os.Executable()
	if err != nil {
		updatecheck.ReleaseRefreshLease()
		return
	}
	cmd := exec.Command(executable)
	cmd.Env = append(os.Environ(), updateRefreshChildEnv+"=1")
	if err := cmd.Start(); err != nil {
		updatecheck.ReleaseRefreshLease()
		return
	}
	_ = cmd.Process.Release()
}

func shouldRefreshAfterCommand(root *cobra.Command) bool {
	if flagDryRun {
		return false
	}
	cmd, _, err := root.Find(os.Args[1:])
	if err != nil || cmd == nil {
		return false
	}
	for current := cmd; current != nil; current = current.Parent() {
		if current.Name() == "sync" && current.Parent() != nil && current.Parent().Name() == "skills" {
			return false
		}
		switch current.Name() {
		case "update", "doctor", "completion":
			return false
		}
	}
	return true
}

func offerInteractiveUpdate() (bool, error) {
	info := updatecheck.Pending()
	if info == nil || !isTerminal(os.Stdin) || !isTerminal(os.Stdout) || !isTerminal(os.Stderr) {
		return false, nil
	}
	fmt.Fprintf(os.Stderr, "%s\nUpdate now? [y/N] ", info.Message())
	var answer string
	if _, err := fmt.Fscanln(os.Stdin, &answer); err != nil {
		return false, nil
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes", nil
}

func isTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}

// unknownCommandError produces a typed error with a nearest-candidate hint.
func unknownCommandError(root *cobra.Command, err error) *errs.Error {
	// Extract the offending token: `unknown command "foo" for "vertc"`.
	attempted := ""
	if i := strings.Index(err.Error(), "\""); i >= 0 {
		rest := err.Error()[i+1:]
		if j := strings.Index(rest, "\""); j >= 0 {
			attempted = rest[:j]
		}
	}
	e := errs.New("vertc.cli.unknown_command", errs.TypeNotFound,
		"unknown command %q", attempted)
	if cand := nearestCommand(root, attempted); cand != "" {
		e = e.WithHint("did you mean `%s %s`? run `%s --help` to list commands", meta.BinName, cand, meta.BinName)
	} else {
		e = e.WithHint("run `%s --help` to list available commands", meta.BinName)
	}
	return e
}

// nearestCommand returns the registered command name closest to attempted.
func nearestCommand(root *cobra.Command, attempted string) string {
	if attempted == "" {
		return ""
	}
	best, bestDist := "", 1<<30
	for _, c := range root.Commands() {
		if c.Hidden {
			continue
		}
		name := c.Name()
		d := levenshtein(attempted, name)
		if d < bestDist {
			best, bestDist = name, d
		}
	}
	// Only suggest when reasonably close.
	if bestDist <= len(attempted)/2+2 {
		return best
	}
	return ""
}

// levenshtein is the classic edit distance.
func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur := make([]int, len(rb)+1)
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min3(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(rb)]
}

func min3(a, b, c int) int {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	return m
}
