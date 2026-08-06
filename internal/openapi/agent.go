// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package openapi

import (
	"context"
	"encoding/json"
	"net/http"
)

type AgentClient struct {
	OpenAPI *Client
	Version string
}

func NewAgentClient(client *Client) AgentClient {
	return AgentClient{OpenAPI: client, Version: DefaultVersion}
}

func (c AgentClient) StartVoiceChat(ctx context.Context, req StartVoiceChatRequest) (*StartVoiceChatResult, error) {
	// StartVoiceChat's Result is a bare "ok" string with no structured payload;
	// unmarshaling it into a struct fails. The TaskId is the client-supplied one,
	// so skip Result parsing and echo it back.
	if err := c.invokeJSON(ctx, "StartVoiceChat", req, nil); err != nil {
		return nil, err
	}
	return &StartVoiceChatResult{TaskID: req.TaskID}, nil
}

func (c AgentClient) UpdateVoiceChat(ctx context.Context, req UpdateVoiceChatRequest) error {
	return c.invokeJSON(ctx, "UpdateVoiceChat", req, nil)
}

func (c AgentClient) StopVoiceChat(ctx context.Context, req StopVoiceChatRequest) error {
	return c.invokeJSON(ctx, "StopVoiceChat", req, nil)
}

func (c AgentClient) invokeJSON(ctx context.Context, action string, req any, result any) error {
	body, err := json.Marshal(req)
	if err != nil {
		return &APIError{Action: action, Message: "marshal request: " + err.Error()}
	}
	version := c.Version
	if version == "" {
		version = DefaultVersion
	}
	return c.OpenAPI.InvokeResult(ctx, InvokeOptions{
		Action:  action,
		Version: version,
		Method:  http.MethodPost,
		Body:    body,
	}, result)
}
