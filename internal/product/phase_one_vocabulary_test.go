// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package product_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFirstStageContainsNoSecondProductVocabulary(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	forbidden := "byte" + "plus"
	manifest, err := os.ReadFile(filepath.Join(root, ".externalinclude"))
	scanAll := os.IsNotExist(err)
	if err != nil && !scanAll {
		t.Fatal(err)
	}
	var included []string
	for _, line := range strings.Split(string(manifest), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			included = append(included, line)
		}
	}
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		if entry.IsDir() {
			return nil
		}
		public := scanAll
		for _, rule := range included {
			if rel == rule || strings.HasSuffix(rule, "/") && strings.HasPrefix(rel, rule) {
				public = true
				break
			}
		}
		if !public {
			return nil
		}
		if strings.Contains(strings.ToLower(rel), forbidden) {
			t.Errorf("second-product path is outside planning metadata: %s", rel)
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if strings.Contains(strings.ToLower(string(raw)), forbidden) {
			t.Errorf("second-product vocabulary is outside planning metadata: %s", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
