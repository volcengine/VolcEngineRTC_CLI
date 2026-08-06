// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package openapi

import "encoding/json"

// Request/response types for the RTC conversational-AI OpenAPI (StartVoiceChat
// Version 2025-06-01). Integrated (console-agent) model: the full `Config`
// (ASR/TTS/LLM/VAD/…) is generated in the console and passed through verbatim as
// raw JSON; the CLI only fills the top-level identity params and injects
// AgentConfig.UserId / TargetUserId. So `Config` is opaque and `AgentConfig` is
// a map we augment, rather than fields the CLI models field-by-field.

// StartVoiceChatRequest launches an agent into a room.
type StartVoiceChatRequest struct {
	AppID       string          `json:"AppId"`
	BusinessID  string          `json:"BusinessId,omitempty"`
	RoomID      string          `json:"RoomId"`
	TaskID      string          `json:"TaskId"`
	Config      json.RawMessage `json:"Config,omitempty"`
	AgentConfig map[string]any  `json:"AgentConfig,omitempty"`
}

// StartVoiceChatResult is the StartVoiceChat Result payload.
type StartVoiceChatResult struct {
	TaskID string `json:"TaskId"`
}

// UpdateVoiceChatRequest preserves the hidden legacy command's full-template
// shape. The 2025-06-01 API documents Command + partial Parameters instead;
// new callers must not treat this legacy type as the native Update contract.
type UpdateVoiceChatRequest struct {
	AppID       string          `json:"AppId"`
	RoomID      string          `json:"RoomId"`
	TaskID      string          `json:"TaskId"`
	Config      json.RawMessage `json:"Config,omitempty"`
	AgentConfig map[string]any  `json:"AgentConfig,omitempty"`
}

// StopVoiceChatRequest ends a running task.
type StopVoiceChatRequest struct {
	AppID  string `json:"AppId"`
	RoomID string `json:"RoomId"`
	TaskID string `json:"TaskId"`
}
