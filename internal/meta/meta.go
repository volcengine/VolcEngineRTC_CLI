// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

// Package meta is the single source of truth for the CLI's identity.
//
// Every help text, error hint, skill and affordance resolves the binary name
// from BinName rather than hard-coding a string. Release builds may override
// the identity with ldflags.
package meta

// These values may be overridden at build time via -ldflags, e.g.
//
//	go build -ldflags "-X github.com/volcengine/VolcEngineRTC_CLI/internal/meta.BinName=volc-aiav"
var (
	// BinName is the invoked command name / binary name.
	BinName = "vertc"

	// Version is the CLI version; injected via ldflags in release builds.
	Version = "0.0.1-dev"

	// ReleaseMarker gives cross-platform postflight an unambiguous binary image
	// marker that cannot be confused with a version inside embedded Skill text.
	// The trailing semicolon is a required boundary: release builds inject
	// "VERTC_RELEASE_VERSION=<Version>;".
	ReleaseMarker = "VERTC_RELEASE_VERSION=0.0.1-dev;"

	// Commit is the build commit; injected via ldflags in release builds.
	Commit = "unknown"

	// BuildDate is the build timestamp; injected via ldflags in release builds.
	BuildDate = "unknown"
)

// BusinessID is the stable RTC attribution tag for traffic started by this
// product. It intentionally does not follow release-time BinName aliases.
const BusinessID = "vertc_cli"

// UserAgentProduct is the stable product token used for RTC OpenAPI request
// attribution. Unlike BinName, it does not change for release-time aliases.
const UserAgentProduct = "vertc"

// ConfigFileName is the canonical unified config file name.
const ConfigFileName = "vertc.config.yaml"
