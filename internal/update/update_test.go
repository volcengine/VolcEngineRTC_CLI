// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package update

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func clearAutomationEnvironment(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"CI", "CONTINUOUS_INTEGRATION", "GITHUB_ACTIONS", "GITLAB_CI",
		"BUILDKITE", "JENKINS_URL", "TF_BUILD", "CIRCLECI", "TRAVIS",
		"TEAMCITY_VERSION", "CODEBUILD_BUILD_ID",
	} {
		t.Setenv(key, "")
	}
}

func TestIsNewer(t *testing.T) {
	tests := []struct {
		current, latest string
		want            bool
	}{
		{"1.0.0", "1.0.1", true},
		{"v1.2.3", "1.2.3", false},
		{"1.0.0-beta.1", "1.0.0", true},
		{"1.0.0-beta.2", "1.0.0-beta.10", true},
		{"0.1.0-dev", "1.0.0", true},
		{"git-deadbeef", "1.0.0", false},
	}
	for _, tt := range tests {
		if got := IsNewer(tt.current, tt.latest); got != tt.want {
			t.Errorf("IsNewer(%q, %q) = %v, want %v", tt.current, tt.latest, got, tt.want)
		}
	}
}

func TestRefreshAndCheckCached(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VERTC_STATE_DIR", dir)
	clearAutomationEnvironment(t)
	t.Setenv("VERTC_NO_UPDATE_NOTIFIER", "")
	oldURL, oldClient, oldNow := registryURL, httpClient, now
	registryURL = "https://registry.npmjs.org/test"
	httpClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"version":"1.2.0"}`)),
		}, nil
	})}
	now = func() time.Time { return time.Unix(1000, 0) }
	t.Cleanup(func() { registryURL, httpClient, now = oldURL, oldClient, oldNow; SetPending(nil) })
	if err := RefreshCache(context.Background(), "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if info := CheckCached("1.0.0"); info == nil || info.Latest != "1.2.0" {
		t.Fatalf("unexpected info: %+v", info)
	}
	if _, err := os.Stat(filepath.Join(dir, "update-state.json")); err != nil {
		t.Fatal(err)
	}
}

func TestNotifierGates(t *testing.T) {
	t.Setenv("VERTC_STATE_DIR", t.TempDir())
	t.Setenv("VERTC_NO_UPDATE_NOTIFIER", "1")
	if got := CheckCached("1.0.0"); got != nil {
		t.Fatalf("expected disabled notifier, got %+v", got)
	}
}

func TestNeedsRefresh(t *testing.T) {
	t.Setenv("VERTC_STATE_DIR", t.TempDir())
	clearAutomationEnvironment(t)
	t.Setenv("VERTC_NO_UPDATE_NOTIFIER", "")
	if !NeedsRefresh("1.0.0") {
		t.Fatal("missing cache should refresh")
	}
	if NeedsRefresh("1.0.0-dev") {
		t.Fatal("development build should not refresh")
	}
}

func TestCachedEvidenceDistinguishesCurrentUnknownAndSkipped(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VERTC_STATE_DIR", dir)
	clearAutomationEnvironment(t)
	t.Setenv("VERTC_NO_UPDATE_NOTIFIER", "")
	oldNow := now
	now = func() time.Time { return time.Unix(2000, 0) }
	t.Cleanup(func() { now = oldNow })
	if got := CachedEvidence("1.0.0").Status; got != CacheUnknown {
		t.Fatalf("missing cache status=%s", got)
	}
	data := `{"latest":"1.0.0","checked_at":"1970-01-01T00:33:20Z"}`
	if err := os.WriteFile(filepath.Join(dir, "update-state.json"), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := CachedEvidence("1.0.0").Status; got != CacheCurrent {
		t.Fatalf("fresh cache status=%s", got)
	}
	t.Setenv("GITHUB_ACTIONS", "true")
	if got := CachedEvidence("1.0.0").Status; got != CacheSkipped {
		t.Fatalf("CI cache status=%s", got)
	}
}

func TestFetchLatestRejectsOversizedBody(t *testing.T) {
	oldURL, oldClient := registryURL, httpClient
	registryURL = "https://registry.npmjs.org/test"
	httpClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(strings.Repeat("x", maxBodySize+1)))}, nil
	})}
	t.Cleanup(func() { registryURL, httpClient = oldURL, oldClient })
	if _, err := FetchLatest(context.Background()); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected body limit error, got %v", err)
	}
}

func TestRefreshLeaseCoalescesAndExpires(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VERTC_STATE_DIR", dir)
	oldNow := now
	now = func() time.Time { return time.Unix(5000, 0) }
	t.Cleanup(func() { now = oldNow; ReleaseRefreshLease() })
	if !TryAcquireRefreshLease() {
		t.Fatal("first process should acquire lease")
	}
	if TryAcquireRefreshLease() {
		t.Fatal("second process should be suppressed")
	}
	lease := filepath.Join(dir, "update-refresh.lease")
	stale := now().Add(-2 * leaseTTL)
	if err := os.Chtimes(lease, stale, stale); err != nil {
		t.Fatal(err)
	}
	if !TryAcquireRefreshLease() {
		t.Fatal("stale lease should be reclaimed")
	}
}
