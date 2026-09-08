// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package topicdocs

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
)

type observedRequest struct {
	method  string
	ua      string
	session string
	headers http.Header
}

type mcpFixture struct {
	protocol     string
	issuedID     string
	tools        []toolDescription
	toolText     map[string]string
	toolContent  map[string][]contentBlock
	toolIsError  map[string]bool
	contentType  string
	mu           sync.Mutex
	requests     []observedRequest
	cleanupCount int
	cleanupUA    string
	cleanupHead  http.Header
}

func standardTools() []toolDescription {
	noExtra := false
	return []toolDescription{
		{Name: "search_docs", InputSchema: toolSchema{Type: "object", Properties: map[string]schemaProperty{"query": {Type: "string"}}, AdditionalProperties: &noExtra}},
		{Name: "fetch_doc", InputSchema: toolSchema{Type: "object", Properties: map[string]schemaProperty{"id": {Type: "string"}}, AdditionalProperties: &noExtra}},
		{Name: "list_docs", InputSchema: toolSchema{Type: "object", Properties: map[string]schemaProperty{}, AdditionalProperties: &noExtra}},
	}
}

func (f *mcpFixture) serveHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Method == http.MethodDelete {
		f.cleanupCount++
		f.cleanupUA = r.UserAgent()
		f.cleanupHead = r.Header.Clone()
		if r.Header.Get(sessionIDHeader) != f.issuedID {
			http.Error(w, "missing session", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	var request struct {
		ID     int64  `json:"id"`
		Method string `json:"method"`
		Params struct {
			Name string `json:"name"`
		} `json:"params"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.requests = append(f.requests, observedRequest{
		method: request.Method, ua: r.UserAgent(), session: r.Header.Get(sessionIDHeader), headers: r.Header.Clone(),
	})
	if f.issuedID != "" && request.Method == "initialize" {
		w.Header().Set(sessionIDHeader, f.issuedID)
	}
	if request.Method == "notifications/initialized" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	var result any
	switch request.Method {
	case "initialize":
		protocol := f.protocol
		if protocol == "" {
			protocol = OfferedProtocol
		}
		result = map[string]any{
			"protocolVersion": protocol,
			"serverInfo":      map[string]string{"name": "fixture-mcp", "version": "1.0.0"},
		}
	case "tools/list":
		tools := f.tools
		if tools == nil {
			tools = standardTools()
		}
		result = toolsListResult{Tools: tools}
	case "tools/call":
		content := f.toolContent[request.Params.Name]
		if content == nil {
			content = []contentBlock{{Type: "text", Text: f.toolText[request.Params.Name]}}
		}
		result = toolCallResult{Content: content, IsError: f.toolIsError[request.Params.Name]}
	default:
		http.Error(w, "unexpected method", http.StatusBadRequest)
		return
	}
	payload, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
	if f.contentType == contentTypeEvents {
		w.Header().Set("Content-Type", contentTypeEvents)
		_, _ = fmt.Fprintf(w, "event: message\ndata: %s\n\n", payload)
		return
	}
	w.Header().Set("Content-Type", contentTypeJSON)
	_, _ = w.Write(payload)
}

func fixtureClient(t *testing.T, fixture *mcpFixture, options ...Option) (*Client, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(fixture.serveHTTP))
	t.Cleanup(server.Close)
	client, err := newClient(server.URL, "9.8.7", nil, options...)
	if err != nil {
		t.Fatal(err)
	}
	client.sleep = func(context.Context, time.Duration) error { return nil }
	return client, server
}

func assertCode(t *testing.T, err error, want string) *errs.Error {
	t.Helper()
	typed, ok := errs.As(err)
	if !ok || typed.Code != want {
		t.Fatalf("error = %#v, want %s", err, want)
	}
	return typed
}

func assertHeadersAllowlisted(t *testing.T, headers http.Header) {
	t.Helper()
	allowed := map[string]bool{
		"Accept": true, "Accept-Encoding": true, "Content-Length": true,
		"Content-Type": true, "User-Agent": true, http.CanonicalHeaderKey(sessionIDHeader): true,
	}
	for name := range headers {
		if !allowed[name] {
			t.Fatalf("unexpected outbound header %s", name)
		}
	}
}

func TestSearchLifecycleAndNormalization(t *testing.T) {
	fixture := &mcpFixture{
		protocol: LegacyProtocol,
		issuedID: "fixture-id",
		toolText: map[string]string{"search_docs": `[{"id":"doc-b","score":0.8,"highlight":{"title":["<hl>RTC</hl> B"]}},{"id":"doc-a","score":0.7,"highlight":{"content":["A <hl>match</hl>"]}}]`},
	}
	client, _ := fixtureClient(t, fixture)
	result, err := client.Search(context.Background(), "  rtc  ", 1)
	if err != nil {
		t.Fatal(err)
	}
	if result.Query != "rtc" || result.Count != 1 || len(result.Results) != 1 || result.Results[0].ID != "doc-b" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if got := result.Results[0].Snippets["title"][0]; got != "RTC B" {
		t.Fatalf("highlight = %q", got)
	}
	if result.MCP.ProtocolVersion != LegacyProtocol || result.MCP.ServerName != "fixture-mcp" {
		t.Fatalf("mcp metadata = %#v", result.MCP)
	}
	fixture.mu.Lock()
	requests := append([]observedRequest(nil), fixture.requests...)
	fixture.mu.Unlock()
	wantMethods := []string{"initialize", "notifications/initialized", "tools/list", "tools/call"}
	if len(requests) != len(wantMethods) {
		t.Fatalf("requests = %d", len(requests))
	}
	for i, request := range requests {
		if request.method != wantMethods[i] || request.ua != "vertc/9.8.7" {
			t.Fatalf("request %d = %#v", i, request)
		}
		if i > 0 && request.session != fixture.issuedID {
			t.Fatalf("session not propagated at request %d", i)
		}
		assertHeadersAllowlisted(t, request.headers)
	}
	if err := client.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if fixture.cleanupCount != 1 {
		t.Fatalf("cleanup count = %d", fixture.cleanupCount)
	}
	if fixture.cleanupUA != "vertc/9.8.7" {
		t.Fatalf("cleanup user agent = %q", fixture.cleanupUA)
	}
	assertHeadersAllowlisted(t, fixture.cleanupHead)
}

func TestFetchPreservesBytesAndMapsNotFound(t *testing.T) {
	markdown := "# 标题\n\nline  \n"
	client, _ := fixtureClient(t, &mcpFixture{toolText: map[string]string{"fetch_doc": markdown}})
	result, err := client.Fetch(context.Background(), "doc/1")
	if err != nil {
		t.Fatal(err)
	}
	if result.Content != markdown || result.Bytes != len([]byte(markdown)) || result.SHA256 != "dc8cb7dbdc7232696ed27e10d4ee177f7bf062048f8b9627a1c1fa23c0591821" {
		t.Fatalf("fetch result = %#v", result)
	}

	notFound, _ := fixtureClient(t, &mcpFixture{
		toolContent: map[string][]contentBlock{"fetch_doc": {{Type: "text", Text: "Document not found: missing"}}},
		toolIsError: map[string]bool{"fetch_doc": true},
	})
	_, err = notFound.Fetch(context.Background(), "missing")
	assertCode(t, err, "vertc.docs.document_not_found")
}

func TestListParsesFiltersAndPages(t *testing.T) {
	index := "# 实时音视频\n\n> Format: - [directory/title](id): summary\n\n## 实时音视频\n\n" +
		"- [Audio Basics](audio/basic): Start RTC audio\n- [Video Guide](video/guide): Publish video\n- [Audio Advanced](audio/advanced): Tune quality\n"
	client, _ := fixtureClient(t, &mcpFixture{toolText: map[string]string{"list_docs": index}})
	result, err := client.List(context.Background(), "AUDIO", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 3 || result.FilteredTotal != 2 || result.Count != 1 || result.Documents[0].ID != "audio/advanced" || result.NextOffset != nil {
		t.Fatalf("list result = %#v", result)
	}
}

func TestLocalValidationMakesNoRequest(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer server.Close()
	client, err := newClient(server.URL, "1.0.0", nil)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		run  func() error
	}{
		{"empty search", func() error { _, e := client.Search(context.Background(), " ", 10); return e }},
		{"search limit", func() error { _, e := client.Search(context.Background(), "x", MaxSearchLimit+1); return e }},
		{"empty id", func() error { _, e := client.Fetch(context.Background(), " "); return e }},
		{"negative offset", func() error { _, e := client.List(context.Background(), "", -1, 20); return e }},
		{"list limit", func() error { _, e := client.List(context.Background(), "", 0, 0); return e }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { assertCode(t, tc.run(), "vertc.docs.invalid_argument") })
	}
	if requests != 0 {
		t.Fatalf("network requests = %d", requests)
	}
}

func TestSchemaAndContentFailClosed(t *testing.T) {
	noExtra := false
	drift := standardTools()
	drift[0].InputSchema = toolSchema{Type: "object", Properties: map[string]schemaProperty{"q": {Type: "string"}}, AdditionalProperties: &noExtra}
	client, _ := fixtureClient(t, &mcpFixture{tools: drift})
	_, err := client.Search(context.Background(), "rtc", 10)
	assertCode(t, err, "vertc.docs.tool_schema_changed")

	cases := []struct {
		name    string
		content []contentBlock
		isError bool
		code    string
	}{
		{"empty", []contentBlock{}, false, "vertc.docs.protocol_error"},
		{"multiple", []contentBlock{{Type: "text", Text: "a"}, {Type: "text", Text: "b"}}, false, "vertc.docs.protocol_error"},
		{"non text", []contentBlock{{Type: "image", Text: "x"}}, false, "vertc.docs.protocol_error"},
		{"tool error", []contentBlock{{Type: "text", Text: "failed"}}, true, "vertc.docs.tool_error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := fixtureClient(t, &mcpFixture{
				toolContent: map[string][]contentBlock{"fetch_doc": tc.content},
				toolIsError: map[string]bool{"fetch_doc": tc.isError},
			})
			_, e := c.Fetch(context.Background(), "id")
			assertCode(t, e, tc.code)
		})
	}
}

func TestToolRequiredSchemaCompatibility(t *testing.T) {
	valid := standardTools()
	valid[0].InputSchema.Required = []string{"query"}
	valid[1].InputSchema.Required = []string{"id"}
	for _, name := range []string{"search_docs", "fetch_doc", "list_docs"} {
		if err := validateTool(valid, name); err != nil {
			t.Fatalf("valid required schema for %s: %v", name, err)
		}
	}

	cases := []struct {
		name     string
		tool     string
		required []string
	}{
		{"unknown", "search_docs", []string{"unknown"}},
		{"duplicate", "search_docs", []string{"query", "query"}},
		{"property mismatch", "fetch_doc", []string{"query"}},
		{"list requires input", "list_docs", []string{"query"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tools := standardTools()
			for i := range tools {
				if tools[i].Name == tc.tool {
					tools[i].InputSchema.Required = tc.required
				}
			}
			assertCode(t, validateTool(tools, tc.tool), "vertc.docs.tool_schema_changed")
		})
	}
}

func TestPaginatedToolSchemaCompatibility(t *testing.T) {
	noExtra := false
	tools := standardTools()
	tools[1].InputSchema = toolSchema{
		Type: "object", AdditionalProperties: &noExtra, Required: []string{"id"},
		Properties: map[string]schemaProperty{
			"id": {Type: "string"}, "line_offset": {Type: "integer"}, "line_limit": {Type: "integer"},
		},
	}
	if err := validateTool(tools, "fetch_doc"); err != nil {
		t.Fatalf("paginated fetch schema: %v", err)
	}
	tools[2].InputSchema = toolSchema{
		Type: "object", AdditionalProperties: &noExtra,
		Properties: map[string]schemaProperty{
			"line_offset": {Type: "integer"}, "line_limit": {Type: "integer"}, "grep": {Type: "string"},
		},
	}
	if err := validateTool(tools, "list_docs"); err != nil {
		t.Fatalf("paginated list schema: %v", err)
	}

	fixture := &mcpFixture{
		tools: tools, toolText: map[string]string{
			"fetch_doc": "# RTC\n\n<<<PAGE_INFO total_lines=1 has_more=false next_line_offset=none>>>\n<<<END_OF_DOCUMENT>>>",
		},
	}
	client, _ := fixtureClient(t, fixture)
	result, err := client.Fetch(context.Background(), "doc")
	if err != nil {
		t.Fatal(err)
	}
	if result.Content != "# RTC\n" {
		t.Fatalf("content = %q", result.Content)
	}

	listFixture := &mcpFixture{
		tools: tools, toolText: map[string]string{
			"list_docs": "# RTC\n\n> Format: - [directory/title](id): summary\n\n- [Audio](audio/id): summary\n\n<<<PAGE_INFO total_lines=5 has_more=false next_line_offset=none>>>\n<<<END_OF_DOCUMENT>>>",
		},
	}
	client, _ = fixtureClient(t, listFixture)
	listed, err := client.List(context.Background(), "", 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if listed.Count != 1 || listed.Documents[0].ID != "audio/id" {
		t.Fatalf("list result = %#v", listed)
	}
}

func TestRequiredSchemaDriftStopsBeforeToolCall(t *testing.T) {
	tools := standardTools()
	tools[0].InputSchema.Required = []string{"query", "query"}
	fixture := &mcpFixture{tools: tools}
	client, _ := fixtureClient(t, fixture)
	_, err := client.Search(context.Background(), "rtc", 10)
	assertCode(t, err, "vertc.docs.tool_schema_changed")
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	for _, request := range fixture.requests {
		if request.method == "tools/call" {
			t.Fatal("schema drift still reached tools/call")
		}
	}
}

func TestMalformedNestedPayloads(t *testing.T) {
	search, _ := fixtureClient(t, &mcpFixture{toolText: map[string]string{"search_docs": `{not-json`}})
	_, err := search.Search(context.Background(), "rtc", 10)
	assertCode(t, err, "vertc.docs.protocol_error")

	list, _ := fixtureClient(t, &mcpFixture{toolText: map[string]string{"list_docs": `not an index entry`}})
	_, err = list.List(context.Background(), "", 0, 20)
	assertCode(t, err, "vertc.docs.protocol_error")
}

func TestSearchNestedPayloadContract(t *testing.T) {
	empty, err := parseSearchResults(`[]`)
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty=%#v err=%v", empty, err)
	}

	invalid := []string{
		`null`,
		`{}`,
		`[null]`,
		`[{"score":1,"highlight":{}}]`,
		`[{"id":"doc","highlight":{}}]`,
		`[{"id":"doc","score":1}]`,
		`[{"id":"doc","score":1,"highlight":null}]`,
		`[{"id":"doc","score":1,"highlight":{"title":null}}]`,
	}
	for _, payload := range invalid {
		if _, err := parseSearchResults(payload); err == nil {
			t.Errorf("payload unexpectedly accepted: %s", payload)
		} else {
			assertCode(t, err, "vertc.docs.protocol_error")
		}
	}
}

func TestSearchCleansOnlyTitleAndContentHighlights(t *testing.T) {
	hits, err := parseSearchResults(`[{"id":"doc","score":1,"highlight":{"title":["<hl>Title</hl>"],"content":["<hl>Body</hl>"],"metadata":["literal <hl>value</hl>"]}}]`)
	if err != nil {
		t.Fatal(err)
	}
	if hits[0].Snippets["title"][0] != "Title" || hits[0].Snippets["content"][0] != "Body" {
		t.Fatalf("known snippets = %#v", hits[0].Snippets)
	}
	if hits[0].Snippets["metadata"][0] != "literal <hl>value</hl>" {
		t.Fatalf("metadata was rewritten: %#v", hits[0].Snippets["metadata"])
	}
}

func TestListSummaryMarkdownLinkDoesNotReplacePrimaryLink(t *testing.T) {
	documents, err := parseIndex(`- [Title](doc-id): See [guide](guide-id)`)
	if err != nil {
		t.Fatal(err)
	}
	if len(documents) != 1 || documents[0].Title != "Title" || documents[0].ID != "doc-id" || documents[0].Summary != "See [guide](guide-id)" {
		t.Fatalf("documents = %#v", documents)
	}
}

func TestSSEAndEnvelopeValidation(t *testing.T) {
	client, _ := fixtureClient(t, &mcpFixture{
		contentType: contentTypeEvents,
		toolText:    map[string]string{"fetch_doc": "# SSE"},
	})
	result, err := client.Fetch(context.Background(), "sse")
	if err != nil || result.Content != "# SSE" {
		t.Fatalf("result=%#v err=%v", result, err)
	}

	bad := []struct {
		name string
		body string
	}{
		{"wrong id", `{"jsonrpc":"2.0","id":2,"result":{}}`},
		{"wrong version", `{"jsonrpc":"1.0","id":1,"result":{}}`},
		{"both", `{"jsonrpc":"2.0","id":1,"result":{},"error":{"code":-1,"message":"x"}}`},
		{"neither", `{"jsonrpc":"2.0","id":1}`},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			_, e := decodeRPC([]byte(tc.body), contentTypeJSON, 1)
			assertCode(t, e, "vertc.docs.protocol_error")
		})
	}
}

func TestSSEMatchingEventReturnsBeforeStreamEOF(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", contentTypeEvents)
		_, _ = fmt.Fprintf(w, "event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":%d,\"result\":{}}\n\n", request.ID)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	client, err := newClient(server.URL, "1.0.0", nil)
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	var result map[string]any
	if err := client.rpc(context.Background(), "initialize", map[string]any{}, time.Second, &result); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("SSE response waited for EOF: %s", elapsed)
	}
}

func TestSuccessfulStatusBodyReadFailureRetries(t *testing.T) {
	var attempts atomic.Int32
	valid := `{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-03-26"}}`
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		attempt := attempts.Add(1)
		body := io.ReadCloser(&errorBody{data: []byte(valid), err: io.EOF})
		if attempt == 1 {
			body = &errorBody{data: []byte(`{"jsonrpc":"2.0"`), err: io.ErrUnexpectedEOF}
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{contentTypeJSON}},
			Body:       body,
		}, nil
	})
	client, err := newClient("https://fixture.invalid/mcp", "1.0.0", &http.Client{Transport: transport})
	if err != nil {
		t.Fatal(err)
	}
	client.sleep = func(context.Context, time.Duration) error { return nil }
	var result initializeResult
	if err := client.rpc(context.Background(), "initialize", map[string]any{}, time.Second, &result); err != nil {
		t.Fatal(err)
	}
	if attempts.Load() != 2 || result.ProtocolVersion != OfferedProtocol {
		t.Fatalf("attempts=%d result=%#v", attempts.Load(), result)
	}
}

func TestSuccessfulStatusBodyDeadlineMapsTimeout(t *testing.T) {
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{contentTypeJSON}},
			Body:       &contextBody{ctx: request.Context()},
		}, nil
	})
	client, err := newClient("https://fixture.invalid/mcp", "1.0.0", &http.Client{Transport: transport})
	if err != nil {
		t.Fatal(err)
	}
	var result initializeResult
	err = client.rpc(context.Background(), "initialize", map[string]any{}, 20*time.Millisecond, &result)
	assertCode(t, err, "vertc.docs.timeout")
}

func TestRetryAfterDateAndLargeDeltaAreBounded(t *testing.T) {
	now := time.Date(2026, time.August, 11, 8, 0, 0, 0, time.UTC)
	date := now.Add(3 * time.Second).Format(http.TimeFormat)
	if got := retryDelay(1, date, now); got != 3*time.Second {
		t.Fatalf("HTTP-date delay = %s", got)
	}
	if got := retryDelay(1, "999999999999999999", now); got != maxRetryAfter {
		t.Fatalf("large delta delay = %s", got)
	}
	if got := retryDelay(1, "999999999999999999999999999999", now); got != maxRetryAfter {
		t.Fatalf("overflowing delta delay = %s", got)
	}
	if got := retryDelay(1, now.Add(-time.Second).Format(http.TimeFormat), now); got != 0 {
		t.Fatalf("past date delay = %s", got)
	}
}

func TestUnsupportedProtocolAndRPCErrorMapping(t *testing.T) {
	client, _ := fixtureClient(t, &mcpFixture{protocol: "2099-01-01"})
	_, err := client.Fetch(context.Background(), "id")
	assertCode(t, err, "vertc.docs.unsupported_protocol")

	cases := []struct {
		rpc  *rpcError
		code string
	}{
		{&rpcError{Code: -32602, Data: json.RawMessage(`{"reason":"query is required"}`)}, "vertc.docs.invalid_argument"},
		{&rpcError{Code: -32601}, "vertc.docs.tool_unavailable"},
		{&rpcError{Code: -32000}, "vertc.docs.protocol_error"},
	}
	for _, tc := range cases {
		assertCode(t, mapRPCError(tc.rpc), tc.code)
	}

	httpCases := []struct {
		status int
		code   string
	}{
		{http.StatusUnauthorized, "vertc.docs.unauthorized"},
		{http.StatusForbidden, "vertc.docs.unauthorized"},
		{http.StatusTooManyRequests, "vertc.docs.rate_limited"},
		{http.StatusBadRequest, "vertc.docs.http_error"},
	}
	for _, tc := range httpCases {
		assertCode(t, httpStatusError(tc.status, "initialize", "request-id", "1"), tc.code)
	}
}

func TestHTTPStatusRetryAndSafeDetails(t *testing.T) {
	var attempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.Header().Set("Retry-After", "99")
		w.Header().Set(requestIDHeader, "safe-request-id")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte("secret response body"))
	}))
	defer server.Close()
	var events []RetryEvent
	client, err := newClient(server.URL, "1.0.0", nil, WithRetryObserver(func(event RetryEvent) { events = append(events, event) }))
	if err != nil {
		t.Fatal(err)
	}
	client.sleep = func(context.Context, time.Duration) error { return nil }
	_, err = client.Fetch(context.Background(), "secret-query")
	typed := assertCode(t, err, "vertc.docs.rate_limited")
	if attempts != maxAttempts || len(events) != maxAttempts-1 {
		t.Fatalf("attempts=%d events=%#v", attempts, events)
	}
	if events[0].DelayMS != maxRetryAfter.Milliseconds() || typed.Details["request_id"] != "safe-request-id" {
		t.Fatalf("event=%#v details=%#v", events[0], typed.Details)
	}
	encoded, _ := json.Marshal(typed)
	for _, secret := range []string{"secret response body", "secret-query", "Retry-After"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("error leaked %q: %s", secret, encoded)
		}
	}
}

func TestRedirectIsRejected(t *testing.T) {
	var targetHits int
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { targetHits++ }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer redirect.Close()
	client, err := newClient(redirect.URL, "1.0.0", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Fetch(context.Background(), "id")
	assertCode(t, err, "vertc.docs.http_error")
	if targetHits != 0 {
		t.Fatalf("redirect target hits = %d", targetHits)
	}
}

func TestTimeoutAndDecodedResponseLimit(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			time.Sleep(100 * time.Millisecond)
			w.WriteHeader(http.StatusNoContent)
		}))
		defer server.Close()
		client, err := newClient(server.URL, "1.0.0", nil)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		_, err = client.Fetch(ctx, "id")
		assertCode(t, err, "vertc.docs.timeout")
	})

	t.Run("gzip decoded size", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Encoding", "gzip")
			w.Header().Set("Content-Type", contentTypeJSON)
			zipper := gzip.NewWriter(w)
			_, _ = zipper.Write([]byte(strings.Repeat("x", int(MaxResponseBytes)+1)))
			_ = zipper.Close()
		}))
		defer server.Close()
		client, err := newClient(server.URL, "1.0.0", nil)
		if err != nil {
			t.Fatal(err)
		}
		_, err = client.Fetch(context.Background(), "id")
		assertCode(t, err, "vertc.docs.response_too_large")
	})
}

func TestTransportFailureAndCleanupFailureAreTyped(t *testing.T) {
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("offline") })
	client, err := newClient("https://fixture.invalid/mcp", "1.0.0", &http.Client{Transport: transport})
	if err != nil {
		t.Fatal(err)
	}
	client.sleep = func(context.Context, time.Duration) error { return nil }
	_, err = client.Fetch(context.Background(), "id")
	assertCode(t, err, "vertc.docs.request_failed")
	client.sessionID = "session"
	assertCode(t, client.Close(context.Background()), "vertc.docs.request_failed")
}

func TestProductionConstructionIgnoresEndpointAndHeaderEnvironment(t *testing.T) {
	t.Setenv("VERTC_DOCS_ENDPOINT", "https://attacker.invalid/mcp")
	t.Setenv("VERTC_DOCS_AUTHORIZATION", "Bearer secret")
	client, err := New("1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if client.endpoint != productionEndpoint || client.userAgent != "vertc/1.2.3" {
		t.Fatalf("endpoint=%q ua=%q", client.endpoint, client.userAgent)
	}
	if err := validateProductionEndpoint(productionEndpoint); err != nil {
		t.Fatal(err)
	}
	for _, endpoint := range []string{
		"http://topic.bytedance.com/mcp/rtc",
		"https://evil.example/mcp/rtc",
		"https://topic.bytedance.com/mcp/other",
		"https://topic.bytedance.com/mcp/rtc?endpoint=other",
		"https://topic.bytedance.com/mcp/%72tc",
	} {
		if err := validateProductionEndpoint(endpoint); err == nil {
			t.Errorf("production validator accepted %q", endpoint)
		}
	}
	request, err := http.NewRequest(http.MethodGet, "https://example.invalid", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.http.CheckRedirect(request, []*http.Request{request}); !errors.Is(err, http.ErrUseLastResponse) {
		t.Fatalf("redirect policy error = %v", err)
	}
	if len(client.http.Transport.(*http.Transport).ProxyConnectHeader) != 0 {
		t.Fatal("client transport unexpectedly carries proxy credentials")
	}
}

func TestRetryEventAndErrorDetailsUseAllowlistedFieldsOnly(t *testing.T) {
	eventType, _ := json.Marshal(RetryEvent{Phase: "tools/call", Attempt: 2, Status: 503, DelayMS: 10})
	var fields map[string]any
	if err := json.Unmarshal(eventType, &fields); err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{"Phase": true, "Attempt": true, "Status": true, "DelayMS": true}
	for name := range fields {
		if !allowed[name] {
			t.Fatalf("retry event exposes unexpected field %q: %s", name, eventType)
		}
	}
	typed := httpStatusError(http.StatusBadGateway, "tools/call", "request-id", "")
	encoded, _ := json.Marshal(typed)
	for _, allowed := range []string{"phase", "status", "request_id"} {
		if !strings.Contains(string(encoded), allowed) {
			t.Fatalf("missing allowlisted field %q: %s", allowed, encoded)
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return fn(request) }

type errorBody struct {
	data []byte
	err  error
	done bool
}

func (b *errorBody) Read(p []byte) (int, error) {
	if !b.done && len(b.data) != 0 {
		b.done = true
		return copy(p, b.data), nil
	}
	return 0, b.err
}

func (*errorBody) Close() error { return nil }

type contextBody struct{ ctx context.Context }

func (b *contextBody) Read([]byte) (int, error) {
	<-b.ctx.Done()
	return 0, b.ctx.Err()
}

func (*contextBody) Close() error { return nil }
