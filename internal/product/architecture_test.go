// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package product_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSharedCodeDoesNotBranchOnProductIdentity(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	for _, directory := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, directory), func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			rel = filepath.ToSlash(rel)
			if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || strings.HasPrefix(rel, "internal/product/") {
				return nil
			}
			raw, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			text := string(raw)
			for _, forbidden := range []string{
				"product.SelectedID", "Current().ID ==", "Current().ID !=",
				"profile.ID ==", "profile.ID !=", "switch profile.ID", "switch product.Current().ID",
			} {
				if strings.Contains(text, forbidden) {
					t.Errorf("%s branches on product identity with %q; use profile data, capabilities, or a consumer-owned registry", rel, forbidden)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestDefaultRegistrationsStayInDedicatedFiles(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	allowed := map[string]bool{
		"cmd/dev_setup_default.go":                   true,
		"internal/openapi/agent_contract_default.go": true,
		"internal/template/registry_default.go":      true,
	}
	registrations := []string{"registerDevSetupContract(", "registerVoiceChatWireContract(", "registerRegistryProvider("}
	for _, directory := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, directory), func(path string, entry os.DirEntry, err error) error {
			if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			rel = filepath.ToSlash(rel)
			raw, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			for _, registration := range registrations {
				if strings.Contains(string(raw), registration) && !strings.Contains(string(raw), "func "+registration) && !allowed[rel] {
					t.Errorf("implementation registration escaped its dedicated default file: %s", rel)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
