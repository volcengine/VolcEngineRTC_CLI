// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

// Package paths owns user-level lifecycle state paths for vertc.
package paths

import (
	"os"
	"path/filepath"
	"runtime"
)

// StateDir returns the platform-standard private directory used for update and
// skills state. VERTC_STATE_DIR is primarily useful for tests and managed
// environments that need a deterministic location.
func StateDir() string {
	if dir := os.Getenv("VERTC_STATE_DIR"); dir != "" {
		return dir
	}
	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, "vertc")
}

// WriteFileAtomic writes a private state file without exposing partial JSON to
// concurrent CLI processes.
func WriteFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil && runtime.GOOS != "windows" {
		return err
	}
	f, err := os.CreateTemp(dir, ".vertc-state-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		_ = os.Remove(path)
	}
	return os.Rename(tmp, path)
}
