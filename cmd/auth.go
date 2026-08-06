// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/affordance"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/auth"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/meta"
)

const authRefreshSkew = 15 * time.Minute

const (
	authBrowserAsk    = "ask"
	authBrowserOpen   = "open"
	authBrowserManual = "manual"
)

var (
	authOpenBrowser   = auth.OpenBrowser
	authHTTPClient    = http.DefaultClient
	authRandomURLSafe = auth.RandomURLSafe
	authIsTerminal    = func(reader io.Reader) bool {
		file, ok := reader.(*os.File)
		return ok && term.IsTerminal(int(file.Fd()))
	}
	oauthAuthorizeEndpoint = auth.DefaultAuthorizeEndpoint
	oauthTokenEndpoint     = auth.DefaultTokenEndpoint
)

type authLoginResult struct {
	Authenticated   bool   `json:"authenticated"`
	TokenLocation   string `json:"token_location"`
	ExpiresAt       string `json:"expires_at,omitempty"`
	HasRefreshToken bool   `json:"has_refresh_token"`
	ClientID        string `json:"client_id"`
	Scope           string `json:"scope"`
}

type authLoginAuthorizationResult struct {
	AuthorizationRequired bool   `json:"authorization_required"`
	AuthorizationURL      string `json:"authorization_url"`
	ExpiresAt             string `json:"expires_at"`
}

type authStatusResult struct {
	Authenticated   bool   `json:"authenticated"`
	TokenLocation   string `json:"token_location"`
	ExpiresAt       string `json:"expires_at,omitempty"`
	Expired         bool   `json:"expired"`
	Refreshed       bool   `json:"refreshed"`
	HasAccessToken  bool   `json:"has_access_token"`
	HasRefreshToken bool   `json:"has_refresh_token"`
	TokenType       string `json:"token_type,omitempty"`
	ClientID        string `json:"client_id,omitempty"`
	Scope           string `json:"scope,omitempty"`
	ObtainedAt      string `json:"obtained_at,omitempty"`
}

func newAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Authenticate and inspect VolcEngine Signin status",
	}
	cmd.AddCommand(newAuthLoginCmd(), newAuthRefreshCmd(), newAuthStatusCmd(), newAuthLogoutCmd())
	affordance.Attach(cmd, affordance.Affordance{
		When:     []string{"Before Console/OpenAPI-backed commands which need a Signin token"},
		Examples: []string{meta.BinName + " auth login", meta.BinName + " auth status"},
	})
	return cmd
}

func newAuthLoginCmd() *cobra.Command {
	var (
		browserMode     string
		credentialStore string
		force           bool
		start           bool
		resume          bool
		timeout         time.Duration
	)
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Log in with VolcEngine Signin using Authorization Code + PKCE",
		RunE: func(c *cobra.Command, args []string) error {
			if err := validateAuthBrowserMode(browserMode); err != nil {
				return err
			}
			if err := validateAuthLoginPhases(c, browserMode, credentialStore, force, start, resume, timeout); err != nil {
				return err
			}
			if resume {
				return resumeAuthLogin(c)
			}
			if credentialStore != "" {
				restoreStore, err := auth.SetTokenStoreModeForCommand(credentialStore)
				if err != nil {
					return err
				}
				defer restoreStore()
			}
			if !force {
				token, reusable, refreshed, err := reusableAuthLogin(c.Context())
				if err != nil {
					return err
				}
				if reusable {
					if credentialStore != "" {
						if err := auth.PersistStoreMode(credentialStore); err != nil {
							return err
						}
					}
					if refreshed {
						out().Progress("refreshed existing Signin login in %s", auth.TokenLocation())
					} else {
						out().Progress("already logged in; reusing token in %s", auth.TokenLocation())
					}
					return out().Data(newAuthLoginResult(token))
				}
			}
			effectiveBrowserMode, err := resolveAuthBrowserMode(c, browserMode)
			if err != nil {
				return err
			}
			manual := effectiveBrowserMode == authBrowserManual

			clientID := auth.DefaultLocalClientID
			if manual {
				clientID = auth.DefaultRemoteClientID
			}
			verifier, challenge, err := auth.NewPKCEPair()
			if err != nil {
				return errs.Wrap(err, "vertc.cli.internal", "generate PKCE pair: %s", err)
			}
			state, err := authRandomURLSafe(32)
			if err != nil {
				return errs.Wrap(err, "vertc.cli.internal", "generate OAuth state: %s", err)
			}
			nonce, err := authRandomURLSafe(16)
			if err != nil {
				return errs.Wrap(err, "vertc.cli.internal", "generate OAuth nonce: %s", err)
			}

			var callbackServer *auth.CallbackServer
			var redirectURI string
			if manual {
				redirectURI = auth.DefaultRemoteRedirectURI
			} else {
				callbackServer, err = auth.StartCallbackServer(auth.DefaultRedirectURI)
				if err != nil {
					return err
				}
				redirectURI = callbackServer.RedirectURI
				defer func() {
					shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
					defer cancel()
					_ = callbackServer.Shutdown(shutdownCtx)
				}()
			}

			loginURL, err := auth.BuildAuthorizeURL(auth.AuthorizeParams{
				AuthorizeEndpoint: oauthAuthorizeEndpoint,
				ClientID:          clientID,
				RedirectURI:       redirectURI,
				State:             state,
				Nonce:             nonce,
				CodeChallenge:     challenge,
			})
			if err != nil {
				return err
			}

			authorizationCode := ""
			if manual {
				if start {
					now := time.Now()
					pending, err := auth.NewPendingAuthorization(state, verifier, clientID, redirectURI, credentialStore, now, timeout)
					if err != nil {
						return err
					}
					if err := auth.SavePendingAuthorization(pending); err != nil {
						return err
					}
					out().Progress("manual authorization started; run `%s auth login --resume` with the returned code before %s", meta.BinName, pending.ExpiresAt)
					return out().Data(authLoginAuthorizationResult{
						AuthorizationRequired: true,
						AuthorizationURL:      loginURL,
						ExpiresAt:             pending.ExpiresAt,
					})
				}
				out().Progress("open this URL in your browser:\n%s", loginURL)
				authorizationCode, err = readAuthorizationCode(c, state)
				if err != nil {
					return err
				}
			} else {
				if err := authOpenBrowser(loginURL); err != nil {
					out().Warn("unable to open browser automatically: %s", err)
					out().Progress("open this URL manually:\n%s", loginURL)
				} else {
					out().Progress("browser opened for VolcEngine Signin login")
				}
				waitCtx, cancel := context.WithTimeout(c.Context(), timeout)
				defer cancel()
				var callback auth.CallbackResult
				select {
				case callback = <-callbackServer.Results:
				case <-waitCtx.Done():
					return errs.New("vertc.auth.authorization_failed", errs.TypeAuth,
						"login timed out waiting for callback on %s", callbackServer.RedirectURI)
				}
				if err := auth.CallbackAuthorizationError(callback); err != nil {
					return err
				}
				if callback.Code == "" {
					return errs.New("vertc.auth.authorization_failed", errs.TypeAuth,
						"authorization callback did not include code")
				}
				if callback.State != state {
					return errs.New("vertc.auth.authorization_failed", errs.TypeAuth,
						"authorization state mismatch")
				}
				authorizationCode = callback.Code
			}

			exchangeCtx, cancel := context.WithTimeout(c.Context(), 30*time.Second)
			defer cancel()
			token, err := auth.ExchangeCode(exchangeCtx, authHTTPClient, oauthTokenEndpoint, clientID, redirectURI, authorizationCode, verifier)
			if err != nil {
				return err
			}
			if err := auth.SaveToken(token); err != nil {
				return err
			}
			out().Progress("logged in; token stored in %s", auth.TokenLocation())

			return out().Data(newAuthLoginResult(token))
		},
	}
	cmd.Flags().StringVar(&browserMode, "browser", authBrowserAsk, "authorization mode: ask, open, or manual")
	cmd.Flags().StringVar(&credentialStore, "store", "", "credential store: file or keyring (default: persisted choice, initially file)")
	cmd.Flags().BoolVar(&force, "force", false, "reauthorize even when a usable Signin token is stored")
	cmd.Flags().BoolVar(&start, "start", false, "start a resumable manual authorization and exit")
	cmd.Flags().BoolVar(&resume, "resume", false, "finish the pending manual authorization using a code from stdin")
	cmd.Flags().DurationVar(&timeout, "timeout", 5*time.Minute, "time to wait for or resume browser authorization")
	affordance.Attach(cmd, affordance.Affordance{
		When:     []string{"Establishing Signin credentials for Console/OpenAPI-backed commands"},
		Avoid:    []string{"Offline-only flows such as token issue/check"},
		Prereq:   []string{"An interactive terminal, or an explicit --browser=open choice / --browser=manual --start then --resume sequence in agent/headless environments"},
		Examples: []string{meta.BinName + " auth login", meta.BinName + " auth login --browser=manual --start", meta.BinName + " auth login --resume", meta.BinName + " auth login --store=keyring --browser=open"},
	})
	return cmd
}

func validateAuthLoginPhases(cmd *cobra.Command, browserMode, credentialStore string, force, start, resume bool, timeout time.Duration) error {
	if timeout <= 0 {
		return errs.New("vertc.cli.invalid_flag", errs.TypeValidation,
			"authorization timeout must be greater than zero").WithParam("--timeout")
	}
	if start && resume {
		return errs.New("vertc.cli.invalid_flag", errs.TypeValidation,
			"--start and --resume cannot be used together").WithParam("--resume")
	}
	if start && browserMode != authBrowserManual {
		return errs.New("vertc.cli.invalid_flag", errs.TypeValidation,
			"--start requires --browser=manual").WithParam("--start")
	}
	if !resume {
		return nil
	}
	if cmd.Flags().Changed("browser") || credentialStore != "" || force || start || cmd.Flags().Changed("timeout") {
		return errs.New("vertc.cli.invalid_flag", errs.TypeValidation,
			"--resume uses the browser, store, force, and timeout settings saved by --start").WithParam("--resume").
			WithHint("pipe the returned authorization code to `%s auth login --resume` without other login flags", meta.BinName)
	}
	return nil
}

func resumeAuthLogin(cmd *cobra.Command) error {
	now := time.Now()
	pending, err := auth.LoadPendingAuthorization(now)
	if err != nil {
		return err
	}
	authorizationCode, err := readAuthorizationCode(cmd, pending.State)
	if err != nil {
		return err
	}
	claimed, cleanup, err := auth.ClaimPendingAuthorization(pending.State, now)
	if err != nil {
		return err
	}
	defer cleanup()

	if claimed.CredentialStore != "" {
		restoreStore, err := auth.SetTokenStoreModeForCommand(claimed.CredentialStore)
		if err != nil {
			return err
		}
		defer restoreStore()
	}
	exchangeCtx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
	defer cancel()
	token, err := auth.ExchangeCode(exchangeCtx, authHTTPClient, oauthTokenEndpoint, claimed.ClientID, claimed.RedirectURI, authorizationCode, claimed.CodeVerifier)
	if err != nil {
		return err
	}
	if err := auth.SaveToken(token); err != nil {
		return err
	}
	out().Progress("logged in; token stored in %s", auth.TokenLocation())
	return out().Data(newAuthLoginResult(token))
}

func validateAuthBrowserMode(mode string) error {
	switch mode {
	case authBrowserAsk, authBrowserOpen, authBrowserManual:
		return nil
	default:
		return errs.New("vertc.cli.invalid_flag", errs.TypeValidation,
			"invalid browser mode %q", mode).WithParam("--browser").
			WithHint("use --browser=ask, --browser=open, or --browser=manual")
	}
}

func resolveAuthBrowserMode(cmd *cobra.Command, mode string) (string, error) {
	if mode != authBrowserAsk {
		return mode, nil
	}
	if !authIsTerminal(cmd.InOrStdin()) {
		return "", errs.New("vertc.auth.interaction_required", errs.TypePrecondition,
			"Signin authorization requires an explicit browser mode in a non-interactive terminal").
			WithParam("--browser").
			WithHint("rerun with `%s auth login --browser=open` after user consent, or use `%s auth login --browser=manual --start` then `%s auth login --resume`", meta.BinName, meta.BinName, meta.BinName)
	}
	out().Progress("Signin requires authorization and may store refreshable credentials in %s", auth.TokenLocation())
	out().Progress("choose login mode: [o] open browser, [m] manual link/code, [c] cancel")
	line, err := readInputLine(cmd.InOrStdin())
	if err != nil {
		return "", errs.Wrap(err, "vertc.cli.internal", "read login choice: %s", err)
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "o", "open":
		return authBrowserOpen, nil
	case "m", "manual":
		return authBrowserManual, nil
	case "c", "cancel", "":
		return "", errs.New("vertc.auth.authorization_failed", errs.TypeAuth, "Signin authorization canceled")
	default:
		return "", errs.New("vertc.auth.authorization_failed", errs.TypeAuth,
			"unknown login choice %q", strings.TrimSpace(line)).
			WithHint("choose o, m, or c")
	}
}

func reusableAuthLogin(ctx context.Context) (auth.TokenSet, bool, bool, error) {
	stored, err := auth.LoadToken()
	if errors.Is(err, os.ErrNotExist) {
		return auth.TokenSet{}, false, false, nil
	}
	if err != nil {
		if typed, ok := errs.As(err); ok && typed.Code == "vertc.auth.storage_unavailable" {
			// Do not reuse credentials from unavailable or unsafe storage, but let
			// login continue so SaveToken can repair a recoverable auth file.
			return auth.TokenSet{}, false, false, nil
		}
		return auth.TokenSet{}, false, false, err
	}
	if strings.TrimSpace(stored.AccessToken) == "" {
		return auth.TokenSet{}, false, false, nil
	}
	if !stored.Expired(time.Now(), authRefreshSkew) {
		return stored, true, false, nil
	}

	refreshCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	refreshed, didRefresh, err := auth.EnsureFreshToken(refreshCtx, authHTTPClient, authRefreshSkew, oauthTokenEndpoint)
	if err == nil {
		return refreshed, true, didRefresh, nil
	}
	if typed, ok := errs.As(err); ok && typed.Code == "vertc.auth.storage_unavailable" {
		return auth.TokenSet{}, false, false, err
	}
	return auth.TokenSet{}, false, false, nil
}

func newAuthLoginResult(token auth.TokenSet) authLoginResult {
	return authLoginResult{
		Authenticated:   true,
		TokenLocation:   auth.TokenLocation(),
		ExpiresAt:       token.ExpiresAt,
		HasRefreshToken: token.RefreshToken != "",
		ClientID:        token.ClientID,
		Scope:           token.Scope,
	}
}

func newAuthRefreshCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "refresh",
		Short: "Refresh the stored Signin token",
		RunE: func(c *cobra.Command, args []string) error {
			stored, err := auth.LoadToken()
			if errors.Is(err, os.ErrNotExist) {
				return errs.New("vertc.auth.not_authenticated", errs.TypeAuth,
					"not logged in").WithHint("run `%s auth login` first", meta.BinName)
			}
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(c.Context(), 30*time.Second)
			defer cancel()
			refreshed, err := auth.RefreshToken(ctx, authHTTPClient, oauthTokenEndpoint, stored)
			if err != nil {
				return err
			}
			if err := auth.SaveToken(refreshed); err != nil {
				return err
			}
			out().Progress("refreshed token in %s", auth.TokenLocation())
			return out().Data(authLoginResult{
				Authenticated:   true,
				TokenLocation:   auth.TokenLocation(),
				ExpiresAt:       refreshed.ExpiresAt,
				HasRefreshToken: refreshed.RefreshToken != "",
				ClientID:        refreshed.ClientID,
				Scope:           refreshed.Scope,
			})
		},
	}
}

func newAuthStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show current Signin authentication status",
		RunE: func(c *cobra.Command, args []string) error {
			_, err := auth.LoadToken()
			if errors.Is(err, os.ErrNotExist) {
				return out().Data(authStatusResult{Authenticated: false, TokenLocation: auth.TokenLocation()})
			}
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(c.Context(), 30*time.Second)
			defer cancel()
			token, refreshed, err := auth.EnsureFreshToken(ctx, authHTTPClient, authRefreshSkew, oauthTokenEndpoint)
			if err != nil {
				return err
			}
			return out().Data(authStatusResult{
				Authenticated:   true,
				TokenLocation:   auth.TokenLocation(),
				ExpiresAt:       token.ExpiresAt,
				Expired:         token.Expired(time.Now(), 0),
				Refreshed:       refreshed,
				HasAccessToken:  token.AccessToken != "",
				HasRefreshToken: token.RefreshToken != "",
				TokenType:       token.TokenType,
				ClientID:        token.ClientID,
				Scope:           token.Scope,
				ObtainedAt:      token.ObtainedAt,
			})
		},
	}
}

func newAuthLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Clear stored Signin tokens",
		RunE: func(c *cobra.Command, args []string) error {
			if err := auth.DeleteToken(); err != nil {
				return err
			}
			return out().Data(map[string]any{"authenticated": false, "token_location": auth.TokenLocation()})
		},
	}
}

func readAuthorizationCode(cmd *cobra.Command, expectedState string) (string, error) {
	out().Progress("paste authorization code:")
	line, err := readInputLine(cmd.InOrStdin())
	if err != nil && !errors.Is(err, io.EOF) {
		return "", errs.Wrap(err, "vertc.cli.internal", "read authorization code: %s", err)
	}
	code, state, err := parseAuthorizationCodeInput(line)
	if err != nil {
		return "", err
	}
	if expectedState != "" && state != expectedState {
		return "", errs.New("vertc.auth.authorization_failed", errs.TypeAuth,
			"authorization state mismatch")
	}
	return code, nil
}

func readInputLine(reader io.Reader) (string, error) {
	var value strings.Builder
	var one [1]byte
	for {
		n, err := reader.Read(one[:])
		if n == 1 {
			if one[0] == '\n' {
				return value.String(), nil
			}
			if one[0] != '\r' {
				_ = value.WriteByte(one[0])
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) && value.Len() > 0 {
				return value.String(), nil
			}
			return value.String(), err
		}
		if n == 0 {
			return value.String(), fmt.Errorf("reader returned no data")
		}
	}
}

func parseAuthorizationCodeInput(input string) (string, string, error) {
	value := strings.TrimSpace(input)
	if value == "" {
		return "", "", errs.New("vertc.auth.authorization_failed", errs.TypeAuth,
			"authorization code is required")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return "", "", errs.Wrap(err, "vertc.auth.authorization_failed",
			"decode authorization code: %s", err)
	}
	values, err := url.ParseQuery(string(decoded))
	if err != nil {
		return "", "", errs.Wrap(err, "vertc.auth.authorization_failed",
			"parse authorization code payload: %s", err)
	}
	if authErr := authorizationErrorFromValues(values); authErr != nil {
		return "", "", authErr
	}
	code := strings.TrimSpace(values.Get("code"))
	state := strings.TrimSpace(values.Get("state"))
	if code == "" {
		return "", "", errs.New("vertc.auth.authorization_failed", errs.TypeAuth,
			"authorization code payload did not include code")
	}
	if state == "" {
		return "", "", errs.New("vertc.auth.authorization_failed", errs.TypeAuth,
			"authorization code payload did not include state")
	}
	return code, state, nil
}

func authorizationErrorFromValues(values url.Values) error {
	authError := queryValue(values, "error", "Error")
	if authError == "" {
		return nil
	}
	description := queryValue(values, "error_description", "ErrorDescription", "errorDescription")
	if description != "" {
		return errs.New("vertc.auth.authorization_failed", errs.TypeAuth,
			"authorization failed: %s: %s", authError, description)
	}
	return errs.New("vertc.auth.authorization_failed", errs.TypeAuth,
		"authorization failed: %s", authError)
}

func queryValue(values url.Values, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(values.Get(key)); value != "" {
			return value
		}
	}
	return ""
}
