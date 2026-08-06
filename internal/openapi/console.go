// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package openapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Console setup OpenAPI versions. Each action pins its own Version, so —
// unlike AgentClient (DefaultVersion 2025-06-01) — ConsoleClient carries no
// single default and passes Version per call.
const (
	VersionDescribeApps      = "2020-12-01" // DescribeNewRtcApps, DescribeAppKeys
	VersionAibotx            = "2025-08-01" // AibotxQuery (Action name carries the x)
	DescribeNewRTCAppsAction = "DescribeNewRtcApps"
	DescribeAppKeysAction    = "DescribeAppKeys"
	RTCAppManagementVersion  = VersionDescribeApps
	AibotxQueryAction        = "AibotxQuery"
	RTCAppStatusActive       = "1"
)

// ConsoleClient wraps the read-only OpenAPI actions used to configure a
// voice-agent project: DescribeNewRtcApps (→ AppId), DescribeAppKeys (→ AppKey)
// and AibotxQuery (→ bot Config/AgentConfig template). It reuses the shared
// Signin-STS-signed Client.InvokeResult; each method supplies its own Version.
type ConsoleClient struct {
	OpenAPI *Client
}

// NewConsoleClient builds a ConsoleClient over the given signed client.
func NewConsoleClient(client *Client) ConsoleClient {
	return ConsoleClient{OpenAPI: client}
}

// RtcApp is one entry of DescribeNewRtcApps' AppList. Only the fields dev setup
// needs are modeled; unknown fields are ignored. AppKey is empty here (it comes
// from DescribeAppKeys).
type RtcApp struct {
	AppID  string `json:"AppId"`
	Name   string `json:"Name"`
	Status string `json:"-"`
}

// IsActive reports whether the console marks the RTC application as usable.
// Keep the service status mapping here so future wire values do not leak into
// command orchestration.
func (a RtcApp) IsActive() bool {
	return strings.TrimSpace(a.Status) == RTCAppStatusActive
}

func (a *RtcApp) UnmarshalJSON(data []byte) error {
	var raw struct {
		AppID          string          `json:"AppId"`
		Name           string          `json:"Name"`
		Status         json.RawMessage `json:"Status"`
		InstanceStatus json.RawMessage `json:"InstanceStatus"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	a.AppID, a.Name = raw.AppID, raw.Name
	status := raw.Status
	if len(status) == 0 || string(status) == "null" {
		status = raw.InstanceStatus
	}
	if len(status) == 0 || string(status) == "null" {
		return nil
	}
	if err := json.Unmarshal(status, &a.Status); err == nil {
		return nil
	}
	var number json.Number
	if err := json.Unmarshal(status, &number); err != nil {
		return fmt.Errorf("decode RTC app status: %w", err)
	}
	a.Status = number.String()
	return nil
}

type describeNewRtcAppsResult struct {
	AppList []RtcApp `json:"AppList"`
}

// DescribeNewRtcApps lists the account's RTC apps under ProjectName=default and
// returns them so dev setup can resolve a target AppId. It pages through every
// result (advancing Offset by Limit until a short page) so accounts with more
// than one page of apps stay fully selectable — a bounded loop guards against a
// misbehaving backend that never returns a short page.
func (c ConsoleClient) DescribeNewRtcApps(ctx context.Context, offset, limit int) ([]RtcApp, error) {
	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	var all []RtcApp
	seen := map[string]bool{}
	for pages := 0; pages < 100; pages++ {
		query := url.Values{}
		query.Set("Offset", strconv.Itoa(offset))
		query.Set("Limit", strconv.Itoa(limit))
		query.Set("ProjectName", "default")
		var result describeNewRtcAppsResult
		if err := c.OpenAPI.InvokeResult(ctx, InvokeOptions{
			Action:  "DescribeNewRtcApps",
			Version: VersionDescribeApps,
			Method:  http.MethodGet,
			Query:   query,
		}, &result); err != nil {
			return nil, err
		}
		added := 0
		for _, app := range result.AppList {
			if id := strings.TrimSpace(app.AppID); id != "" {
				if seen[id] {
					continue
				}
				seen[id] = true
			}
			all = append(all, app)
			added++
		}
		// Stop on a short page, or when a full page contributed nothing new — a
		// backend that ignores Offset/Limit would otherwise loop to the page cap.
		if len(result.AppList) < limit || added == 0 {
			break
		}
		offset += limit
	}
	return all, nil
}

// AppKeys is the DescribeAppKeys Result: the primary AppKey plus a secondary.
// AppKey is a secret — callers MUST keep it env-only and never log it.
type AppKeys struct {
	AppID           string `json:"AppId"`
	AppKey          string `json:"AppKey"`
	SecondaryAppKey string `json:"SecondaryAppKey"`
}

// DescribeAppKeys fetches the AppKey for a given AppId. The Signin STS is
// sufficient — the console's MFA is a UI gate and does not apply at the OpenAPI
// layer.
func (c ConsoleClient) DescribeAppKeys(ctx context.Context, appID string) (AppKeys, error) {
	body, err := json.Marshal(map[string]string{"AppId": appID})
	if err != nil {
		return AppKeys{}, &APIError{Action: "DescribeAppKeys", Message: "marshal request: " + err.Error()}
	}
	var result AppKeys
	if err := c.OpenAPI.InvokeResult(ctx, InvokeOptions{
		Action:  "DescribeAppKeys",
		Version: VersionDescribeApps,
		Method:  http.MethodPost,
		Body:    body,
	}, &result); err != nil {
		return AppKeys{}, err
	}
	return result, nil
}

// Bot is one entry of AibotxQuery's Bots list. Config is passed through verbatim
// (as generated by the console) and AgentConfig is a map; together they form the
// VoiceChat scene configuration consumed by `vertc agent start`.
type Bot struct {
	ID          string          `json:"Id"`
	Name        string          `json:"Name"`
	Config      json.RawMessage `json:"Config"`
	AgentConfig map[string]any  `json:"AgentConfig"`
}

type aibotxQueryRequest struct {
	PageNum  int    `json:"PageNum"`
	Limit    int    `json:"Limit"`
	Iterator string `json:"Iterator"`
}

type aibotxQueryResult struct {
	Bots []Bot `json:"Bots"`
}

// AibotxQuery lists every conversational-AI bot so dev can offer multi-selection.
func (c ConsoleClient) AibotxQuery(ctx context.Context, pageNum, limit int) ([]Bot, error) {
	if pageNum <= 0 {
		pageNum = 1
	}
	if limit <= 0 {
		limit = 12
	}
	var all []Bot
	seenBots := map[string]bool{}
	for pages := 0; pages < 100; pages++ {
		body, err := json.Marshal(aibotxQueryRequest{PageNum: pageNum, Limit: limit, Iterator: ""})
		if err != nil {
			return nil, &APIError{Action: AibotxQueryAction, Message: "marshal request: " + err.Error()}
		}
		var result aibotxQueryResult
		if err := c.OpenAPI.InvokeResult(ctx, InvokeOptions{
			Action: AibotxQueryAction, Version: VersionAibotx,
			Method: http.MethodPost, Body: body,
		}, &result); err != nil {
			return nil, err
		}
		for _, bot := range result.Bots {
			bot.ID = strings.TrimSpace(bot.ID)
			if bot.ID == "" || seenBots[bot.ID] {
				continue
			}
			seenBots[bot.ID] = true
			all = append(all, bot)
		}
		if len(result.Bots) < limit {
			break
		}
		pageNum++
	}
	return all, nil
}
