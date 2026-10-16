// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package openapi

import (
	"encoding/json"
	"testing"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/product"
)

func TestSoleDefaultVoiceChatWireContractPreservesRequests(t *testing.T) {
	contract := lookupVoiceChatWireContract(product.DefaultID)
	if contract == nil || !contract.SupportsUpdate() || registeredVoiceChatContractCount() != 1 {
		t.Fatalf("unexpected default contract registry")
	}
	start := sampleStartReq()
	got, err := json.Marshal(contract.EncodeStart(start))
	if err != nil {
		t.Fatal(err)
	}
	want, err := json.Marshal(start)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("encoded start = %s, want %s", got, want)
	}
	stop := StopVoiceChatRequest{AppID: "app", RoomID: "room", TaskID: "task"}
	got, _ = json.Marshal(contract.EncodeStop(stop))
	want, _ = json.Marshal(stop)
	if string(got) != string(want) {
		t.Fatalf("encoded stop = %s, want %s", got, want)
	}
}

func TestMissingVoiceChatWireContractFailsClosed(t *testing.T) {
	client := AgentClient{}
	if err := client.requireContract("StartVoiceChat"); err == nil {
		t.Fatal("missing contract was accepted")
	}
}
