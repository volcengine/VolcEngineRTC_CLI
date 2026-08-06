// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package main

import skillcontent "github.com/volcengine/VolcEngineRTC_CLI/skills"

// EmbeddedSkills keeps official scene skills in the versioned release binary
// so future local inspection/brand-index serving uses the exact CLI content.
var EmbeddedSkills = skillcontent.Content
