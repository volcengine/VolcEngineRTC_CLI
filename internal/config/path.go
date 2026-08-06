// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"path/filepath"
	"strings"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
)

// ProjectFilePath validates that a project-relative file reference (e.g.
// agent.config_file) stays inside the project directory and returns the joined
// target path. It rejects absolute paths and paths that escape dir via "..",
// so a config value from an untrusted project cannot make the CLI read or write
// files outside dir (path traversal).
func ProjectFilePath(dir, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errs.New("vertc.config.invalid_field", errs.TypeValidation,
			"empty file path").WithParam("agent.config_file")
	}
	if filepath.IsAbs(name) {
		return "", errs.New("vertc.config.invalid_field", errs.TypeValidation,
			"path %q must be project-relative, not absolute", name).
			WithParam("agent.config_file").
			WithHint("use a path inside the project, e.g. server/scenes/default.json")
	}
	target := filepath.Join(dir, name)
	rel, err := filepath.Rel(dir, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errs.New("vertc.config.invalid_field", errs.TypeValidation,
			"path %q escapes the project directory", name).
			WithParam("agent.config_file").
			WithHint("use a path inside the project, e.g. server/scenes/default.json")
	}
	return target, nil
}
