// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

// Package product owns the build-selected product contract. Scalar values live
// in Profile; consumer packages own behavior interfaces that use the profile.
package product

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// SelectedID is the build-selected profile identifier. Release builds may
// override it with -ldflags. Unknown identifiers fail closed in Current.
var SelectedID = DefaultID

// Capabilities describes supported command flows. It must not be used for
// request encoding or resource-discovery behavior; consumer-owned interfaces
// handle those differences.
type Capabilities struct {
	RemoteOAuth       bool
	RTCAppBootstrap   bool
	BotSceneDiscovery bool
	AgentUpdate       bool
	RemoteTemplate    bool
}

// Profile contains immutable scalar contracts that are known to vary between
// product implementations. It intentionally contains no maps, slices, or
// function fields, so callers receive an independent value copy.
type Profile struct {
	ID                     string
	DisplayName            string
	BinaryName             string
	ConfigFileName         string
	BusinessID             string
	UserAgentProduct       string
	ProjectRegion          string
	OpenAPIEndpoint        string
	OpenAPIService         string
	OpenAPIRegion          string
	VoiceChatVersion       string
	AgentUserIDField       string
	AgentTargetUserIDField string
	SDKName                string
	SDKVersion             string
	DocsMCPEndpoint        string
	AccessKeyEnv           string
	SecretKeyEnv           string
	SessionTokenEnv        string
	AuthStorageNamespace   string
	OAuthAuthorizeEndpoint string
	OAuthTokenEndpoint     string
	OAuthLocalClientID     string
	OAuthRemoteClientID    string
	OAuthScope             string
	OAuthRemoteRedirectURI string
	OAuthRefreshSkew       time.Duration
	Capabilities           Capabilities
}

var (
	registryMu sync.RWMutex
	registry   = map[string]Profile{}
)

func register(profile Profile) {
	if err := validate(profile); err != nil {
		panic(err)
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, exists := registry[profile.ID]; exists {
		panic(fmt.Sprintf("product profile %q is already registered", profile.ID))
	}
	registry[profile.ID] = profile
}

// Current returns a copy of the selected immutable profile. It panics for an
// invalid build selection because continuing with partial defaults could send
// credentials or signed requests to the wrong service.
func Current() Profile {
	profile, ok := Lookup(SelectedID)
	if !ok {
		panic(fmt.Sprintf("unknown product profile %q", SelectedID))
	}
	return profile
}

// Lookup returns a copy of a registered profile.
func Lookup(id string) (Profile, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	profile, ok := registry[strings.TrimSpace(id)]
	return profile, ok
}

// RegisteredIDs returns a sorted snapshot for architecture tests and build
// diagnostics. The first-stage contract requires exactly one entry.
func RegisteredIDs() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	ids := make([]string, 0, len(registry))
	for id := range registry {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func validate(profile Profile) error {
	required := map[string]string{
		"id": profile.ID, "display name": profile.DisplayName, "binary name": profile.BinaryName,
		"config file name": profile.ConfigFileName, "business id": profile.BusinessID,
		"user agent product": profile.UserAgentProduct, "project region": profile.ProjectRegion,
		"openapi endpoint": profile.OpenAPIEndpoint, "openapi service": profile.OpenAPIService,
		"openapi region": profile.OpenAPIRegion, "voicechat version": profile.VoiceChatVersion,
		"agent user id field": profile.AgentUserIDField, "agent target user id field": profile.AgentTargetUserIDField,
		"sdk name": profile.SDKName, "sdk version": profile.SDKVersion, "docs mcp endpoint": profile.DocsMCPEndpoint,
		"access key env": profile.AccessKeyEnv, "secret key env": profile.SecretKeyEnv,
		"session token env": profile.SessionTokenEnv, "auth storage namespace": profile.AuthStorageNamespace,
		"oauth authorize endpoint": profile.OAuthAuthorizeEndpoint, "oauth token endpoint": profile.OAuthTokenEndpoint,
		"oauth local client id": profile.OAuthLocalClientID, "oauth scope": profile.OAuthScope,
	}
	for field, value := range required {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("product profile %s is required", field)
		}
	}
	if profile.Capabilities.RemoteOAuth && (strings.TrimSpace(profile.OAuthRemoteClientID) == "" || strings.TrimSpace(profile.OAuthRemoteRedirectURI) == "") {
		return fmt.Errorf("product profile remote OAuth requires client id and redirect URI")
	}
	if profile.OAuthRefreshSkew <= 0 {
		return fmt.Errorf("product profile oauth refresh skew must be positive")
	}
	return nil
}
