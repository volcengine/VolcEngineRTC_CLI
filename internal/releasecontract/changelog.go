// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package releasecontract

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidateReleaseChangelog requires one complete version entry in CHANGELOG.md.
func ValidateReleaseChangelog(root, version string) error {
	path := filepath.Join(root, "CHANGELOG.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read CHANGELOG.md: %w", err)
	}
	heading := "## " + version
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	for index, line := range lines {
		if strings.TrimSpace(line) != heading {
			continue
		}
		end := len(lines)
		for next := index + 1; next < len(lines); next++ {
			if strings.HasPrefix(strings.TrimSpace(lines[next]), "## ") {
				end = next
				break
			}
		}
		entry := strings.TrimSpace(strings.Join(lines[index+1:end], "\n"))
		if entry == "" {
			return fmt.Errorf("CHANGELOG.md entry %q is empty", heading)
		}
		if strings.EqualFold(entry, "Initial public release preparation.") {
			return fmt.Errorf("CHANGELOG.md entry %q contains only placeholder content", heading)
		}
		return nil
	}
	return fmt.Errorf("CHANGELOG.md has no matching %q entry", heading)
}
