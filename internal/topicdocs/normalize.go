// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package topicdocs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
)

type searchWireHit struct {
	ID        *string                     `json:"id"`
	Score     *float64                    `json:"score"`
	Highlight *map[string]json.RawMessage `json:"highlight"`
}

var indexLine = regexp.MustCompile(`^- \[(.+?)\]\(([^()]+)\)(?:: ?(.*))?$`)
var indexHeading = regexp.MustCompile(`^#{1,6} [^#].*$`)

const indexFormatNotice = "> Format: - [directory/title](id): summary"

// Search performs one complete lifecycle and preserves upstream ranking.
func (c *Client) Search(parent context.Context, query string, limit int) (SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return SearchResult{}, errs.New("vertc.docs.invalid_argument", errs.TypeValidation,
			"search query must not be empty").WithParam("query")
	}
	if limit < 1 || limit > MaxSearchLimit {
		return SearchResult{}, errs.New("vertc.docs.invalid_argument", errs.TypeValidation,
			"search limit must be between 1 and %d", MaxSearchLimit).WithParam("--limit")
	}
	ctx, cancel := c.commandContext(parent)
	defer cancel()
	if _, err := c.prepare(ctx, "search_docs"); err != nil {
		return SearchResult{}, err
	}
	text, err := c.callTool(ctx, "search_docs", map[string]any{"query": query})
	if err != nil {
		return SearchResult{}, err
	}
	hits, err := parseSearchResults(text)
	if err != nil {
		return SearchResult{}, err
	}
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return SearchResult{Provider: Provider, Query: query, Limit: limit, Count: len(hits), Results: hits, MCP: c.meta}, nil
}

func parseSearchResults(text string) ([]SearchHit, error) {
	raw := bytes.TrimSpace([]byte(text))
	if len(raw) == 0 || raw[0] != '[' {
		return nil, searchPayloadError(nil)
	}
	var wire []searchWireHit
	if err := json.Unmarshal(raw, &wire); err != nil || wire == nil {
		return nil, searchPayloadError(err)
	}
	hits := make([]SearchHit, 0, len(wire))
	for _, item := range wire {
		if item.ID == nil || strings.TrimSpace(*item.ID) == "" || item.Score == nil || item.Highlight == nil {
			return nil, searchPayloadError(nil)
		}
		snippets := make(map[string][]string, len(*item.Highlight))
		for field, rawValues := range *item.Highlight {
			if bytes.Equal(bytes.TrimSpace(rawValues), []byte("null")) {
				return nil, searchPayloadError(nil)
			}
			var values []string
			if err := json.Unmarshal(rawValues, &values); err != nil || values == nil {
				return nil, searchPayloadError(err)
			}
			copied := append([]string(nil), values...)
			if field == "title" || field == "content" {
				for i, value := range copied {
					copied[i] = strings.ReplaceAll(strings.ReplaceAll(value, "<hl>", ""), "</hl>", "")
				}
			}
			snippets[field] = copied
		}
		hits = append(hits, SearchHit{ID: *item.ID, Score: *item.Score, Snippets: snippets})
	}
	return hits, nil
}

func searchPayloadError(cause error) error {
	typed := errs.New("vertc.docs.protocol_error", errs.TypeIO,
		"search_docs returned a malformed nested payload")
	if cause != nil {
		return typed.WithCause(cause)
	}
	return typed
}

// Fetch performs one complete lifecycle and preserves the extracted Markdown.
func (c *Client) Fetch(parent context.Context, id string) (FetchResult, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return FetchResult{}, errs.New("vertc.docs.invalid_argument", errs.TypeValidation,
			"document id must not be empty").WithParam("doc-id")
	}
	ctx, cancel := c.commandContext(parent)
	defer cancel()
	schema, err := c.prepare(ctx, "fetch_doc")
	if err != nil {
		return FetchResult{}, err
	}
	arguments := map[string]any{"id": id}
	_, paginated := schema.Properties["line_limit"]
	if paginated {
		arguments["line_limit"] = maxToolLines
	}
	text, err := c.callTool(ctx, "fetch_doc", arguments)
	if err != nil {
		return FetchResult{}, err
	}
	if paginated {
		text, err = stripCompleteDocumentMarker(text)
		if err != nil {
			return FetchResult{}, err
		}
	}
	sum := sha256.Sum256([]byte(text))
	return FetchResult{
		Provider: Provider, ID: id, ContentType: "text/markdown", Bytes: len([]byte(text)),
		SHA256: hex.EncodeToString(sum[:]), Content: text, MCP: c.meta,
	}, nil
}

// List performs one complete lifecycle, then filters and pages locally.
func (c *Client) List(parent context.Context, query string, offset, limit int) (ListResult, error) {
	if offset < 0 {
		return ListResult{}, errs.New("vertc.docs.invalid_argument", errs.TypeValidation,
			"list offset must not be negative").WithParam("--offset")
	}
	if limit < 1 || limit > MaxListLimit {
		return ListResult{}, errs.New("vertc.docs.invalid_argument", errs.TypeValidation,
			"list limit must be between 1 and %d", MaxListLimit).WithParam("--limit")
	}
	ctx, cancel := c.commandContext(parent)
	defer cancel()
	schema, err := c.prepare(ctx, "list_docs")
	if err != nil {
		return ListResult{}, err
	}
	arguments := map[string]any{}
	_, paginated := schema.Properties["line_limit"]
	if paginated {
		arguments["line_limit"] = maxToolLines
	}
	text, err := c.callTool(ctx, "list_docs", arguments)
	if err != nil {
		return ListResult{}, err
	}
	if paginated {
		text, err = stripCompleteDocumentMarker(text)
		if err != nil {
			return ListResult{}, err
		}
	}
	documents, err := parseIndex(text)
	if err != nil {
		return ListResult{}, err
	}
	filtered := documents
	query = strings.TrimSpace(query)
	if query != "" {
		needle := strings.ToLower(query)
		filtered = make([]Document, 0)
		for _, doc := range documents {
			if strings.Contains(strings.ToLower(doc.Title), needle) || strings.Contains(strings.ToLower(doc.Summary), needle) {
				filtered = append(filtered, doc)
			}
		}
	}
	start := offset
	if start > len(filtered) {
		start = len(filtered)
	}
	end := start + limit
	if end > len(filtered) {
		end = len(filtered)
	}
	page := append([]Document(nil), filtered[start:end]...)
	var next *int
	if end < len(filtered) {
		value := end
		next = &value
	}
	return ListResult{
		Provider: Provider, Query: query, Offset: offset, Limit: limit, Count: len(page),
		Total: len(documents), FilteredTotal: len(filtered), NextOffset: next, Documents: page, MCP: c.meta,
	}, nil
}

func parseIndex(text string) ([]Document, error) {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	documents := make([]Document, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// The observed list payload wraps entries in Markdown headings and one
		// exact grammar notice. Ignore only those known container lines; every
		// other non-empty line remains a fail-closed contract violation.
		if indexHeading.MatchString(line) || line == indexFormatNotice {
			continue
		}
		match := indexLine.FindStringSubmatch(line)
		if match == nil || strings.TrimSpace(match[1]) == "" || strings.TrimSpace(match[2]) == "" {
			return nil, errs.New("vertc.docs.protocol_error", errs.TypeIO,
				"list_docs returned a malformed index entry")
		}
		documents = append(documents, Document{
			Title: strings.TrimSpace(match[1]), ID: strings.TrimSpace(match[2]), Summary: strings.TrimSpace(match[3]),
		})
	}
	if len(documents) == 0 {
		return nil, errs.New("vertc.docs.protocol_error", errs.TypeIO,
			"list_docs returned an empty index")
	}
	return documents, nil
}
