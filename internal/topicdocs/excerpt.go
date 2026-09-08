// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package topicdocs

import (
	"sort"
	"strings"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
)

// MaxExcerptBytes bounds matched Markdown injected into an Agent context.
const MaxExcerptBytes = 64 << 10

type markdownLine struct {
	start int
	end   int
	text  string
}

type markdownBlock struct {
	start int
	end   int
}

// MatchFetch extracts complete matching Markdown sections/tables locally while
// retaining the identity metadata of the full fetched document.
func MatchFetch(full FetchResult, terms []string) (FetchExcerptResult, error) {
	normalized, folded, err := normalizeMatchTerms(terms)
	if err != nil {
		return FetchExcerptResult{}, err
	}
	blocks := matchingMarkdownBlocks(full.Content, folded)
	excerpt, complete := joinMarkdownBlocks(full.Content, blocks)
	matchedTerms := make([]string, 0, len(normalized))
	foldedExcerpt := strings.ToLower(excerpt)
	for i, term := range folded {
		if strings.Contains(foldedExcerpt, term) {
			matchedTerms = append(matchedTerms, normalized[i])
		}
	}
	return FetchExcerptResult{
		Provider: full.Provider, ID: full.ID, ContentType: full.ContentType,
		Bytes: full.Bytes, SHA256: full.SHA256, MCP: full.MCP,
		Excerpt: excerpt, ExcerptBytes: len([]byte(excerpt)), MatchTerms: normalized, MatchedTerms: matchedTerms,
		MatchCount: len(blocks), Complete: complete, Truncated: !complete,
	}, nil
}

func normalizeMatchTerms(terms []string) ([]string, []string, error) {
	if len(terms) == 0 {
		return nil, nil, errs.New("vertc.docs.invalid_argument", errs.TypeValidation,
			"at least one non-empty match term is required").WithParam("--match")
	}
	seen := map[string]bool{}
	normalized := make([]string, 0, len(terms))
	folded := make([]string, 0, len(terms))
	for _, term := range terms {
		term = strings.TrimSpace(term)
		if term == "" {
			return nil, nil, errs.New("vertc.docs.invalid_argument", errs.TypeValidation,
				"match terms must not be empty").WithParam("--match")
		}
		lower := strings.ToLower(term)
		if seen[lower] {
			continue
		}
		seen[lower] = true
		normalized = append(normalized, term)
		folded = append(folded, lower)
	}
	return normalized, folded, nil
}

func matchingMarkdownBlocks(markdown string, terms []string) []markdownBlock {
	lines := splitMarkdownLines(markdown)
	blocks := make([]markdownBlock, 0)
	seen := map[markdownBlock]bool{}
	for _, term := range terms {
		matchedHeadings := make([]markdownBlock, 0)
		matchedBody := make([]markdownBlock, 0)
		for i, line := range lines {
			if !strings.Contains(strings.ToLower(line.text), term) {
				continue
			}
			if level := headingLevel(line.text); level > 0 {
				if level == 1 {
					matchedHeadings = append(matchedHeadings, markdownBlock{start: line.start, end: line.end})
				} else if block, ok := containingHeadingSection(lines, i, len(markdown)); ok {
					matchedHeadings = append(matchedHeadings, block)
				}
				continue
			}
			block, ok := containingPipeTable(lines, i)
			if !ok {
				block, ok = containingHeadingSection(lines, i, len(markdown))
			}
			if ok {
				matchedBody = append(matchedBody, block)
			}
		}
		matches := matchedBody
		if len(matchedHeadings) > 0 {
			matches = matchedHeadings
		}
		for _, block := range matches {
			if !seen[block] {
				seen[block] = true
				blocks = append(blocks, block)
			}
		}
	}
	return mergeMarkdownBlocks(blocks)
}

func splitMarkdownLines(markdown string) []markdownLine {
	if markdown == "" {
		return nil
	}
	lines := make([]markdownLine, 0, strings.Count(markdown, "\n")+1)
	start := 0
	for start < len(markdown) {
		end := strings.IndexByte(markdown[start:], '\n')
		if end < 0 {
			end = len(markdown)
		} else {
			end += start + 1
		}
		text := strings.TrimSuffix(markdown[start:end], "\n")
		text = strings.TrimSuffix(text, "\r")
		lines = append(lines, markdownLine{start: start, end: end, text: text})
		start = end
	}
	return lines
}

func containingPipeTable(lines []markdownLine, at int) (markdownBlock, bool) {
	if !looksLikePipeRow(lines[at].text) {
		return markdownBlock{}, false
	}
	start, end := at, at+1
	for start > 0 && looksLikePipeRow(lines[start-1].text) {
		start--
	}
	for end < len(lines) && looksLikePipeRow(lines[end].text) {
		end++
	}
	hasDelimiter := false
	for i := start; i < end; i++ {
		if isPipeDelimiter(lines[i].text) {
			hasDelimiter = true
			break
		}
	}
	if !hasDelimiter {
		return markdownBlock{}, false
	}
	return markdownBlock{start: lines[start].start, end: lines[end-1].end}, true
}

func looksLikePipeRow(line string) bool {
	line = strings.TrimSpace(line)
	return line != "" && strings.Contains(line, "|")
}

func isPipeDelimiter(line string) bool {
	line = strings.Trim(strings.TrimSpace(line), "|")
	if line == "" || !strings.Contains(line, "-") {
		return false
	}
	for _, cell := range strings.Split(line, "|") {
		cell = strings.Trim(strings.TrimSpace(cell), ":")
		if len(cell) < 3 || strings.Trim(cell, "-") != "" {
			return false
		}
	}
	return true
}

func containingHeadingSection(lines []markdownLine, at, documentEnd int) (markdownBlock, bool) {
	heading := -1
	level := 0
	for i := at; i >= 0; i-- {
		if candidate := headingLevel(lines[i].text); candidate > 0 {
			heading, level = i, candidate
			break
		}
	}
	if heading < 0 {
		return markdownBlock{}, false
	}
	if level == 1 {
		return markdownBlock{start: lines[at].start, end: lines[at].end}, true
	}
	end := documentEnd
	for i := heading + 1; i < len(lines); i++ {
		if candidate := headingLevel(lines[i].text); candidate > 0 && candidate <= level {
			end = lines[i].start
			break
		}
	}
	return markdownBlock{start: lines[heading].start, end: end}, true
}

func headingLevel(line string) int {
	line = strings.TrimLeft(line, " \t")
	level := 0
	for level < len(line) && level < 6 && line[level] == '#' {
		level++
	}
	if level == 0 || level >= len(line) || (line[level] != ' ' && line[level] != '\t') {
		return 0
	}
	return level
}

func mergeMarkdownBlocks(blocks []markdownBlock) []markdownBlock {
	if len(blocks) < 2 {
		return blocks
	}
	sort.Slice(blocks, func(i, j int) bool {
		if blocks[i].start == blocks[j].start {
			return blocks[i].end < blocks[j].end
		}
		return blocks[i].start < blocks[j].start
	})
	merged := blocks[:1]
	for _, block := range blocks[1:] {
		last := &merged[len(merged)-1]
		if block.start < last.end {
			if block.end > last.end {
				last.end = block.end
			}
			continue
		}
		merged = append(merged, block)
	}
	return merged
}

func joinMarkdownBlocks(markdown string, blocks []markdownBlock) (string, bool) {
	var out strings.Builder
	complete := true
	for _, block := range blocks {
		content := markdown[block.start:block.end]
		separator := ""
		if out.Len() > 0 {
			separator = "\n\n"
		}
		if out.Len()+len(separator)+len(content) > MaxExcerptBytes {
			complete = false
			continue
		}
		out.WriteString(separator)
		out.WriteString(content)
	}
	return out.String(), complete
}
