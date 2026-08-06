// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
)

func TestDefaultStoreUsesSecureFileWithoutKeyring(t *testing.T) {
	home := t.TempDir()
	t.Setenv("VERTC_HOME", home)
	restore := SetTokenStoreForTest(nil)
	defer restore()

	previousGet, previousSet := keyringGet, keyringSet
	keyringGet = func(string, string) (string, error) {
		t.Fatal("default file store must not read keyring")
		return "", nil
	}
	keyringSet = func(string, string, string) error {
		t.Fatal("default file store must not write keyring")
		return nil
	}
	defer func() { keyringGet, keyringSet = previousGet, previousSet }()

	want := TokenSet{AccessToken: "access", RefreshToken: "refresh"}
	if err := SaveToken(want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadToken()
	if err != nil {
		t.Fatal(err)
	}
	if got.AccessToken != want.AccessToken || got.RefreshToken != want.RefreshToken {
		t.Fatalf("token = %+v", got)
	}
	path := filepath.Join(home, "auth.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("auth mode = %04o", info.Mode().Perm())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"version": 1`) || !strings.Contains(string(data), `"store": "file"`) {
		t.Fatalf("unexpected envelope: %s", data)
	}
}

func TestAuthFilePathRejectsUnavailableUserHome(t *testing.T) {
	t.Setenv("VERTC_HOME", "")
	previousUserHomeDir := userHomeDir
	userHomeDir = func() (string, error) { return "", errors.New("home unavailable") }
	defer func() { userHomeDir = previousUserHomeDir }()

	path, err := authFilePath()
	if path != "" {
		t.Fatalf("auth path = %q", path)
	}
	assertStorageError(t, err)
	if !strings.Contains(err.Error(), "resolve auth file") {
		t.Fatalf("error = %v", err)
	}
}

func TestFileStoreAtomicallyReplacesToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	store := FileTokenStore{Path: path}
	if err := store.Save(TokenSet{AccessToken: "old"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(TokenSet{AccessToken: "new", RefreshToken: "refresh"}); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.AccessToken != "new" || got.RefreshToken != "refresh" {
		t.Fatalf("token = %+v", got)
	}
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".auth-*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary files remain: %v", matches)
	}
}

func TestFileStoreRejectsUnsafePaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission and symlink contract")
	}
	t.Run("permissions", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "auth.json")
		if err := os.WriteFile(path, []byte(`{"version":1,"store":"file","token":{}}`), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := (FileTokenStore{Path: path}).Load()
		assertStorageError(t, err)
	})
	t.Run("symlink", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "target")
		if err := os.WriteFile(target, []byte(`{"version":1,"store":"file","token":{}}`), 0o600); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "auth.json")
		if err := os.Symlink(target, path); err != nil {
			t.Fatal(err)
		}
		_, err := (FileTokenStore{Path: path}).Load()
		assertStorageError(t, err)
		assertStorageError(t, (FileTokenStore{Path: path}).Save(TokenSet{AccessToken: "secret"}))
	})
	t.Run("non-regular", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "auth.json")
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
		_, err := (FileTokenStore{Path: path}).Load()
		assertStorageError(t, err)
	})
}

// TestInsecureAuthFileRecoversViaResolvingStore proves the strict read guard
// (refuse to reuse a group/other-readable credential file) no longer locks the
// user out: re-login (Save) repairs the file to 0600 and logout (Delete) removes
// it, even though a plain Load still refuses to reuse it.
func TestInsecureAuthFileRecoversViaResolvingStore(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission contract")
	}
	home := t.TempDir()
	t.Setenv("VERTC_HOME", home)
	restore := SetTokenStoreForTest(nil) // exercise the default resolving store
	defer restore()

	path := filepath.Join(home, "auth.json")
	// Simulate a botched restore / umask: a valid file envelope with lax perms.
	if err := os.WriteFile(path, []byte(`{"version":1,"store":"file","token":{"access_token":"old"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadToken(); err == nil {
		t.Fatal("LoadToken must refuse an insecure file")
	}
	// Re-login recovers and repairs permissions.
	if err := SaveToken(TokenSet{AccessToken: "new"}); err != nil {
		t.Fatalf("SaveToken recovery: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("auth mode after recovery = %04o, want 0600", info.Mode().Perm())
	}
	if got, err := LoadToken(); err != nil || got.AccessToken != "new" {
		t.Fatalf("LoadToken after recovery: token=%+v err=%v", got, err)
	}
	// Logout also recovers even if the file is re-laxed.
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := DeleteToken(); err != nil {
		t.Fatalf("DeleteToken recovery: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("auth file still present after logout: %v", err)
	}
}

// TestInsecureKeyringFileKeepsKeyringRoutingOnRecovery proves the recovery path
// does not silently downgrade a keyring setup to file storage: even when the
// keyring metadata file is group/other-readable, logout still clears the OS
// keyring and Save still writes to the keyring (keeping store=keyring).
func TestInsecureKeyringFileKeepsKeyringRoutingOnRecovery(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission contract")
	}
	home := t.TempDir()
	t.Setenv("VERTC_HOME", home)
	restore := SetTokenStoreForTest(nil) // exercise the default resolving store
	defer restore()

	var stored string
	previousGet, previousSet, previousDelete := keyringGet, keyringSet, keyringDelete
	keyringSet = func(_, _, v string) error { stored = v; return nil }
	keyringGet = func(string, string) (string, error) {
		if stored == "" {
			return "", keyring.ErrNotFound
		}
		return stored, nil
	}
	keyringDelete = func(string, string) error { stored = ""; return nil }
	defer func() { keyringGet, keyringSet, keyringDelete = previousGet, previousSet, previousDelete }()
	stored = `{"access_token":"keyring-token"}`

	path := filepath.Join(home, "auth.json")
	writeInsecureKeyringMeta := func() {
		if err := os.WriteFile(path, []byte(`{"version":1,"store":"keyring"}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// logout must clear the OS keyring, not merely delete the metadata file.
	writeInsecureKeyringMeta()
	if err := DeleteToken(); err != nil {
		t.Fatalf("DeleteToken: %v", err)
	}
	if stored != "" {
		t.Fatal("logout left the keyring token behind (routed to file store)")
	}

	// Save must keep writing to the keyring — never downgrade store to file.
	writeInsecureKeyringMeta()
	if err := SaveToken(TokenSet{AccessToken: "new-keyring"}); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}
	if stored == "" {
		t.Fatal("Save did not write to the keyring")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"store": "keyring"`) {
		t.Fatalf("Save downgraded the store selection: %s", data)
	}
}

func TestSelectedStoreIsKeyringRequiresValidEnvelope(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
		want    bool
	}{
		{name: "valid keyring", content: `{"version":1,"store":"keyring"}`, want: true},
		{name: "missing version", content: `{"store":"keyring"}`},
		{name: "wrong version", content: `{"version":2,"store":"keyring"}`},
		{name: "file store", content: `{"version":1,"store":"file"}`},
		{name: "malformed", content: `{"version":1,"store":"keyring"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "auth.json")
			if err := os.WriteFile(path, []byte(tc.content), 0o600); err != nil {
				t.Fatal(err)
			}
			if got := selectedStoreIsKeyring(path); got != tc.want {
				t.Fatalf("selectedStoreIsKeyring() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestExplicitKeyringPersistsSelectionWithoutToken(t *testing.T) {
	home := t.TempDir()
	t.Setenv("VERTC_HOME", home)
	restoreStore, err := SetTokenStoreModeForCommand(StoreKeyring)
	if err != nil {
		t.Fatal(err)
	}
	defer restoreStore()

	previousGet, previousSet, previousDelete := keyringGet, keyringSet, keyringDelete
	var serialized string
	keyringSet = func(_, _, value string) error { serialized = value; return nil }
	keyringGet = func(string, string) (string, error) {
		if serialized == "" {
			return "", keyring.ErrNotFound
		}
		return serialized, nil
	}
	keyringDelete = func(string, string) error { serialized = ""; return nil }
	defer func() { keyringGet, keyringSet, keyringDelete = previousGet, previousSet, previousDelete }()

	if err := SaveToken(TokenSet{AccessToken: "access-keyring", RefreshToken: "refresh-keyring"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(home, "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "access-keyring") || strings.Contains(string(data), "refresh-keyring") {
		t.Fatalf("keyring metadata leaked token: %s", data)
	}
	if !strings.Contains(string(data), `"store": "keyring"`) {
		t.Fatalf("missing keyring selection: %s", data)
	}
}

func TestKeyringSaveRollsBackWhenMetadataWriteFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}

	previousSet, previousDelete := keyringSet, keyringDelete
	keyringSet = func(string, string, string) error { return nil }
	deleteCalls := 0
	keyringDelete = func(string, string) error {
		deleteCalls++
		return nil
	}
	defer func() { keyringSet, keyringDelete = previousSet, previousDelete }()

	err := (KeyringTokenStore{MetadataPath: path}).Save(TokenSet{AccessToken: "access"})
	assertStorageError(t, err)
	if deleteCalls != 1 {
		t.Fatalf("keyring rollback calls = %d", deleteCalls)
	}

	keyringDelete = func(string, string) error { return errors.New("rollback denied") }
	err = (KeyringTokenStore{MetadataPath: path}).Save(TokenSet{AccessToken: "access"})
	assertStorageError(t, err)
	if !strings.Contains(err.Error(), "rollback token") {
		t.Fatalf("rollback error = %v", err)
	}
}

func TestSTSProviderUsesDefaultFileStoreWithoutKeyring(t *testing.T) {
	home := t.TempDir()
	t.Setenv("VERTC_HOME", home)
	restore := SetTokenStoreForTest(nil)
	defer restore()
	previousGet := keyringGet
	keyringGet = func(string, string) (string, error) {
		t.Fatal("file-backed STS provider must not read keyring")
		return "", nil
	}
	defer func() { keyringGet = previousGet }()

	sts, err := json.Marshal(STSCredential{AccessKeyID: "AKID", SecretAccessKey: "SECRET", SessionToken: "SESSION"})
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveToken(TokenSet{AccessToken: string(sts)}); err != nil {
		t.Fatal(err)
	}
	credential, err := (STSProvider{}).Credentials(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if credential.AccessKeyID != "AKID" || credential.SecretAccessKey != "SECRET" || credential.SessionToken != "SESSION" {
		t.Fatalf("credential = %+v", credential)
	}
}

func TestDeletePreservesExplicitKeyringSelection(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "auth.json")
	previousDelete := keyringDelete
	keyringDelete = func(string, string) error { return keyring.ErrNotFound }
	defer func() { keyringDelete = previousDelete }()

	store := KeyringTokenStore{MetadataPath: path}
	if err := store.Delete(); err != nil {
		t.Fatal(err)
	}
	envelope, err := readAuthEnvelope(path)
	if err != nil {
		t.Fatal(err)
	}
	if envelope.Store != StoreKeyring || envelope.Token != nil {
		t.Fatalf("envelope = %+v", envelope)
	}
}

func TestPersistExplicitKeyringModeContainsNoToken(t *testing.T) {
	home := t.TempDir()
	t.Setenv("VERTC_HOME", home)
	if err := PersistStoreMode(StoreKeyring); err != nil {
		t.Fatal(err)
	}
	envelope, err := readAuthEnvelope(filepath.Join(home, "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	if envelope.Store != StoreKeyring || envelope.Token != nil {
		t.Fatalf("envelope = %+v", envelope)
	}
}

func assertStorageError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected storage error")
	}
	if errors.Is(err, os.ErrNotExist) {
		t.Fatalf("got not-exist instead of storage error: %v", err)
	}
	typed, ok := asTypedError(err)
	if !ok || typed.Code != "vertc.auth.storage_unavailable" {
		t.Fatalf("error = %v", err)
	}
}

func asTypedError(err error) (*errs.Error, bool) {
	return errs.As(err)
}
