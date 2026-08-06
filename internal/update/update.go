// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

// Package update discovers newer rtc-cli releases without putting network I/O
// on normal command paths.
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/meta"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/paths"
)

const (
	RegistryURL = "https://registry.npmjs.org/@volcengine%2Frtc-cli/latest"
	cacheTTL    = 24 * time.Hour
	leaseTTL    = time.Minute
	maxBodySize = 64 << 10
)

var (
	httpClient  = &http.Client{Timeout: 15 * time.Second}
	registryURL = RegistryURL
	now         = time.Now
	pendingMu   sync.RWMutex
	pending     *Info
)

type cacheState struct {
	Latest    string    `json:"latest"`
	CheckedAt time.Time `json:"checked_at"`
}

// CacheStatus describes the strength of cached update evidence.
type CacheStatus string

const (
	CacheAvailable CacheStatus = "available"
	CacheCurrent   CacheStatus = "current"
	CacheUnknown   CacheStatus = "unknown"
	CacheSkipped   CacheStatus = "skipped"
)

// CacheEvidence is the cache-only update verdict used by notices and doctor.
type CacheEvidence struct {
	Status CacheStatus
	Info   *Info
}

// Info describes an available release.
type Info struct {
	Current string `json:"current"`
	Latest  string `json:"latest"`
}

func (i Info) Message() string {
	return fmt.Sprintf("%s %s available, current %s, run: %s update", meta.BinName, i.Latest, i.Current, meta.BinName)
}

// Notice returns the stable Agent-facing notice payload.
func (i Info) Notice() map[string]any {
	return map[string]any{
		"current": i.Current,
		"latest":  i.Latest,
		"message": i.Message(),
		"command": meta.BinName + " update",
	}
}

// SetPending stores an available update for the current process.
func SetPending(info *Info) {
	pendingMu.Lock()
	pending = info
	pendingMu.Unlock()
}

// Pending returns a copy of the current process notice.
func Pending() *Info {
	pendingMu.RLock()
	defer pendingMu.RUnlock()
	if pending == nil {
		return nil
	}
	copy := *pending
	return &copy
}

func statePath() string { return filepath.Join(paths.StateDir(), "update-state.json") }
func leasePath() string { return filepath.Join(paths.StateDir(), "update-refresh.lease") }

// AutomationDisabled applies the common lifecycle suppression policy.
func AutomationDisabled() bool {
	for _, key := range []string{"CI", "CONTINUOUS_INTEGRATION", "GITHUB_ACTIONS", "GITLAB_CI", "BUILDKITE", "JENKINS_URL", "TF_BUILD", "CIRCLECI", "TRAVIS", "TEAMCITY_VERSION", "CODEBUILD_BUILD_ID"} {
		if os.Getenv(key) != "" {
			return true
		}
	}
	return false
}

// NeedsRefresh reports whether a release build should start a cache refresh.
// It performs local I/O only.
func NeedsRefresh(current string) bool {
	if notifierDisabled(current) {
		return false
	}
	state, err := readState()
	return err != nil || now().Sub(state.CheckedAt) >= cacheTTL
}

// CheckCached performs no network I/O and returns an available update, if any.
func CheckCached(current string) *Info {
	evidence := CachedEvidence(current)
	return evidence.Info
}

// CachedEvidence performs no network I/O and never treats missing evidence as current.
func CachedEvidence(current string) CacheEvidence {
	if notifierDisabled(current) {
		return CacheEvidence{Status: CacheSkipped}
	}
	state, err := readState()
	if err != nil || now().Sub(state.CheckedAt) > cacheTTL {
		return CacheEvidence{Status: CacheUnknown}
	}
	if _, ok := parseSemver(state.Latest); !ok {
		return CacheEvidence{Status: CacheUnknown}
	}
	if IsNewer(current, state.Latest) {
		return CacheEvidence{Status: CacheAvailable, Info: &Info{Current: cleanVersion(current), Latest: cleanVersion(state.Latest)}}
	}
	return CacheEvidence{Status: CacheCurrent}
}

// RefreshCache refreshes a stale cache. Errors are deliberately returned only
// for explicit callers; root startup treats this as best effort.
func RefreshCache(ctx context.Context, current string) error {
	if !NeedsRefresh(current) {
		return nil
	}
	latest, err := FetchLatest(ctx)
	if err != nil {
		return err
	}
	data, err := json.Marshal(cacheState{Latest: latest, CheckedAt: now().UTC()})
	if err != nil {
		return err
	}
	if err := paths.WriteFileAtomic(statePath(), data); err != nil {
		return err
	}
	if IsNewer(current, latest) {
		SetPending(&Info{Current: cleanVersion(current), Latest: cleanVersion(latest)})
	}
	return nil
}

// TryAcquireRefreshLease coalesces refreshes across concurrently starting processes.
func TryAcquireRefreshLease() bool {
	path := leasePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return false
	}
	acquire := func() (*os.File, error) {
		return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	}
	f, err := acquire()
	if err != nil {
		if info, statErr := os.Stat(path); statErr != nil || now().Sub(info.ModTime()) < leaseTTL {
			return false
		}
		_ = os.Remove(path)
		f, err = acquire()
	}
	if err != nil {
		return false
	}
	_, _ = fmt.Fprintf(f, "%d\n", now().Unix())
	_ = f.Close()
	return true
}

// ReleaseRefreshLease releases a lease owned by the refresh parent/child pair.
func ReleaseRefreshLease() { _ = os.Remove(leasePath()) }

// FetchLatest queries npm for the latest published package version.
func FetchLatest(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, registryURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("npm registry returned HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBodySize+1))
	if err != nil {
		return "", err
	}
	if len(data) > maxBodySize {
		return "", fmt.Errorf("npm registry response exceeds %d bytes", maxBodySize)
	}
	var body struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &body); err != nil {
		return "", err
	}
	if _, ok := parseSemver(body.Version); !ok {
		return "", fmt.Errorf("npm registry returned invalid version %q", body.Version)
	}
	return cleanVersion(body.Version), nil
}

func readState() (cacheState, error) {
	var state cacheState
	data, err := os.ReadFile(statePath())
	if err != nil {
		return state, err
	}
	err = json.Unmarshal(data, &state)
	return state, err
}

func notifierDisabled(current string) bool {
	if os.Getenv("VERTC_NO_UPDATE_NOTIFIER") != "" || AutomationDisabled() {
		return true
	}
	parsed, ok := parseSemver(current)
	if !ok {
		return true
	}
	for _, id := range parsed.pre {
		if strings.EqualFold(id, "dev") || strings.EqualFold(id, "dirty") {
			return true
		}
	}
	return strings.Contains(current, "-g")
}

var semverPattern = regexp.MustCompile(`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z.-]+))?(?:\+[0-9A-Za-z.-]+)?$`)

type semver struct {
	major, minor, patch int
	pre                 []string
}

func cleanVersion(v string) string { return strings.TrimPrefix(strings.TrimSpace(v), "v") }

func parseSemver(v string) (semver, bool) {
	m := semverPattern.FindStringSubmatch(strings.TrimSpace(v))
	if m == nil {
		return semver{}, false
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	patch, _ := strconv.Atoi(m[3])
	s := semver{major: major, minor: minor, patch: patch}
	if m[4] != "" {
		s.pre = strings.Split(m[4], ".")
	}
	return s, true
}

// IsNewer reports whether latest is a valid semantic version greater than current.
func IsNewer(current, latest string) bool {
	a, okA := parseSemver(current)
	b, okB := parseSemver(latest)
	if !okA || !okB {
		return false
	}
	return compare(a, b) < 0
}

func compare(a, b semver) int {
	for _, pair := range [][2]int{{a.major, b.major}, {a.minor, b.minor}, {a.patch, b.patch}} {
		if pair[0] < pair[1] {
			return -1
		}
		if pair[0] > pair[1] {
			return 1
		}
	}
	if len(a.pre) == 0 && len(b.pre) != 0 {
		return 1
	}
	if len(a.pre) != 0 && len(b.pre) == 0 {
		return -1
	}
	for i := 0; i < len(a.pre) && i < len(b.pre); i++ {
		if a.pre[i] == b.pre[i] {
			continue
		}
		ai, aerr := strconv.Atoi(a.pre[i])
		bi, berr := strconv.Atoi(b.pre[i])
		switch {
		case aerr == nil && berr == nil:
			if ai < bi {
				return -1
			}
			return 1
		case aerr == nil:
			return -1
		case berr == nil:
			return 1
		case a.pre[i] < b.pre[i]:
			return -1
		default:
			return 1
		}
	}
	if len(a.pre) < len(b.pre) {
		return -1
	}
	if len(a.pre) > len(b.pre) {
		return 1
	}
	return 0
}
