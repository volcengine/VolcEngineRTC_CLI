// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package openapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newConsoleTestClient(server *httptest.Server) ConsoleClient {
	client := NewClient(staticProvider{credential: Credential{
		AccessKeyID:     "ak",
		SecretAccessKey: "sk",
		SessionToken:    "session",
	}})
	client.Endpoint = server.URL
	client.HTTP = server.Client()
	return NewConsoleClient(client)
}

func TestDescribeNewRtcAppsEncodesQueryAndDecodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		q := r.URL.Query()
		if q.Get("Action") != "DescribeNewRtcApps" || q.Get("Version") != VersionDescribeApps {
			t.Errorf("action/version = %q/%q", q.Get("Action"), q.Get("Version"))
		}
		if q.Get("ProjectName") != "default" {
			t.Errorf("ProjectName = %q, want default", q.Get("ProjectName"))
		}
		if q.Get("Offset") != "0" || q.Get("Limit") != "100" {
			t.Errorf("Offset/Limit = %q/%q", q.Get("Offset"), q.Get("Limit"))
		}
		if r.Header.Get("Authorization") == "" {
			t.Error("request not signed")
		}
		_, _ = io.WriteString(w, `{"ResponseMetadata":{"RequestId":"rid"},"Result":{"AppList":[{"AppId":"app-1","Name":"first","Status":"1"},{"AppId":"app-2","Name":"second","InstanceStatus":1}]}}`)
	}))
	defer srv.Close()

	apps, err := newConsoleTestClient(srv).DescribeNewRtcApps(context.Background(), 0, 0)
	if err != nil {
		t.Fatalf("DescribeNewRtcApps: %v", err)
	}
	if len(apps) != 2 || apps[0].Status != "1" || apps[1].Name != "second" || apps[1].Status != "1" {
		t.Fatalf("apps = %+v", apps)
	}
}

func TestDescribeNewRtcAppsPaginatesAndDeduplicates(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		q := r.URL.Query()
		switch requests {
		case 1:
			if q.Get("Offset") != "0" || q.Get("Limit") != "2" {
				t.Fatalf("first page Offset/Limit = %q/%q", q.Get("Offset"), q.Get("Limit"))
			}
			_, _ = io.WriteString(w, `{"Result":{"AppList":[{"AppId":"app-1","Status":"1"},{"AppId":"app-2","Status":"1"}]}}`)
		case 2:
			if q.Get("Offset") != "2" || q.Get("Limit") != "2" {
				t.Fatalf("second page Offset/Limit = %q/%q", q.Get("Offset"), q.Get("Limit"))
			}
			// app-2 repeats across the page boundary; a full page keeps paging.
			_, _ = io.WriteString(w, `{"Result":{"AppList":[{"AppId":"app-2","Status":"1"},{"AppId":"app-3","Status":"1"}]}}`)
		case 3:
			if q.Get("Offset") != "4" {
				t.Fatalf("third page Offset = %q", q.Get("Offset"))
			}
			_, _ = io.WriteString(w, `{"Result":{"AppList":[]}}`) // short page → stop
		default:
			t.Fatalf("unexpected request %d", requests)
		}
	}))
	defer srv.Close()

	apps, err := newConsoleTestClient(srv).DescribeNewRtcApps(context.Background(), 0, 2)
	if err != nil {
		t.Fatalf("DescribeNewRtcApps: %v", err)
	}
	if len(apps) != 3 || apps[0].AppID != "app-1" || apps[2].AppID != "app-3" {
		t.Fatalf("apps = %+v (want app-1,app-2,app-3 deduplicated)", apps)
	}
}

func TestRtcAppIsActiveUsesConsoleStatusMapping(t *testing.T) {
	for _, tc := range []struct {
		status string
		want   bool
	}{
		{status: "1", want: true},
		{status: " 1 ", want: true},
		{status: "0", want: false},
		{status: "Running", want: false},
	} {
		if got := (RtcApp{Status: tc.status}).IsActive(); got != tc.want {
			t.Errorf("status %q: IsActive() = %v, want %v", tc.status, got, tc.want)
		}
	}
}

func TestAibotxQueryPaginatesAndDeduplicatesBots(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		var req aibotxQueryRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		switch requests {
		case 1:
			if req.Iterator != "" || req.PageNum != 1 {
				t.Fatalf("first request = %+v", req)
			}
			_, _ = io.WriteString(w, `{"Result":{"Bots":[{"Id":"one"},{"Id":"two"}],"Iterator":"ignored-response-cursor"}}`)
		case 2:
			if req.Iterator != "" || req.PageNum != 2 {
				t.Fatalf("second request = %+v", req)
			}
			_, _ = io.WriteString(w, `{"Result":{"Bots":[{"Id":"two"},{"Id":"three"},{"Name":"missing-id"}]}}`)
		case 3:
			if req.Iterator != "" || req.PageNum != 3 {
				t.Fatalf("third request = %+v", req)
			}
			_, _ = io.WriteString(w, `{"Result":{"Bots":[]}}`)
		default:
			t.Fatalf("unexpected request %d", requests)
		}
	}))
	defer srv.Close()

	bots, err := newConsoleTestClient(srv).AibotxQuery(context.Background(), 0, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(bots) != 3 || bots[0].ID != "one" || bots[2].ID != "three" {
		t.Fatalf("bots = %+v", bots)
	}
}

func TestDescribeAppKeysPostsAppIDAndDecodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Query().Get("Version") != VersionDescribeApps {
			t.Errorf("version = %q", r.URL.Query().Get("Version"))
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"AppId":"app-1"`) {
			t.Errorf("body missing AppId: %s", body)
		}
		_, _ = io.WriteString(w, `{"ResponseMetadata":{"RequestId":"rid"},"Result":{"AppId":"app-1","AppKey":"0123456789abcdef0123456789abcdef","SecondaryAppKey":"secondary"}}`) // public-scan: allow; gitleaks:allow — synthetic test credential
	}))
	defer srv.Close()

	keys, err := newConsoleTestClient(srv).DescribeAppKeys(context.Background(), "app-1")
	if err != nil {
		t.Fatalf("DescribeAppKeys: %v", err)
	}
	if keys.AppKey != "0123456789abcdef0123456789abcdef" || keys.SecondaryAppKey != "secondary" {
		t.Fatalf("keys = %+v", keys)
	}
}

func TestAibotxQueryPostsPagingAndDecodesTemplate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("Action") != "AibotxQuery" || r.URL.Query().Get("Version") != VersionAibotx {
			t.Errorf("action/version = %q/%q", r.URL.Query().Get("Action"), r.URL.Query().Get("Version"))
		}
		var req aibotxQueryRequest
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("bad request body %s: %v", body, err)
		}
		if req.PageNum != 1 || req.Limit != 12 {
			t.Errorf("paging = %+v, want default 1/12", req)
		}
		_, _ = io.WriteString(w, `{"ResponseMetadata":{"RequestId":"rid"},"Result":{"Bots":[{"Id":"bot-1","Name":"assistant","Config":{"LLMConfig":{"Mode":"ArkV3","EndPointId":"ep-test"}},"AgentConfig":{"UserId":"agent-01"}}]}}`)
	}))
	defer srv.Close()

	bots, err := newConsoleTestClient(srv).AibotxQuery(context.Background(), 0, 0)
	if err != nil {
		t.Fatalf("AibotxQuery: %v", err)
	}
	if len(bots) != 1 || bots[0].ID != "bot-1" {
		t.Fatalf("bots = %+v", bots)
	}
	// Config is captured verbatim so dev setup can place it in VoiceChat.Config.
	if !strings.Contains(string(bots[0].Config), `"Mode":"ArkV3"`) {
		t.Fatalf("bot Config = %s", bots[0].Config)
	}
	if bots[0].AgentConfig["UserId"] != "agent-01" {
		t.Fatalf("bot AgentConfig = %+v", bots[0].AgentConfig)
	}
}

func TestConsoleClientAPIErrorIsParseable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"ResponseMetadata":{"RequestId":"rid","Error":{"Code":"InvalidParameter","Message":"bad AppId"}}}`)
	}))
	defer srv.Close()

	_, err := newConsoleTestClient(srv).DescribeAppKeys(context.Background(), "nope")
	ae, ok := err.(*APIError)
	if !ok {
		t.Fatalf("err = %T, want *APIError", err)
	}
	if ae.Code != "InvalidParameter" || ae.RequestID != "rid" {
		t.Fatalf("APIError = %+v", ae)
	}
}
