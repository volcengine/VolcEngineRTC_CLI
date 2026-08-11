// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package template

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/config"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
)

const testScene = `{"SceneConfig":{"Name":"Default"},"VoiceChat":{"Config":{"ASRConfig":{"Provider":"volcano","ProviderParams":{}},"LLMConfig":{"Mode":"ArkV3","EndPointId":"ep-test"},"TTSConfig":{"Provider":"volcano","ProviderParams":{}}},"AgentConfig":{"UserId":"voice_agent"}}}`

func remoteFixture() map[string]string {
	return map[string]string{
		"vertc.template.yaml":          "version: 1\nscene: voice-agent\nplatform: web\nagent_config: server/scenes/default.json\nrequired_files:\n  - package.json\n  - server/scenes/default.json\n  - vertc.taskfile.yaml\n",
		"vertc.taskfile.yaml":          "version: 2\nscene: voice-agent\nplatform: web\nsdk:\n  name: '@volcengine/rtc'\n  version: '4.68.1'\nruntime:\n  topology: web-server\n  agent_control: server\n  ports:\n    web: 3000\n    server: 3001\ntasks:\n  dev:\n    - yarn dev\n",
		"server/scenes/default.json":   testScene,
		"package.json":                 `{"scripts":{"dev":"echo dev"}}`,
		"server/app.js":                `businessId: env.VERTC_BUSINESS_ID?.trim() || undefined; openApiUserAgent: env.VERTC_OPENAPI_USER_AGENT?.trim() || undefined; BusinessId: config.businessId; BusinessId: config.businessId; requestData.headers["User-Agent"] = config.openApiUserAgent; const signer = new Signer(requestData, config.service);`,
		"web/src/store/slices/room.ts": `BusinessId?: string`,
		"web/src/lib/useCommon.ts":     `await RtcClient.createEngine(); if (rtc.BusinessId) { RtcClient.setBusinessId(rtc.BusinessId); } await RtcClient.joinRoom();`,
	}
}

func TestVoiceAgentBusinessIDContractRejectsMissingOrMisorderedHandoff(t *testing.T) {
	fixture := remoteFixture()
	fixture["web/src/lib/useCommon.ts"] = `await RtcClient.createEngine(); await RtcClient.joinRoom(); RtcClient.setBusinessId(rtc.BusinessId);`
	assertErrorCode(t, validateVoiceAgentBusinessIDContract(fixture), "vertc.template.contract_invalid")
}

func TestVoiceAgentOpenAPIUserAgentContractRejectsMissingHandoff(t *testing.T) {
	fixture := remoteFixture()
	fixture["server/app.js"] = strings.ReplaceAll(fixture["server/app.js"], `requestData.headers["User-Agent"] = config.openApiUserAgent;`, "")
	assertErrorCode(t, validateVoiceAgentBusinessIDContract(fixture), "vertc.template.contract_invalid")
}

func TestRemoteDownloadURLUsesImmutableCodeloadArchive(t *testing.T) {
	got, err := remoteDownloadURL(RemoteSource{Repository: "volcengine/rtc-aigc-demo"}, "0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	want := "https://codeload.github.com/volcengine/rtc-aigc-demo/tar.gz/0123456789abcdef"
	if got != want {
		t.Fatalf("codeload URL = %q, want %q", got, want)
	}
}

func TestFetchRemoteCachesAndWorksOffline(t *testing.T) {
	archive := archiveMap(t, "repo-commit", remoteFixture())
	sum := sha256.Sum256(archive)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = w.Write(archive)
	}))
	source := RemoteSource{URL: server.URL, Commit: "commit", SHA256: fmt.Sprintf("%x", sum)}
	cache := t.TempDir()
	files, err := Resolve(context.Background(), Template{Available: true, Remote: &source}, nil, RemoteOptions{CacheDir: cache})
	if err != nil || len(files) != len(remoteFixture()) {
		t.Fatalf("FetchRemote = %d files, %v", len(files), err)
	}
	server.Close()
	if _, err := FetchRemote(context.Background(), source, RemoteOptions{CacheDir: cache}); err != nil {
		t.Fatalf("offline cache fetch: %v", err)
	}
	if requests.Load() != 1 {
		t.Fatalf("requests = %d, want 1", requests.Load())
	}
	target := filepath.Join(t.TempDir(), "project")
	if err := MaterializeRemote(target, files, false); err != nil {
		t.Fatalf("materialize: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "server/scenes/default.json")); err != nil {
		t.Fatalf("scene not materialized: %v", err)
	}
}

func TestFetchRemoteDryRunDoesNotWriteCache(t *testing.T) {
	archive := archiveMap(t, "repo-commit", remoteFixture())
	sum := sha256.Sum256(archive)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(archive) }))
	defer server.Close()
	cache := filepath.Join(t.TempDir(), "missing-cache")
	_, err := FetchRemote(context.Background(), RemoteSource{URL: server.URL, Commit: "commit", SHA256: fmt.Sprintf("%x", sum)}, RemoteOptions{CacheDir: cache, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cache); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote cache: %v", err)
	}
	target := filepath.Join(t.TempDir(), "project")
	if err := MaterializeRemote(target, []RenderedFile{{Path: "package.json"}}, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote target: %v", err)
	}
}

func TestFetchRemoteRejectsChecksumTraversalAndInvalidContract(t *testing.T) {
	valid := archiveMap(t, "repo-commit", remoteFixture())
	serverFor := func(raw []byte) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(raw) }))
	}
	server := serverFor(valid)
	_, err := FetchRemote(context.Background(), RemoteSource{URL: server.URL, Commit: "commit", SHA256: strings.Repeat("0", 64)}, RemoteOptions{CacheDir: t.TempDir()})
	server.Close()
	assertErrorCode(t, err, "vertc.template.checksum_mismatch")

	malicious := archiveMap(t, "../escape", map[string]string{"file": "bad"})
	sum := sha256.Sum256(malicious)
	server = serverFor(malicious)
	_, err = FetchRemote(context.Background(), RemoteSource{URL: server.URL, Commit: "commit", SHA256: fmt.Sprintf("%x", sum)}, RemoteOptions{CacheDir: t.TempDir()})
	server.Close()
	assertErrorCode(t, err, "vertc.template.contract_invalid")

	fixture := remoteFixture()
	delete(fixture, "vertc.taskfile.yaml")
	invalid := archiveMap(t, "repo-commit", fixture)
	sum = sha256.Sum256(invalid)
	server = serverFor(invalid)
	_, err = FetchRemote(context.Background(), RemoteSource{URL: server.URL, Commit: "commit", SHA256: fmt.Sprintf("%x", sum)}, RemoteOptions{CacheDir: t.TempDir()})
	server.Close()
	assertErrorCode(t, err, "vertc.template.contract_invalid")
}

func TestFetchRemoteReportsDownloadFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	_, err := FetchRemote(context.Background(), RemoteSource{
		URL: server.URL, Commit: "commit", SHA256: strings.Repeat("0", 64),
	}, RemoteOptions{CacheDir: t.TempDir()})
	assertErrorCode(t, err, "vertc.template.download_failed")
}

func TestMaterializeRemoteRequiresSafeEmptyTargetAndCleansUp(t *testing.T) {
	if err := MaterializeRemote("", nil, false); err == nil {
		t.Fatal("empty target should fail")
	}
	parent := t.TempDir()
	target := filepath.Join(parent, "project")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := MaterializeRemote(target, []RenderedFile{{Path: "../escape", Content: "bad"}}, false); err == nil {
		t.Fatal("unsafe output should fail")
	}
	entries, err := os.ReadDir(parent)
	if err != nil || len(entries) != 1 || entries[0].Name() != "project" {
		t.Fatalf("failed install left staging files: %v, %+v", err, entries)
	}
}

func TestMaterializeRemoteSupportsCurrentEmptyDirectory(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, "project")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(target); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	if err := MaterializeRemote(".", []RenderedFile{{Path: "package.json", Content: "{}"}}, false); err != nil {
		t.Fatalf("materialize current directory: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "package.json")); err != nil {
		t.Fatalf("package.json not materialized: %v", err)
	}
}

func archiveMap(t *testing.T, root string, files map[string]string) []byte {
	t.Helper()
	var out bytes.Buffer
	gz := gzip.NewWriter(&out)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{
		Typeflag:   tar.TypeXGlobalHeader,
		PAXRecords: map[string]string{"comment": "codeload fixture"},
	}); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		body := []byte(content)
		if err := tw.WriteHeader(&tar.Header{Name: root + "/" + name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func archiveDirectory(t *testing.T, dir string) []byte {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != dir {
				switch d.Name() {
				case ".git", "node_modules", "build", "coverage":
					return filepath.SkipDir
				}
			}
			return nil
		}
		if d.Name() == ".env.local" || d.Name() == ".eslintcache" || d.Name() == ".DS_Store" {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = string(raw)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return archiveMap(t, "rtc-aigc-demo-local", files)
}

func TestLocalDemoWorktreeRemoteContract(t *testing.T) {
	dir := os.Getenv("VERTC_DEMO_TEMPLATE_DIR")
	if dir == "" {
		t.Skip("set VERTC_DEMO_TEMPLATE_DIR for the local cross-repository integration test")
	}
	archive := archiveDirectory(t, dir)
	sum := sha256.Sum256(archive)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.Copy(w, bytes.NewReader(archive)) }))
	defer server.Close()
	files, err := FetchRemote(context.Background(), RemoteSource{URL: server.URL, Commit: "local-worktree", SHA256: fmt.Sprintf("%x", sum)}, RemoteOptions{CacheDir: t.TempDir(), DryRun: true})
	if err != nil {
		t.Fatalf("local demo contract: %v", err)
	}
	if len(files) < 10 {
		t.Fatalf("local demo archive unexpectedly small: %d files", len(files))
	}
	cfg := config.Default("local-demo", "voice-agent", "web")
	configRaw, err := config.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	files = append(files, RenderedFile{Path: "vertc.config.yaml", Content: string(configRaw), Bytes: len(configRaw)})
	target := filepath.Join(t.TempDir(), "project")
	if err := MaterializeRemote(target, files, false); err != nil {
		t.Fatalf("materialize local demo: %v", err)
	}
	loaded, err := config.Load(filepath.Join(target, "vertc.config.yaml"))
	if err != nil || loaded.Agent.ConfigFile != "server/scenes/default.json" {
		t.Fatalf("generated config = %+v, %v", loaded, err)
	}
	if _, err := LoadTaskfile(target); err != nil {
		t.Fatalf("generated taskfile: %v", err)
	}
}

func assertErrorCode(t *testing.T, err error, want string) {
	t.Helper()
	typed, ok := errs.As(err)
	if !ok || typed.Code != want {
		t.Fatalf("error = %v, want %s", err, want)
	}
}
