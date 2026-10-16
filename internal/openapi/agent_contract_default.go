// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package openapi

import "github.com/volcengine/VolcEngineRTC_CLI/internal/product"

type defaultVoiceChatWireContract struct{}

func (defaultVoiceChatWireContract) EncodeStart(request StartVoiceChatRequest) any {
	return request
}

func (defaultVoiceChatWireContract) EncodeUpdate(request UpdateVoiceChatRequest) any {
	return request
}

func (defaultVoiceChatWireContract) EncodeStop(request StopVoiceChatRequest) any {
	return request
}

func (defaultVoiceChatWireContract) SupportsUpdate() bool { return true }

func init() {
	registerVoiceChatWireContract(product.DefaultID, defaultVoiceChatWireContract{})
}
