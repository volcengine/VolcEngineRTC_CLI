// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

// Package template implements the scene × platform template registry. Templates
// may be embedded in the binary or pinned to an immutable remote archive;
// `template list` enumerates the available templates.
package template

import (
	"fmt"
	"sync"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/product"
)

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

type registryProvider interface {
	Templates() []Template
}

var (
	registryProvidersMu sync.RWMutex
	registryProviders   = map[string]registryProvider{}
)

func registerRegistryProvider(id string, provider registryProvider) {
	if id == "" || provider == nil {
		panic("template registry provider requires an id and implementation")
	}
	registryProvidersMu.Lock()
	defer registryProvidersMu.Unlock()
	if _, exists := registryProviders[id]; exists {
		panic("template registry provider already registered: " + id)
	}
	registryProviders[id] = provider
}

func currentRegistry() []Template {
	id := product.Current().ID
	registryProvidersMu.RLock()
	provider := registryProviders[id]
	registryProvidersMu.RUnlock()
	if provider == nil {
		panic(fmt.Sprintf("template registry provider is not registered for product %q", id))
	}
	return provider.Templates()
}

func registeredRegistryProviderCount() int {
	registryProvidersMu.RLock()
	defer registryProvidersMu.RUnlock()
	return len(registryProviders)
}

// List returns all registry entries.
func List() []Template {
	registry := currentRegistry()
	out := make([]Template, len(registry))
	copy(out, registry)
	return out
}

// Available returns only the instantiable templates.
func Available() []Template {
	registry := currentRegistry()
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
	registry := currentRegistry()
	for _, t := range registry {
		if t.Scene == scene && t.Platform == platform {
			return t, true
		}
	}
	return Template{}, false
}
