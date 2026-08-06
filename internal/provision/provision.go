// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

// Package provision validates and writes conversational-AI bot scene options
// discovered by the interactive dev setup flow.
package provision

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/config"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/openapi"
)

// DefaultConfigFile is the agent template file provision writes when the config
// does not name one explicitly. It mirrors cmd/agent.go's fallback.
const DefaultConfigFile = "server/scenes/default.json"

// BotLister is the read-only bot query used by dev's multi-scene setup.
// The real openapi.ConsoleClient satisfies it, and tests inject a fake.
type BotLister interface {
	AibotxQuery(ctx context.Context, pageNum, limit int) ([]openapi.Bot, error)
}

// SceneResult describes one bot-backed alternative scene written by dev.
type SceneResult struct {
	BotID   string `json:"bot_id"`
	BotName string `json:"bot_name,omitempty"`
	Path    string `json:"path"`
}

// ListBots is the AibotxQuery entry used by interactive dev setup.
func ListBots(ctx context.Context, api BotLister) ([]openapi.Bot, error) {
	bots, err := api.AibotxQuery(ctx, 1, 0)
	if err != nil {
		return nil, err
	}
	if len(bots) == 0 {
		return nil, errs.New("vertc.provision.no_bot", errs.TypeNotFound,
			"no conversational-AI bot found under this account").
			WithHint("create a bot in 控制台 > 我的智能体 first, then re-run")
	}
	return bots, nil
}

// WriteBotScenes writes selected bots as stable bot-<id>.json alternatives in
// the configured scene directory. The existing default scene and unrelated
// scenes are preserved.
func WriteBotScenes(dir, configFile string, bots []openapi.Bot, dryRun bool) ([]SceneResult, error) {
	if len(bots) == 0 {
		return nil, errs.New("vertc.provision.not_selected", errs.TypeValidation,
			"at least one conversational-AI bot must be selected")
	}
	if strings.TrimSpace(configFile) == "" {
		configFile = DefaultConfigFile
	}
	defaultPath, err := config.ProjectFilePath(dir, configFile)
	if err != nil {
		return nil, err
	}
	base, err := os.ReadFile(defaultPath)
	if err != nil {
		return nil, errs.New("vertc.provision.write_failed", errs.TypeIO,
			"read default scene %q before provisioning: %s", defaultPath, err).
			WithHint("restore the template scene before running dev")
	}

	type pendingScene struct {
		result SceneResult
		data   []byte
	}
	pending := make([]pendingScene, 0, len(bots))
	seenBots := make(map[string]bool, len(bots))
	seenPaths := make(map[string]bool, len(bots))
	for index, bot := range bots {
		botID := strings.TrimSpace(bot.ID)
		if botID == "" {
			return nil, errs.New("vertc.provision.no_bot", errs.TypeValidation,
				"selected bot at index %d has no Id", index)
		}
		if seenBots[botID] {
			return nil, errs.New("vertc.provision.not_selected", errs.TypeValidation,
				"bot %q was selected more than once", botID)
		}
		seenBots[botID] = true

		relative := filepath.Join(filepath.Dir(configFile), botSceneFileName(bot))
		path, err := config.ProjectFilePath(dir, relative)
		if err != nil {
			return nil, err
		}
		pathKey := strings.ToLower(path)
		if seenPaths[pathKey] {
			return nil, errs.New("vertc.provision.write_failed", errs.TypeValidation,
				"selected bots resolve to the same scene path %q", path)
		}
		seenPaths[pathKey] = true

		data, err := marshalSceneTemplate(bot, base, true)
		if err != nil {
			return nil, err
		}
		if err := config.ValidateServerAgentConfigJSON(data); err != nil {
			return nil, errs.New("vertc.provision.write_failed", errs.TypeValidation,
				"provisioned scene for bot %q is invalid: %s", botID, err)
		}
		pending = append(pending, pendingScene{
			result: SceneResult{BotID: botID, BotName: bot.Name, Path: path},
			data:   data,
		})
	}

	results := make([]SceneResult, len(pending))
	for index, scene := range pending {
		results[index] = scene.result
		if dryRun {
			continue
		}
		if err := atomicWriteFile(scene.result.Path, scene.data, 0o644); err != nil {
			return nil, errs.New("vertc.provision.write_failed", errs.TypeIO,
				"write bot scene %q: %s", scene.result.Path, err)
		}
	}
	return results, nil
}

func botSceneFileName(bot openapi.Bot) string {
	original := strings.TrimSpace(bot.ID)
	var slug strings.Builder
	for _, char := range original {
		switch {
		case char >= 'a' && char <= 'z', char >= 'A' && char <= 'Z', char >= '0' && char <= '9':
			slug.WriteRune(char)
		case char == '-' || char == '_':
			slug.WriteRune(char)
		default:
			slug.WriteByte('-')
		}
	}
	normalized := strings.Trim(slug.String(), "-_")
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(original)))[:8]
	if normalized == "" {
		normalized = hash
	} else if normalized != original || len(normalized) > 48 {
		if len(normalized) > 39 {
			normalized = normalized[:39]
		}
		normalized = strings.Trim(normalized, "-_") + "-" + hash
	}
	return "bot-" + normalized + ".json"
}

func marshalSceneTemplate(bot openapi.Bot, existing []byte, updateSceneName bool) ([]byte, error) {
	if isEmptyJSON(bot.Config) {
		return nil, errs.New("vertc.provision.no_bot", errs.TypeNotFound,
			"selected bot %q has an empty Config", bot.ID).
			WithHint("finish configuring the bot in 控制台 > 我的智能体, then re-run")
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(existing, &root); err != nil {
		return nil, errs.New("vertc.provision.write_failed", errs.TypeValidation,
			"existing scene is not valid JSON: %s", err)
	}
	if _, ok := root["SceneConfig"]; !ok {
		return nil, errs.New("vertc.provision.write_failed", errs.TypeValidation,
			"existing scene must contain SceneConfig")
	}
	if updateSceneName {
		var sceneConfig map[string]any
		if err := json.Unmarshal(root["SceneConfig"], &sceneConfig); err != nil || sceneConfig == nil {
			return nil, errs.New("vertc.provision.write_failed", errs.TypeValidation,
				"existing SceneConfig must be an object")
		}
		name := strings.TrimSpace(bot.Name)
		if name == "" {
			name = strings.TrimSpace(bot.ID)
		}
		if _, ok := sceneConfig["Name"]; ok {
			sceneConfig["Name"] = name
		} else {
			sceneConfig["name"] = name
		}
		sceneConfigRaw, err := json.Marshal(sceneConfig)
		if err != nil {
			return nil, errs.Wrap(err, "vertc.cli.internal", "marshal SceneConfig: %s", err)
		}
		root["SceneConfig"] = sceneConfigRaw
	}
	var voiceChat map[string]json.RawMessage
	if err := json.Unmarshal(root["VoiceChat"], &voiceChat); err != nil || voiceChat == nil {
		return nil, errs.New("vertc.provision.write_failed", errs.TypeValidation,
			"existing scene must contain a VoiceChat object")
	}
	agentConfigValue := make(map[string]any, len(bot.AgentConfig)+1)
	for key, value := range bot.AgentConfig {
		agentConfigValue[key] = value
	}
	userID, _ := agentConfigValue["UserId"].(string)
	if strings.TrimSpace(userID) == "" {
		var existingAgentConfig map[string]any
		if err := json.Unmarshal(voiceChat["AgentConfig"], &existingAgentConfig); err != nil || existingAgentConfig == nil {
			return nil, errs.New("vertc.provision.write_failed", errs.TypeValidation,
				"existing VoiceChat.AgentConfig must be an object")
		}
		if fallback, ok := existingAgentConfig["UserId"].(string); ok && strings.TrimSpace(fallback) != "" {
			agentConfigValue["UserId"] = fallback
		}
	}
	agentConfig, err := json.Marshal(agentConfigValue)
	if err != nil {
		return nil, errs.Wrap(err, "vertc.cli.internal", "marshal VoiceChat.AgentConfig: %s", err)
	}
	voiceChat["Config"] = bot.Config
	voiceChat["AgentConfig"] = agentConfig
	voiceChatRaw, err := json.Marshal(voiceChat)
	if err != nil {
		return nil, errs.Wrap(err, "vertc.cli.internal", "marshal VoiceChat config: %s", err)
	}
	root["VoiceChat"] = voiceChatRaw
	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, errs.Wrap(err, "vertc.cli.internal", "marshal agent template: %s", err)
	}
	return append(data, '\n'), nil
}

func atomicWriteFile(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".vertc-scene-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func isEmptyJSON(raw json.RawMessage) bool {
	s := string(bytes.TrimSpace(raw))
	return s == "" || s == "{}" || s == "null"
}
