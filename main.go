// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

// Command vertc is the VolcEngine AI audio/video developer-workflow CLI.
// See internal/meta for the single identity source of truth.
package main

import (
	"os"

	"github.com/volcengine/VolcEngineRTC_CLI/cmd"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/meta"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/telemetry"
)

func main() {
	telemetry.InitializeInvocation(telemetry.ResolveInvocationContextOptions{
		CLIName:    meta.UserAgentProduct,
		CLIVersion: meta.Version,
	})
	os.Exit(cmd.Execute())
}
