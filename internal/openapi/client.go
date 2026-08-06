// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package openapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
)

// Defaults for the RTC OpenAPI.
const (
	DefaultEndpoint = "https://rtc.volcengineapi.com"
	DefaultRegion   = "cn-north-1"
	DefaultService  = "rtc"
	DefaultVersion  = "2025-06-01"
)

// Credential is the signing material used for Volcengine OpenAPI calls.
// SessionToken is required for Signin-issued STS credentials.
type Credential struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
}

// CredentialProvider supplies fresh signing material for each request.
type CredentialProvider interface {
	Credentials(context.Context) (Credential, error)
}

// Client is the unified signed Volcengine OpenAPI client.
type Client struct {
	Endpoint  string
	Service   string
	Region    string
	UserAgent string
	HTTP      *http.Client
	Now       func() time.Time
	Provider  CredentialProvider
}

// NewClient builds a client with pinned RTC defaults and the given provider.
func NewClient(provider CredentialProvider) *Client {
	return &Client{
		Endpoint: DefaultEndpoint,
		Service:  DefaultService,
		Region:   DefaultRegion,
		HTTP:     &http.Client{Timeout: 15 * time.Second},
		Provider: provider,
	}
}

type InvokeOptions struct {
	Action  string
	Version string
	Method  string
	Query   url.Values
	Body    []byte
	Header  http.Header
}

type Response struct {
	StatusCode int
	Header     http.Header
	Body       []byte
	RequestID  string
}

// APIError is a structured Volcengine OpenAPI error (from ResponseMetadata.Error
// or a transport/HTTP failure). Callers map it to a typed CLI error.code.
type APIError struct {
	Action     string
	Code       string
	Message    string
	RequestID  string
	HTTPStatus int
}

func (e *APIError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("%s failed: %s: %s (RequestId=%s)", e.Action, e.Code, e.Message, e.RequestID)
	}
	return fmt.Sprintf("%s failed: HTTP %d: %s", e.Action, e.HTTPStatus, e.Message)
}

// responseEnvelope is the common Volcengine response wrapper.
type responseEnvelope struct {
	ResponseMetadata struct {
		RequestID string `json:"RequestId"`
		Action    string `json:"Action"`
		Error     *struct {
			Code    string `json:"Code"`
			Message string `json:"Message"`
		} `json:"Error"`
	} `json:"ResponseMetadata"`
	Result json.RawMessage `json:"Result"`
}

func (c *Client) Invoke(ctx context.Context, options InvokeOptions) (Response, error) {
	if c == nil {
		return Response{}, openAPIErr("vertc.openapi.invalid_request", "openapi client is nil")
	}
	if c.Provider == nil {
		return Response{}, openAPIErr("vertc.openapi.invalid_request", "openapi credential provider is required")
	}
	if strings.TrimSpace(c.endpoint()) == "" {
		return Response{}, openAPIErr("vertc.openapi.invalid_request", "openapi endpoint is required")
	}
	if strings.TrimSpace(options.Action) == "" {
		return Response{}, openAPIErr("vertc.openapi.invalid_request", "openapi action is required")
	}
	if strings.TrimSpace(options.Version) == "" {
		return Response{}, openAPIErr("vertc.openapi.invalid_request", "openapi version is required")
	}
	method := strings.ToUpper(strings.TrimSpace(options.Method))
	if method == "" {
		method = http.MethodPost
	}
	body := options.Body
	if len(body) == 0 && method != http.MethodGet {
		body = []byte("{}")
	}

	endpointURL, err := url.Parse(c.endpoint())
	if err != nil {
		return Response{}, openAPIErr("vertc.openapi.invalid_request", "parse openapi endpoint: %s", err)
	}
	if endpointURL.Scheme == "" || endpointURL.Host == "" {
		return Response{}, openAPIErr("vertc.openapi.invalid_request", "openapi endpoint must include scheme and host")
	}
	query := endpointURL.Query()
	for key, values := range options.Query {
		for _, value := range values {
			query.Add(key, value)
		}
	}
	query.Set("Action", options.Action)
	query.Set("Version", options.Version)
	endpointURL.RawQuery = query.Encode()
	if endpointURL.Path == "" {
		endpointURL.Path = "/"
	}

	req, err := http.NewRequestWithContext(ctx, method, endpointURL.String(), bytes.NewReader(body))
	if err != nil {
		return Response{}, openAPIErr("vertc.openapi.request_failed", "create openapi request: %s", err)
	}
	req.Host = endpointURL.Host
	for key, values := range options.Header {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	credential, err := c.Provider.Credentials(ctx)
	if err != nil {
		return Response{}, err
	}
	if err := SignRequest(req, credential, SignOptions{Service: c.service(), Region: c.region(), Now: c.now(), Body: body}); err != nil {
		return Response{}, openAPIErr("vertc.openapi.sign_failed", "sign openapi request: %s", err)
	}

	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return Response{}, openAPIErr("vertc.openapi.request_failed", "send openapi request: %s", err)
	}
	defer resp.Body.Close()
	respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if readErr != nil {
		return Response{}, openAPIErr("vertc.openapi.request_failed", "read openapi response: %s", readErr)
	}

	var env responseEnvelope
	envOK := json.Unmarshal(respBody, &env) == nil
	if envOK && env.ResponseMetadata.Error != nil && env.ResponseMetadata.Error.Code != "" {
		return Response{}, &APIError{
			Action:     options.Action,
			Code:       env.ResponseMetadata.Error.Code,
			Message:    env.ResponseMetadata.Error.Message,
			RequestID:  env.ResponseMetadata.RequestID,
			HTTPStatus: resp.StatusCode,
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := strings.TrimSpace(string(respBody))
		if message == "" {
			message = http.StatusText(resp.StatusCode)
		}
		requestID := ""
		if envOK {
			requestID = env.ResponseMetadata.RequestID
		}
		return Response{}, &APIError{
			Action:     options.Action,
			Message:    truncate(message, 200),
			RequestID:  requestID,
			HTTPStatus: resp.StatusCode,
		}
	}

	requestID := ""
	if envOK {
		requestID = env.ResponseMetadata.RequestID
	}
	return Response{StatusCode: resp.StatusCode, Header: resp.Header.Clone(), Body: respBody, RequestID: requestID}, nil
}

func (c *Client) InvokeResult(ctx context.Context, options InvokeOptions, result any) error {
	response, err := c.Invoke(ctx, options)
	if err != nil {
		return err
	}
	var env responseEnvelope
	if err := json.Unmarshal(response.Body, &env); err != nil {
		return &APIError{
			Action:     options.Action,
			Message:    fmt.Sprintf("unparseable response: %s", truncate(string(response.Body), 200)),
			RequestID:  response.RequestID,
			HTTPStatus: response.StatusCode,
		}
	}
	if result == nil || len(env.Result) == 0 {
		return nil
	}
	return json.Unmarshal(env.Result, result)
}

func (c *Client) endpoint() string {
	if c.Endpoint != "" {
		return c.Endpoint
	}
	return DefaultEndpoint
}

func (c *Client) service() string {
	if c.Service != "" {
		return c.Service
	}
	return DefaultService
}

func (c *Client) region() string {
	if c.Region != "" {
		return c.Region
	}
	return DefaultRegion
}

func (c *Client) now() time.Time {
	if c.Now != nil {
		return c.Now().UTC()
	}
	return time.Now().UTC()
}

func openAPIErr(code string, format string, args ...any) *errs.Error {
	errType := errs.TypeValidation
	if code == "vertc.openapi.request_failed" {
		errType = errs.TypeIO
	}
	return errs.New(code, errType, format, args...)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
