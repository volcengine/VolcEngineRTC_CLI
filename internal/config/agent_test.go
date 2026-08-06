// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"encoding/json"
	"strings"
	"testing"
)

const validServerAgentConfig = `{
  "SceneConfig": {"Name": "Default"},
  "VoiceChat": {
    "Config": {
      "ASRConfig": {"Provider": "volcano", "ProviderParams": {}},
      "LLMConfig": {"Mode": "ArkV3", "EndPointId": "ep-test"},
      "TTSConfig": {"Provider": "volcano_bidirection", "ProviderParams": {}}
    },
    "AgentConfig": {"UserId": "voice_agent"}
  }
}`

const validCustomLLMServerAgentConfig = `{"SceneConfig": {"name": "DeepSeek voice assistant"}, "VoiceChat": {"Config": {"ASRConfig": {"Provider": "volcano", "ProviderParams": {}}, "LLMConfig": {"Mode": "CustomLLM", "Url": "https://api.deepseek.com/chat/completions", "ModelName": "deepseek-v4-flash", "Custom": "{\"thinking\":{\"type\":\"disabled\"}}"}, "TTSConfig": {"Provider": "volcano_bidirection", "ProviderParams": {}}}, "AgentConfig": {"UserId": "deepseek_voice_agent"}}}`

const customLLMCredentialField = "API" + "Key"

func mutateCustomLLMScene(t *testing.T, mutate func(map[string]any)) []byte {
	t.Helper()
	var scene map[string]any
	if err := json.Unmarshal([]byte(validCustomLLMServerAgentConfig), &scene); err != nil {
		t.Fatal(err)
	}
	voiceChat := scene["VoiceChat"].(map[string]any)
	configValue := voiceChat["Config"].(map[string]any)
	llm := configValue["LLMConfig"].(map[string]any)
	mutate(llm)
	raw, err := json.Marshal(scene)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestValidateServerAgentConfigJSONAcceptsCustomLLM(t *testing.T) {
	if err := ValidateServerAgentConfigJSON([]byte(validCustomLLMServerAgentConfig)); err != nil {
		t.Fatalf("valid CustomLLM config rejected: %v", err)
	}
}

func TestValidateServerAgentConfigJSONKeepsArkV3Contract(t *testing.T) {
	if err := ValidateServerAgentConfigJSON([]byte(validServerAgentConfig)); err != nil {
		t.Fatalf("valid ArkV3 config rejected: %v", err)
	}
}

func TestValidateServerAgentConfigJSONRejectsArkV3WithoutEndPointID(t *testing.T) {
	raw := strings.Replace(validServerAgentConfig, `, "EndPointId": "ep-test"`, "", 1)
	err := ValidateServerAgentConfigJSON([]byte(raw))
	if err == nil || !strings.Contains(err.Error(), "VoiceChat.Config.LLMConfig.EndPointId") {
		t.Fatalf("missing ArkV3 EndPointId error = %v, want field path", err)
	}
}

func TestValidateServerAgentConfigJSONAcceptsArkV3ModelName(t *testing.T) {
	raw := strings.Replace(validServerAgentConfig,
		`"Mode": "ArkV3", "EndPointId": "ep-test"`,
		`"Mode": "ArkV3", "ModelName": "doubao-seed-2-0-lite-260428"`, 1)
	if err := ValidateServerAgentConfigJSON([]byte(raw)); err != nil {
		t.Fatalf("ArkV3 ModelName rejected: %v", err)
	}
}

func TestValidateServerAgentConfigJSONRejectsMalformedArkV3UnionFields(t *testing.T) {
	for _, raw := range []string{
		strings.Replace(validServerAgentConfig,
			`"Mode": "ArkV3", "EndPointId": "ep-test"`,
			`"Mode": "ArkV3", "EndPointId": 42, "ModelName": "doubao-seed-2-0-lite-260428"`, 1),
		strings.Replace(validServerAgentConfig,
			`"Mode": "ArkV3", "EndPointId": "ep-test"`,
			`"Mode": "ArkV3", "EndPointId": "ep-test", "ModelName": 42`, 1),
	} {
		if err := ValidateServerAgentConfigJSON([]byte(raw)); err == nil || !strings.Contains(err.Error(), "must be a string") {
			t.Fatalf("malformed ArkV3 union field error = %v", err)
		}
	}
}

func TestValidateServerAgentConfigJSONAcceptsCustomLLMWithoutOptionalModelOrAPIKey(t *testing.T) {
	raw := mutateCustomLLMScene(t, func(llm map[string]any) {
		delete(llm, "ModelName")
		delete(llm, customLLMCredentialField)
	})
	if err := ValidateServerAgentConfigJSON(raw); err != nil {
		t.Fatalf("optional CustomLLM ModelName/APIKey rejected: %v", err)
	}
}

func TestValidateServerAgentConfigJSONRejectsInvalidCustomLLMCustomJSON(t *testing.T) {
	raw := strings.Replace(validCustomLLMServerAgentConfig,
		`"Custom": "{\"thinking\":{\"type\":\"disabled\"}}"`,
		`"Custom": "{not-json}"`, 1)
	err := ValidateServerAgentConfigJSON([]byte(raw))
	if err == nil || !strings.Contains(err.Error(), "VoiceChat.Config.LLMConfig.Custom") || !strings.Contains(err.Error(), "valid JSON") {
		t.Fatalf("invalid Custom JSON error = %v, want field path and valid JSON guidance", err)
	}
}

func TestValidateServerAgentConfigJSONAcceptsEmptyCustomLLMCustom(t *testing.T) {
	raw := strings.Replace(validCustomLLMServerAgentConfig,
		`"Custom": "{\"thinking\":{\"type\":\"disabled\"}}"`,
		`"Custom": ""`, 1)
	if err := ValidateServerAgentConfigJSON([]byte(raw)); err != nil {
		t.Fatalf("empty optional Custom rejected: %v", err)
	}
}

func TestValidateServerAgentConfigJSONRejectsNonObjectCustomLLMExtraHeader(t *testing.T) {
	raw := strings.Replace(validCustomLLMServerAgentConfig,
		`"Custom": "{\"thinking\":{\"type\":\"disabled\"}}"`,
		`"Custom": "{\"thinking\":{\"type\":\"disabled\"}}", "ExtraHeader": ["not", "an", "object"]`, 1)
	err := ValidateServerAgentConfigJSON([]byte(raw))
	if err == nil || !strings.Contains(err.Error(), "VoiceChat.Config.LLMConfig.ExtraHeader") {
		t.Fatalf("non-object ExtraHeader error = %v, want field path", err)
	}
}

func TestValidateServerAgentConfigJSONRejectsNonBooleanCustomLLMAutoActive(t *testing.T) {
	raw := strings.Replace(validCustomLLMServerAgentConfig,
		`"Mode": "CustomLLM",`, `"Mode": "CustomLLM", "AutoActive": "true",`, 1)
	err := ValidateServerAgentConfigJSON([]byte(raw))
	if err == nil || !strings.Contains(err.Error(), "VoiceChat.Config.LLMConfig.AutoActive") {
		t.Fatalf("non-boolean AutoActive error = %v, want field path", err)
	}
}

func TestValidateServerAgentConfigJSONRejectsInvalidCustomLLMFeatureJSON(t *testing.T) {
	raw := strings.Replace(validCustomLLMServerAgentConfig,
		`"Mode": "CustomLLM",`, `"Mode": "CustomLLM", "Feature": "{not-json}",`, 1)
	err := ValidateServerAgentConfigJSON([]byte(raw))
	if err == nil || !strings.Contains(err.Error(), "VoiceChat.Config.LLMConfig.Feature") {
		t.Fatalf("invalid Feature error = %v, want field path", err)
	}
}

func TestValidateServerAgentConfigJSONRejectsHTTPCustomLLMWithoutFeatureOptIn(t *testing.T) {
	raw := strings.Replace(validCustomLLMServerAgentConfig,
		"https://api.deepseek.com/chat/completions", "http://localhost:8080/chat", 1)
	err := ValidateServerAgentConfigJSON([]byte(raw))
	if err == nil || !strings.Contains(err.Error(), "VoiceChat.Config.LLMConfig.Feature") {
		t.Fatalf("HTTP without Feature opt-in error = %v, want Feature path", err)
	}
}

func TestValidateServerAgentConfigJSONRejectsHTTPFeatureFalseWithUnrelatedPlaceholder(t *testing.T) {
	raw := strings.Replace(validCustomLLMServerAgentConfig,
		"https://api.deepseek.com/chat/completions", "http://localhost:8080/chat", 1)
	raw = strings.Replace(raw,
		`"Mode": "CustomLLM",`, `"Mode": "CustomLLM", "Feature": "{\"Http\":false,\"Other\":\"${FEATURE_VALUE}\"}",`, 1)
	err := ValidateServerAgentConfigJSON([]byte(raw))
	if err == nil || !strings.Contains(err.Error(), "VoiceChat.Config.LLMConfig.Feature") {
		t.Fatalf("HTTP with false Feature opt-in and unrelated placeholder error = %v, want Feature path", err)
	}
}

func TestValidateServerAgentConfigJSONAcceptsHTTPCustomLLMWithFeatureOptIn(t *testing.T) {
	raw := strings.Replace(validCustomLLMServerAgentConfig,
		"https://api.deepseek.com/chat/completions", "http://localhost:8080/chat", 1)
	raw = strings.Replace(raw,
		`"Mode": "CustomLLM",`, `"Mode": "CustomLLM", "Feature": "{\"Http\":true}",`, 1)
	if err := ValidateServerAgentConfigJSON([]byte(raw)); err != nil {
		t.Fatalf("HTTP with Feature opt-in rejected: %v", err)
	}
}

func TestValidateServerAgentConfigJSONAcceptsHTTPFeatureTrueWithUnrelatedPlaceholder(t *testing.T) {
	raw := strings.Replace(validCustomLLMServerAgentConfig,
		"https://api.deepseek.com/chat/completions", "http://localhost:8080/chat", 1)
	raw = strings.Replace(raw,
		`"Mode": "CustomLLM",`, `"Mode": "CustomLLM", "Feature": "{\"Http\":true,\"Other\":\"${FEATURE_VALUE}\"}",`, 1)
	if err := ValidateServerAgentConfigJSON([]byte(raw)); err != nil {
		t.Fatalf("HTTP with Feature opt-in and unrelated placeholder rejected: %v", err)
	}
}

func TestValidateServerAgentConfigJSONAcceptsCustomLLMURLPlaceholder(t *testing.T) {
	raw := strings.Replace(validCustomLLMServerAgentConfig,
		"https://api.deepseek.com/chat/completions", "${CUSTOM_LLM_URL}", 1)
	if err := ValidateServerAgentConfigJSON([]byte(raw)); err != nil {
		t.Fatalf("CustomLLM Url placeholder rejected: %v", err)
	}
}

func TestValidateServerAgentConfigJSONAcceptsCustomLLMFeaturePlaceholder(t *testing.T) {
	raw := strings.Replace(validCustomLLMServerAgentConfig,
		"https://api.deepseek.com/chat/completions", "http://localhost:8080/chat", 1)
	raw = strings.Replace(raw,
		`"Mode": "CustomLLM",`, `"Mode": "CustomLLM", "Feature": "${CUSTOM_LLM_FEATURE}",`, 1)
	if err := ValidateServerAgentConfigJSON([]byte(raw)); err != nil {
		t.Fatalf("CustomLLM Feature placeholder rejected: %v", err)
	}
}

func TestValidateServerAgentConfigJSONRejectsUnsupportedLiteralCustomLLMScheme(t *testing.T) {
	raw := strings.Replace(validCustomLLMServerAgentConfig,
		"https://api.deepseek.com/chat/completions", "ftp://example.com/chat", 1)
	err := ValidateServerAgentConfigJSON([]byte(raw))
	if err == nil || !strings.Contains(err.Error(), "VoiceChat.Config.LLMConfig.Url") {
		t.Fatalf("unsupported URL scheme error = %v, want Url path", err)
	}
}

func TestValidateServerAgentConfigJSONRejectsKnownInvalidSchemeWithURLPlaceholder(t *testing.T) {
	raw := strings.Replace(validCustomLLMServerAgentConfig,
		"https://api.deepseek.com/chat/completions", "ftp://example.com/${CUSTOM_LLM_PATH}", 1)
	err := ValidateServerAgentConfigJSON([]byte(raw))
	if err == nil || !strings.Contains(err.Error(), "VoiceChat.Config.LLMConfig.Url") {
		t.Fatalf("known invalid URL scheme with placeholder error = %v, want Url path", err)
	}
}

func TestValidateServerAgentConfigJSONRejectsNonStringCustomLLMModelName(t *testing.T) {
	raw := strings.Replace(validCustomLLMServerAgentConfig,
		`"ModelName": "deepseek-v4-flash"`, `"ModelName": 42`, 1)
	err := ValidateServerAgentConfigJSON([]byte(raw))
	if err == nil || !strings.Contains(err.Error(), "VoiceChat.Config.LLMConfig.ModelName") {
		t.Fatalf("non-string ModelName error = %v, want field path", err)
	}
}

func TestValidateServerAgentConfigJSONRejectsNonStringCustomLLMAPIKey(t *testing.T) {
	raw := mutateCustomLLMScene(t, func(llm map[string]any) {
		llm[customLLMCredentialField] = []string{"not-a-string"}
	})
	err := ValidateServerAgentConfigJSON(raw)
	if err == nil || !strings.Contains(err.Error(), "VoiceChat.Config.LLMConfig.APIKey") {
		t.Fatalf("non-string APIKey error = %v, want field path", err)
	}
}

func TestValidateServerAgentConfigJSONDoesNotExposeCustomLLMAPIKey(t *testing.T) {
	sentinel := strings.Repeat("x", 37)
	raw := mutateCustomLLMScene(t, func(llm map[string]any) {
		llm[customLLMCredentialField] = sentinel
		delete(llm, "Url")
	})
	err := ValidateServerAgentConfigJSON(raw)
	if err == nil || strings.Contains(err.Error(), sentinel) {
		t.Fatalf("missing Url error exposed APIKey: %v", err)
	}
}

func TestValidateServerAgentConfigJSON(t *testing.T) {
	if err := ValidateServerAgentConfigJSON([]byte(validServerAgentConfig)); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}

	tests := map[string]string{
		`{}`: "SceneConfig",
		`{"SceneConfig":{},"VoiceChat":{"Config":{},"AgentConfig":{"UserId":"voice_agent"}}}`: "ASRConfig",
		`{"SceneConfig":{},"VoiceChat":{"Config":{"ASRConfig":{"Provider":"volcano","ProviderParams":{}},"LLMConfig":{"Mode":"legacy"},"TTSConfig":{"Provider":"volcano","ProviderParams":{}}},"AgentConfig":{"UserId":"voice_agent"}}}`:                     "VoiceChat.Config.LLMConfig.Mode",
		`{"SceneConfig":{},"VoiceChat":{"Config":{"ASRConfig":{"Provider":"volcano","ProviderParams":{}},"LLMConfig":{"Mode":"ArkV3","EndPointId":"ep-test"},"TTSConfig":{"Provider":"other","ProviderParams":{}}},"AgentConfig":{"UserId":"voice_agent"}}}`: "TTSConfig",
	}
	for raw, want := range tests {
		if err := ValidateServerAgentConfigJSON([]byte(raw)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("ValidateServerAgentConfigJSON(%s) error = %v, want %q", raw, err, want)
		}
	}
	withoutUserID := `{"SceneConfig":{},"VoiceChat":{"Config":{"ASRConfig":{"Provider":"volcano","ProviderParams":{}},"LLMConfig":{"Mode":"ArkV3","EndPointId":"ep-test"},"TTSConfig":{"Provider":"volcano","ProviderParams":{}}},"AgentConfig":{}}}`
	if err := ValidateServerAgentConfigJSON([]byte(withoutUserID)); err == nil || !strings.Contains(err.Error(), "VoiceChat.AgentConfig.UserId") {
		t.Fatalf("missing UserId error = %v", err)
	}
}

func TestVoiceAgentDefaultsSeeded(t *testing.T) {
	c := Default("demo", "voice-agent", "web")
	// TargetUserID is intentionally left empty so it auto-follows rtc.user_id
	// (agent start falls back to rtc.user_id when target is unset).
	if c.Agent.UserID != "voice_agent" || c.Agent.ConfigFile != "server/scenes/default.json" || c.Agent.TargetUserID != "" {
		t.Fatalf("voice-agent defaults not seeded: %+v", c.Agent)
	}
	if c.Features.MCP {
		t.Fatal("voice-agent quickstart must not enable a business capability by default")
	}
	// A non-agent config must NOT get agent defaults.
	vc := Default("demo", "test-scene", "web")
	if vc.Agent.ConfigFile != "" {
		t.Fatalf("non-agent scene should not seed agent config: %+v", vc.Agent)
	}
}

func TestVoiceAgentValidationWarnsOnUnsetConfigFile(t *testing.T) {
	t.Setenv("RTC_APP_ID", "app")
	c := Default("demo", "voice-agent", "web")
	c.Agent.ConfigFile = "" // unset -> runtime falls back to the default scene
	rep := Validate(c)
	// Unset config_file must not be an error: `agent start` still runs via the
	// The default scene fallback remains available, so validation only warns.
	if rep.HasErrors() {
		t.Fatalf("unset config_file should not error; got %+v", rep.Findings)
	}
	found := false
	for _, f := range rep.Findings {
		if f.Field == "agent.config_file" && f.Severity == SevWarn {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected warn on agent.config_file; got %+v", rep.Findings)
	}
}

func TestVoiceAgentValidationPasses(t *testing.T) {
	t.Setenv("RTC_APP_ID", "app")
	c := Default("demo", "voice-agent", "web")
	c.Agent.ConfigSource = AgentConfigSourceDefault
	if rep := Validate(c); rep.HasErrors() {
		t.Fatalf("expected no errors, got %+v", rep.Findings)
	}
}

func TestTargetUserConsistencyWarn(t *testing.T) {
	t.Setenv("RTC_APP_ID", "app")
	c := Default("demo", "voice-agent", "web")
	c.Agent.TargetUserID = "someone-else" // != rtc.user_id (user-01)
	warned := false
	for _, f := range Validate(c).Findings {
		if f.Field == "agent.target_user_id" && f.Severity == SevWarn {
			warned = true
		}
	}
	if !warned {
		t.Fatalf("expected target_user_id mismatch WARN")
	}
}

func TestAgentFieldSetGet(t *testing.T) {
	c := Default("demo", "voice-agent", "web")
	if err := c.Set("agent.config_file", "custom.json"); err != nil {
		t.Fatal(err)
	}
	v, err := c.Get("agent.config_file")
	if err != nil || v != "custom.json" {
		t.Fatalf("set/get agent field mismatch: %q %v", v, err)
	}
}
