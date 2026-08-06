// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

// Package auth implements the VolcEngine Signin public-client OAuth flow.
package auth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/meta"
)

const (
	DefaultAuthorizeEndpoint = "https://signin.volcengine.com/authorize/oauth/authorize"
	DefaultTokenEndpoint     = "https://signin.volcengine.com/authorize/oauth/token"
	DefaultRedirectURI       = "http://127.0.0.1:0"
	DefaultRemoteRedirectURI = DefaultAuthorizeEndpoint
	DefaultScope             = "Console:All:All"
	DefaultLocalClientID     = "trn:signin:::devtools/same-device"
	DefaultRemoteClientID    = "trn:signin:::devtools/cross-device"
	TokenTypeAccessTokenSTS  = "urn:ietf:params:oauth:token-type:access_token_sts"
)

// AuthorizeParams is the fixed OAuth authorize request surface used by login.
type AuthorizeParams struct {
	AuthorizeEndpoint string
	ClientID          string
	RedirectURI       string
	State             string
	Nonce             string
	CodeChallenge     string
}

// TokenSet is the non-secret metadata plus secret tokens stored by the selected
// credential store. It must never be serialized to project config or output.
type TokenSet struct {
	AccessToken     string `json:"access_token,omitempty"`
	RefreshToken    string `json:"refresh_token,omitempty"`
	IDToken         string `json:"id_token,omitempty"`
	TokenType       string `json:"token_type,omitempty"`
	IssuedTokenType string `json:"issued_token_type,omitempty"`
	ExpiresIn       int64  `json:"expires_in,omitempty"`
	ExpiresAt       string `json:"expires_at,omitempty"`
	Scope           string `json:"scope,omitempty"`
	ObtainedAt      string `json:"obtained_at,omitempty"`
	ClientID        string `json:"client_id,omitempty"`
}

type tokenError struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// BuildAuthorizeURL builds the Signin authorize URL. The scope is intentionally
// fixed by the CLI and not user-configurable.
func BuildAuthorizeURL(params AuthorizeParams) (string, error) {
	endpoint, err := url.Parse(params.AuthorizeEndpoint)
	if err != nil {
		return "", authErr("vertc.auth.invalid_callback", "invalid authorize endpoint: %s", err)
	}
	values := endpoint.Query()
	values.Set("response_type", "code")
	values.Set("client_id", params.ClientID)
	values.Set("redirect_uri", params.RedirectURI)
	values.Set("scope", DefaultScope)
	values.Set("state", params.State)
	values.Set("nonce", params.Nonce)
	values.Set("code_challenge", params.CodeChallenge)
	values.Set("code_challenge_method", CodeChallengeMethodS256)
	endpoint.RawQuery = values.Encode()
	return endpoint.String(), nil
}

// ExchangeCode exchanges an authorization code for Console Credential tokens.
func ExchangeCode(ctx context.Context, httpClient *http.Client, tokenEndpoint, clientID, redirectURI, code, codeVerifier string) (TokenSet, error) {
	values := url.Values{}
	values.Set("grant_type", "authorization_code")
	values.Set("client_id", clientID)
	values.Set("code", code)
	values.Set("redirect_uri", redirectURI)
	values.Set("code_verifier", codeVerifier)
	token, err := postTokenForm(ctx, httpClient, tokenEndpoint, values)
	if err != nil {
		return TokenSet{}, err
	}
	token.ClientID = clientID
	if token.Scope == "" {
		token.Scope = DefaultScope
	}
	return token, nil
}

// RefreshToken refreshes an existing Console Credential token.
func RefreshToken(ctx context.Context, httpClient *http.Client, tokenEndpoint string, stored TokenSet) (TokenSet, error) {
	if strings.TrimSpace(stored.RefreshToken) == "" {
		return TokenSet{}, errs.New("vertc.auth.not_authenticated", errs.TypeAuth,
			"stored token has no refresh_token").
			WithHint("run `%s auth login` again", meta.BinName)
	}
	clientID := strings.TrimSpace(stored.ClientID)
	if clientID == "" {
		return TokenSet{}, errs.New("vertc.auth.not_authenticated", errs.TypeAuth,
			"stored token has no client_id for refresh").
			WithHint("run `%s auth login` again", meta.BinName)
	}
	values := url.Values{}
	values.Set("grant_type", "refresh_token")
	values.Set("client_id", clientID)
	values.Set("refresh_token", stored.RefreshToken)
	values.Set("scope", DefaultScope)
	token, err := postTokenForm(ctx, httpClient, tokenEndpoint, values)
	if err != nil {
		return TokenSet{}, err
	}
	token.ClientID = clientID
	if token.RefreshToken == "" {
		token.RefreshToken = stored.RefreshToken
	}
	if token.IDToken == "" {
		token.IDToken = stored.IDToken
	}
	if token.Scope == "" {
		token.Scope = DefaultScope
	}
	return token, nil
}

// NormalizeTokenSet fills metadata derived from the token response.
func NormalizeTokenSet(token TokenSet, now time.Time) TokenSet {
	if token.TokenType == "" && token.AccessToken != "" {
		token.TokenType = "Bearer"
	}
	if token.Scope == "" {
		token.Scope = DefaultScope
	}
	token.ObtainedAt = now.UTC().Format(time.RFC3339)
	if token.ExpiresIn > 0 {
		token.ExpiresAt = now.Add(time.Duration(token.ExpiresIn) * time.Second).UTC().Format(time.RFC3339)
	}
	return token
}

// Expired reports whether the token is expired or inside the given refresh skew.
func (t TokenSet) Expired(now time.Time, skew time.Duration) bool {
	if strings.TrimSpace(t.ExpiresAt) == "" {
		return false
	}
	expiresAt, err := time.Parse(time.RFC3339, t.ExpiresAt)
	if err != nil {
		return true
	}
	return !now.Add(skew).Before(expiresAt)
}

func postTokenForm(ctx context.Context, httpClient *http.Client, tokenEndpoint string, values url.Values) (TokenSet, error) {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return TokenSet{}, authErr("vertc.auth.token_exchange_failed", "create token request: %s", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return TokenSet{}, authErr("vertc.auth.token_exchange_failed", "token endpoint request failed: %s", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return TokenSet{}, parseTokenError(resp)
	}
	var token TokenSet
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return TokenSet{}, authErr("vertc.auth.token_exchange_failed", "parse token response: %s", err)
	}
	return NormalizeTokenSet(token, time.Now()), nil
}

func parseTokenError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var apiErr tokenError
	if err := json.Unmarshal(body, &apiErr); err == nil && apiErr.Error != "" {
		if apiErr.ErrorDescription != "" {
			return errs.New("vertc.auth.token_exchange_failed", errs.TypeAuth,
				"token endpoint returned %s: %s", apiErr.Error, apiErr.ErrorDescription)
		}
		return errs.New("vertc.auth.token_exchange_failed", errs.TypeAuth,
			"token endpoint returned %s", apiErr.Error)
	}
	text := strings.TrimSpace(string(body))
	if text == "" {
		text = http.StatusText(resp.StatusCode)
	}
	return errs.New("vertc.auth.token_exchange_failed", errs.TypeAuth,
		"token endpoint returned HTTP %s: %s", strconv.Itoa(resp.StatusCode), text)
}

func authErr(code string, format string, args ...any) *errs.Error {
	return errs.New(code, errs.TypeAuth, format, args...)
}
