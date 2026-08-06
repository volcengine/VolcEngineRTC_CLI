// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

// Package template implements the scene × platform template registry. Templates
// may be embedded in the binary or pinned to an immutable remote archive;
// `template list` enumerates the available templates.
package template

// SDK describes the pinned SDK a template introduces.
type SDK struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Template is one scene × platform entry in the registry.
type Template struct {
	Scene       string `json:"scene"`
	Platform    string `json:"platform"`
	Title       string `json:"title"`
	Description string `json:"description"`
	SDK         SDK    `json:"sdk"`
	// Available is false for a not-yet-instantiable entry; render/remote reject
	// it before materializing files. The published registry has no such entries.
	Available bool `json:"available"`
	// Default marks the preferred first scene.
	Default bool `json:"default,omitempty"`
	// Remote pins an immutable archive by commit and SHA-256. Nil selects the
	// embedded renderer.
	Remote *RemoteSource `json:"remote,omitempty"`
	// dir is the embedded subdirectory under files/ (available templates only).
	dir string
}

// Key is the "scene/platform" registry key.
func (t Template) Key() string { return t.Scene + "/" + t.Platform }

// rtcWebSDKVersion pins the RTC Web SDK version. It matches the error-code
// knowledge base (Web 4.68). Bump here to move the web templates forward.
const rtcWebSDKVersion = "4.68.1"

const (
	voiceAgentRemoteCommit     = "11fa59e07cf25ee8b195cf212c970fcbc174559a"
	voiceAgentProductionSHA256 = "fcbd8ad16c0ea35411d4f7f4ece0e640ba651393b921b4413231ce227448249e"
)

// The URL and checksum are variables so subprocess E2E tests can replace them
// with a local httptest archive through linker flags. Production builds retain
// the codeload URL derived from Repository and the published checksum above.
var (
	voiceAgentRemoteURL    string
	voiceAgentRemoteSHA256 = voiceAgentProductionSHA256
)

// registry is the scene × platform table. The current release ships the AI
// conversation scene (voice-agent × web). Additional scenes can be registered
// without adding non-instantiable placeholder rows.
var registry = []Template{
	{
		Scene:       "voice-agent",
		Platform:    "web",
		Title:       "Voice AI demo (web)",
		Description: "Full rtc-aigc-demo with a React web app, Koa server, multi-scene VoiceChat, and server-managed agent lifecycle.",
		SDK:         SDK{Name: "@volcengine/rtc", Version: rtcWebSDKVersion},
		Available:   true,
		Default:     true,
		Remote: &RemoteSource{
			Repository: "volcengine/rtc-aigc-demo",
			URL:        voiceAgentRemoteURL,
			Commit:     voiceAgentRemoteCommit,
			SHA256:     voiceAgentRemoteSHA256,
		},
	},
}

// List returns all registry entries.
func List() []Template {
	out := make([]Template, len(registry))
	copy(out, registry)
	return out
}

// Available returns only the instantiable templates.
func Available() []Template {
	var out []Template
	for _, t := range registry {
		if t.Available {
			out = append(out, t)
		}
	}
	return out
}

// Find looks up a template by scene and platform.
func Find(scene, platform string) (Template, bool) {
	for _, t := range registry {
		if t.Scene == scene && t.Platform == platform {
			return t, true
		}
	}
	return Template{}, false
}
