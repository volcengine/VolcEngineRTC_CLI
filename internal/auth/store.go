// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/meta"
)

const (
	tokenStoreAccount   = "default"
	authEnvelopeVersion = 1
	StoreFile           = "file"
	StoreKeyring        = "keyring"
)

type TokenStore interface {
	Load() (TokenSet, error)
	Save(TokenSet) error
	Delete() error
	Location() string
}

var (
	tokenStoreMu  sync.Mutex
	tokenStore    TokenStore
	keyringGet    = keyring.Get
	keyringSet    = keyring.Set
	keyringDelete = keyring.Delete
	userHomeDir   = os.UserHomeDir
)

func LoadToken() (TokenSet, error) {
	return currentTokenStore().Load()
}

func SaveToken(token TokenSet) error {
	return currentTokenStore().Save(token)
}

func DeleteToken() error {
	return currentTokenStore().Delete()
}

func TokenLocation() string {
	return currentTokenStore().Location()
}

func IsAuthenticated() bool {
	token, err := LoadToken()
	return err == nil && token.AccessToken != ""
}

// EnsureFreshToken loads the stored token and refreshes it when it is already
// expired or within skew of expiry.
func EnsureFreshToken(ctx context.Context, httpClient *http.Client, skew time.Duration, tokenEndpoint string) (TokenSet, bool, error) {
	token, err := LoadToken()
	if errors.Is(err, os.ErrNotExist) {
		return TokenSet{}, false, errs.New("vertc.auth.not_authenticated", errs.TypeAuth,
			"not logged in").
			WithHint("run `%s auth login` first", meta.BinName)
	}
	if err != nil {
		return TokenSet{}, false, err
	}
	if token.AccessToken == "" {
		return TokenSet{}, false, errs.New("vertc.auth.not_authenticated", errs.TypeAuth,
			"stored token does not include access_token").
			WithHint("run `%s auth login` again", meta.BinName)
	}
	if !token.Expired(time.Now(), skew) {
		return token, false, nil
	}
	refreshed, err := RefreshToken(ctx, httpClient, tokenEndpoint, token)
	if err != nil {
		return TokenSet{}, false, err
	}
	if err := SaveToken(refreshed); err != nil {
		return TokenSet{}, false, err
	}
	return refreshed, true, nil
}

func SetTokenStoreForTest(store TokenStore) func() {
	return swapTokenStore(store)
}

func swapTokenStore(store TokenStore) func() {
	tokenStoreMu.Lock()
	previous := tokenStore
	tokenStore = store
	tokenStoreMu.Unlock()
	return func() {
		tokenStoreMu.Lock()
		defer tokenStoreMu.Unlock()
		tokenStore = previous
	}
}

func currentTokenStore() TokenStore {
	tokenStoreMu.Lock()
	defer tokenStoreMu.Unlock()
	if tokenStore != nil {
		return tokenStore
	}
	return resolvingTokenStore{}
}

// SetTokenStoreModeForCommand temporarily selects an explicit production store.
// The selected store persists its own mode metadata only after Save succeeds.
func SetTokenStoreModeForCommand(mode string) (func(), error) {
	path, err := authFilePath()
	if err != nil {
		return nil, err
	}
	store, err := tokenStoreForMode(mode, path)
	if err != nil {
		return nil, err
	}
	return swapTokenStore(store), nil
}

// PersistStoreMode records a successfully used explicit store without copying
// token material between stores.
func PersistStoreMode(mode string) error {
	if err := ValidateStoreMode(mode); err != nil {
		return err
	}
	if mode == StoreFile {
		return nil // A successful file load/save already carries the mode.
	}
	path, err := authFilePath()
	if err != nil {
		return err
	}
	return writeAuthEnvelope(path, authEnvelope{Version: authEnvelopeVersion, Store: StoreKeyring})
}

func ValidateStoreMode(mode string) error {
	if mode == StoreFile || mode == StoreKeyring {
		return nil
	}
	return errs.New("vertc.cli.invalid_flag", errs.TypeValidation,
		"invalid credential store %q", mode).WithParam("--store").
		WithHint("use --store=file or --store=keyring")
}

type authEnvelope struct {
	Version int       `json:"version"`
	Store   string    `json:"store"`
	Token   *TokenSet `json:"token,omitempty"`
}

type resolvingTokenStore struct{}

func (resolvingTokenStore) resolve() (TokenStore, error) {
	path, err := authFilePath()
	if err != nil {
		return nil, err
	}
	envelope, err := readAuthEnvelope(path)
	if errors.Is(err, os.ErrNotExist) {
		return FileTokenStore{Path: path}, nil
	}
	if err != nil {
		return nil, err
	}
	return tokenStoreForMode(envelope.Store, path)
}

func (s resolvingTokenStore) Load() (TokenSet, error) {
	store, err := s.resolve()
	if err != nil {
		return TokenSet{}, err
	}
	return store.Load()
}

func (s resolvingTokenStore) Save(token TokenSet) error {
	store, err := s.resolveForWrite()
	if err != nil {
		return err
	}
	return store.Save(token)
}

func (s resolvingTokenStore) Delete() error {
	store, err := s.resolveForWrite()
	if err != nil {
		return err
	}
	return store.Delete()
}

// resolveForWrite selects the store for Save/Delete. Unlike resolve (used by
// Load, which refuses to reuse a tampered or insecure file), an existing file
// that cannot be read — insecure permissions or corruption — must not lock the
// user out of `auth login`/`auth logout`. Recovery falls back to the default
// file store, which Save atomically rewrites with 0600 permissions and Delete
// removes. A well-formed keyring selection is still honored.
func (resolvingTokenStore) resolveForWrite() (TokenStore, error) {
	path, err := authFilePath()
	if err != nil {
		return nil, err
	}
	envelope, err := readAuthEnvelope(path)
	if err != nil {
		// An existing file that cannot be securely read (insecure permissions,
		// corruption) must not lock the user out of login/logout. Recover with the
		// default file store — but if the selected store was keyring, keep routing
		// there so logout still clears the OS keyring and Save does not silently
		// downgrade keyring storage to an on-disk file token.
		if selectedStoreIsKeyring(path) {
			return KeyringTokenStore{MetadataPath: path}, nil
		}
		return FileTokenStore{Path: path}, nil
	}
	return tokenStoreForMode(envelope.Store, path)
}

// selectedStoreIsKeyring best-effort reads only the store selection from an auth
// file that readAuthEnvelope refused (insecure permissions or corruption), never
// trusting its token contents, so keyring routing survives recovery. Symlinks,
// non-regular files, and oversized files are rejected.
func selectedStoreIsKeyring(path string) bool {
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return false
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	openedInfo, err := f.Stat()
	if err != nil || !openedInfo.Mode().IsRegular() || openedInfo.Size() > 1<<20 || !os.SameFile(info, openedInfo) {
		return false
	}

	var envelope authEnvelope
	if err := json.NewDecoder(io.LimitReader(f, 1<<20)).Decode(&envelope); err != nil {
		return false
	}
	return envelope.Version == authEnvelopeVersion &&
		ValidateStoreMode(envelope.Store) == nil && envelope.Store == StoreKeyring
}

func (s resolvingTokenStore) Location() string {
	store, err := s.resolve()
	if err != nil {
		return "auth-file:unavailable"
	}
	return store.Location()
}

func tokenStoreForMode(mode, path string) (TokenStore, error) {
	if err := ValidateStoreMode(mode); err != nil {
		return nil, err
	}
	if mode == StoreKeyring {
		return KeyringTokenStore{MetadataPath: path}, nil
	}
	return FileTokenStore{Path: path}, nil
}

func authFilePath() (string, error) {
	home := strings.TrimSpace(os.Getenv("VERTC_HOME"))
	if home == "" {
		userHome, err := userHomeDir()
		if err != nil {
			return "", storageErr("resolve auth file", err)
		}
		home = filepath.Join(userHome, ".vertc")
	}
	return filepath.Join(home, "auth.json"), nil
}

type KeyringTokenStore struct {
	MetadataPath string
}

func (KeyringTokenStore) Load() (TokenSet, error) {
	serialized, err := keyringGet(meta.BinName, tokenStoreAccount)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return TokenSet{}, os.ErrNotExist
		}
		return TokenSet{}, storageErr("read token from OS keyring", err)
	}
	var token TokenSet
	if err := json.Unmarshal([]byte(serialized), &token); err != nil {
		return TokenSet{}, storageErr("parse token from OS keyring", err)
	}
	return token, nil
}

func (s KeyringTokenStore) Save(token TokenSet) error {
	data, err := json.Marshal(token)
	if err != nil {
		return storageErr("serialize token for OS keyring", err)
	}
	if err := keyringSet(meta.BinName, tokenStoreAccount, string(data)); err != nil {
		return storageErr("write token to OS keyring", err)
	}
	if s.MetadataPath != "" {
		if metadataErr := writeAuthEnvelope(s.MetadataPath, authEnvelope{Version: authEnvelopeVersion, Store: StoreKeyring}); metadataErr != nil {
			if rollbackErr := keyringDelete(meta.BinName, tokenStoreAccount); rollbackErr != nil && !errors.Is(rollbackErr, keyring.ErrNotFound) {
				return storageErr("rollback token after keyring metadata failure",
					fmt.Errorf("metadata: %v; rollback: %w", metadataErr, rollbackErr))
			}
			return metadataErr
		}
	}
	return nil
}

func (s KeyringTokenStore) Delete() error {
	err := keyringDelete(meta.BinName, tokenStoreAccount)
	if err == nil || errors.Is(err, keyring.ErrNotFound) {
		if s.MetadataPath != "" {
			return writeAuthEnvelope(s.MetadataPath, authEnvelope{Version: authEnvelopeVersion, Store: StoreKeyring})
		}
		return nil
	}
	return storageErr("delete token from OS keyring", err)
}

func (KeyringTokenStore) Location() string {
	return fmt.Sprintf("os-keychain:%s/%s", meta.BinName, tokenStoreAccount)
}

type MemoryTokenStore struct {
	mu    sync.Mutex
	token TokenSet
	ok    bool
}

func NewMemoryTokenStore() *MemoryTokenStore { return &MemoryTokenStore{} }

func (s *MemoryTokenStore) Load() (TokenSet, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.ok {
		return TokenSet{}, os.ErrNotExist
	}
	return s.token, nil
}

func (s *MemoryTokenStore) Save(token TokenSet) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.token = token
	s.ok = true
	return nil
}

func (s *MemoryTokenStore) Delete() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.token = TokenSet{}
	s.ok = false
	return nil
}

func (s *MemoryTokenStore) Location() string {
	sum := sha256.Sum256([]byte(meta.BinName))
	return "memory:" + hex.EncodeToString(sum[:4])
}

func storageErr(action string, err error) *errs.Error {
	return errs.New("vertc.auth.storage_unavailable", errs.TypeAuth,
		"%s: %s", action, err).
		WithHint("check the selected credential store and its permissions, then retry")
}
