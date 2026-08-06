// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package template

import (
	"bytes"
	"embed"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/config"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/meta"
)

//go:embed all:files
var templateFS embed.FS

// RenderedFile is a single generated file (path relative to project root).
type RenderedFile struct {
	Path    string `json:"path"`
	Content string `json:"-"`
	Bytes   int    `json:"bytes"`
}

// renderData is the data model exposed to template files.
type renderData struct {
	*config.Config
	BinName    string
	SDKVersion string
}

var funcs = template.FuncMap{
	// jsstring safely encodes a Go string as a JS/JSON string literal.
	"jsstring": func(s string) (string, error) {
		b, err := json.Marshal(s)
		return string(b), err
	},
}

// Render instantiates the template for cfg into an ordered list of files.
// It does not touch the filesystem — callers use it for both --dry-run and the
// real write; dry-run previews the exact file tree.
func Render(t Template, cfg *config.Config) ([]RenderedFile, error) {
	if !t.Available {
		return nil, errs.New("vertc.template.not_found", errs.TypeNotFound,
			"template %s is reserved and not yet available", t.Key()).
			WithHint("run `%s template list` to see available combinations", meta.BinName)
	}
	root := "files/" + t.dir
	data := renderData{Config: cfg, BinName: meta.BinName, SDKVersion: t.SDK.Version}

	var files []RenderedFile
	err := fs.WalkDir(templateFS, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		raw, err := templateFS.ReadFile(p)
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(p, root+"/")
		outName := strings.TrimSuffix(rel, ".tmpl")

		tmpl, err := template.New(rel).Funcs(funcs).Parse(string(raw))
		if err != nil {
			return errs.New("vertc.template.render_failed", errs.TypeIO,
				"parse template %s: %s", rel, err)
		}
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, data); err != nil {
			return errs.New("vertc.template.render_failed", errs.TypeIO,
				"render template %s: %s", rel, err)
		}
		files = append(files, RenderedFile{
			Path:    filepath.ToSlash(outName),
			Content: buf.String(),
			Bytes:   buf.Len(),
		})
		return nil
	})
	if err != nil {
		if e, ok := errs.As(err); ok {
			return nil, e
		}
		return nil, errs.New("vertc.template.render_failed", errs.TypeIO,
			"walk template files: %s", err)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

// Write materializes rendered files under destDir, creating parent dirs.
func Write(destDir string, files []RenderedFile) error {
	for _, f := range files {
		full := filepath.Join(destDir, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return errs.New("vertc.template.render_failed", errs.TypeIO,
				"create dir for %s: %s", f.Path, err)
		}
		mode := os.FileMode(0o644)
		if err := os.WriteFile(full, []byte(f.Content), mode); err != nil {
			return errs.New("vertc.template.render_failed", errs.TypeIO,
				"write %s: %s", f.Path, err)
		}
	}
	return nil
}
