// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package template

import "github.com/volcengine/VolcEngineRTC_CLI/internal/product"

const rtcWebSDKVersion = product.DefaultSDKVersion

const (
	voiceAgentRemoteCommit     = "9ecaee85a6b5a1a7376ec048156d736d9ffc9db3"
	voiceAgentProductionSHA256 = "3c58038767cb7838e3aa0def31511374ad6ccd722e76a6827e844e5148473c8f"
)

// The URL and checksum are variables so subprocess E2E tests can replace them
// with a local httptest archive through linker flags. Production builds retain
// the codeload URL derived from Repository and the published checksum above.
var (
	voiceAgentRemoteURL    string
	voiceAgentRemoteSHA256 = voiceAgentProductionSHA256
)

type defaultRegistryProvider struct{}

func (defaultRegistryProvider) Templates() []Template {
	return []Template{{
		Scene:       "voice-agent",
		Platform:    "web",
		Title:       "Voice AI demo (web)",
		Description: "Full rtc-aigc-demo with a React web app, Koa server, multi-scene VoiceChat, and server-managed agent lifecycle.",
		SDK:         SDK{Name: product.Current().SDKName, Version: rtcWebSDKVersion},
		Available:   true,
		Default:     true,
		Remote: &RemoteSource{
			Repository: "volcengine/rtc-aigc-demo",
			URL:        voiceAgentRemoteURL,
			Commit:     voiceAgentRemoteCommit,
			SHA256:     voiceAgentRemoteSHA256,
		},
	}}
}

func init() {
	registerRegistryProvider(product.DefaultID, defaultRegistryProvider{})
}
