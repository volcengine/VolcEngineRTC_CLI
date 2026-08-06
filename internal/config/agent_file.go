// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ServerScene is the configuration contract shared by the remote voice-agent
// template, doctor, provision, and the agent runtime bridge. AgentConfig.UserId
// is part of the persisted scene contract; only TargetUserId is runtime-only.
type ServerScene struct {
	SceneConfig map[string]any `json:"SceneConfig"`
	VoiceChat   struct {
		Config      json.RawMessage `json:"Config"`
		AgentConfig map[string]any  `json:"AgentConfig"`
	} `json:"VoiceChat"`
}

// ParseServerSceneJSON decodes and validates a scene configuration. The old
// top-level Config + AgentConfig shape is intentionally not accepted.
func ParseServerSceneJSON(raw []byte) (*ServerScene, error) {
	var scene ServerScene
	if err := json.Unmarshal(raw, &scene); err != nil {
		return nil, fmt.Errorf("not valid JSON: %w", err)
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("must be a JSON object")
	}
	if _, ok := root["SceneConfig"]; !ok || scene.SceneConfig == nil {
		return nil, fmt.Errorf("must contain a SceneConfig object")
	}
	if _, ok := root["VoiceChat"]; !ok {
		return nil, fmt.Errorf("must contain a VoiceChat object")
	}
	if isEmptyRawJSON(scene.VoiceChat.Config) || scene.VoiceChat.AgentConfig == nil {
		return nil, fmt.Errorf("VoiceChat must contain Config and AgentConfig objects")
	}
	userID, ok := scene.VoiceChat.AgentConfig["UserId"].(string)
	if !ok || strings.TrimSpace(userID) == "" {
		return nil, fmt.Errorf("VoiceChat.AgentConfig.UserId is required")
	}

	var configValue map[string]any
	if err := json.Unmarshal(scene.VoiceChat.Config, &configValue); err != nil || configValue == nil {
		return nil, fmt.Errorf("VoiceChat.Config must be an object")
	}
	asr, ok := configValue["ASRConfig"].(map[string]any)
	if !ok || asr["Provider"] != "volcano" || !isJSONObject(asr["ProviderParams"]) {
		return nil, fmt.Errorf("VoiceChat.Config.ASRConfig must use a Volcano ProviderParams object")
	}
	llm, ok := configValue["LLMConfig"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("VoiceChat.Config.LLMConfig must be an object")
	}
	mode, _ := llm["Mode"].(string)
	if autoActive, exists := llm["AutoActive"]; exists {
		if _, ok := autoActive.(bool); !ok {
			return nil, fmt.Errorf("VoiceChat.Config.LLMConfig.AutoActive must be a boolean")
		}
	}
	switch mode {
	case "ArkV3":
		endpointID := ""
		if value, exists := llm["EndPointId"]; exists {
			var ok bool
			endpointID, ok = value.(string)
			if !ok {
				return nil, fmt.Errorf("VoiceChat.Config.LLMConfig.EndPointId must be a string")
			}
		}
		modelName := ""
		if value, exists := llm["ModelName"]; exists {
			var ok bool
			modelName, ok = value.(string)
			if !ok {
				return nil, fmt.Errorf("VoiceChat.Config.LLMConfig.ModelName must be a string")
			}
		}
		if strings.TrimSpace(endpointID) == "" && strings.TrimSpace(modelName) == "" {
			return nil, fmt.Errorf("VoiceChat.Config.LLMConfig.EndPointId or ModelName is required for ArkV3")
		}
	case "CustomLLM":
		urlValue, ok := llm["Url"].(string)
		if !ok || strings.TrimSpace(urlValue) == "" {
			return nil, fmt.Errorf("VoiceChat.Config.LLMConfig.Url is required for CustomLLM")
		}
		if modelName, exists := llm["ModelName"]; exists {
			if _, ok := modelName.(string); !ok {
				return nil, fmt.Errorf("VoiceChat.Config.LLMConfig.ModelName must be a string")
			}
		}
		if apiKey, exists := llm["APIKey"]; exists {
			if _, ok := apiKey.(string); !ok {
				return nil, fmt.Errorf("VoiceChat.Config.LLMConfig.APIKey must be a string")
			}
		}
		var featureValue map[string]any
		featureUnresolved := false
		if feature, exists := llm["Feature"]; exists {
			featureJSON, ok := feature.(string)
			if !ok {
				return nil, fmt.Errorf("VoiceChat.Config.LLMConfig.Feature must be a string containing a JSON object")
			}
			featureJSON = strings.TrimSpace(featureJSON)
			if featureJSON != "" {
				if err := json.Unmarshal([]byte(featureJSON), &featureValue); err != nil || featureValue == nil {
					featureUnresolved = envRefPattern.FindString(featureJSON) == featureJSON
					if !featureUnresolved {
						return nil, fmt.Errorf("VoiceChat.Config.LLMConfig.Feature must be a string containing a JSON object")
					}
				}
			}
		}
		literalURL := strings.ToLower(strings.TrimSpace(urlValue))
		switch {
		case strings.HasPrefix(literalURL, "https://"):
		case strings.HasPrefix(literalURL, "http://"):
			if !featureUnresolved && featureValue["Http"] != true {
				return nil, fmt.Errorf("VoiceChat.Config.LLMConfig.Feature must contain {\"Http\":true} for an HTTP Url")
			}
		case strings.HasPrefix(literalURL, "${"):
		default:
			return nil, fmt.Errorf("VoiceChat.Config.LLMConfig.Url must use HTTPS, or HTTP with Feature opt-in")
		}
		if custom, exists := llm["Custom"]; exists {
			customJSON, ok := custom.(string)
			if !ok || (strings.TrimSpace(customJSON) != "" && !json.Valid([]byte(customJSON))) {
				return nil, fmt.Errorf("VoiceChat.Config.LLMConfig.Custom must be a string containing valid JSON")
			}
		}
		if extraHeader, exists := llm["ExtraHeader"]; exists && !isJSONObject(extraHeader) {
			return nil, fmt.Errorf("VoiceChat.Config.LLMConfig.ExtraHeader must be a JSON object")
		}
	default:
		return nil, fmt.Errorf("VoiceChat.Config.LLMConfig.Mode must be ArkV3 or CustomLLM")
	}
	tts, ok := configValue["TTSConfig"].(map[string]any)
	provider, _ := tts["Provider"].(string)
	if !ok || (provider != "volcano" && provider != "volcano_bidirection") || !isJSONObject(tts["ProviderParams"]) {
		return nil, fmt.Errorf("VoiceChat.Config.TTSConfig must use a Volcano ProviderParams object")
	}
	return &scene, nil
}

// ValidateServerAgentConfigJSON applies the shared scene contract.
func ValidateServerAgentConfigJSON(raw []byte) error {
	_, err := ParseServerSceneJSON(raw)
	return err
}

func isJSONObject(value any) bool {
	_, ok := value.(map[string]any)
	return ok
}

func isEmptyRawJSON(raw json.RawMessage) bool {
	s := strings.TrimSpace(string(raw))
	return s == "" || s == "null"
}
