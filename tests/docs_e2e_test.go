// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDocsCommandsAgainstScriptedMCP(t *testing.T) {
	dir := t.TempDir()
	// If docs accidentally loaded project dotenv, a directory at this path
	// would fail before the MCP request.
	if err := os.Mkdir(filepath.Join(dir, ".env.local"), 0o755); err != nil {
		t.Fatal(err)
	}

	search := run(t, dir, []string{"RTC_APP_KEY=must-not-be-forwarded", "AI_AGENT=e2e", "VE_SKILL_ID=byted-interactai-guide/0.0.4"}, // public-scan: allow; gitleaks:allow — synthetic test credential
		"docs", "search", "audio", "--limit", "1", "--format", "json")
	if search.code != 0 || search.stderr != "" {
		t.Fatalf("search exit=%d stdout=%s stderr=%s", search.code, search.stdout, search.stderr)
	}
	searchData, _ := search.envelope(t)["data"].(map[string]any)
	results, _ := searchData["results"].([]any)
	if searchData["count"] != float64(1) || len(results) != 1 {
		t.Fatalf("search data = %#v", searchData)
	}
	first, _ := results[0].(map[string]any)
	if first["id"] != "rtc/audio" || first["score"] != 0.95 || strings.Contains(search.stdout, "<hl>") {
		t.Fatalf("search result = %#v", first)
	}

	fetch := run(t, dir, []string{"AI_AGENT=e2e"}, "docs", "fetch", "rtc/audio", "--format", "pretty")
	if fetch.code != 0 || fetch.stdout != "# Fixture RTC Document\n\nExact markdown.  \n" || fetch.stderr != "" {
		t.Fatalf("fetch exit=%d stdout=%q stderr=%q", fetch.code, fetch.stdout, fetch.stderr)
	}

	list := run(t, dir, []string{"AI_AGENT=e2e"}, "docs", "list", "--query", "video", "--format", "json")
	if list.code != 0 {
		t.Fatalf("list exit=%d stdout=%s stderr=%s", list.code, list.stdout, list.stderr)
	}
	listData, _ := list.envelope(t)["data"].(map[string]any)
	if listData["total"] != float64(2) || listData["filtered_total"] != float64(1) {
		t.Fatalf("list data = %#v", listData)
	}
}

func TestDocsToolFailureIsTyped(t *testing.T) {
	r := run(t, t.TempDir(), []string{"AI_AGENT=e2e"}, "docs", "search", "force-error", "--format", "json")
	if r.code == 0 {
		t.Fatalf("expected failure: %s", r.stdout)
	}
	errData, _ := r.envelope(t)["error"].(map[string]any)
	if errData["code"] != "vertc.docs.tool_error" || errData["type"] != "precondition" {
		t.Fatalf("error = %#v", errData)
	}
}

func TestDocsArgumentErrorUsesSemanticParam(t *testing.T) {
	r := run(t, t.TempDir(), nil, "docs", "fetch", "--format", "json")
	if r.code == 0 {
		t.Fatalf("expected failure: %s", r.stdout)
	}
	errData, _ := r.envelope(t)["error"].(map[string]any)
	if errData["code"] != "vertc.docs.invalid_argument" || errData["param"] != "doc-id" {
		t.Fatalf("error = %#v", errData)
	}
}
