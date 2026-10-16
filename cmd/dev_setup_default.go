// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"strings"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/config"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/openapi"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/product"
)

type defaultDevSetupContract struct{}

func (defaultDevSetupContract) NewConsoleClient(cfg *config.Config) devConsoleClient {
	return openapi.NewConsoleClient(newAuthenticatedOpenAPIClient(cfg))
}

func (defaultDevSetupContract) CredentialBootstrapRequired(cfg *config.Config, report config.Report, force bool) bool {
	return config.CanBootstrapRTCAppCredentials(cfg, report, force)
}

func (defaultDevSetupContract) BotSceneDiscoveryRequired(cfg *config.Config) bool {
	return cfg != nil && cfg.Project.Scene == "voice-agent" && strings.TrimSpace(cfg.Agent.ConfigSource) == ""
}

func (defaultDevSetupContract) ShouldResolveBotScenes(cfg *config.Config, options devSetupOptions) bool {
	return options.Reconfigure || len(options.BotIDs) > 0 || strings.TrimSpace(cfg.Agent.ConfigSource) == ""
}

func init() {
	registerDevSetupContract(product.DefaultID, defaultDevSetupContract{})
}
