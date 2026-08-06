// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package errs

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestCatalogValid asserts every catalog entry is well-formed and unique.
func TestCatalogValid(t *testing.T) {
	if problems := ValidateCatalog(); len(problems) > 0 {
		t.Fatalf("catalog invalid:\n%s", strings.Join(problems, "\n"))
	}
}

func TestDeprecatedInteractiveRequiredCodeRemainsRegistered(t *testing.T) {
	if !IsRegistered("vertc.dev.interactive_required") {
		t.Fatal("stable error code vertc.dev.interactive_required must remain registered for compatibility")
	}
}

// codeLiteral matches a vertc.<domain>.<subtype> string literal at an error
// emission site: errs.New(...), errs.Wrap(err, ...), or a `.Code = "..."`
// assignment. Anchoring on these sites avoids false positives from filename
// constants like "vertc.config.yaml".
var codeLiteral = regexp.MustCompile(
	`(?:New\(\s*|Wrap\([^,]+,\s*|\.Code\s*=\s*)"(vertc\.[a-z][a-z0-9_]*\.[a-z][a-z0-9_]*)"`)

// TestAllEmittedCodesRegistered walks the module source and asserts every
// vertc.* error-code literal is registered in the catalog. Prevents emitting an unregistered
// code from any command.
func TestAllEmittedCodesRegistered(t *testing.T) {
	root := moduleRoot(t)
	unregistered := map[string][]string{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			base := d.Name()
			if base == ".git" || base == "node_modules" || base == "files" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range codeLiteral.FindAllStringSubmatch(string(src), -1) {
			code := m[1]
			if !IsRegistered(code) {
				rel, _ := filepath.Rel(root, path)
				unregistered[code] = append(unregistered[code], rel)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk source: %v", err)
	}
	if len(unregistered) > 0 {
		var b strings.Builder
		for code, files := range unregistered {
			b.WriteString(code + " (in " + strings.Join(files, ", ") + ")\n")
		}
		t.Fatalf("unregistered error codes emitted:\n%sadd them to errs.Catalog", b.String())
	}
}

// TestCatalogSnapshot compares the catalog codes to a golden snapshot. Run with
// UPDATE_SNAPSHOT=1 to regenerate after an intentional change.
func TestCatalogSnapshot(t *testing.T) {
	codes := make([]string, 0, len(Catalog))
	for _, e := range Catalog {
		codes = append(codes, e.Code)
	}
	sort.Strings(codes)
	got := strings.Join(codes, "\n") + "\n"

	snap := filepath.Join("testdata", "error-codes.snapshot")
	if os.Getenv("UPDATE_SNAPSHOT") == "1" {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(snap, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Log("snapshot updated")
		return
	}
	want, err := os.ReadFile(snap)
	if err != nil {
		t.Fatalf("read snapshot (run with UPDATE_SNAPSHOT=1 to create): %v", err)
	}
	want = bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n"))
	if string(want) != got {
		t.Fatalf("catalog drifted from snapshot; run `UPDATE_SNAPSHOT=1 go test ./internal/errs/`\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}

// moduleRoot returns the vertc module root (two levels up from internal/errs).
func moduleRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(filepath.Join(wd, "..", ".."))
}
