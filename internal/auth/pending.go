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
	"strings"
	"time"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/meta"
)

const (
	pendingAuthorizationVersion = 1
	pendingAuthorizationFile    = "auth-pending.json"
	pendingAuthorizationClaim   = "auth-pending.claim"
)

// PendingAuthorization is the short-lived OAuth context needed to finish a
// cross-process manual login. CodeVerifier must never be printed.
type PendingAuthorization struct {
	Version         int    `json:"version"`
	State           string `json:"state"`
	CodeVerifier    string `json:"code_verifier"`
	ClientID        string `json:"client_id"`
	RedirectURI     string `json:"redirect_uri"`
	CredentialStore string `json:"credential_store,omitempty"`
	CreatedAt       string `json:"created_at"`
	ExpiresAt       string `json:"expires_at"`
}

// NewPendingAuthorization creates a validated, versioned pending record.
func NewPendingAuthorization(state, verifier, clientID, redirectURI, credentialStore string, now time.Time, ttl time.Duration) (PendingAuthorization, error) {
	pending := PendingAuthorization{
		Version:         pendingAuthorizationVersion,
		State:           strings.TrimSpace(state),
		CodeVerifier:    strings.TrimSpace(verifier),
		ClientID:        strings.TrimSpace(clientID),
		RedirectURI:     strings.TrimSpace(redirectURI),
		CredentialStore: strings.TrimSpace(credentialStore),
		CreatedAt:       now.UTC().Format(time.RFC3339),
		ExpiresAt:       now.Add(ttl).UTC().Format(time.RFC3339),
	}
	if err := validatePendingAuthorization(pending, now, false); err != nil {
		return PendingAuthorization{}, err
	}
	return pending, nil
}

// PendingAuthorizationLocation returns the private user-level pending path.
func PendingAuthorizationLocation() (string, error) {
	authPath, err := authFilePath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(authPath), pendingAuthorizationFile), nil
}

// SavePendingAuthorization atomically replaces the single pending transaction.
func SavePendingAuthorization(pending PendingAuthorization) error {
	if err := validatePendingAuthorization(pending, time.Now(), false); err != nil {
		return err
	}
	path, err := PendingAuthorizationLocation()
	if err != nil {
		return err
	}
	if err := removePendingClaim(path); err != nil {
		return err
	}
	return writePendingAuthorization(path, pending)
}

// LoadPendingAuthorization safely loads a live pending transaction.
func LoadPendingAuthorization(now time.Time) (PendingAuthorization, error) {
	path, err := PendingAuthorizationLocation()
	if err != nil {
		return PendingAuthorization{}, err
	}
	pending, err := readPendingAuthorization(path)
	if errors.Is(err, os.ErrNotExist) {
		return PendingAuthorization{}, pendingAuthorizationError("no pending manual authorization")
	}
	if err != nil {
		return PendingAuthorization{}, err
	}
	if err := validatePendingAuthorization(pending, now, true); err != nil {
		if typed, ok := errs.As(err); ok && typed.Code == "vertc.auth.authorization_failed" {
			_ = os.Remove(path)
		}
		return PendingAuthorization{}, err
	}
	return pending, nil
}

// ClaimPendingAuthorization validates state and atomically makes the pending
// transaction unavailable to all other processes. cleanup removes the private
// claim file after the exchange attempt.
func ClaimPendingAuthorization(expectedState string, now time.Time) (pending PendingAuthorization, cleanup func(), err error) {
	pending, err = LoadPendingAuthorization(now)
	if err != nil {
		return PendingAuthorization{}, nil, err
	}
	if strings.TrimSpace(expectedState) != pending.State {
		return PendingAuthorization{}, nil, pendingAuthorizationError("authorization state mismatch")
	}
	path, err := PendingAuthorizationLocation()
	if err != nil {
		return PendingAuthorization{}, nil, err
	}
	claimPath := filepath.Join(filepath.Dir(path), pendingAuthorizationClaim)
	if err := os.Link(path, claimPath); err != nil {
		if errors.Is(err, os.ErrExist) || errors.Is(err, os.ErrNotExist) {
			return PendingAuthorization{}, nil, pendingAuthorizationError("pending manual authorization was already consumed")
		}
		return PendingAuthorization{}, nil, storageErr("claim pending authorization", err)
	}
	cleanup = func() { _ = os.Remove(claimPath) }
	if err := os.Remove(path); err != nil {
		cleanup()
		return PendingAuthorization{}, nil, storageErr("consume pending authorization", err)
	}
	claimed, err := readPendingAuthorization(claimPath)
	if err != nil {
		cleanup()
		return PendingAuthorization{}, nil, err
	}
	if claimed.State != pending.State || claimed.CodeVerifier != pending.CodeVerifier {
		cleanup()
		return PendingAuthorization{}, nil, storageErr("claim pending authorization", fmt.Errorf("pending authorization changed while being claimed"))
	}
	return claimed, cleanup, nil
}

func readPendingAuthorization(path string) (PendingAuthorization, error) {
	info, err := safeAuthFileInfo(path)
	if err != nil {
		return PendingAuthorization{}, err
	}
	if !info.Mode().IsRegular() {
		return PendingAuthorization{}, storageErr("read pending authorization", fmt.Errorf("path is not a regular file"))
	}
	if hasInsecurePOSIXPermissions(info) {
		return PendingAuthorization{}, storageErr("read pending authorization", fmt.Errorf("permissions %04o allow group or other access", info.Mode().Perm()))
	}
	if info.Size() > 1<<20 {
		return PendingAuthorization{}, storageErr("read pending authorization", fmt.Errorf("file exceeds 1 MiB limit"))
	}
	f, err := os.Open(path)
	if err != nil {
		return PendingAuthorization{}, storageErr("open pending authorization", err)
	}
	defer f.Close()
	var pending PendingAuthorization
	decoder := json.NewDecoder(io.LimitReader(f, 1<<20))
	if err := decoder.Decode(&pending); err != nil {
		return PendingAuthorization{}, storageErr("parse pending authorization", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			err = fmt.Errorf("multiple JSON values")
		}
		return PendingAuthorization{}, storageErr("parse pending authorization", err)
	}
	return pending, nil
}

func writePendingAuthorization(path string, pending PendingAuthorization) error {
	dir := filepath.Dir(path)
	if err := ensureSecureAuthDir(dir); err != nil {
		return err
	}
	if info, err := safeAuthFileInfo(path); err == nil {
		if !info.Mode().IsRegular() {
			return storageErr("write pending authorization", fmt.Errorf("path is not a regular file"))
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".auth-pending-*.tmp")
	if err != nil {
		return storageErr("create temporary pending authorization", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return storageErr("secure temporary pending authorization", err)
	}
	encoder := json.NewEncoder(tmp)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(pending); err != nil {
		_ = tmp.Close()
		return storageErr("serialize pending authorization", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return storageErr("sync pending authorization", err)
	}
	if err := tmp.Close(); err != nil {
		return storageErr("close pending authorization", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return storageErr("replace pending authorization", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return storageErr("secure pending authorization", err)
	}
	return nil
}

func removePendingClaim(pendingPath string) error {
	claimPath := filepath.Join(filepath.Dir(pendingPath), pendingAuthorizationClaim)
	info, err := safeAuthFileInfo(claimPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return storageErr("remove stale pending authorization claim", fmt.Errorf("path is not a regular file"))
	}
	if err := os.Remove(claimPath); err != nil {
		return storageErr("remove stale pending authorization claim", err)
	}
	return nil
}

func validatePendingAuthorization(pending PendingAuthorization, now time.Time, enforceExpiry bool) error {
	if pending.Version != pendingAuthorizationVersion {
		return storageErr("parse pending authorization", fmt.Errorf("unsupported version %d", pending.Version))
	}
	if pending.State == "" || pending.CodeVerifier == "" || pending.ClientID == "" || pending.RedirectURI == "" {
		return storageErr("parse pending authorization", fmt.Errorf("required OAuth context is missing"))
	}
	if pending.CredentialStore != "" {
		if err := ValidateStoreMode(pending.CredentialStore); err != nil {
			return storageErr("parse pending authorization", err)
		}
	}
	createdAt, err := time.Parse(time.RFC3339, pending.CreatedAt)
	if err != nil {
		return storageErr("parse pending authorization", fmt.Errorf("invalid created_at"))
	}
	expiresAt, err := time.Parse(time.RFC3339, pending.ExpiresAt)
	if err != nil || !expiresAt.After(createdAt) {
		return storageErr("parse pending authorization", fmt.Errorf("invalid expires_at"))
	}
	if enforceExpiry && !now.Before(expiresAt) {
		return pendingAuthorizationError("pending manual authorization expired")
	}
	return nil
}

func pendingAuthorizationError(message string) *errs.Error {
	return errs.New("vertc.auth.authorization_failed", errs.TypeAuth, "%s", message).
		WithHint("run `%s auth login --browser=manual --start` and authorize the new URL", meta.BinName)
}
