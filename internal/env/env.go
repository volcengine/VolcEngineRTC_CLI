// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

// Package env reads/merges/writes a local dotenv file for a project. `env write`
// collects credentials and base config without echoing secret values (design
// run-and-diagnose: 敏感值不明文回显).
package env

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
)

// FileName is the local env file written by `env write`.
const FileName = ".env.local"

// sensitiveKeys are never echoed back in full.
var sensitiveKeys = map[string]bool{
	"RTC_APP_KEY":    true,
	"VITE_RTC_TOKEN": true,
	"RTC_TOKEN":      true,
}

// IsSensitive reports whether a key holds a secret.
func IsSensitive(key string) bool {
	if sensitiveKeys[strings.ToUpper(key)] {
		return true
	}
	up := strings.ToUpper(key)
	return strings.Contains(up, "KEY") || strings.Contains(up, "SECRET") || strings.Contains(up, "TOKEN")
}

// Mask redacts a secret value for display.
func Mask(val string) string {
	if val == "" {
		return ""
	}
	if len(val) <= 4 {
		return "****"
	}
	return val[:2] + strings.Repeat("*", len(val)-4) + val[len(val)-2:]
}

// Path returns the env file path for a project dir.
func Path(dir string) string { return filepath.Join(dir, FileName) }

// LoadIntoProcess loads the nearest .env.local (walking up from dir) into the
// process environment, WITHOUT overriding variables already set (the real
// environment wins, standard dotenv precedence). It makes credentials written by
// `env write` / interactive dev setup (e.g. RTC_APP_ID / RTC_APP_KEY) visible to the
// CLI's own `${ENV}` resolution — the same file the web runtime (Vite) reads — so
// `token issue` / `doctor` / `dev` work without a manual `export`. Returns the
// keys it set. A missing file is not an error.
func LoadIntoProcess(dir string) ([]string, error) {
	path, ok := findUp(dir, FileName)
	if !ok {
		return nil, nil
	}
	values, err := Load(path)
	if err != nil {
		return nil, err
	}
	var set []string
	for k, v := range values {
		if _, exists := os.LookupEnv(k); exists {
			continue // real environment wins
		}
		if err := os.Setenv(k, v); err != nil {
			return set, errs.Wrap(err, "vertc.cli.internal", "set env %s: %s", k, err)
		}
		set = append(set, k)
	}
	return set, nil
}

// findUp walks up from dir looking for name, returning its path when found.
func findUp(dir, name string) (string, bool) {
	if dir == "" {
		dir = "."
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", false
	}
	for {
		p := filepath.Join(abs, name)
		if _, err := os.Stat(p); err == nil {
			return p, true
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return "", false
		}
		abs = parent
	}
}

// Load parses a dotenv file into a map (missing file → empty map).
func Load(path string) (map[string]string, error) {
	m := map[string]string{}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return m, nil
		}
		return nil, errs.Wrap(err, "vertc.cli.internal", "read env file: %s", err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		key, value, ok := parseLine(sc.Text())
		if !ok {
			continue
		}
		m[key] = value
	}
	return m, sc.Err()
}

// parseLine parses one dotenv line into (key, value). It mirrors Node dotenv /
// Vite semantics — the same file the web runtime reads — so a value the frontend
// resolves one way never resolves differently for the CLI: an optional `export `
// prefix is dropped, surrounding matched quotes are stripped, and an inline `#`
// comment on an unquoted value is removed. Blank/comment/malformed lines yield
// ok=false.
func parseLine(raw string) (key, value string, ok bool) {
	line := strings.TrimSpace(raw)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	if rest := strings.TrimPrefix(line, "export "); rest != line {
		line = strings.TrimSpace(rest)
	}
	k, v, found := strings.Cut(line, "=")
	if !found {
		return "", "", false
	}
	k = strings.TrimSpace(k)
	if k == "" {
		return "", "", false
	}
	return k, parseValue(v), true
}

// parseValue applies dotenv value semantics: a value wrapped in matching single
// or double quotes returns its inner content verbatim — spaces and `#` preserved,
// escape sequences (\n, \t, …) NOT expanded, and anything after the closing quote
// (e.g. a trailing comment) ignored; an unquoted value is trimmed and an inline
// comment (whitespace followed by `#`) is dropped.
func parseValue(raw string) string {
	v := strings.TrimSpace(raw)
	if len(v) > 0 && (v[0] == '"' || v[0] == '\'') {
		if end := strings.IndexByte(v[1:], v[0]); end >= 0 {
			return v[1 : 1+end]
		}
	}
	for i := 1; i < len(v); i++ {
		if v[i] == '#' && (v[i-1] == ' ' || v[i-1] == '\t') {
			return strings.TrimSpace(v[:i])
		}
	}
	return v
}

// Write merges updates into the env file at path, preserving existing keys.
// Returns the set of keys that were written (for a masked summary).
func Write(path string, updates map[string]string) ([]string, error) {
	existing, err := Load(path)
	if err != nil {
		return nil, err
	}
	var written []string
	for k, v := range updates {
		existing[k] = v
		written = append(written, k)
	}
	keys := make([]string, 0, len(existing))
	for k := range existing {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString("# Written by `vertc env write`. Secrets are env-only; do not commit.\n")
	for _, k := range keys {
		fmt.Fprintf(&b, "%s=%s\n", k, existing[k])
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, errs.New("vertc.env.write_failed", errs.TypeIO, "create dir: %s", err)
		}
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		return nil, errs.New("vertc.env.write_failed", errs.TypeIO, "write %q: %s", path, err)
	}
	sort.Strings(written)
	return written, nil
}
