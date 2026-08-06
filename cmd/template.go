// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"
	"io"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/template"
)

// templateListResult is the structured output of `init --list`.
type templateListResult struct {
	Templates []templateEntry `json:"templates"`
}

type templateEntry struct {
	Scene       string `json:"scene"`
	Platform    string `json:"platform"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Available   bool   `json:"available"`
	Default     bool   `json:"default,omitempty"`
	SDK         string `json:"sdk,omitempty"`
}

func (r templateListResult) Pretty(w io.Writer) {
	fmt.Fprintln(w, "scene × platform templates:")
	for _, t := range r.Templates {
		status := "reserved"
		if t.Available {
			status = "available"
		}
		mark := ""
		if t.Default {
			mark = "  (default)"
		}
		fmt.Fprintf(w, "  [%s] %-13s × %-6s  %s%s\n", status, t.Scene, t.Platform, t.Title, mark)
	}
}

// emitTemplateList backs `init --list`: it enumerates the registry so callers
// can discover the scaffoldable scene × platform combinations.
func emitTemplateList() error {
	var entries []templateEntry
	for _, t := range template.List() {
		e := templateEntry{
			Scene: t.Scene, Platform: t.Platform, Title: t.Title,
			Description: t.Description, Available: t.Available, Default: t.Default,
		}
		if t.Available {
			e.SDK = t.SDK.Name + "@" + t.SDK.Version
		}
		entries = append(entries, e)
	}
	return out().Data(templateListResult{Templates: entries})
}
