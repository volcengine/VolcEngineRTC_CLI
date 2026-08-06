// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"fmt"
	"html"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
)

type CallbackResult struct {
	Code             string
	State            string
	Error            string
	ErrorDescription string
}

type CallbackServer struct {
	RedirectURI string
	Results     <-chan CallbackResult
	server      *http.Server
}

func StartCallbackServer(rawRedirectURI string) (*CallbackServer, error) {
	redirectURL, err := parseLoopbackRedirect(rawRedirectURI)
	if err != nil {
		return nil, err
	}
	listener, actualRedirectURL, err := listenLoopbackRedirect(redirectURL)
	if err != nil {
		return nil, err
	}
	actualRedirectURI := actualRedirectURL.String()
	expectedPath := actualRedirectURL.EscapedPath()
	if expectedPath == "" {
		expectedPath = "/"
	}

	resultCh := make(chan CallbackResult, 1)
	var once sync.Once
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != expectedPath {
			http.NotFound(w, r)
			return
		}
		query := r.URL.Query()
		result := CallbackResult{
			Code:             strings.TrimSpace(query.Get("code")),
			State:            strings.TrimSpace(query.Get("state")),
			Error:            queryValue(query, "error", "Error"),
			ErrorDescription: queryValue(query, "error_description", "ErrorDescription", "errorDescription"),
		}
		once.Do(func() {
			resultCh <- result
			close(resultCh)
		})
		writeCallbackPage(w, result)
	})

	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			once.Do(func() {
				resultCh <- CallbackResult{Error: err.Error()}
				close(resultCh)
			})
		}
	}()
	return &CallbackServer{RedirectURI: actualRedirectURI, Results: resultCh, server: server}, nil
}

func listenLoopbackRedirect(redirectURL *url.URL) (net.Listener, *url.URL, error) {
	port := redirectURL.Port()
	attempts := 1
	if port == "0" {
		attempts = 20
	}
	for attempt := 0; attempt < attempts; attempt++ {
		listener, err := net.Listen("tcp", net.JoinHostPort(redirectURL.Hostname(), port))
		if err != nil {
			return nil, nil, authErr("vertc.auth.invalid_callback", "listen on callback address: %s", err)
		}
		_, actualPort, err := net.SplitHostPort(listener.Addr().String())
		if err != nil {
			_ = listener.Close()
			return nil, nil, authErr("vertc.auth.invalid_callback", "resolve callback listener address: %s", err)
		}
		actualPortNumber, err := strconv.Atoi(actualPort)
		if err != nil {
			_ = listener.Close()
			return nil, nil, authErr("vertc.auth.invalid_callback", "invalid allocated loopback port: %s", actualPort)
		}
		if actualPortNumber <= 40000 {
			_ = listener.Close()
			if port == "0" {
				continue
			}
			return nil, nil, authErr("vertc.auth.invalid_callback", "loopback redirect_uri port must be greater than 40000")
		}
		actualRedirectURL := *redirectURL
		actualRedirectURL.Host = net.JoinHostPort(redirectURL.Hostname(), actualPort)
		return listener, &actualRedirectURL, nil
	}
	return nil, nil, authErr("vertc.auth.invalid_callback", "allocated loopback port must be greater than 40000")
}

func RedirectURIWithRandomPort(rawRedirectURI string) (string, error) {
	redirectURL, err := parseLoopbackRedirect(rawRedirectURI)
	if err != nil {
		return "", err
	}
	if redirectURL.Port() != "0" {
		return redirectURL.String(), nil
	}
	for attempt := 0; attempt < 20; attempt++ {
		listener, err := net.Listen("tcp", net.JoinHostPort(redirectURL.Hostname(), "0"))
		if err != nil {
			return "", authErr("vertc.auth.invalid_callback", "allocate callback port: %s", err)
		}
		_, actualPort, splitErr := net.SplitHostPort(listener.Addr().String())
		closeErr := listener.Close()
		if splitErr != nil {
			return "", authErr("vertc.auth.invalid_callback", "resolve callback listener address: %s", splitErr)
		}
		if closeErr != nil {
			return "", authErr("vertc.auth.invalid_callback", "close callback listener: %s", closeErr)
		}
		actualPortNumber, err := strconv.Atoi(actualPort)
		if err != nil {
			return "", authErr("vertc.auth.invalid_callback", "invalid allocated loopback port: %s", actualPort)
		}
		if actualPortNumber <= 40000 {
			continue
		}
		redirectURL.Host = net.JoinHostPort(redirectURL.Hostname(), actualPort)
		return redirectURL.String(), nil
	}
	return "", authErr("vertc.auth.invalid_callback", "allocated loopback port must be greater than 40000")
}

func (s *CallbackServer) Shutdown(ctx context.Context) error {
	if s == nil || s.server == nil {
		return nil
	}
	return s.server.Shutdown(ctx)
}

func parseLoopbackRedirect(rawRedirectURI string) (*url.URL, error) {
	redirectURL, err := url.Parse(rawRedirectURI)
	if err != nil {
		return nil, authErr("vertc.auth.invalid_callback", "parse redirect_uri: %s", err)
	}
	if redirectURL.Scheme != "http" {
		return nil, authErr("vertc.auth.invalid_callback", "loopback redirect_uri must use http scheme")
	}
	host := redirectURL.Hostname()
	if host != "127.0.0.1" && host != "localhost" {
		return nil, authErr("vertc.auth.invalid_callback", "loopback redirect_uri host must be 127.0.0.1 or localhost")
	}
	port := redirectURL.Port()
	if port == "" {
		return nil, authErr("vertc.auth.invalid_callback", "loopback redirect_uri must include a port")
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil {
		return nil, authErr("vertc.auth.invalid_callback", "invalid loopback redirect_uri port: %s", port)
	}
	if portNumber != 0 && portNumber <= 40000 {
		return nil, authErr("vertc.auth.invalid_callback", "loopback redirect_uri port must be greater than 40000")
	}
	return redirectURL, nil
}

func queryValue(values url.Values, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(values.Get(key)); value != "" {
			return value
		}
	}
	return ""
}

func writeCallbackPage(w http.ResponseWriter, result CallbackResult) {
	title := "VeRTC CLI Login"
	heading := "Authorization callback received"
	message := "Return to the CLI to finish login."
	if result.Error != "" {
		heading = "Login failed"
		message = result.Error
		if result.ErrorDescription != "" {
			message = fmt.Sprintf("%s: %s", result.Error, result.ErrorDescription)
		}
	} else if result.Code == "" {
		heading = "Login failed"
		message = "Authorization callback did not include a code."
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprintf(w, "<!doctype html><meta charset=\"utf-8\"><title>%s</title><h1>%s</h1><p>%s</p>",
		html.EscapeString(title),
		html.EscapeString(heading),
		html.EscapeString(message),
	)
}

func CallbackAuthorizationError(result CallbackResult) error {
	if result.Error == "" {
		return nil
	}
	if result.ErrorDescription != "" {
		return errs.New("vertc.auth.authorization_failed", errs.TypeAuth,
			"authorization failed: %s: %s", result.Error, result.ErrorDescription)
	}
	return errs.New("vertc.auth.authorization_failed", errs.TypeAuth,
		"authorization failed: %s", result.Error)
}
