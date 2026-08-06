// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/auth"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/config"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/telemetry"
)

func TestAuthLoginExposesOnlySupportedFlags(t *testing.T) {
	cmd := newAuthLoginCmd()
	for _, name := range []string{"timeout", "browser", "store", "force", "start", "resume"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Fatalf("missing login flag %q", name)
		}
	}
	if cmd.Flags().Lookup("no-browser") != nil {
		t.Fatal("login should not expose removed --no-browser flag")
	}
	for _, name := range []string{"client-id", "redirect-uri", "scope", "auth-url", "token-url", "port", "no-provision", "app-id", "bot-id"} {
		if cmd.Flags().Lookup(name) != nil {
			t.Fatalf("login should not expose %q", name)
		}
	}
}

func TestAuthLoginAskRequiresInteractionOutsideTTY(t *testing.T) {
	restoreStore := auth.SetTokenStoreForTest(auth.NewMemoryTokenStore())
	defer restoreStore()
	restoreGlobals := overrideAuthGlobals(t)
	defer restoreGlobals()
	authIsTerminal = func(io.Reader) bool { return false }
	authOpenBrowser = func(string) error {
		t.Fatal("browser must not open without explicit consent")
		return nil
	}
	authRandomURLSafe = func(int) (string, error) {
		t.Fatal("PKCE state must not be generated before consent")
		return "", nil
	}

	cmd := NewRootCmd()
	cmd.SetIn(strings.NewReader(""))
	cmd.SetArgs([]string{"auth", "login"})
	err := cmd.Execute()
	typed, ok := errs.As(err)
	if !ok || typed.Code != "vertc.auth.interaction_required" || typed.Param != "--browser" {
		t.Fatalf("err = %v", err)
	}
}

func TestAuthLoginAskCanBeCanceledBeforeSideEffects(t *testing.T) {
	restoreStore := auth.SetTokenStoreForTest(auth.NewMemoryTokenStore())
	defer restoreStore()
	restoreGlobals := overrideAuthGlobals(t)
	defer restoreGlobals()
	authIsTerminal = func(io.Reader) bool { return true }
	authOpenBrowser = func(string) error {
		t.Fatal("browser must not open after cancellation")
		return nil
	}
	authRandomURLSafe = func(int) (string, error) {
		t.Fatal("PKCE state must not be generated after cancellation")
		return "", nil
	}

	cmd := NewRootCmd()
	cmd.SetIn(strings.NewReader("c\n"))
	cmd.SetArgs([]string{"auth", "login"})
	err := cmd.Execute()
	typed, ok := errs.As(err)
	if !ok || typed.Code != "vertc.auth.authorization_failed" {
		t.Fatalf("err = %v", err)
	}
}

func TestAuthLoginAskResolvesInteractiveChoices(t *testing.T) {
	restoreGlobals := overrideAuthGlobals(t)
	defer restoreGlobals()
	authIsTerminal = func(io.Reader) bool { return true }
	for _, tc := range []struct {
		input string
		want  string
	}{
		{input: "o\n", want: authBrowserOpen},
		{input: "manual\n", want: authBrowserManual},
	} {
		cmd := NewRootCmd()
		cmd.SetIn(strings.NewReader(tc.input))
		got, err := resolveAuthBrowserMode(cmd, authBrowserAsk)
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Fatalf("input %q: mode = %q", tc.input, got)
		}
	}
}

func TestAuthLoginAskUsesConsistentAuthorizationErrorType(t *testing.T) {
	restoreGlobals := overrideAuthGlobals(t)
	defer restoreGlobals()
	authIsTerminal = func(io.Reader) bool { return true }

	cmd := NewRootCmd()
	cmd.SetIn(strings.NewReader("surprise\n"))
	_, err := resolveAuthBrowserMode(cmd, authBrowserAsk)
	typed, ok := errs.As(err)
	if !ok || typed.Code != "vertc.auth.authorization_failed" || typed.Type != errs.TypeAuth {
		t.Fatalf("err = %v", err)
	}
}

func TestAuthLoginRejectsInvalidBrowserAndStore(t *testing.T) {
	for _, tc := range []struct {
		args  []string
		param string
	}{
		{args: []string{"auth", "login", "--browser=surprise"}, param: "--browser"},
		{args: []string{"auth", "login", "--store=unknown"}, param: "--store"},
	} {
		cmd := NewRootCmd()
		cmd.SetArgs(tc.args)
		err := cmd.Execute()
		typed, ok := errs.As(err)
		if !ok || typed.Code != "vertc.cli.invalid_flag" || typed.Param != tc.param {
			t.Fatalf("args %v: err = %v", tc.args, err)
		}
	}
}

func TestAuthLoginRejectsInvalidPhaseFlagCombinations(t *testing.T) {
	for _, args := range [][]string{
		{"auth", "login", "--start"},
		{"auth", "login", "--browser=manual", "--start", "--resume"},
		{"auth", "login", "--resume", "--browser=manual"},
		{"auth", "login", "--resume", "--store=file"},
		{"auth", "login", "--resume", "--force"},
		{"auth", "login", "--resume", "--timeout=1m"},
		{"auth", "login", "--timeout=0"},
	} {
		cmd := NewRootCmd()
		cmd.SetArgs(args)
		err := cmd.Execute()
		typed, ok := errs.As(err)
		if !ok || typed.Code != "vertc.cli.invalid_flag" {
			t.Fatalf("args %v: err = %v", args, err)
		}
	}
}

func TestAuthLoginReusesHealthyStoredToken(t *testing.T) {
	store := auth.NewMemoryTokenStore()
	restoreStore := auth.SetTokenStoreForTest(store)
	defer restoreStore()
	restoreGlobals := overrideAuthGlobals(t)
	defer restoreGlobals()

	want := auth.TokenSet{
		AccessToken:  "access-existing",
		RefreshToken: "refresh-old",
		ClientID:     auth.DefaultLocalClientID,
		Scope:        auth.DefaultScope,
		ExpiresAt:    time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
	}
	if err := auth.SaveToken(want); err != nil {
		t.Fatal(err)
	}
	authOpenBrowser = func(string) error {
		t.Fatal("browser should not open for a healthy stored token")
		return nil
	}
	authHTTPClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("healthy stored token should not make an OAuth request")
		return nil, nil
	})}

	cmd := NewRootCmd()
	cmd.SetArgs([]string{"auth", "login"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	got, err := auth.LoadToken()
	if err != nil {
		t.Fatal(err)
	}
	if got.AccessToken != want.AccessToken || got.RefreshToken != want.RefreshToken {
		t.Fatalf("stored token changed: %+v", got)
	}
}

func TestReusableAuthLoginContinuesAfterStorageUnavailable(t *testing.T) {
	restoreStore := auth.SetTokenStoreForTest(loadErrorTokenStore{
		err: errs.New("vertc.auth.storage_unavailable", errs.TypeAuth, "unsafe auth file"),
	})
	defer restoreStore()

	token, reusable, refreshed, err := reusableAuthLogin(context.Background())
	if err != nil {
		t.Fatalf("reusableAuthLogin: %v", err)
	}
	if token.AccessToken != "" || reusable || refreshed {
		t.Fatalf("result = token=%+v reusable=%v refreshed=%v", token, reusable, refreshed)
	}
}

func TestAuthLoginRefreshesNearExpiryWithoutBrowser(t *testing.T) {
	store := auth.NewMemoryTokenStore()
	restoreStore := auth.SetTokenStoreForTest(store)
	defer restoreStore()
	restoreGlobals := overrideAuthGlobals(t)
	defer restoreGlobals()

	if err := auth.SaveToken(auth.TokenSet{
		AccessToken:  "access-old",
		RefreshToken: "refresh-old",
		ClientID:     auth.DefaultLocalClientID,
		ExpiresAt:    time.Now().Add(time.Minute).UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatal(err)
	}
	authOpenBrowser = func(string) error {
		t.Fatal("browser should not open after a successful refresh")
		return nil
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if got := r.PostForm.Get("grant_type"); got != "refresh_token" {
			t.Fatalf("grant_type = %q", got)
		}
		_ = json.NewEncoder(w).Encode(auth.TokenSet{AccessToken: "access-new", TokenType: "Bearer", ExpiresIn: 3600})
	}))
	defer server.Close()
	oauthTokenEndpoint = server.URL

	cmd := NewRootCmd()
	cmd.SetArgs([]string{"auth", "login"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	got, err := auth.LoadToken()
	if err != nil {
		t.Fatal(err)
	}
	if got.AccessToken != "access-new" || got.RefreshToken != "refresh-old" {
		t.Fatalf("stored token = %+v", got)
	}
}

func TestAuthLoginFallsBackWhenStoredTokenCannotRefresh(t *testing.T) {
	store := auth.NewMemoryTokenStore()
	restoreStore := auth.SetTokenStoreForTest(store)
	defer restoreStore()
	restoreGlobals := overrideAuthGlobals(t)
	defer restoreGlobals()
	authRandomURLSafe = func(length int) (string, error) {
		if length == 32 {
			return "state-fallback", nil
		}
		return "nonce-fallback", nil
	}
	if err := auth.SaveToken(auth.TokenSet{
		AccessToken:  "access-expired",
		RefreshToken: "refresh-invalid",
		ClientID:     auth.DefaultRemoteClientID,
		ExpiresAt:    time.Now().Add(-time.Minute).UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatal(err)
	}
	var grants []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		grant := r.PostForm.Get("grant_type")
		grants = append(grants, grant)
		if grant == "refresh_token" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(auth.TokenSet{AccessToken: "access-reauthorized", RefreshToken: "refresh-new", ExpiresIn: 3600})
	}))
	defer server.Close()
	oauthTokenEndpoint = server.URL
	oauthAuthorizeEndpoint = "https://signin.test/authorize"

	code := base64.RawURLEncoding.EncodeToString([]byte("code=code-fallback&state=state-fallback"))
	cmd := NewRootCmd()
	cmd.SetIn(strings.NewReader(code + "\n"))
	cmd.SetArgs([]string{"auth", "login", "--browser=manual"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(grants, ","); got != "refresh_token,authorization_code" {
		t.Fatalf("grant sequence = %q", got)
	}
}

func TestInitAndDevExposeUnifiedSetupSurface(t *testing.T) {
	initCmd := newInitCmd()
	for _, name := range []string{"provision", "no-provision", "app-id", "bot-id"} {
		if initCmd.Flags().Lookup(name) != nil {
			t.Fatalf("init should not expose %q", name)
		}
	}
	dev := newDevCmd()
	for _, name := range []string{"reconfigure", "app-id", "bot-id"} {
		if dev.Flags().Lookup(name) == nil {
			t.Fatalf("dev should expose --%s", name)
		}
	}
	if dev.Flags().Lookup("list-resources") != nil {
		t.Fatal("dev should not expose --list-resources")
	}
}

func TestAgentFacingCommandHelpDoesNotRecommendAppKeyArgument(t *testing.T) {
	for _, cmd := range []*cobra.Command{newDevCmd(), newEnvCmd(), newInitCmd()} {
		var output bytes.Buffer
		cmd.SetOut(&output)
		cmd.SetErr(&output)
		if err := cmd.Help(); err != nil {
			t.Fatalf("%s help: %v", cmd.Name(), err)
		}
		if strings.Contains(output.String(), "--app-key") {
			t.Fatalf("%s help recommends an AppKey argument:\n%s", cmd.Name(), output.String())
		}
	}
}

func TestAuthRefreshDoesNotExposeScope(t *testing.T) {
	cmd := newAuthRefreshCmd()
	if cmd.Flags().Lookup("scope") != nil {
		t.Fatal("refresh should not expose scope flag")
	}
}

func TestAuthCommandsIgnoreProjectDotenv(t *testing.T) {
	restoreStore := auth.SetTokenStoreForTest(auth.NewMemoryTokenStore())
	defer restoreStore()
	previousWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previousWD) })
	if err := os.Mkdir(filepath.Join(dir, ".env.local"), 0o755); err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{{"auth"}, {"auth", "status"}} {
		cmd := NewRootCmd()
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("%v inspected project dotenv: %v", args, err)
		}
	}
}

func TestOpenAPICommandHiddenFromRootHelp(t *testing.T) {
	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "openapi") {
		t.Fatalf("root help should not expose openapi command:\n%s", out.String())
	}
}

func TestAuthenticatedOpenAPIClientKeepsDefaultTimeout(t *testing.T) {
	restoreGlobals := overrideAuthGlobals(t)
	defer restoreGlobals()

	authHTTPClient = http.DefaultClient
	client := newAuthenticatedOpenAPIClient(nil)
	if client.HTTP == nil || client.HTTP.Timeout != 15*time.Second {
		t.Fatalf("default openapi timeout = %v, want 15s", client.HTTP.Timeout)
	}
	provider, ok := client.Provider.(auth.STSProvider)
	if !ok {
		t.Fatalf("provider = %T, want auth.STSProvider", client.Provider)
	}
	if provider.HTTPClient == nil || provider.HTTPClient.Timeout != 15*time.Second {
		t.Fatalf("default sts timeout = %v, want 15s", provider.HTTPClient.Timeout)
	}
}

func TestAuthenticatedOpenAPIClientUsesInvocationUserAgent(t *testing.T) {
	stdinIsTTY, stdoutIsTTY := false, false
	telemetry.InitializeInvocation(telemetry.ResolveInvocationContextOptions{
		CLIName:    "vertc",
		CLIVersion: "1.2.3",
		Env: map[string]string{
			"VE_SKILL_ID": "byted-interactai-guide/0.0.1",
		},
		StdinIsTTY:  &stdinIsTTY,
		StdoutIsTTY: &stdoutIsTTY,
	})

	client := newAuthenticatedOpenAPIClient(nil)
	want := "vertc/1.2.3 invocation/skill skill/byted-interactai-guide#0.0.1"
	if client.UserAgent != want {
		t.Fatalf("user agent = %q, want %q", client.UserAgent, want)
	}
}

func TestAuthenticatedOpenAPIClientPreservesInjectedClient(t *testing.T) {
	restoreGlobals := overrideAuthGlobals(t)
	defer restoreGlobals()

	injected := &http.Client{Timeout: 3 * time.Second}
	authHTTPClient = injected
	client := newAuthenticatedOpenAPIClient(nil)
	if client.HTTP != injected {
		t.Fatal("openapi client did not preserve injected HTTP client")
	}
	provider := client.Provider.(auth.STSProvider)
	if provider.HTTPClient != injected {
		t.Fatal("sts provider did not preserve injected HTTP client")
	}
}

func TestAgentStartWithoutAuthReturnsAuthError(t *testing.T) {
	restoreStore := auth.SetTokenStoreForTest(auth.NewMemoryTokenStore())
	defer restoreStore()
	restoreGlobals := overrideAuthGlobals(t)
	defer restoreGlobals()

	previousWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	t.Cleanup(func() {
		if err := os.Chdir(previousWD); err != nil {
			t.Fatal(err)
		}
	})
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default("demo", "voice-agent", "web")
	if err := config.Save(cfg, config.Path(dir)); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RTC_APP_ID", "appid1234567890123456789")
	// Console-integrated model: a valid template must exist so buildStartRequest
	// succeeds and the flow reaches the auth check.
	tpl := `{"SceneConfig":{"Name":"Default"},"VoiceChat":{"Config":{"ASRConfig":{"Provider":"volcano","ProviderParams":{}},"LLMConfig":{"Mode":"ArkV3","EndPointId":"ep-test"},"TTSConfig":{"Provider":"volcano","ProviderParams":{}}},"AgentConfig":{"UserId":"voice_agent"}}}`
	if err := os.MkdirAll(filepath.Join("server", "scenes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("server", "scenes", "default.json"), []byte(tpl), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := NewRootCmd()
	cmd.SetArgs([]string{"agent", "start"})
	err = cmd.Execute()
	typed, ok := errs.As(err)
	if !ok || typed.Code != "vertc.auth.not_authenticated" {
		t.Fatalf("err = %v, want vertc.auth.not_authenticated", err)
	}
}

func TestAuthLoginLocalUsesSameDeviceClientAndRandomCallback(t *testing.T) {
	store := auth.NewMemoryTokenStore()
	restoreStore := auth.SetTokenStoreForTest(store)
	defer restoreStore()
	restoreGlobals := overrideAuthGlobals(t)
	defer restoreGlobals()

	var tokenClientID string
	var tokenRedirectURI string
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		tokenClientID = r.PostForm.Get("client_id")
		tokenRedirectURI = r.PostForm.Get("redirect_uri")
		if got := r.PostForm.Get("grant_type"); got != "authorization_code" {
			t.Fatalf("grant_type = %q", got)
		}
		if got := r.PostForm.Get("code_verifier"); got == "" {
			t.Fatal("code_verifier is empty")
		}
		if got := r.PostForm.Get("client_secret"); got != "" {
			t.Fatalf("client_secret = %q, want empty", got)
		}
		_ = json.NewEncoder(w).Encode(auth.TokenSet{AccessToken: "access-1", RefreshToken: "refresh-1", TokenType: "Bearer", ExpiresIn: 3600})
	}))
	defer tokenServer.Close()
	oauthTokenEndpoint = tokenServer.URL
	oauthAuthorizeEndpoint = "https://signin.test/authorize"

	openedLoginURLCh := make(chan string, 1)
	authOpenBrowser = func(targetURL string) error {
		openedLoginURLCh <- targetURL
		return nil
	}

	cmd := NewRootCmd()
	cmd.SetArgs([]string{"auth", "login", "--browser=open", "--timeout", "5s"})
	errCh := make(chan error, 1)
	go func() { errCh <- cmd.Execute() }()

	loginURL := <-openedLoginURLCh
	parsedLoginURL, err := url.Parse(loginURL)
	if err != nil {
		t.Fatal(err)
	}
	if got := parsedLoginURL.Query().Get("client_id"); got != auth.DefaultLocalClientID {
		t.Fatalf("client_id = %q", got)
	}
	if got := parsedLoginURL.Query().Get("scope"); got != auth.DefaultScope {
		t.Fatalf("scope = %q", got)
	}
	redirectURI := parsedLoginURL.Query().Get("redirect_uri")
	parsedRedirectURI, err := url.Parse(redirectURI)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(parsedRedirectURI.Port())
	if err != nil || port <= 40000 || parsedRedirectURI.Path != "" {
		t.Fatalf("redirect_uri = %s", redirectURI)
	}
	callbackURL := redirectURI + "?code=code-1&state=" + url.QueryEscape(parsedLoginURL.Query().Get("state"))
	resp, err := http.Get(callbackURL)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("auth login did not complete")
	}
	if tokenClientID != auth.DefaultLocalClientID || tokenRedirectURI != redirectURI {
		t.Fatalf("token client/redirect = %q %q", tokenClientID, tokenRedirectURI)
	}
	token, err := auth.LoadToken()
	if err != nil {
		t.Fatal(err)
	}
	if token.AccessToken != "access-1" || token.ClientID != auth.DefaultLocalClientID {
		t.Fatalf("stored token = %+v", token)
	}
}

func TestAuthLoginManualUsesCrossDeviceClientAndManualCode(t *testing.T) {
	store := auth.NewMemoryTokenStore()
	restoreStore := auth.SetTokenStoreForTest(store)
	defer restoreStore()
	restoreGlobals := overrideAuthGlobals(t)
	defer restoreGlobals()
	authRandomURLSafe = func(length int) (string, error) {
		if length == 32 {
			return "state-remote", nil
		}
		return "nonce-remote", nil
	}
	if err := auth.SaveToken(auth.TokenSet{
		AccessToken:  "access-existing",
		RefreshToken: "refresh-old",
		ClientID:     auth.DefaultLocalClientID,
		ExpiresAt:    time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatal(err)
	}

	var tokenClientID string
	var tokenRedirectURI string
	var tokenCode string
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		tokenClientID = r.PostForm.Get("client_id")
		tokenRedirectURI = r.PostForm.Get("redirect_uri")
		tokenCode = r.PostForm.Get("code")
		_ = json.NewEncoder(w).Encode(auth.TokenSet{AccessToken: "access-remote", RefreshToken: "refresh-remote", TokenType: "Bearer", ExpiresIn: 3600})
	}))
	defer tokenServer.Close()
	oauthTokenEndpoint = tokenServer.URL
	oauthAuthorizeEndpoint = "https://signin.test/authorize"

	cmd := NewRootCmd()
	authorizationCode := base64.RawURLEncoding.EncodeToString([]byte("code=code-remote&state=state-remote"))
	cmd.SetIn(strings.NewReader(authorizationCode + "\n"))
	cmd.SetArgs([]string{"auth", "login", "--browser=manual", "--force"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if tokenClientID != auth.DefaultRemoteClientID {
		t.Fatalf("client_id = %q", tokenClientID)
	}
	if tokenRedirectURI != auth.DefaultRemoteRedirectURI {
		t.Fatalf("redirect_uri = %q, want %q", tokenRedirectURI, auth.DefaultRemoteRedirectURI)
	}
	if tokenCode != "code-remote" {
		t.Fatalf("code = %q", tokenCode)
	}
}

func TestAuthLoginManualStartAndResumeAcrossCommands(t *testing.T) {
	store := auth.NewMemoryTokenStore()
	restoreStore := auth.SetTokenStoreForTest(store)
	defer restoreStore()
	restoreGlobals := overrideAuthGlobals(t)
	defer restoreGlobals()
	t.Setenv("VERTC_HOME", t.TempDir())
	authRandomURLSafe = func(length int) (string, error) {
		if length == 32 {
			return "state-two-phase", nil
		}
		return "nonce-two-phase", nil
	}
	oauthAuthorizeEndpoint = "https://signin.test/authorize"

	start := NewRootCmd()
	start.SetArgs([]string{"auth", "login", "--browser=manual", "--start"})
	if err := start.Execute(); err != nil {
		t.Fatal(err)
	}
	pending, err := auth.LoadPendingAuthorization(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if pending.State != "state-two-phase" || pending.CodeVerifier == "" || pending.ClientID != auth.DefaultRemoteClientID {
		t.Fatalf("pending = %+v", pending)
	}
	if _, err := auth.LoadToken(); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("start saved token: %v", err)
	}

	var tokenCode, tokenVerifier string
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		tokenCode = r.PostForm.Get("code")
		tokenVerifier = r.PostForm.Get("code_verifier")
		_ = json.NewEncoder(w).Encode(auth.TokenSet{AccessToken: "access-2p", RefreshToken: "refresh-2p", ExpiresIn: 3600})
	}))
	defer tokenServer.Close()
	oauthTokenEndpoint = tokenServer.URL
	authorizationCode := base64.RawURLEncoding.EncodeToString([]byte("code=code-two-phase&state=state-two-phase"))
	resume := NewRootCmd()
	resume.SetIn(strings.NewReader(authorizationCode + "\n"))
	resume.SetArgs([]string{"auth", "login", "--resume"})
	if err := resume.Execute(); err != nil {
		t.Fatal(err)
	}
	if tokenCode != "code-two-phase" || tokenVerifier != pending.CodeVerifier {
		t.Fatalf("token exchange code/verifier = %q %q", tokenCode, tokenVerifier)
	}
	token, err := auth.LoadToken()
	if err != nil {
		t.Fatal(err)
	}
	if token.AccessToken != "access-2p" || token.ClientID != auth.DefaultRemoteClientID {
		t.Fatalf("stored token = %+v", token)
	}
	if _, err := auth.LoadPendingAuthorization(time.Now()); err == nil {
		t.Fatal("resume left pending authorization reusable")
	}
}

func TestAuthLoginResumeMismatchKeepsPendingAndSkipsExchange(t *testing.T) {
	restoreStore := auth.SetTokenStoreForTest(auth.NewMemoryTokenStore())
	defer restoreStore()
	restoreGlobals := overrideAuthGlobals(t)
	defer restoreGlobals()
	t.Setenv("VERTC_HOME", t.TempDir())
	now := time.Now()
	pending, err := auth.NewPendingAuthorization("expected", "verifier", auth.DefaultRemoteClientID, auth.DefaultRemoteRedirectURI, "", now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.SavePendingAuthorization(pending); err != nil {
		t.Fatal(err)
	}
	authHTTPClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("state mismatch must not call token endpoint")
		return nil, nil
	})}
	wrongCode := base64.RawURLEncoding.EncodeToString([]byte("code=wrong&state=wrong"))
	cmd := NewRootCmd()
	cmd.SetIn(strings.NewReader(wrongCode + "\n"))
	cmd.SetArgs([]string{"auth", "login", "--resume"})
	err = cmd.Execute()
	if typed, ok := errs.As(err); !ok || typed.Code != "vertc.auth.authorization_failed" || !strings.Contains(typed.Message, "state mismatch") {
		t.Fatalf("err = %v", err)
	}
	if _, err := auth.LoadPendingAuthorization(time.Now()); err != nil {
		t.Fatalf("mismatch consumed pending authorization: %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type loadErrorTokenStore struct {
	err error
}

func (s loadErrorTokenStore) Load() (auth.TokenSet, error) { return auth.TokenSet{}, s.err }
func (loadErrorTokenStore) Save(auth.TokenSet) error       { return nil }
func (loadErrorTokenStore) Delete() error                  { return nil }
func (loadErrorTokenStore) Location() string               { return "test:load-error" }

func TestAuthLoginManualRejectsStateMismatch(t *testing.T) {
	restoreStore := auth.SetTokenStoreForTest(auth.NewMemoryTokenStore())
	defer restoreStore()
	restoreGlobals := overrideAuthGlobals(t)
	defer restoreGlobals()
	authRandomURLSafe = func(length int) (string, error) {
		if length == 32 {
			return "state-remote", nil
		}
		return "nonce-remote", nil
	}

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("token endpoint should not be called when state mismatches")
	}))
	defer tokenServer.Close()
	oauthTokenEndpoint = tokenServer.URL
	oauthAuthorizeEndpoint = "https://signin.test/authorize"

	cmd := NewRootCmd()
	authorizationCode := base64.RawURLEncoding.EncodeToString([]byte("code=code-remote&state=wrong-state"))
	cmd.SetIn(strings.NewReader(authorizationCode + "\n"))
	cmd.SetArgs([]string{"auth", "login", "--browser=manual"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "authorization state mismatch") {
		t.Fatalf("err = %v, want state mismatch", err)
	}
}

func TestParseAuthorizationCodeInputRequiresBase64PayloadState(t *testing.T) {
	if _, _, err := parseAuthorizationCodeInput("code-remote"); err == nil {
		t.Fatal("expected raw code to be rejected")
	}
	withoutState := base64.RawURLEncoding.EncodeToString([]byte("code=code-remote"))
	if _, _, err := parseAuthorizationCodeInput(withoutState); err == nil {
		t.Fatal("expected missing state to be rejected")
	}
}

func TestAuthStatusAutoRefreshes(t *testing.T) {
	restoreStore := auth.SetTokenStoreForTest(auth.NewMemoryTokenStore())
	defer restoreStore()
	restoreGlobals := overrideAuthGlobals(t)
	defer restoreGlobals()

	if err := auth.SaveToken(auth.TokenSet{
		AccessToken:  "old",
		RefreshToken: "refresh-old",
		ClientID:     auth.DefaultLocalClientID,
		ExpiresAt:    time.Now().Add(time.Minute).UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatal(err)
	}
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.PostForm.Get("grant_type") != "refresh_token" {
			t.Fatalf("form = %v", r.PostForm)
		}
		_ = json.NewEncoder(w).Encode(auth.TokenSet{AccessToken: "new", TokenType: "Bearer", ExpiresIn: 3600})
	}))
	defer tokenServer.Close()
	oauthTokenEndpoint = tokenServer.URL

	cmd := NewRootCmd()
	cmd.SetArgs([]string{"auth", "status"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	token, err := auth.LoadToken()
	if err != nil {
		t.Fatal(err)
	}
	if token.AccessToken != "new" || token.RefreshToken != "refresh-old" {
		t.Fatalf("token = %+v", token)
	}
}

func TestOpenAPIInvokeUsesStoredSTSToken(t *testing.T) {
	restoreStore := auth.SetTokenStoreForTest(auth.NewMemoryTokenStore())
	defer restoreStore()
	restoreGlobals := overrideAuthGlobals(t)
	defer restoreGlobals()

	stsJSON, _ := json.Marshal(auth.STSCredential{AccessKeyID: "AKID", SecretAccessKey: "SECRET", SessionToken: "SESSION"})
	if err := auth.SaveToken(auth.TokenSet{
		AccessToken:  string(stsJSON),
		RefreshToken: "signin-refresh",
		ClientID:     auth.DefaultLocalClientID,
		TokenType:    auth.TokenTypeAccessTokenSTS,
		ExpiresAt:    time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatal(err)
	}
	var sawOpenAPI bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawOpenAPI = true
		if r.URL.Query().Get("Action") != "DescribeSomething" || r.URL.Query().Get("Version") != "2020-01-01" {
			t.Fatalf("query = %s", r.URL.RawQuery)
		}
		if r.Header.Get("X-Security-Token") != "SESSION" {
			t.Fatalf("X-Security-Token = %q", r.Header.Get("X-Security-Token"))
		}
		if !strings.Contains(r.Header.Get("Authorization"), "Credential=AKID/") {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Result": map[string]any{"Ok": true}})
	}))
	defer server.Close()

	cmd := NewRootCmd()
	cmd.SetArgs([]string{
		"openapi", "invoke",
		"--endpoint", server.URL,
		"--action", "DescribeSomething",
		"--version", "2020-01-01",
		"--body", `{"Limit":1}`,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !sawOpenAPI {
		t.Fatal("openapi endpoint was not called")
	}
}

func TestOpenAPIInvokeReturnsMalformedConfigError(t *testing.T) {
	previousWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	t.Cleanup(func() {
		if err := os.Chdir(previousWD); err != nil {
			t.Fatal(err)
		}
	})
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("vertc.config.yaml", []byte("openapi:\n  endpoint: ["), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := NewRootCmd()
	cmd.SetArgs([]string{"openapi", "invoke", "--action", "DescribeSomething", "--version", "2020-01-01"})
	err = cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "vertc.config.parse_error") {
		t.Fatalf("err = %v, want config parse error", err)
	}
}

func overrideAuthGlobals(t *testing.T) func() {
	t.Helper()
	oldBrowser := authOpenBrowser
	oldClient := authHTTPClient
	oldRandom := authRandomURLSafe
	oldIsTerminal := authIsTerminal
	oldAuthorize := oauthAuthorizeEndpoint
	oldToken := oauthTokenEndpoint
	return func() {
		authOpenBrowser = oldBrowser
		authHTTPClient = oldClient
		authRandomURLSafe = oldRandom
		authIsTerminal = oldIsTerminal
		oauthAuthorizeEndpoint = oldAuthorize
		oauthTokenEndpoint = oldToken
	}
}
