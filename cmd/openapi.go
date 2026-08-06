// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/auth"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/config"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/openapi"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/telemetry"
)

type openAPIInvokeResult struct {
	StatusCode int             `json:"status_code"`
	Body       json.RawMessage `json:"body,omitempty"`
	RawBody    string          `json:"raw_body,omitempty"`
}

func newOpenAPICmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "openapi",
		Short:  "Invoke VolcEngine OpenAPI with stored Signin STS credentials",
		Hidden: true,
	}
	cmd.AddCommand(newOpenAPIInvokeCmd())
	return cmd
}

func newOpenAPIInvokeCmd() *cobra.Command {
	var (
		endpoint  string
		service   string
		region    string
		action    string
		version   string
		method    string
		bodyArg   string
		queryArgs []string
		timeout   time.Duration
	)
	cmd := &cobra.Command{
		Use:    "invoke",
		Short:  "Invoke a signed OpenAPI request",
		Hidden: true,
		RunE: func(c *cobra.Command, args []string) error {
			cfg := config.Default("", "", "")
			if loaded, _, err := config.LoadNearest("."); err == nil {
				cfg = loaded
			} else if configErr, ok := errs.As(err); !ok || configErr.Code != "vertc.config.not_found" {
				return err
			}
			if endpoint == "" {
				endpoint = cfg.OpenAPI.Endpoint
			}
			if service == "" {
				service = cfg.OpenAPI.Service
			}
			if region == "" {
				region = cfg.OpenAPI.Region
			}
			body, err := readBody(bodyArg)
			if err != nil {
				return err
			}
			query, err := parseQueryArgs(queryArgs)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(c.Context(), timeout)
			defer cancel()
			client := newAuthenticatedOpenAPIClient(&config.Config{OpenAPI: config.OpenAPI{
				Endpoint: endpoint,
				Service:  service,
				Region:   region,
			}})
			response, err := client.Invoke(ctx, openapi.InvokeOptions{
				Action:  action,
				Version: version,
				Method:  method,
				Query:   query,
				Body:    body,
			})
			if err != nil {
				return mapOpenAPIError(err)
			}
			result := openAPIInvokeResult{StatusCode: response.StatusCode}
			if json.Valid(response.Body) {
				result.Body = json.RawMessage(response.Body)
			} else {
				result.RawBody = string(response.Body)
			}
			return out().Data(result)
		},
	}
	cmd.Flags().StringVar(&endpoint, "endpoint", "", "OpenAPI endpoint")
	cmd.Flags().StringVar(&service, "service", "", "OpenAPI signing service")
	cmd.Flags().StringVar(&region, "region", "", "OpenAPI signing region")
	cmd.Flags().StringVar(&action, "action", "", "OpenAPI Action query parameter")
	cmd.Flags().StringVar(&version, "version", "", "OpenAPI Version query parameter")
	cmd.Flags().StringVar(&method, "method", http.MethodPost, "HTTP method")
	cmd.Flags().StringVar(&bodyArg, "body", "", "JSON request body or @file; POST defaults to {}")
	cmd.Flags().StringArrayVar(&queryArgs, "query", nil, "additional query parameter as key=value; repeatable")
	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "end-to-end timeout")
	_ = cmd.MarkFlagRequired("action")
	_ = cmd.MarkFlagRequired("version")
	return cmd
}

func readBody(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	data, err := readMaybeFile(value)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(data) == "" {
		return nil, nil
	}
	if !json.Valid([]byte(data)) {
		return nil, errs.New("vertc.openapi.invalid_request", errs.TypeValidation,
			"request body must be valid JSON")
	}
	return []byte(data), nil
}

func readMaybeFile(value string) (string, error) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "@") {
		path := strings.TrimPrefix(value, "@")
		data, err := os.ReadFile(path)
		if err != nil {
			return "", errs.New("vertc.openapi.invalid_request", errs.TypeValidation,
				"read request body file %q: %s", path, err)
		}
		return string(data), nil
	}
	return value, nil
}

func parseQueryArgs(args []string) (url.Values, error) {
	values := url.Values{}
	for _, arg := range args {
		key, value, ok := strings.Cut(arg, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, errs.New("vertc.openapi.invalid_request", errs.TypeValidation,
				"query must be key=value: %s", arg)
		}
		values.Add(key, value)
	}
	return values, nil
}

func newAuthenticatedOpenAPIClient(cfg *config.Config) *openapi.Client {
	client := openapi.NewClient(auth.STSProvider{})
	if userAgent, ok := telemetry.GetInvocationUserAgent(); ok {
		client.UserAgent = userAgent
	}
	httpClient := authHTTPClient
	if httpClient == nil || httpClient == http.DefaultClient {
		httpClient = client.HTTP
	}
	client.Provider = auth.STSProvider{
		HTTPClient:    httpClient,
		RefreshSkew:   authRefreshSkew,
		TokenEndpoint: oauthTokenEndpoint,
	}
	if cfg != nil {
		client.Endpoint = cfg.OpenAPI.Endpoint
		client.Service = cfg.OpenAPI.Service
		client.Region = cfg.OpenAPI.Region
	}
	client.HTTP = httpClient
	return client
}
