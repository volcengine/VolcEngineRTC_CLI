// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

// Package topicdocs provides the narrow, read-only Topic RTC documentation
// MCP client used by the docs command group.
package topicdocs

import (
	"fmt"
	"io"
)

const (
	Provider                 = "topic-rtc"
	OfferedProtocol          = "2025-03-26"
	LegacyProtocol           = "2024-11-05"
	MaxResponseBytes   int64 = 2 << 20
	MaxSearchLimit           = 50
	MaxListLimit             = 100
	productionEndpoint       = "https://topic.bytedance.com/mcp/rtc"
)

// MCPMeta identifies the negotiated service contract for one command.
type MCPMeta struct {
	ProtocolVersion string `json:"protocol_version"`
	ServerName      string `json:"server_name"`
	ServerVersion   string `json:"server_version"`
}

// SearchHit is one upstream-ranked documentation result.
type SearchHit struct {
	ID       string              `json:"id"`
	Score    float64             `json:"score"`
	Snippets map[string][]string `json:"snippets,omitempty"`
}

// SearchResult is the normalized search command payload.
type SearchResult struct {
	Provider string      `json:"provider"`
	Query    string      `json:"query"`
	Limit    int         `json:"limit"`
	Count    int         `json:"count"`
	Results  []SearchHit `json:"results"`
	MCP      MCPMeta     `json:"mcp"`
}

// Pretty renders search results without changing the JSON contract.
func (r SearchResult) Pretty(w io.Writer) {
	if len(r.Results) == 0 {
		fmt.Fprintln(w, "No matching RTC documents found.")
		return
	}
	for i, hit := range r.Results {
		fmt.Fprintf(w, "%d. %s  score=%g\n", i+1, hit.ID, hit.Score)
		if titles := hit.Snippets["title"]; len(titles) > 0 {
			fmt.Fprintf(w, "   %s\n", titles[0])
		} else if contents := hit.Snippets["content"]; len(contents) > 0 {
			fmt.Fprintf(w, "   %s\n", contents[0])
		}
	}
}

// FetchResult is one byte-preserved Markdown document.
type FetchResult struct {
	Provider    string  `json:"provider"`
	ID          string  `json:"id"`
	ContentType string  `json:"content_type"`
	Bytes       int     `json:"bytes"`
	SHA256      string  `json:"sha256"`
	Content     string  `json:"content"`
	MCP         MCPMeta `json:"mcp"`
}

// Pretty writes the fetched Markdown directly for terminal consumption.
func (r FetchResult) Pretty(w io.Writer) { fmt.Fprint(w, r.Content) }

// FetchExcerptResult is a local excerpt over one full fetched document. Bytes
// and SHA256 still identify the full document; Excerpt is the only Markdown
// body emitted when docs fetch uses --match.
type FetchExcerptResult struct {
	Provider     string   `json:"provider"`
	ID           string   `json:"id"`
	ContentType  string   `json:"content_type"`
	Bytes        int      `json:"bytes"`
	SHA256       string   `json:"sha256"`
	Excerpt      string   `json:"excerpt"`
	ExcerptBytes int      `json:"excerpt_bytes"`
	MatchTerms   []string `json:"match_terms"`
	MatchedTerms []string `json:"matched_terms"`
	MatchCount   int      `json:"match_count"`
	Complete     bool     `json:"complete"`
	Truncated    bool     `json:"truncated"`
	MCP          MCPMeta  `json:"mcp"`
}

// Pretty writes only the matched Markdown excerpt.
func (r FetchExcerptResult) Pretty(w io.Writer) { fmt.Fprint(w, r.Excerpt) }

// Document is one parsed list_docs index entry.
type Document struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Summary string `json:"summary,omitempty"`
}

// ListResult is a locally filtered and paginated index page.
type ListResult struct {
	Provider      string     `json:"provider"`
	Query         string     `json:"query,omitempty"`
	Offset        int        `json:"offset"`
	Limit         int        `json:"limit"`
	Count         int        `json:"count"`
	Total         int        `json:"total"`
	FilteredTotal int        `json:"filtered_total"`
	NextOffset    *int       `json:"next_offset"`
	Documents     []Document `json:"documents"`
	MCP           MCPMeta    `json:"mcp"`
}

// Pretty renders the local list page as a compact index.
func (r ListResult) Pretty(w io.Writer) {
	if len(r.Documents) == 0 {
		fmt.Fprintln(w, "No RTC documents found.")
		return
	}
	for _, doc := range r.Documents {
		fmt.Fprintf(w, "%s\t%s\n", doc.ID, doc.Title)
	}
	fmt.Fprintf(w, "\n%d result(s); filtered=%d total=%d\n", len(r.Documents), r.FilteredTotal, r.Total)
}

// RetryEvent is safe progress metadata; it deliberately excludes request and
// response payloads, headers, session ids, queries and document content.
type RetryEvent struct {
	Phase   string
	Attempt int
	Status  int
	DelayMS int64
}
