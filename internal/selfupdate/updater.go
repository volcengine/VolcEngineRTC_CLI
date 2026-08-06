// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

// Package selfupdate updates npm-managed vertc installations and synchronizes
// the official Agent skills shipped by the repository.
package selfupdate

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/meta"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/skillscheck"
	skillcontent "github.com/volcengine/VolcEngineRTC_CLI/skills"
)

const (
	NpmPackage = "@volcengine/rtc-cli"
	SkillsRepo = "volcengine/VolcEngineRTC_CLI"
)

type InstallMethod string

const (
	InstallNPM    InstallMethod = "npm"
	InstallManual InstallMethod = "manual"
)

var (
	ExecutableOverride   func() (string, error)
	NpmInstallOverride   func(context.Context, string, string) error
	VerifyOverride       func(context.Context, string, string) error
	SkillsSyncOverride   func(context.Context, string) (SkillsSyncResult, error)
	SkillsAddOverride    func(context.Context, string, ...string) error
	SkillsRemoveOverride func(context.Context, string, ...string) error
)

// SkillsSyncResult reports recoverable target-specific synchronization warnings.
type SkillsSyncResult struct {
	Warnings []string
}

// DetectInstallMethod classifies launchers below node_modules as npm-managed.
func DetectInstallMethod() (InstallMethod, string, error) {
	var (
		path string
		err  error
	)
	if ExecutableOverride != nil {
		path, err = ExecutableOverride()
	} else {
		path, err = os.Executable()
	}
	if err != nil {
		return InstallManual, "", err
	}
	resolved, resolveErr := filepath.EvalSymlinks(path)
	if resolveErr == nil {
		path = resolved
	}
	normalized := strings.ToLower(filepath.ToSlash(path))
	if strings.Contains(normalized, "/node_modules/") {
		return InstallNPM, path, nil
	}
	return InstallManual, path, nil
}

// RunNpmInstall installs an exact package version globally.
func RunNpmInstall(ctx context.Context, version, executablePath string) error {
	if NpmInstallOverride != nil {
		return NpmInstallOverride(ctx, version, executablePath)
	}
	prefix, err := npmPrefix(executablePath)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "npm", "install", "-g", NpmPackage+"@"+strings.TrimPrefix(version, "v"))
	cmd.Env = append(os.Environ(), "npm_config_prefix="+prefix)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	return cmd.Run()
}

// RollbackBinary restores the installer backup when one exists. It is safe to
// call after either npm or verification failure on every platform.
func RollbackBinary(executablePath string) error {
	backup := executablePath + ".old"
	if _, err := os.Stat(backup); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := os.Remove(executablePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(backup, executablePath)
}

// HasBackup reports whether the installer retained a previous executable.
func HasBackup(executablePath string) bool {
	_, err := os.Stat(executablePath + ".old")
	return err == nil
}

// CleanupBackup removes a verified binary's stale installer backup.
func CleanupBackup(executablePath string) error {
	err := os.Remove(executablePath + ".old")
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// CleanupHealthyBackup removes the sibling Windows backup after this binary
// has started successfully. Direct binary launches do not pass through run.js.
func CleanupHealthyBackup() {
	if runtime.GOOS != "windows" {
		return
	}
	path, err := os.Executable()
	if err == nil {
		_ = CleanupBackup(path)
	}
}

func npmPrefix(executablePath string) (string, error) {
	clean := filepath.Clean(executablePath)
	parts := strings.Split(filepath.ToSlash(clean), "/")
	index := -1
	for i, part := range parts {
		if strings.EqualFold(part, "node_modules") {
			index = i
			break
		}
	}
	if index <= 0 {
		return "", fmt.Errorf("cannot derive npm prefix from %s", executablePath)
	}
	prefixParts := parts[:index]
	if len(prefixParts) > 0 && strings.EqualFold(prefixParts[len(prefixParts)-1], "lib") {
		prefixParts = prefixParts[:len(prefixParts)-1]
	}
	prefix := strings.Join(prefixParts, "/")
	if filepath.IsAbs(clean) && filepath.VolumeName(clean) == "" && !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	return filepath.FromSlash(prefix), nil
}

// VerifyBinary verifies the newly resolved binary reports the expected version.
func VerifyBinary(ctx context.Context, expected, executablePath string) error {
	if VerifyOverride != nil {
		return VerifyOverride(ctx, expected, executablePath)
	}
	path := executablePath
	if path == "" {
		var err error
		path, err = exec.LookPath(meta.BinName)
		if err != nil {
			return err
		}
	}
	cmd := exec.CommandContext(ctx, path, "--version")
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("verify %s: %w: %s", path, err, strings.TrimSpace(output.String()))
	}
	fields := strings.Fields(output.String())
	if len(fields) < 2 {
		return fmt.Errorf("unexpected version output %q", strings.TrimSpace(output.String()))
	}
	got := strings.TrimPrefix(fields[1], "v")
	want := strings.TrimPrefix(expected, "v")
	if got != want {
		return fmt.Errorf("installed version %s, expected %s", got, want)
	}
	return nil
}

// SyncSkills installs all official repository skills globally and records the
// binary version after the external tool succeeds or only skips a known
// project-only target.
func SyncSkills(ctx context.Context, version string) (SkillsSyncResult, error) {
	if SkillsSyncOverride != nil {
		return SkillsSyncOverride(ctx, version)
	}
	var result SkillsSyncResult
	temp, err := os.MkdirTemp("", "vertc-skills-*")
	if err != nil {
		return result, err
	}
	defer os.RemoveAll(temp)
	if err := materializeSkills(temp); err != nil {
		return result, err
	}
	warning, err := runSkillsAdd(ctx, temp, "-g", "--copy", "-y")
	if err != nil {
		return result, err
	}
	if warning != "" {
		result.Warnings = append(result.Warnings, warning)
	}
	for _, name := range []string{"byted-interactai-voice-agent", "vertc-voice-agent"} {
		if err := runSkillsRemove(ctx, name, "-g", "-y"); err != nil {
			return result, fmt.Errorf("remove retired skill %s: %w", name, err)
		}
	}
	return result, skillscheck.MarkSynced(version)
}

func runSkillsAdd(ctx context.Context, source string, args ...string) (string, error) {
	if SkillsAddOverride != nil {
		return "", SkillsAddOverride(ctx, source, args...)
	}
	cmdArgs := append([]string{"skills", "add", source}, args...)
	cmd := exec.CommandContext(ctx, "npx", cmdArgs...)
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	err := cmd.Run()
	if skillsAddOnlyUnsupportedPromptScript(output.String()) {
		return "PromptScript was skipped because it does not support global Skill installation; other supported Agent targets were synchronized", nil
	}
	if output.Len() > 0 {
		_, _ = os.Stderr.Write(output.Bytes())
	}
	return "", err
}

func skillsAddOnlyUnsupportedPromptScript(output string) bool {
	return strings.Contains(output, "Failed to install 1") &&
		strings.Count(output, "✗") == 1 &&
		strings.Contains(output, "PromptScript does not support global skill installation")
}

func runSkillsRemove(ctx context.Context, name string, args ...string) error {
	if SkillsRemoveOverride != nil {
		return SkillsRemoveOverride(ctx, name, args...)
	}
	cmdArgs := append([]string{"skills", "remove", name}, args...)
	cmd := exec.CommandContext(ctx, "npx", cmdArgs...)
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	err := cmd.Run()
	if output.Len() > 0 {
		_, _ = os.Stderr.Write(output.Bytes())
	}
	if err != nil {
		return err
	}
	if reason := skillsRemoveOutputError(output.String()); reason != "" {
		return fmt.Errorf("skills remove reported failure: %s", reason)
	}
	return nil
}

func skillsRemoveOutputError(output string) string {
	for _, marker := range []string{"Failed to remove", "Could not remove skill", "Could not scan directory"} {
		if strings.Contains(output, marker) {
			return marker
		}
	}
	return ""
}

func materializeSkills(root string) error {
	return fs.WalkDir(skillcontent.Content, ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.Contains(path, "/") {
			return nil
		}
		data, err := skillcontent.Content.ReadFile(path)
		if err != nil {
			return err
		}
		destination := filepath.Join(root, "skills", filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
			return err
		}
		return os.WriteFile(destination, data, 0o600)
	})
}
