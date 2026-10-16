// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package openapi

import "sync"

// voiceChatWireContract is owned by the OpenAPI consumer. Implementations
// encode only the product-specific request shape and action support; signing,
// transport, errors, and output remain shared.
type voiceChatWireContract interface {
	EncodeStart(StartVoiceChatRequest) any
	EncodeUpdate(UpdateVoiceChatRequest) any
	EncodeStop(StopVoiceChatRequest) any
	SupportsUpdate() bool
}

var (
	voiceChatContractsMu sync.RWMutex
	voiceChatContracts   = map[string]voiceChatWireContract{}
)

func registerVoiceChatWireContract(id string, contract voiceChatWireContract) {
	if id == "" || contract == nil {
		panic("VoiceChat wire contract requires an id and implementation")
	}
	voiceChatContractsMu.Lock()
	defer voiceChatContractsMu.Unlock()
	if _, exists := voiceChatContracts[id]; exists {
		panic("VoiceChat wire contract already registered: " + id)
	}
	voiceChatContracts[id] = contract
}

func lookupVoiceChatWireContract(id string) voiceChatWireContract {
	voiceChatContractsMu.RLock()
	defer voiceChatContractsMu.RUnlock()
	return voiceChatContracts[id]
}

func registeredVoiceChatContractCount() int {
	voiceChatContractsMu.RLock()
	defer voiceChatContractsMu.RUnlock()
	return len(voiceChatContracts)
}
