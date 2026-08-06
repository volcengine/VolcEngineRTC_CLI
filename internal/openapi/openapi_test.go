// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package openapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
)

func TestSignRequestAddsVolcengineHeaders(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "https://rtc.volcengineapi.com/?Action=Ping&Version=2020-01-01", strings.NewReader(`{"Limit":1}`))
	if err != nil {
		t.Fatal(err)
	}
	err = SignRequest(req, Credential{
		AccessKeyID:     "AKID",
		SecretAccessKey: "SECRET",
		SessionToken:    "SESSION",
	}, SignOptions{
		Service: "rtc",
		Region:  "cn-north-1",
		Now:     time.Date(2026, 7, 7, 10, 0, 0, 0, time.UTC),
		Body:    []byte(`{"Limit":1}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if req.Header.Get("X-Date") != "20260707T100000Z" {
		t.Fatalf("X-Date = %q", req.Header.Get("X-Date"))
	}
	if req.Header.Get("X-Security-Token") != "SESSION" {
		t.Fatalf("X-Security-Token = %q", req.Header.Get("X-Security-Token"))
	}
	if got := req.Header.Get("X-Content-Sha256"); got != "55522f708dcfebccb7bd3e8d0001a53ecaf2beca9ca801f1e9161e24215faa99" {
		t.Fatalf("X-Content-Sha256 = %q", got)
	}
	authz := req.Header.Get("Authorization")
	if !strings.Contains(authz, "HMAC-SHA256 Credential=AKID/20260707/cn-north-1/rtc/request") {
		t.Fatalf("Authorization = %s", authz)
	}
	if !strings.Contains(authz, "SignedHeaders=content-type;host;x-content-sha256;x-date;x-security-token") {
		t.Fatalf("Authorization signed headers = %s", authz)
	}
}

func TestClientInvokeSignsAndSendsOpenAPIRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("Action") != "DescribeSomething" {
			t.Fatalf("Action = %q", r.URL.Query().Get("Action"))
		}
		if r.URL.Query().Get("Version") != "2020-01-01" {
			t.Fatalf("Version = %q", r.URL.Query().Get("Version"))
		}
		if r.Header.Get("X-Security-Token") != "SESSION" {
			t.Fatalf("X-Security-Token = %q", r.Header.Get("X-Security-Token"))
		}
		if r.Header.Get("Authorization") == "" {
			t.Fatal("Authorization header is empty")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer server.Close()

	client := NewClient(staticProvider{credential: Credential{
		AccessKeyID:     "AKID",
		SecretAccessKey: "SECRET",
		SessionToken:    "SESSION",
	}})
	client.Endpoint = server.URL
	client.HTTP = server.Client()
	client.Now = func() time.Time {
		return time.Date(2026, 7, 7, 10, 0, 0, 0, time.UTC)
	}
	response, err := client.Invoke(context.Background(), InvokeOptions{
		Action:  "DescribeSomething",
		Version: "2020-01-01",
		Body:    []byte(`{"Limit":1}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("StatusCode = %d", response.StatusCode)
	}
}

func TestClientInvokeAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ResponseMetadata": map[string]any{
				"RequestId": "rid",
				"Error": map[string]any{
					"Code":    "InvalidParameter",
					"Message": "bad request",
				},
			},
		})
	}))
	defer server.Close()

	client := NewClient(staticProvider{credential: Credential{AccessKeyID: "AKID", SecretAccessKey: "SECRET", SessionToken: "SESSION"}})
	client.Endpoint = server.URL
	client.HTTP = server.Client()
	_, err := client.Invoke(context.Background(), InvokeOptions{Action: "DescribeSomething", Version: "2020-01-01"})
	ae, ok := err.(*APIError)
	if !ok {
		t.Fatalf("err = %T, want *APIError", err)
	}
	if ae.Code != "InvalidParameter" || ae.RequestID != "rid" {
		t.Fatalf("APIError = %+v", ae)
	}
}

func TestClientInvokeRequestFailedErrorType(t *testing.T) {
	client := NewClient(staticProvider{credential: Credential{AccessKeyID: "AKID", SecretAccessKey: "SECRET", SessionToken: "SESSION"}})
	_, err := client.Invoke(context.Background(), InvokeOptions{
		Action:  "DescribeSomething",
		Version: "2020-01-01",
		Method:  "BAD\nMETHOD",
	})
	if err == nil {
		t.Fatal("expected request creation error")
	}
	typed, ok := errs.As(err)
	if !ok {
		t.Fatalf("err = %T, want *errs.Error", err)
	}
	if typed.Code != "vertc.openapi.request_failed" || typed.Type != errs.TypeIO {
		t.Fatalf("typed err = %s/%s, want request_failed/io", typed.Code, typed.Type)
	}
}

func TestClientInvokeProviderError(t *testing.T) {
	wantErr := errs.New("vertc.auth.not_authenticated", errs.TypeAuth, "not logged in")
	client := NewClient(staticProvider{err: wantErr})
	_, err := client.Invoke(context.Background(), InvokeOptions{Action: "DescribeSomething", Version: "2020-01-01"})
	if err != wantErr {
		t.Fatalf("err = %v, want provider error", err)
	}
}
