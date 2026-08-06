// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

type FileTokenStore struct {
	Path string
}

func (s FileTokenStore) Load() (TokenSet, error) {
	envelope, err := readAuthEnvelope(s.Path)
	if err != nil {
		return TokenSet{}, err
	}
	if envelope.Store != StoreFile || envelope.Token == nil {
		return TokenSet{}, os.ErrNotExist
	}
	return *envelope.Token, nil
}

func (s FileTokenStore) Save(token TokenSet) error {
	return writeAuthEnvelope(s.Path, authEnvelope{
		Version: authEnvelopeVersion,
		Store:   StoreFile,
		Token:   &token,
	})
}

func (s FileTokenStore) Delete() error {
	info, err := safeAuthFileInfo(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return storageErr("delete auth file", fmt.Errorf("path is not a regular file"))
	}
	if err := os.Remove(s.Path); err != nil {
		return storageErr("delete auth file", err)
	}
	return nil
}

func (s FileTokenStore) Location() string { return "file:" + s.Path }

func readAuthEnvelope(path string) (authEnvelope, error) {
	info, err := safeAuthFileInfo(path)
	if err != nil {
		return authEnvelope{}, err
	}
	if !info.Mode().IsRegular() {
		return authEnvelope{}, storageErr("read auth file", fmt.Errorf("path is not a regular file"))
	}
	if hasInsecurePOSIXPermissions(info) {
		return authEnvelope{}, storageErr("read auth file", fmt.Errorf("permissions %04o allow group or other access", info.Mode().Perm()))
	}
	f, err := os.Open(path)
	if err != nil {
		return authEnvelope{}, storageErr("open auth file", err)
	}
	defer f.Close()
	var envelope authEnvelope
	decoder := json.NewDecoder(io.LimitReader(f, 1<<20))
	if err := decoder.Decode(&envelope); err != nil {
		return authEnvelope{}, storageErr("parse auth file", err)
	}
	if envelope.Version != authEnvelopeVersion {
		return authEnvelope{}, storageErr("parse auth file", fmt.Errorf("unsupported version %d", envelope.Version))
	}
	if err := ValidateStoreMode(envelope.Store); err != nil {
		return authEnvelope{}, storageErr("parse auth file", err)
	}
	return envelope, nil
}

func safeAuthFileInfo(path string) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, os.ErrNotExist
	}
	if err != nil {
		return nil, storageErr("inspect auth file", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, storageErr("inspect auth file", fmt.Errorf("symbolic links are not allowed"))
	}
	return info, nil
}

func writeAuthEnvelope(path string, envelope authEnvelope) error {
	dir := filepath.Dir(path)
	if err := ensureSecureAuthDir(dir); err != nil {
		return err
	}
	if info, err := safeAuthFileInfo(path); err == nil {
		if !info.Mode().IsRegular() {
			return storageErr("write auth file", fmt.Errorf("path is not a regular file"))
		}
		// A group/other-accessible existing file is repaired here rather than
		// rejected: the atomic replace below rewrites it as a fresh 0600 file, so
		// refusing would only lock the user out of re-login with no in-CLI recovery.
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".auth-*.tmp")
	if err != nil {
		return storageErr("create temporary auth file", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return storageErr("secure temporary auth file", err)
	}
	encoder := json.NewEncoder(tmp)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(envelope); err != nil {
		_ = tmp.Close()
		return storageErr("serialize auth file", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return storageErr("sync auth file", err)
	}
	if err := tmp.Close(); err != nil {
		return storageErr("close auth file", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return storageErr("replace auth file", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return storageErr("secure auth file", err)
	}
	return nil
}

func ensureSecureAuthDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return storageErr("create auth directory", err)
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return storageErr("inspect auth directory", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return storageErr("inspect auth directory", fmt.Errorf("path is not a real directory"))
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return storageErr("secure auth directory", err)
	}
	info, err = os.Lstat(dir)
	if err != nil {
		return storageErr("inspect auth directory", err)
	}
	if hasInsecurePOSIXPermissions(info) {
		return storageErr("inspect auth directory", fmt.Errorf("permissions %04o allow group or other access", info.Mode().Perm()))
	}
	return nil
}

func hasInsecurePOSIXPermissions(info os.FileInfo) bool {
	return runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0
}
