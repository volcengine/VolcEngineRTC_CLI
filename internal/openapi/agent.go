// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package openapi

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/product"
)

type AgentClient struct {
	OpenAPI  *Client
	Version  string
	contract voiceChatWireContract
}

func NewAgentClient(client *Client) AgentClient {
	profile := product.Current()
	return AgentClient{OpenAPI: client, Version: profile.VoiceChatVersion, contract: lookupVoiceChatWireContract(profile.ID)}
}

func (c AgentClient) StartVoiceChat(ctx context.Context, req StartVoiceChatRequest) (*StartVoiceChatResult, error) {
	// StartVoiceChat's Result is a bare "ok" string with no structured payload;
	// unmarshaling it into a struct fails. The TaskId is the client-supplied one,
	// so skip Result parsing and echo it back.
	if err := c.requireContract("StartVoiceChat"); err != nil {
		return nil, err
	}
	if err := c.invokeJSON(ctx, "StartVoiceChat", c.contract.EncodeStart(req), nil); err != nil {
		return nil, err
	}
	return &StartVoiceChatResult{TaskID: req.TaskID}, nil
}

func (c AgentClient) UpdateVoiceChat(ctx context.Context, req UpdateVoiceChatRequest) error {
	if err := c.requireContract("UpdateVoiceChat"); err != nil {
		return err
	}
	if !c.contract.SupportsUpdate() {
		return &APIError{Action: "UpdateVoiceChat", Message: "action is not supported by the selected VoiceChat contract"}
	}
	return c.invokeJSON(ctx, "UpdateVoiceChat", c.contract.EncodeUpdate(req), nil)
}

func (c AgentClient) StopVoiceChat(ctx context.Context, req StopVoiceChatRequest) error {
	if err := c.requireContract("StopVoiceChat"); err != nil {
		return err
	}
	return c.invokeJSON(ctx, "StopVoiceChat", c.contract.EncodeStop(req), nil)
}

func (c AgentClient) requireContract(action string) error {
	if c.contract == nil {
		return &APIError{Action: action, Message: "VoiceChat wire contract is not registered for the selected product"}
	}
	return nil
}

func (c AgentClient) invokeJSON(ctx context.Context, action string, req any, result any) error {
	body, err := json.Marshal(req)
	if err != nil {
		return &APIError{Action: action, Message: "marshal request: " + err.Error()}
	}
	version := c.Version
	if version == "" {
		version = product.Current().VoiceChatVersion
	}
	return c.OpenAPI.InvokeResult(ctx, InvokeOptions{
		Action:  action,
		Version: version,
		Method:  http.MethodPost,
		Body:    body,
	}, result)
}
