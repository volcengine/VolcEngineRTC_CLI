// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

// Package skillcontent exposes the official skills embedded in this exact CLI
// build so self-update never installs content from a moving default branch.
package skillcontent

import "embed"

// Content contains every skill directory. Top-level Go files are filtered when
// the content is materialized for the external skills installer.
//
//go:embed *
var Content embed.FS
