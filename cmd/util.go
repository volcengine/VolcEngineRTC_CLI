// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package cmd

import "path/filepath"

// projectDir returns the directory containing the config at path.
// When path is empty (no config found), it falls back to the current dir.
func projectDir(path string) string {
	if path == "" {
		return "."
	}
	return filepath.Dir(path)
}
