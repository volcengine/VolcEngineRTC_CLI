// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"sync"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/config"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/product"
)

// devSetupContract is owned by command orchestration. It decides when and how
// the project setup flow discovers resources; persistence and rollback remain
// in the shared command implementation.
type devSetupContract interface {
	NewConsoleClient(*config.Config) devConsoleClient
	CredentialBootstrapRequired(*config.Config, config.Report, bool) bool
	BotSceneDiscoveryRequired(*config.Config) bool
	ShouldResolveBotScenes(*config.Config, devSetupOptions) bool
}

var (
	devSetupContractsMu sync.RWMutex
	devSetupContracts   = map[string]devSetupContract{}
)

func registerDevSetupContract(id string, contract devSetupContract) {
	if id == "" || contract == nil {
		panic("dev setup contract requires an id and implementation")
	}
	devSetupContractsMu.Lock()
	defer devSetupContractsMu.Unlock()
	if _, exists := devSetupContracts[id]; exists {
		panic("dev setup contract already registered: " + id)
	}
	devSetupContracts[id] = contract
}

func currentDevSetupContract() (devSetupContract, error) {
	id := product.Current().ID
	devSetupContractsMu.RLock()
	contract := devSetupContracts[id]
	devSetupContractsMu.RUnlock()
	if contract == nil {
		return nil, errs.New("vertc.cli.internal", errs.TypeInternal,
			"project setup contract is not registered for product %q", id)
	}
	return contract, nil
}

func registeredDevSetupContractCount() int {
	devSetupContractsMu.RLock()
	defer devSetupContractsMu.RUnlock()
	return len(devSetupContracts)
}
