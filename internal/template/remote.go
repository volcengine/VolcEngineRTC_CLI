// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package template

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/config"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
)

const maxRemoteArchiveBytes = 128 << 20

// RemoteSource identifies an immutable codeload-style archive. Commit and
// SHA256 are deliberately mandatory: callers must never use a floating branch.
type RemoteSource struct {
	Repository string // GitHub owner/repository; production downloads use codeload
	URL        string // optional test override
	Commit     string
	SHA256     string
}

type RemoteOptions struct {
	CacheDir string
	DryRun   bool
	Client   *http.Client
}

type templateManifest struct {
	Version       int      `yaml:"version"`
	Scene         string   `yaml:"scene"`
	Platform      string   `yaml:"platform"`
	AgentConfig   string   `yaml:"agent_config"`
	RequiredFiles []string `yaml:"required_files"`
}

// Resolve renders an embedded template or fetches an immutable remote source.
// A registry entry switches behavior only when Remote is non-nil.
func Resolve(ctx context.Context, t Template, cfg *config.Config, opts RemoteOptions) ([]RenderedFile, error) {
	if !t.Available {
		return nil, errs.New("vertc.template.not_found", errs.TypeNotFound,
			"template %s is reserved and not yet available", t.Key())
	}
	if t.Remote == nil {
		return Render(t, cfg)
	}
	return FetchRemote(ctx, *t.Remote, opts)
}

// FetchRemote downloads (or reuses) an immutable archive, verifies it, safely
// extracts it in memory, and validates the template contract. It never writes
// the target project; DryRun additionally forbids cache writes.
func FetchRemote(ctx context.Context, source RemoteSource, opts RemoteOptions) ([]RenderedFile, error) {
	if strings.TrimSpace(source.Commit) == "" {
		return nil, contractError("remote source requires an immutable commit")
	}
	commit := strings.TrimSpace(source.Commit)
	if commit == "." || commit == ".." || filepath.Base(commit) != commit || strings.ContainsAny(commit, `/\\`) {
		return nil, contractError("remote commit contains an unsafe path")
	}
	downloadURL, err := remoteDownloadURL(source, commit)
	if err != nil {
		return nil, err
	}
	want, err := hex.DecodeString(source.SHA256)
	if err != nil || len(want) != sha256.Size {
		return nil, contractError("remote source requires a 64-character SHA-256")
	}
	cacheDir := opts.CacheDir
	if cacheDir == "" {
		base, err := os.UserCacheDir()
		if err != nil {
			return nil, errs.Wrap(err, "vertc.template.download_failed", "resolve template cache: %s", err)
		}
		cacheDir = filepath.Join(base, "vertc", "templates")
	}
	cachePath := filepath.Join(cacheDir, commit, strings.ToLower(source.SHA256)+".tar.gz")
	archive, cacheErr := os.ReadFile(cachePath)
	if cacheErr != nil && !os.IsNotExist(cacheErr) {
		return nil, errs.Wrap(cacheErr, "vertc.template.download_failed", "read template cache: %s", cacheErr)
	}
	cacheHit := cacheErr == nil && len(archive) > 0
	if !cacheHit {
		archive, err = downloadArchive(ctx, downloadURL, opts.Client)
		if err != nil {
			return nil, err
		}
	}
	if err := verifyArchive(archive, want); err != nil {
		return nil, err
	}
	files, err := extractArchive(archive)
	if err != nil {
		return nil, err
	}
	if err := validateRemoteContract(files); err != nil {
		return nil, err
	}
	if cacheHit || opts.DryRun {
		return files, nil
	}
	if err := writeCacheAtomic(cachePath, archive); err != nil {
		return nil, errs.Wrap(err, "vertc.template.download_failed", "write template cache: %s", err)
	}
	return files, nil
}

func remoteDownloadURL(source RemoteSource, commit string) (string, error) {
	if override := strings.TrimSpace(source.URL); override != "" {
		return override, nil
	}
	repository := strings.TrimSpace(source.Repository)
	parts := strings.Split(repository, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || parts[0] == "." || parts[0] == ".." ||
		parts[1] == "." || parts[1] == ".." || strings.ContainsAny(repository, `\\?#`) {
		return "", contractError("remote source requires a GitHub owner/repository")
	}
	return "https://codeload.github.com/" + repository + "/tar.gz/" + commit, nil
}

// MaterializeRemote atomically installs validated remote files into an absent
// or empty target directory. On failure it removes its staging directory.
func MaterializeRemote(dest string, files []RenderedFile, dryRun bool) error {
	if strings.TrimSpace(dest) == "" {
		return contractError("template target is empty")
	}
	absDest, err := filepath.Abs(dest)
	if err != nil {
		return errs.Wrap(err, "vertc.template.render_failed", "resolve template target: %s", err)
	}
	dest = absDest
	for _, file := range files {
		clean := path.Clean(file.Path)
		if clean != file.Path || clean == "." || path.IsAbs(clean) || strings.HasPrefix(clean, "../") {
			return contractError("unsafe output path %q", file.Path)
		}
	}
	existed := false
	if entries, err := os.ReadDir(dest); err == nil {
		existed = true
		if len(entries) != 0 {
			return errs.New("vertc.init.target_exists", errs.TypePrecondition,
				"target directory %q is not empty", dest)
		}
	} else if !os.IsNotExist(err) {
		return errs.Wrap(err, "vertc.template.render_failed", "inspect target: %s", err)
	}
	if dryRun {
		return nil
	}
	parent := filepath.Dir(dest)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return errs.Wrap(err, "vertc.template.render_failed", "create target parent: %s", err)
	}
	stage, err := os.MkdirTemp(parent, ".vertc-template-*")
	if err != nil {
		return errs.Wrap(err, "vertc.template.render_failed", "create template staging directory: %s", err)
	}
	defer os.RemoveAll(stage)
	if err := Write(stage, files); err != nil {
		return err
	}
	if existed {
		if err := os.Remove(dest); err != nil {
			return errs.Wrap(err, "vertc.template.render_failed", "replace empty target: %s", err)
		}
	}
	if err := os.Rename(stage, dest); err != nil {
		if existed {
			_ = os.Mkdir(dest, 0o755)
		}
		return errs.Wrap(err, "vertc.template.render_failed", "install template: %s", err)
	}
	return nil
}

func downloadArchive(ctx context.Context, url string, client *http.Client) ([]byte, error) {
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, errs.Wrap(err, "vertc.template.download_failed", "create template request: %s", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, errs.Wrap(err, "vertc.template.download_failed", "download template: %s", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errs.New("vertc.template.download_failed", errs.TypeIO,
			"download template: HTTP %s", resp.Status)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxRemoteArchiveBytes+1))
	if err != nil {
		return nil, errs.Wrap(err, "vertc.template.download_failed", "read template archive: %s", err)
	}
	if len(raw) > maxRemoteArchiveBytes {
		return nil, errs.New("vertc.template.download_failed", errs.TypeIO, "template archive exceeds 128 MiB")
	}
	return raw, nil
}

func verifyArchive(raw, want []byte) error {
	got := sha256.Sum256(raw)
	if !bytes.Equal(got[:], want) {
		return errs.New("vertc.template.checksum_mismatch", errs.TypeValidation,
			"template checksum mismatch: got %x", got)
	}
	return nil
}

func extractArchive(raw []byte) ([]RenderedFile, error) {
	gz, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, contractError("open template gzip: %s", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	seen := map[string]bool{}
	var files []RenderedFile
	var archiveRoot string
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, contractError("read template tar: %s", err)
		}
		// GitHub codeload archives may start with a global PAX metadata header.
		// It describes following entries and is not part of the project tree.
		if h.Typeflag == tar.TypeXGlobalHeader {
			continue
		}
		name := strings.ReplaceAll(h.Name, "\\", "/")
		clean := path.Clean(name)
		if name == "" || path.IsAbs(name) || clean == ".." || strings.HasPrefix(clean, "../") {
			return nil, contractError("unsafe archive path %q", h.Name)
		}
		parts := strings.Split(clean, "/")
		if len(parts) < 2 {
			if h.Typeflag == tar.TypeDir {
				continue
			}
			return nil, contractError("archive entry %q has no repository root", h.Name)
		}
		if archiveRoot == "" {
			archiveRoot = parts[0]
		} else if parts[0] != archiveRoot {
			return nil, contractError("archive contains multiple roots")
		}
		rel := path.Join(parts[1:]...)
		switch h.Typeflag {
		case tar.TypeDir:
			continue
		case tar.TypeReg:
		default:
			return nil, contractError("archive entry %q is not a regular file", h.Name)
		}
		if seen[rel] {
			return nil, contractError("archive contains duplicate path %q", rel)
		}
		seen[rel] = true
		if h.Size < 0 || h.Size > maxRemoteArchiveBytes {
			return nil, contractError("archive entry %q is too large", h.Name)
		}
		content, err := io.ReadAll(io.LimitReader(tr, h.Size+1))
		if err != nil || int64(len(content)) != h.Size {
			return nil, contractError("read archive entry %q", h.Name)
		}
		files = append(files, RenderedFile{Path: rel, Content: string(content), Bytes: len(content)})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

func validateRemoteContract(files []RenderedFile) error {
	byPath := make(map[string]string, len(files))
	for _, file := range files {
		byPath[file.Path] = file.Content
	}
	raw, ok := byPath["vertc.template.yaml"]
	if !ok {
		return contractError("vertc.template.yaml is missing")
	}
	var manifest templateManifest
	if err := yaml.Unmarshal([]byte(raw), &manifest); err != nil {
		return contractError("parse vertc.template.yaml: %s", err)
	}
	if manifest.Version != 1 || manifest.Scene == "" || manifest.Platform == "" || len(manifest.RequiredFiles) == 0 {
		return contractError("manifest requires version 1, scene, and platform")
	}
	if manifest.AgentConfig != "server/scenes/default.json" {
		return contractError("manifest agent_config must be server/scenes/default.json")
	}
	for _, required := range manifest.RequiredFiles {
		clean := path.Clean(required)
		if clean != required || clean == "." || strings.HasPrefix(clean, "../") {
			return contractError("manifest contains unsafe required_file %q", required)
		}
		if _, ok := byPath[clean]; !ok {
			return contractError("manifest required file %q is missing", clean)
		}
	}
	if err := configScene([]byte(byPath[manifest.AgentConfig])); err != nil {
		return contractError("invalid agent scene: %s", err)
	}
	var taskfile Taskfile
	if err := yaml.Unmarshal([]byte(byPath[TaskfileName]), &taskfile); err != nil {
		return contractError("parse %s: %s", TaskfileName, err)
	}
	if taskfile.Version != 2 || taskfile.Scene != manifest.Scene || taskfile.Platform != manifest.Platform ||
		taskfile.SDK.Name == "" || taskfile.SDK.Version == "" || taskfile.Runtime.Topology != "web-server" ||
		taskfile.Runtime.AgentControl != "server" || len(taskfile.Tasks.Dev) == 0 {
		return contractError("%s does not satisfy the v2 web-server contract", TaskfileName)
	}
	if manifest.Scene == "voice-agent" && manifest.Platform == "web" {
		if err := validateVoiceAgentBusinessIDContract(byPath); err != nil {
			return err
		}
	}
	return nil
}

func validateVoiceAgentBusinessIDContract(files map[string]string) error {
	server := files["server/app.js"]
	if !strings.Contains(server, `businessId: env.VERTC_BUSINESS_ID?.trim() || undefined`) ||
		strings.Count(server, `BusinessId: config.businessId`) < 2 {
		return contractError("voice-agent server must propagate VERTC_BUSINESS_ID to RTC config and StartVoiceChat")
	}
	readUserAgent := strings.Index(server, `openApiUserAgent: env.VERTC_OPENAPI_USER_AGENT?.trim() || undefined`)
	setUserAgent := strings.Index(server, `requestData.headers["User-Agent"] = config.openApiUserAgent`)
	signRequest := strings.Index(server, `const signer = new Signer(requestData, config.service)`)
	if readUserAgent < 0 || setUserAgent < readUserAgent || signRequest < setUserAgent {
		return contractError("voice-agent server must propagate VERTC_OPENAPI_USER_AGENT before signing RTC OpenAPI requests")
	}
	if !strings.Contains(files["web/src/store/slices/room.ts"], `BusinessId?: string`) {
		return contractError("voice-agent web RTCConfig must expose optional BusinessId")
	}
	common := files["web/src/lib/useCommon.ts"]
	createEngine := strings.Index(common, `await RtcClient.createEngine()`)
	guard := strings.Index(common, `if (rtc.BusinessId)`)
	setBusinessID := strings.Index(common, `RtcClient.setBusinessId(rtc.BusinessId)`)
	joinRoom := strings.Index(common, `await RtcClient.joinRoom()`)
	if createEngine < 0 || guard < createEngine || setBusinessID < guard || joinRoom < setBusinessID {
		return contractError("voice-agent web must set optional BusinessId after createEngine and before joinRoom")
	}
	return nil
}

func configScene(raw []byte) error {
	if len(raw) == 0 {
		return fmt.Errorf("file is missing")
	}
	return config.ValidateServerAgentConfigJSON(raw)
}

func contractError(format string, args ...any) error {
	return errs.New("vertc.template.contract_invalid", errs.TypeValidation, format, args...)
}

func writeCacheAtomic(target string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".download-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, target)
}
