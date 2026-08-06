// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
)

func TestPendingAuthorizationSaveLoadAndClaim(t *testing.T) {
	home := t.TempDir()
	t.Setenv("VERTC_HOME", home)
	now := time.Now().UTC().Truncate(time.Second)
	pending, err := NewPendingAuthorization("state-1", "verifier-1", DefaultRemoteClientID, DefaultRemoteRedirectURI, StoreFile, now, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := SavePendingAuthorization(pending); err != nil {
		t.Fatal(err)
	}
	path, err := PendingAuthorizationLocation()
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("pending mode = %v", info.Mode())
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("pending permissions = %04o, want 0600", info.Mode().Perm())
	}
	loaded, err := LoadPendingAuthorization(now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if loaded.State != pending.State || loaded.CodeVerifier != pending.CodeVerifier || loaded.CredentialStore != StoreFile {
		t.Fatalf("loaded pending = %+v", loaded)
	}

	claimed, cleanup, err := ClaimPendingAuthorization("state-1", now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if claimed.CodeVerifier != "verifier-1" {
		t.Fatalf("claimed = %+v", claimed)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pending path remains after claim: %v", err)
	}
	if _, _, err := ClaimPendingAuthorization("state-1", now.Add(time.Minute)); authErrorCode(err) != "vertc.auth.authorization_failed" {
		t.Fatalf("second claim error = %v", err)
	}
}

func TestPendingAuthorizationMismatchDoesNotConsume(t *testing.T) {
	t.Setenv("VERTC_HOME", t.TempDir())
	now := time.Now().UTC().Truncate(time.Second)
	pending, err := NewPendingAuthorization("right-state", "verifier", DefaultRemoteClientID, DefaultRemoteRedirectURI, "", now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := SavePendingAuthorization(pending); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ClaimPendingAuthorization("wrong-state", now); authErrorCode(err) != "vertc.auth.authorization_failed" {
		t.Fatalf("mismatch error = %v", err)
	}
	if _, err := LoadPendingAuthorization(now); err != nil {
		t.Fatalf("mismatch consumed pending authorization: %v", err)
	}
}

func TestPendingAuthorizationExpiryRemovesRecord(t *testing.T) {
	t.Setenv("VERTC_HOME", t.TempDir())
	now := time.Now().UTC().Truncate(time.Second)
	pending, err := NewPendingAuthorization("state", "verifier", DefaultRemoteClientID, DefaultRemoteRedirectURI, "", now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := SavePendingAuthorization(pending); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPendingAuthorization(now.Add(time.Minute)); authErrorCode(err) != "vertc.auth.authorization_failed" {
		t.Fatalf("expiry error = %v", err)
	}
	path, _ := PendingAuthorizationLocation()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expired record remains: %v", err)
	}
}

func TestPendingAuthorizationRejectsUnsafeFiles(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, path string)
	}{
		{
			name: "symlink",
			setup: func(t *testing.T, path string) {
				t.Helper()
				target := filepath.Join(filepath.Dir(path), "target")
				if err := os.WriteFile(target, []byte("{}"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "insecure permissions",
			setup: func(t *testing.T, path string) {
				t.Helper()
				if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "corrupt",
			setup: func(t *testing.T, path string) {
				t.Helper()
				if err := os.WriteFile(path, []byte("not-json"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "oversized",
			setup: func(t *testing.T, path string) {
				t.Helper()
				if err := os.WriteFile(path, make([]byte, (1<<20)+1), 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if runtime.GOOS == "windows" && tc.name == "insecure permissions" {
				t.Skip("POSIX permission check")
			}
			home := t.TempDir()
			t.Setenv("VERTC_HOME", home)
			path, err := PendingAuthorizationLocation()
			if err != nil {
				t.Fatal(err)
			}
			tc.setup(t, path)
			if _, err := LoadPendingAuthorization(time.Now()); authErrorCode(err) != "vertc.auth.storage_unavailable" {
				t.Fatalf("unsafe file error = %v", err)
			}
		})
	}
}

func TestPendingAuthorizationConcurrentClaimAtMostOnce(t *testing.T) {
	t.Setenv("VERTC_HOME", t.TempDir())
	now := time.Now().UTC().Truncate(time.Second)
	pending, err := NewPendingAuthorization("state", "verifier", DefaultRemoteClientID, DefaultRemoteRedirectURI, "", now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := SavePendingAuthorization(pending); err != nil {
		t.Fatal(err)
	}
	const contenders = 8
	start := make(chan struct{})
	results := make(chan bool, contenders)
	var wg sync.WaitGroup
	for range contenders {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, cleanup, err := ClaimPendingAuthorization("state", now)
			if err == nil {
				defer cleanup()
				results <- true
				return
			}
			results <- false
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	winners := 0
	for won := range results {
		if won {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("successful claims = %d, want 1", winners)
	}
}

func authErrorCode(err error) string {
	typed, ok := errs.As(err)
	if !ok {
		return ""
	}
	return typed.Code
}
