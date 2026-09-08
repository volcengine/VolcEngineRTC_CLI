// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/output"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/topicdocs"
)

type fakeTopicDocsClient struct {
	searchResult topicdocs.SearchResult
	fetchResult  topicdocs.FetchResult
	listResult   topicdocs.ListResult
	err          error
	closeErr     error
	searchQuery  string
	searchLimit  int
	fetchID      string
	listQuery    string
	listOffset   int
	listLimit    int
	closed       int
}

func (f *fakeTopicDocsClient) Search(_ context.Context, query string, limit int) (topicdocs.SearchResult, error) {
	f.searchQuery, f.searchLimit = query, limit
	return f.searchResult, f.err
}

func (f *fakeTopicDocsClient) Fetch(_ context.Context, id string) (topicdocs.FetchResult, error) {
	f.fetchID = id
	return f.fetchResult, f.err
}

func (f *fakeTopicDocsClient) List(_ context.Context, query string, offset, limit int) (topicdocs.ListResult, error) {
	f.listQuery, f.listOffset, f.listLimit = query, offset, limit
	return f.listResult, f.err
}

func (f *fakeTopicDocsClient) Close(context.Context) error {
	f.closed++
	return f.closeErr
}

func runDocsCommand(t *testing.T, client topicDocsClient, format output.Format, args ...string) (string, string, error) {
	t.Helper()
	oldFactory, oldFormat, oldDryRun := newTopicDocsClient, flagFormat, flagDryRun
	oldStdout, oldStderr := os.Stdout, os.Stderr
	t.Cleanup(func() {
		newTopicDocsClient, flagFormat, flagDryRun = oldFactory, oldFormat, oldDryRun
		os.Stdout, os.Stderr = oldStdout, oldStderr
	})
	newTopicDocsClient = func() (topicDocsClient, error) { return client, nil }
	flagFormat = string(format)
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout, os.Stderr = stdoutW, stderrW
	cmd := newDocsCmd()
	cmd.SetArgs(args)
	runErr := cmd.Execute()
	_ = stdoutW.Close()
	_ = stderrW.Close()
	stdout, _ := io.ReadAll(stdoutR)
	stderr, _ := io.ReadAll(stderrR)
	_ = stdoutR.Close()
	_ = stderrR.Close()
	os.Stdout, os.Stderr = oldStdout, oldStderr
	return string(stdout), string(stderr), runErr
}

func TestDocsSearchDefaultsAndJSONEnvelope(t *testing.T) {
	fake := &fakeTopicDocsClient{searchResult: topicdocs.SearchResult{
		Provider: topicdocs.Provider, Query: "rtc", Limit: 10, Count: 1,
		Results: []topicdocs.SearchHit{{ID: "doc-1", Score: 0.9}},
	}}
	stdout, stderr, err := runDocsCommand(t, fake, output.FormatJSON, "search", "rtc")
	if err != nil {
		t.Fatal(err)
	}
	if stderr != "" || fake.searchQuery != "rtc" || fake.searchLimit != 10 || fake.closed != 1 {
		t.Fatalf("stderr=%q fake=%#v", stderr, fake)
	}
	var envelope output.Envelope
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.OK || envelope.Data == nil {
		t.Fatalf("envelope = %#v", envelope)
	}
}

func TestDocsFetchPrettyIsExactMarkdown(t *testing.T) {
	markdown := "# Exact\n\ntrailing  \n"
	fake := &fakeTopicDocsClient{fetchResult: topicdocs.FetchResult{Content: markdown}}
	stdout, stderr, err := runDocsCommand(t, fake, output.FormatPretty, "fetch", "docs/id")
	if err != nil {
		t.Fatal(err)
	}
	if stdout != markdown || stderr != "" || fake.fetchID != "docs/id" {
		t.Fatalf("stdout=%q stderr=%q id=%q", stdout, stderr, fake.fetchID)
	}
}

func TestDocsFetchMatchReturnsExcerptWithFullDocumentMetadata(t *testing.T) {
	markdown := "# Doc\n\n## Scope\n\n2025-06-01.\n\n## Provider\n\nProvider details.\n\n## Other\n\nSkip.\n"
	fake := &fakeTopicDocsClient{fetchResult: topicdocs.FetchResult{
		Provider: topicdocs.Provider, ID: "doc-1", ContentType: "text/markdown",
		Bytes: len([]byte(markdown)), SHA256: "full-sha", Content: markdown,
		MCP: topicdocs.MCPMeta{ServerName: "docs", ServerVersion: "1"},
	}}
	stdout, stderr, err := runDocsCommand(t, fake, output.FormatJSON,
		"fetch", "doc-1", "--match", "Provider", "--match", "2025-06-01")
	if err != nil {
		t.Fatal(err)
	}
	if stderr != "" || fake.fetchID != "doc-1" || fake.closed != 1 {
		t.Fatalf("stderr=%q fake=%#v", stderr, fake)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatal(err)
	}
	data, _ := envelope["data"].(map[string]any)
	if _, exists := data["content"]; exists {
		t.Fatalf("matched response leaked full content: %#v", data)
	}
	if data["id"] != "doc-1" || data["bytes"] != float64(len([]byte(markdown))) || data["sha256"] != "full-sha" || data["complete"] != true || data["truncated"] != false {
		t.Fatalf("matched metadata = %#v", data)
	}
	excerpt, _ := data["excerpt"].(string)
	if !strings.Contains(excerpt, "## Scope") || !strings.Contains(excerpt, "## Provider") || strings.Contains(excerpt, "## Other") {
		t.Fatalf("excerpt = %q", excerpt)
	}
}

func TestDocsFetchWithoutMatchKeepsJSONContract(t *testing.T) {
	markdown := "# Exact\n"
	fake := &fakeTopicDocsClient{fetchResult: topicdocs.FetchResult{
		Provider: topicdocs.Provider, ID: "doc-1", ContentType: "text/markdown",
		Bytes: len(markdown), SHA256: "full-sha", Content: markdown,
	}}
	stdout, _, err := runDocsCommand(t, fake, output.FormatJSON, "fetch", "doc-1")
	if err != nil {
		t.Fatal(err)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatal(err)
	}
	data, _ := envelope["data"].(map[string]any)
	for _, key := range []string{"provider", "id", "content_type", "bytes", "sha256", "content", "mcp"} {
		if _, ok := data[key]; !ok {
			t.Errorf("unmatched response missing %q: %#v", key, data)
		}
	}
	for _, key := range []string{"excerpt", "excerpt_bytes", "match_terms", "match_count", "complete", "truncated"} {
		if _, ok := data[key]; ok {
			t.Errorf("unmatched response added %q: %#v", key, data)
		}
	}
}

func TestDocsFetchMatchPrettyWritesOnlyExcerpt(t *testing.T) {
	markdown := "# Doc\n\n## Provider\n\nkeep\n\n## Other\n\nskip\n"
	fake := &fakeTopicDocsClient{fetchResult: topicdocs.FetchResult{Content: markdown}}
	stdout, stderr, err := runDocsCommand(t, fake, output.FormatPretty, "fetch", "doc-1", "--match", "keep")
	if err != nil {
		t.Fatal(err)
	}
	if stderr != "" || !strings.Contains(stdout, "## Provider") || strings.Contains(stdout, "## Other") {
		t.Fatalf("stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestDocsListFlagsAndTableOutput(t *testing.T) {
	next := 7
	fake := &fakeTopicDocsClient{listResult: topicdocs.ListResult{
		Documents: []topicdocs.Document{{ID: "audio/id", Title: "Audio"}},
		Total:     9, FilteredTotal: 4, NextOffset: &next,
	}}
	stdout, _, err := runDocsCommand(t, fake, output.FormatTable,
		"list", "--query", "audio", "--offset", "2", "--limit", "5")
	if err != nil {
		t.Fatal(err)
	}
	if fake.listQuery != "audio" || fake.listOffset != 2 || fake.listLimit != 5 {
		t.Fatalf("list args = %q/%d/%d", fake.listQuery, fake.listOffset, fake.listLimit)
	}
	if !strings.Contains(stdout, "audio/id\tAudio") || !strings.Contains(stdout, "filtered=4 total=9") {
		t.Fatalf("table output = %q", stdout)
	}
}

func TestDocsEmptyResultAndCleanupWarningStayOnCorrectStreams(t *testing.T) {
	fake := &fakeTopicDocsClient{
		searchResult: topicdocs.SearchResult{},
		closeErr:     errors.New("session secret must not be printed"),
	}
	stdout, stderr, err := runDocsCommand(t, fake, output.FormatPretty, "search", "none")
	if err != nil {
		t.Fatal(err)
	}
	if stdout != "No matching RTC documents found.\n" {
		t.Fatalf("stdout = %q", stdout)
	}
	if stderr != "warn: could not clean up RTC docs session\n" || strings.Contains(stderr, "secret") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestDocsArgumentAndClientErrorsRemainTyped(t *testing.T) {
	fake := &fakeTopicDocsClient{}
	_, _, err := runDocsCommand(t, fake, output.FormatJSON, "fetch")
	typed := assertDocsCommandCode(t, err, "vertc.docs.invalid_argument")
	if typed.Param != "doc-id" || typed.Details["usage"] == "" || typed.Details["example"] == "" ||
		!strings.Contains(typed.Hint, "docs fetch <doc-id>") {
		t.Fatalf("error = %#v", typed)
	}
	if fake.closed != 0 {
		t.Fatal("client must not be created for invalid positional arguments")
	}

	invalid := &fakeTopicDocsClient{err: errs.New("vertc.docs.invalid_argument", errs.TypeValidation,
		"search limit must be between 1 and 50").WithParam("--limit")}
	_, _, err = runDocsCommand(t, invalid, output.FormatJSON, "search", "rtc", "--limit", "99")
	typed = assertDocsCommandCode(t, err, "vertc.docs.invalid_argument")
	if !strings.Contains(typed.Hint, `docs search "<query>"`) || typed.Details["example"] == nil {
		t.Fatalf("decorated client error = %#v", typed)
	}

	fake.err = errs.New("vertc.docs.tool_unavailable", errs.TypePrecondition, "unavailable").
		WithDetails(map[string]any{"tool": "search_docs"})
	_, _, err = runDocsCommand(t, fake, output.FormatJSON, "search", "rtc")
	typed = assertDocsCommandCode(t, err, "vertc.docs.tool_unavailable")
	if typed.Details["tool"] != "search_docs" || fake.closed != 1 {
		t.Fatalf("error=%#v closed=%d", typed, fake.closed)
	}
}

func TestDocsSyntaxErrorsExplainArgumentPlacement(t *testing.T) {
	fake := &fakeTopicDocsClient{}
	for _, tc := range []struct {
		args  []string
		param string
		want  string
	}{
		{[]string{"search", "--query", "rtc"}, "flags", `docs search "<query>"`},
		{[]string{"fetch", "--url", "https://example.com"}, "flags", "docs fetch <doc-id>"},
		{[]string{"list", "audio"}, "arguments", `docs list [--query "<text>"]`},
	} {
		_, _, err := runDocsCommand(t, fake, output.FormatJSON, tc.args...)
		typed := assertDocsCommandCode(t, err, "vertc.docs.invalid_argument")
		if typed.Param != tc.param || !strings.Contains(typed.Hint, tc.want) || typed.Details["example"] == "" {
			t.Errorf("args=%v error=%#v", tc.args, typed)
		}
	}
	if fake.closed != 0 {
		t.Fatal("client must not be created for invalid command syntax")
	}
}

func TestDocsReadOnlyCommandsIgnoreDryRun(t *testing.T) {
	oldDryRun := flagDryRun
	flagDryRun = true
	t.Cleanup(func() { flagDryRun = oldDryRun })
	fake := &fakeTopicDocsClient{listResult: topicdocs.ListResult{}}
	_, _, err := runDocsCommand(t, fake, output.FormatJSON, "list")
	if err != nil {
		t.Fatal(err)
	}
	if fake.listOffset != 0 || fake.listLimit != 20 {
		t.Fatalf("dry-run changed read-only defaults: %#v", fake)
	}
}

func TestDocsInvalidFormatFailsBeforeClientConstruction(t *testing.T) {
	oldFactory := newTopicDocsClient
	t.Cleanup(func() { newTopicDocsClient = oldFactory })
	created := 0
	newTopicDocsClient = func() (topicDocsClient, error) {
		created++
		return &fakeTopicDocsClient{}, nil
	}
	root := NewRootCmd()
	root.SetArgs([]string{"docs", "search", "rtc", "--format", "yaml"})
	err := root.Execute()
	assertDocsCommandCode(t, err, "vertc.cli.invalid_flag")
	if created != 0 {
		t.Fatalf("docs client constructed %d time(s)", created)
	}
}

func assertDocsCommandCode(t *testing.T, err error, want string) *errs.Error {
	t.Helper()
	typed, ok := errs.As(err)
	if !ok || typed.Code != want {
		t.Fatalf("error = %#v, want %s", err, want)
	}
	return typed
}
