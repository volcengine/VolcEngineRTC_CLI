// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package topicdocs

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/meta"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/telemetry"
)

const (
	commandTimeout    = 30 * time.Second
	lifecycleTimeout  = 5 * time.Second
	toolCallTimeout   = 15 * time.Second
	cleanupTimeout    = time.Second
	maxAttempts       = 3
	maxRetryAfter     = 5 * time.Second
	maxBackoff        = 2 * time.Second
	requestIDHeader   = "x-tt-logid"
	sessionIDHeader   = "MCP-Session-Id"
	contentTypeJSON   = "application/json"
	contentTypeEvents = "text/event-stream"
)

// Option customizes non-wire client behavior such as retry progress reporting.
type Option func(*Client)

// WithRetryObserver installs a payload-free retry progress callback.
func WithRetryObserver(fn func(RetryEvent)) Option {
	return func(c *Client) { c.onRetry = fn }
}

// Client owns one in-memory MCP lifecycle and is intended for one command.
type Client struct {
	endpoint  string
	version   string
	userAgent string
	http      *http.Client
	onRetry   func(RetryEvent)
	sleep     func(context.Context, time.Duration) error
	now       func() time.Time
	nextID    int64
	sessionID string
	meta      MCPMeta
}

// New creates a production client for the fixed Topic RTC endpoint.
func New(version string, options ...Option) (*Client, error) {
	return newDefaultClient(version, options...)
}

func validateProductionEndpoint(endpoint string) error {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.Host != "topic.bytedance.com" || u.Path != "/mcp/rtc" ||
		u.RawPath != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return errs.New("vertc.docs.invalid_argument", errs.TypeValidation,
			"invalid fixed RTC documentation endpoint")
	}
	return nil
}

func newClient(endpoint, version string, httpClient *http.Client, options ...Option) (*Client, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, errs.New("vertc.docs.invalid_argument", errs.TypeValidation,
			"invalid RTC documentation endpoint").WithParam("endpoint")
	}
	if version == "" {
		return nil, errs.New("vertc.docs.invalid_argument", errs.TypeValidation,
			"CLI version is empty").WithParam("version")
	}
	userAgent := meta.UserAgentProduct + "/" + version
	if invocationUserAgent, ok := telemetry.GetInvocationUserAgent(); ok {
		userAgent = invocationUserAgent
	}
	if httpClient == nil {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		httpClient = &http.Client{
			Transport: transport,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}
	c := &Client{
		endpoint:  endpoint,
		version:   version,
		userAgent: userAgent,
		http:      httpClient,
		nextID:    1,
		sleep: func(ctx context.Context, d time.Duration) error {
			timer := time.NewTimer(d)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				return nil
			}
		},
		now: time.Now,
	}
	for _, apply := range options {
		apply(c)
	}
	return c, nil
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error"`
}

type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type initializeResult struct {
	ProtocolVersion string `json:"protocolVersion"`
	ServerInfo      struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"serverInfo"`
}

type toolSchema struct {
	Type                 string                    `json:"type"`
	Properties           map[string]schemaProperty `json:"properties"`
	Required             []string                  `json:"required,omitempty"`
	AdditionalProperties *bool                     `json:"additionalProperties"`
}

type schemaProperty struct {
	Type string `json:"type"`
}

type toolDescription struct {
	Name        string     `json:"name"`
	InputSchema toolSchema `json:"inputSchema"`
}

type toolsListResult struct {
	Tools []toolDescription `json:"tools"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type toolCallResult struct {
	Content []contentBlock `json:"content"`
	IsError bool           `json:"isError"`
}

func (c *Client) commandContext(parent context.Context) (context.Context, context.CancelFunc) {
	if deadline, ok := parent.Deadline(); ok && time.Until(deadline) <= commandTimeout {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, commandTimeout)
}

func (c *Client) prepare(ctx context.Context, tool string) error {
	var initialized initializeResult
	if err := c.rpc(ctx, "initialize", map[string]any{
		"protocolVersion": OfferedProtocol,
		"capabilities":    map[string]any{},
		"clientInfo": map[string]string{
			"name": meta.UserAgentProduct, "version": c.version,
		},
	}, lifecycleTimeout, &initialized); err != nil {
		return err
	}
	if initialized.ProtocolVersion != OfferedProtocol && initialized.ProtocolVersion != LegacyProtocol {
		return errs.New("vertc.docs.unsupported_protocol", errs.TypePrecondition,
			"RTC documentation MCP negotiated an unsupported protocol version").
			WithDetails(map[string]any{"protocol_version": initialized.ProtocolVersion})
	}
	c.meta = MCPMeta{
		ProtocolVersion: initialized.ProtocolVersion,
		ServerName:      initialized.ServerInfo.Name,
		ServerVersion:   initialized.ServerInfo.Version,
	}
	if err := c.notify(ctx, "notifications/initialized", map[string]any{}, lifecycleTimeout); err != nil {
		return err
	}
	var listed toolsListResult
	if err := c.rpc(ctx, "tools/list", map[string]any{}, lifecycleTimeout, &listed); err != nil {
		return err
	}
	return validateTool(listed.Tools, tool)
}

func validateTool(tools []toolDescription, name string) error {
	var found *toolDescription
	for i := range tools {
		if tools[i].Name == name {
			found = &tools[i]
			break
		}
	}
	if found == nil {
		return errs.New("vertc.docs.tool_unavailable", errs.TypePrecondition,
			"required RTC documentation tool is unavailable").
			WithDetails(map[string]any{"tool": name})
	}
	schema := found.InputSchema
	if schema.Type != "object" || schema.AdditionalProperties == nil || *schema.AdditionalProperties {
		return schemaChanged(name)
	}
	switch name {
	case "search_docs":
		if len(schema.Properties) != 1 || schema.Properties["query"].Type != "string" {
			return schemaChanged(name)
		}
		if !compatibleRequired(schema.Required, "query") {
			return schemaChanged(name)
		}
	case "fetch_doc":
		if len(schema.Properties) != 1 || schema.Properties["id"].Type != "string" {
			return schemaChanged(name)
		}
		if !compatibleRequired(schema.Required, "id") {
			return schemaChanged(name)
		}
	case "list_docs":
		if len(schema.Properties) != 0 || len(schema.Required) != 0 {
			return schemaChanged(name)
		}
	default:
		return errs.New("vertc.docs.tool_unavailable", errs.TypePrecondition,
			"unsupported RTC documentation tool").WithDetails(map[string]any{"tool": name})
	}
	return nil
}

func compatibleRequired(required []string, expected string) bool {
	if len(required) == 0 {
		return true
	}
	return len(required) == 1 && required[0] == expected
}

func schemaChanged(tool string) error {
	return errs.New("vertc.docs.tool_schema_changed", errs.TypePrecondition,
		"RTC documentation tool schema is incompatible").
		WithDetails(map[string]any{"tool": tool})
}

func (c *Client) callTool(ctx context.Context, name string, arguments map[string]any) (string, error) {
	var result toolCallResult
	if err := c.rpc(ctx, "tools/call", map[string]any{
		"name": name, "arguments": arguments,
	}, toolCallTimeout, &result); err != nil {
		return "", err
	}
	if result.IsError {
		if name == "fetch_doc" && len(result.Content) == 1 && result.Content[0].Type == "text" &&
			strings.HasPrefix(result.Content[0].Text, "Document not found: ") {
			return "", errs.New("vertc.docs.document_not_found", errs.TypeNotFound,
				"RTC documentation document was not found").WithParam("doc-id")
		}
		return "", errs.New("vertc.docs.tool_error", errs.TypePrecondition,
			"RTC documentation tool returned an error").
			WithDetails(map[string]any{"tool": name})
	}
	if len(result.Content) != 1 || result.Content[0].Type != "text" || result.Content[0].Text == "" {
		return "", errs.New("vertc.docs.protocol_error", errs.TypeIO,
			"RTC documentation tool returned an invalid content shape").
			WithDetails(map[string]any{"tool": name})
	}
	return result.Content[0].Text, nil
}

func (c *Client) rpc(ctx context.Context, method string, params any, timeout time.Duration, out any) error {
	id := c.nextID
	c.nextID++
	payload, err := json.Marshal(rpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params})
	if err != nil {
		return errs.Wrap(err, "vertc.docs.protocol_error", "encode RTC documentation MCP request")
	}
	body, contentType, err := c.do(ctx, method, payload, timeout, id)
	if err != nil {
		return err
	}
	response, err := decodeRPC(body, contentType, id)
	if err != nil {
		return err
	}
	if response.Error != nil {
		return mapRPCError(response.Error)
	}
	if len(response.Result) == 0 || string(response.Result) == "null" {
		return errs.New("vertc.docs.protocol_error", errs.TypeIO,
			"RTC documentation MCP response omitted result")
	}
	if err := json.Unmarshal(response.Result, out); err != nil {
		return errs.New("vertc.docs.protocol_error", errs.TypeIO,
			"RTC documentation MCP result is malformed").WithCause(err)
	}
	return nil
}

func (c *Client) notify(ctx context.Context, method string, params any, timeout time.Duration) error {
	payload, err := json.Marshal(struct {
		JSONRPC string `json:"jsonrpc"`
		Method  string `json:"method"`
		Params  any    `json:"params"`
	}{JSONRPC: "2.0", Method: method, Params: params})
	if err != nil {
		return errs.Wrap(err, "vertc.docs.protocol_error", "encode RTC documentation MCP notification")
	}
	_, _, err = c.do(ctx, method, payload, timeout, 0)
	return err
}

func decodeRPC(body []byte, _ string, wantID int64) (*rpcResponse, error) {
	raw := body
	var response rpcResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, errs.New("vertc.docs.protocol_error", errs.TypeIO,
			"RTC documentation MCP response is not valid JSON").WithCause(err)
	}
	if response.JSONRPC != "2.0" || responseID(response.ID) != wantID {
		return nil, errs.New("vertc.docs.protocol_error", errs.TypeIO,
			"RTC documentation MCP response envelope is invalid")
	}
	hasResult := len(response.Result) > 0
	if hasResult == (response.Error != nil) {
		return nil, errs.New("vertc.docs.protocol_error", errs.TypeIO,
			"RTC documentation MCP response must contain exactly one of result or error")
	}
	return &response, nil
}

func responseID(raw json.RawMessage) int64 {
	var id int64
	if len(raw) == 0 || json.Unmarshal(raw, &id) != nil {
		return -1
	}
	return id
}

func mapRPCError(e *rpcError) error {
	switch e.Code {
	case -32602:
		reason := "upstream rejected tool arguments"
		var data struct {
			Reason string `json:"reason"`
		}
		if json.Unmarshal(e.Data, &data) == nil && (data.Reason == "query is required" || data.Reason == "id is required") {
			reason = data.Reason
		}
		return errs.New("vertc.docs.invalid_argument", errs.TypeValidation, "%s", reason)
	case -32601:
		return errs.New("vertc.docs.tool_unavailable", errs.TypePrecondition,
			"RTC documentation tool is unavailable")
	default:
		return errs.New("vertc.docs.protocol_error", errs.TypeIO,
			"RTC documentation MCP returned a JSON-RPC error").
			WithDetails(map[string]any{"rpc_code": e.Code})
	}
}

func (c *Client) do(ctx context.Context, phase string, payload []byte, timeout time.Duration, wantID int64) ([]byte, string, error) {
	var lastStatus int
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		phaseCtx, cancel := context.WithTimeout(ctx, timeout)
		req, err := http.NewRequestWithContext(phaseCtx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
		if err != nil {
			cancel()
			return nil, "", errs.New("vertc.docs.invalid_argument", errs.TypeValidation,
				"cannot create RTC documentation request").WithCause(err)
		}
		req.Header.Set("Content-Type", contentTypeJSON)
		req.Header.Set("Accept", contentTypeJSON+", "+contentTypeEvents)
		req.Header.Set("User-Agent", c.userAgent)
		if c.sessionID != "" {
			req.Header.Set(sessionIDHeader, c.sessionID)
		}
		resp, requestErr := c.http.Do(req)
		if requestErr != nil {
			cancel()
			if ctx.Err() != nil || isTimeoutError(requestErr) || errors.Is(requestErr, context.Canceled) {
				return nil, "", timeoutError(phase, requestErr)
			}
			if attempt < maxAttempts {
				if err := c.retry(ctx, phase, attempt, 0, retryDelay(attempt, "", c.now())); err != nil {
					return nil, "", timeoutError(phase, err)
				}
				continue
			}
			return nil, "", errs.New("vertc.docs.request_failed", errs.TypeIO,
				"RTC documentation request failed").WithCause(requestErr).
				WithDetails(map[string]any{"phase": phase})
		}
		lastStatus = resp.StatusCode
		if sid := strings.TrimSpace(resp.Header.Get(sessionIDHeader)); sid != "" {
			c.sessionID = sid
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			contentType := resp.Header.Get("Content-Type")
			body, normalizedType, readErr := readResponse(resp.Body, contentType, wantID)
			phaseErr := phaseCtx.Err()
			cancel()
			if readErr != nil {
				if ctx.Err() != nil || phaseErr != nil || isTimeoutError(readErr) || errors.Is(readErr, context.Canceled) {
					return nil, "", timeoutError(phase, readErr)
				}
				if _, typed := errs.As(readErr); typed {
					return nil, "", readErr
				}
				if isRetryableReadError(readErr) && attempt < maxAttempts {
					if err := c.retry(ctx, phase, attempt, resp.StatusCode, retryDelay(attempt, "", c.now())); err != nil {
						return nil, "", timeoutError(phase, err)
					}
					continue
				}
				return nil, "", errs.New("vertc.docs.request_failed", errs.TypeIO,
					"cannot read RTC documentation response").WithCause(readErr).
					WithDetails(map[string]any{"phase": phase})
			}
			if wantID != 0 && len(body) == 0 {
				return nil, "", errs.New("vertc.docs.protocol_error", errs.TypeIO,
					"RTC documentation MCP returned an empty response").WithDetails(map[string]any{"phase": phase})
			}
			return body, normalizedType, nil
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		requestID := strings.TrimSpace(resp.Header.Get(requestIDHeader))
		retryAfter := resp.Header.Get("Retry-After")
		cancel()
		if retryableStatus(resp.StatusCode) && attempt < maxAttempts {
			if err := c.retry(ctx, phase, attempt, resp.StatusCode, retryDelay(attempt, retryAfter, c.now())); err != nil {
				return nil, "", timeoutError(phase, err)
			}
			continue
		}
		return nil, "", httpStatusError(resp.StatusCode, phase, requestID, retryAfter)
	}
	return nil, "", httpStatusError(lastStatus, phase, "", "")
}

func readResponse(body io.ReadCloser, contentType string, wantID int64) ([]byte, string, error) {
	defer body.Close()
	if wantID != 0 && strings.HasPrefix(strings.ToLower(contentType), contentTypeEvents) {
		message, err := readSSEMessage(body, wantID)
		return message, contentTypeJSON, err
	}
	data, err := io.ReadAll(io.LimitReader(body, MaxResponseBytes+1))
	if err != nil {
		return nil, contentType, err
	}
	if int64(len(data)) > MaxResponseBytes {
		return nil, contentType, errs.New("vertc.docs.response_too_large", errs.TypePrecondition,
			"RTC documentation response exceeded the 2 MiB limit").
			WithDetails(map[string]any{"max_bytes": MaxResponseBytes})
	}
	return data, contentType, nil
}

func readSSEMessage(body io.Reader, wantID int64) ([]byte, error) {
	limited := &io.LimitedReader{R: body, N: MaxResponseBytes + 1}
	reader := bufio.NewReader(limited)
	var data []string
	checkEvent := func() ([]byte, error) {
		if len(data) == 0 {
			return nil, nil
		}
		message := []byte(strings.Join(data, "\n"))
		data = nil
		var candidate rpcResponse
		if json.Unmarshal(message, &candidate) != nil || responseID(candidate.ID) != wantID {
			return nil, nil
		}
		if _, err := decodeRPC(message, contentTypeJSON, wantID); err != nil {
			return nil, err
		}
		return message, nil
	}
	for {
		line, err := reader.ReadString('\n')
		if limited.N == 0 {
			return nil, errs.New("vertc.docs.response_too_large", errs.TypePrecondition,
				"RTC documentation response exceeded the 2 MiB limit").
				WithDetails(map[string]any{"max_bytes": MaxResponseBytes})
		}
		line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		switch {
		case line == "":
			if message, eventErr := checkEvent(); message != nil || eventErr != nil {
				return message, eventErr
			}
		case strings.HasPrefix(line, "data:"):
			value := strings.TrimPrefix(line, "data:")
			value = strings.TrimPrefix(value, " ")
			data = append(data, value)
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				return nil, err
			}
			if message, eventErr := checkEvent(); message != nil || eventErr != nil {
				return message, eventErr
			}
			return nil, errs.New("vertc.docs.protocol_error", errs.TypeIO,
				"RTC documentation MCP event stream omitted the matching response")
		}
	}
}

func isRetryableReadError(err error) bool {
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	var temporaryErr interface{ Temporary() bool }
	return errors.As(err, &temporaryErr) && temporaryErr.Temporary() && !isTimeoutError(err)
}

func isTimeoutError(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var networkErr net.Error
	return errors.As(err, &networkErr) && networkErr.Timeout()
}

func retryableStatus(status int) bool {
	return status == http.StatusTooManyRequests || status == http.StatusBadGateway ||
		status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout
}

func retryDelay(attempt int, retryAfter string, now time.Time) time.Duration {
	if delay, ok := parseRetryAfter(retryAfter, now); ok {
		return delay
	}
	capDelay := 200 * time.Millisecond * time.Duration(1<<(attempt-1))
	if capDelay > maxBackoff {
		capDelay = maxBackoff
	}
	if capDelay <= 0 {
		return 0
	}
	return time.Duration(rand.Int63n(int64(capDelay) + 1))
}

func parseRetryAfter(value string, now time.Time) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	if seconds, err := strconv.ParseUint(value, 10, 64); err == nil {
		maxSeconds := uint64(maxRetryAfter / time.Second)
		if seconds >= maxSeconds {
			return maxRetryAfter, true
		}
		return time.Duration(seconds) * time.Second, true
	}
	if strings.IndexFunc(value, func(r rune) bool { return r < '0' || r > '9' }) == -1 {
		return maxRetryAfter, true
	}
	date, err := http.ParseTime(value)
	if err != nil {
		return 0, false
	}
	delay := date.Sub(now)
	if delay <= 0 {
		return 0, true
	}
	if delay > maxRetryAfter {
		return maxRetryAfter, true
	}
	return delay, true
}

func (c *Client) retry(ctx context.Context, phase string, attempt, status int, delay time.Duration) error {
	if c.onRetry != nil {
		c.onRetry(RetryEvent{Phase: phase, Attempt: attempt + 1, Status: status, DelayMS: delay.Milliseconds()})
	}
	return c.sleep(ctx, delay)
}

func timeoutError(phase string, cause error) error {
	return errs.New("vertc.docs.timeout", errs.TypeIO,
		"RTC documentation request timed out").WithCause(cause).
		WithDetails(map[string]any{"phase": phase})
}

func httpStatusError(status int, phase, requestID, retryAfter string) error {
	details := map[string]any{"phase": phase, "status": status}
	if requestID != "" {
		details["request_id"] = requestID
	}
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return errs.New("vertc.docs.unauthorized", errs.TypeAuth,
			"RTC documentation service rejected the request").WithDetails(details)
	case http.StatusTooManyRequests:
		if d := retryDelay(1, retryAfter, time.Now()); d > 0 {
			details["retry_after_seconds"] = int(d.Seconds())
		}
		return errs.New("vertc.docs.rate_limited", errs.TypePrecondition,
			"RTC documentation service rate limit was exceeded").WithDetails(details)
	default:
		return errs.New("vertc.docs.http_error", errs.TypePrecondition,
			"RTC documentation service returned HTTP %d", status).WithDetails(details)
	}
}

// Close best-effort terminates a server-issued session. Callers should warn on
// failure without replacing an already successful data result.
func (c *Client) Close(parent context.Context) error {
	if c.sessionID == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(parent, cleanupTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.endpoint, nil)
	if err != nil {
		return errs.New("vertc.docs.request_failed", errs.TypeIO,
			"cannot create RTC documentation session cleanup request").WithCause(err)
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set(sessionIDHeader, c.sessionID)
	resp, err := c.http.Do(req)
	if err != nil {
		return errs.New("vertc.docs.request_failed", errs.TypeIO,
			"RTC documentation session cleanup request failed").WithCause(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errs.New("vertc.docs.http_error", errs.TypePrecondition,
			"RTC documentation session cleanup returned HTTP %d", resp.StatusCode).
			WithDetails(map[string]any{"phase": "session/cleanup", "status": resp.StatusCode})
	}
	return nil
}
