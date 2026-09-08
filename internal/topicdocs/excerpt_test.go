// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package topicdocs

import (
	"strings"
	"testing"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
)

func TestMatchFetchExtractsSectionsAndTablesOnce(t *testing.T) {
	markdown := "# Document\n\nIntro.\n\n" +
		"## Scope\n\nAI audio/video interaction.\n\n" +
		"## Provider\n\nProvider details.\n\n### Params\n\nProvider parameters.\n\n" +
		"## Matrix\n\n| Field | Value |\n| --- | --- |\n| API version | 2025-06-01 |\n\n" +
		"## Other\n\nMust stay out.\n"
	full := FetchResult{
		Provider: Provider, ID: "doc-1", ContentType: "text/markdown",
		Bytes: len([]byte(markdown)), SHA256: "full-sha", Content: markdown,
		MCP: MCPMeta{ServerName: "docs"},
	}
	got, err := MatchFetch(full, []string{"Provider", "2025-06-01", " provider "})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != full.ID || got.Bytes != full.Bytes || got.SHA256 != full.SHA256 || got.MCP != full.MCP {
		t.Fatalf("full metadata changed: %#v", got)
	}
	if len(got.MatchTerms) != 2 || got.MatchTerms[0] != "Provider" || got.MatchTerms[1] != "2025-06-01" {
		t.Fatalf("match terms = %#v", got.MatchTerms)
	}
	if len(got.MatchedTerms) != 2 || got.MatchedTerms[0] != "Provider" || got.MatchedTerms[1] != "2025-06-01" {
		t.Fatalf("matched terms = %#v", got.MatchedTerms)
	}
	if got.MatchCount != 2 || !got.Complete || got.Truncated || got.ExcerptBytes != len([]byte(got.Excerpt)) {
		t.Fatalf("excerpt metadata = %#v", got)
	}
	for _, want := range []string{"## Provider", "### Params", "| Field | Value |", "| API version | 2025-06-01 |"} {
		if !strings.Contains(got.Excerpt, want) {
			t.Errorf("excerpt missing %q: %q", want, got.Excerpt)
		}
	}
	if strings.Contains(got.Excerpt, "## Scope") || strings.Contains(got.Excerpt, "## Other") {
		t.Fatalf("excerpt contains unrelated section: %q", got.Excerpt)
	}
}

func TestMatchFetchKeepsRootLevelMatchWithoutOversizedDocument(t *testing.T) {
	markdown := "# Large\n\nneedle\n" + strings.Repeat("界", MaxExcerptBytes)
	got, err := MatchFetch(FetchResult{Content: markdown, Bytes: len([]byte(markdown))}, []string{"needle"})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Complete || got.Truncated || got.MatchCount != 1 || got.Excerpt != "needle\n" || strings.Contains(got.Excerpt, "界") {
		t.Fatalf("root-level excerpt = %#v", got)
	}
}

func TestMatchFetchKeepsCompleteBlocksBeforeOversizedBlock(t *testing.T) {
	markdown := "## Small\n\nneedle keep\n\n## Large\n\nneedle omit\n" + strings.Repeat("x", MaxExcerptBytes)
	got, err := MatchFetch(FetchResult{Content: markdown, Bytes: len(markdown)}, []string{"needle"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Complete || !got.Truncated || got.MatchCount != 2 {
		t.Fatalf("excerpt metadata = %#v", got)
	}
	if !strings.Contains(got.Excerpt, "## Small") || strings.Contains(got.Excerpt, "## Large") {
		t.Fatalf("excerpt must contain only complete fitting blocks: %q", got.Excerpt)
	}
}

func TestMatchFetchPrefersSpecificMatchOverBroadParent(t *testing.T) {
	markdown := "# AibotUpdate\n\nAibotUpdate intro.\n\n## ServiceTier\n\nServiceTier details.\n\n## Other\n\n" +
		strings.Repeat("x", MaxExcerptBytes)
	got, err := MatchFetch(FetchResult{Content: markdown, Bytes: len(markdown)}, []string{"AibotUpdate", "ServiceTier"})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Complete || got.Truncated || !strings.Contains(got.Excerpt, "## ServiceTier") || strings.Contains(got.Excerpt, "## Other") {
		t.Fatalf("specific excerpt = %#v", got)
	}
}

func TestMatchFetchNoMatchesIsCompleteAndEmpty(t *testing.T) {
	got, err := MatchFetch(FetchResult{Content: "# Document\n\nBody.\n"}, []string{"absent"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Excerpt != "" || got.MatchCount != 0 || !got.Complete || got.Truncated {
		t.Fatalf("no-match excerpt = %#v", got)
	}
	if len(got.MatchedTerms) != 0 {
		t.Fatalf("no-match terms = %#v", got.MatchedTerms)
	}
}

func TestMatchFetchRejectsEmptyTerm(t *testing.T) {
	_, err := MatchFetch(FetchResult{Content: "# Document\n"}, []string{" "})
	typed, ok := errs.As(err)
	if !ok || typed.Code != "vertc.docs.invalid_argument" || typed.Param != "--match" {
		t.Fatalf("error = %#v", err)
	}
}
