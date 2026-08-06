// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestPKCEChallenge(t *testing.T) {
	verifier, challenge, err := NewPKCEPair()
	if err != nil {
		t.Fatal(err)
	}
	if verifier == "" || challenge == "" {
		t.Fatalf("empty verifier/challenge: %q %q", verifier, challenge)
	}
	if got := CodeChallengeS256(verifier); got != challenge {
		t.Fatalf("challenge mismatch: %q != %q", got, challenge)
	}
}

func TestExchangeAndRefreshTokenForms(t *testing.T) {
	var forms []url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.Header.Get("User-Agent"), "vertc/") {
			t.Fatalf("token endpoint received invocation User-Agent %q", r.Header.Get("User-Agent"))
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		forms = append(forms, r.PostForm)
		_ = json.NewEncoder(w).Encode(TokenSet{
			AccessToken:  `{"access_key_id":"AK","secret_access_key":"SK","session_token":"ST"}`,
			RefreshToken: "refresh-new",
			TokenType:    "Bearer",
			ExpiresIn:    3600,
		})
	}))
	defer server.Close()

	token, err := ExchangeCode(context.Background(), server.Client(), server.URL, DefaultLocalClientID, "http://127.0.0.1:50001", "code-1", "verifier-1")
	if err != nil {
		t.Fatal(err)
	}
	if token.ClientID != DefaultLocalClientID || token.Scope != DefaultScope {
		t.Fatalf("unexpected token metadata: %+v", token)
	}
	refreshed, err := RefreshToken(context.Background(), server.Client(), server.URL, TokenSet{
		RefreshToken: "refresh-old",
		ClientID:     DefaultLocalClientID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.ClientID != DefaultLocalClientID {
		t.Fatalf("refresh client id = %q", refreshed.ClientID)
	}
	if len(forms) != 2 {
		t.Fatalf("forms = %d", len(forms))
	}
	if forms[0].Get("grant_type") != "authorization_code" || forms[0].Get("code_verifier") != "verifier-1" {
		t.Fatalf("exchange form = %v", forms[0])
	}
	if forms[0].Get("client_secret") != "" {
		t.Fatalf("public client sent client_secret: %v", forms[0])
	}
	if forms[1].Get("grant_type") != "refresh_token" || forms[1].Get("scope") != DefaultScope {
		t.Fatalf("refresh form = %v", forms[1])
	}
}

func TestCallbackServerHandlesUppercaseError(t *testing.T) {
	server := startLoopbackServerForTest(t)
	resp, err := http.Get(server.RedirectURI + "?Error=" + url.QueryEscape("禁止操作") + "&lang=zh")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	result := waitForCallbackResult(t, server)
	if result.Error != "禁止操作" || result.Code != "" {
		t.Fatalf("result = %+v", result)
	}
}

func TestRedirectURIWithRandomPort(t *testing.T) {
	redirectURI, err := RedirectURIWithRandomPort(DefaultRedirectURI)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(redirectURI)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Port() == "" || parsed.Port() == "0" || parsed.Path != "" {
		t.Fatalf("redirect_uri = %s", redirectURI)
	}
}

func TestParseSTSCredential(t *testing.T) {
	credential, err := ParseSTSCredential(`{"AccessKeyID":"AK","SecretAccessKey":"SK","SessionToken":"ST"}`)
	if err != nil {
		t.Fatal(err)
	}
	if credential.AccessKeyID != "AK" || credential.SecretAccessKey != "SK" || credential.SessionToken != "ST" {
		t.Fatalf("credential = %+v", credential)
	}
	if _, err := ParseSTSCredential(`{"access_key_id":"AK"}`); err == nil {
		t.Fatal("expected missing secret/session error")
	}
}

func TestEnsureFreshTokenRefreshesAndStores(t *testing.T) {
	store := NewMemoryTokenStore()
	restore := SetTokenStoreForTest(store)
	defer restore()
	if err := SaveToken(TokenSet{
		AccessToken:  "old",
		RefreshToken: "refresh-old",
		ClientID:     DefaultLocalClientID,
		ExpiresAt:    time.Now().Add(time.Minute).UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.PostForm.Get("grant_type") != "refresh_token" {
			t.Fatalf("form = %v", r.PostForm)
		}
		_ = json.NewEncoder(w).Encode(TokenSet{AccessToken: "new", TokenType: "Bearer", ExpiresIn: 3600})
	}))
	defer server.Close()

	token, refreshed, err := EnsureFreshToken(context.Background(), server.Client(), 5*time.Minute, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if !refreshed || token.AccessToken != "new" || token.RefreshToken != "refresh-old" {
		t.Fatalf("token=%+v refreshed=%t", token, refreshed)
	}
	stored, err := LoadToken()
	if err != nil {
		t.Fatal(err)
	}
	if stored.AccessToken != "new" {
		t.Fatalf("stored = %+v", stored)
	}
}

func TestSTSProviderRefreshesAndReturnsOpenAPICredential(t *testing.T) {
	store := NewMemoryTokenStore()
	restore := SetTokenStoreForTest(store)
	defer restore()
	if err := SaveToken(TokenSet{
		AccessToken:  "old",
		RefreshToken: "refresh-old",
		ClientID:     DefaultLocalClientID,
		ExpiresAt:    time.Now().Add(time.Minute).UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatal(err)
	}
	stsJSON, _ := json.Marshal(STSCredential{AccessKeyID: "AKID", SecretAccessKey: "SECRET", SessionToken: "SESSION"})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(TokenSet{AccessToken: string(stsJSON), TokenType: TokenTypeAccessTokenSTS, ExpiresIn: 3600})
	}))
	defer server.Close()

	credential, err := STSProvider{
		HTTPClient:    server.Client(),
		RefreshSkew:   5 * time.Minute,
		TokenEndpoint: server.URL,
	}.Credentials(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if credential.AccessKeyID != "AKID" || credential.SecretAccessKey != "SECRET" || credential.SessionToken != "SESSION" {
		t.Fatalf("credential = %+v", credential)
	}
}

func startLoopbackServerForTest(t *testing.T) *CallbackServer {
	t.Helper()
	server, err := StartCallbackServer(DefaultRedirectURI)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	})
	return server
}

func waitForCallbackResult(t *testing.T, server *CallbackServer) CallbackResult {
	t.Helper()
	select {
	case result := <-server.Results:
		return result
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for callback result")
	}
	return CallbackResult{}
}

func TestBuildAuthorizeURLFixedScope(t *testing.T) {
	got, err := BuildAuthorizeURL(AuthorizeParams{
		AuthorizeEndpoint: "https://signin.test/authorize",
		ClientID:          DefaultLocalClientID,
		RedirectURI:       "http://127.0.0.1:50001",
		State:             "state",
		Nonce:             "nonce",
		CodeChallenge:     "challenge",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "scope=Console%3AAll%3AAll") {
		t.Fatalf("authorize URL scope mismatch: %s", got)
	}
}
