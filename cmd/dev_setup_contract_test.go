// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/config"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/product"
)

func TestSoleDefaultDevSetupContractPreservesDecisions(t *testing.T) {
	contract, err := currentDevSetupContract()
	if err != nil {
		t.Fatal(err)
	}
	if registeredDevSetupContractCount() != 1 || !strings.Contains(fmt.Sprintf("%T", contract), "defaultDevSetupContract") {
		t.Fatalf("unexpected setup contract registry")
	}
	cfg := config.Default("fixture", "voice-agent", "web")
	report := config.Validate(cfg)
	if !contract.CredentialBootstrapRequired(cfg, report, false) || !contract.BotSceneDiscoveryRequired(cfg) ||
		!contract.ShouldResolveBotScenes(cfg, devSetupOptions{}) {
		t.Fatal("default setup decisions drifted")
	}
	cfg.Agent.ConfigSource = config.AgentConfigSourceDefault
	if contract.BotSceneDiscoveryRequired(cfg) || contract.ShouldResolveBotScenes(cfg, devSetupOptions{}) {
		t.Fatal("configured scene should not be rediscovered")
	}
}

func TestBootstrapMissingSetupContractReturnsTypedError(t *testing.T) {
	id := product.Current().ID
	devSetupContractsMu.Lock()
	contract := devSetupContracts[id]
	delete(devSetupContracts, id)
	devSetupContractsMu.Unlock()
	t.Cleanup(func() {
		devSetupContractsMu.Lock()
		devSetupContracts[id] = contract
		devSetupContractsMu.Unlock()
	})
	restore, err := bootstrapDevRTCAppCredentials(context.Background(), config.Default("demo", "voice-agent", "web"), t.TempDir(), strings.NewReader(""), io.Discard)
	if restore != nil {
		t.Fatal("failed setup returned a restore callback")
	}
	typed, ok := errs.As(err)
	if !ok || typed.Code != "vertc.cli.internal" {
		t.Fatalf("missing setup contract error = %v", err)
	}
}
