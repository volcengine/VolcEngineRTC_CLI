// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

// Package config defines and manages the unified vertc.config.yaml, the single
// source of truth. init generates it, token backfills it, doctor
// validates it, and dev reads it.
package config

// SchemaVersion is the current config schema version.
const SchemaVersion = 1

// Config mirrors vertc.config.yaml. String fields may hold `${ENV}` placeholders
// which are preserved on disk and expanded via Resolve(); AppKey is never stored
// here — it lives only in the environment.
type Config struct {
	Version  int      `yaml:"version"`
	Project  Project  `yaml:"project"`
	RTC      RTC      `yaml:"rtc"`
	Agent    Agent    `yaml:"agent"`
	Features Features `yaml:"features"`
	OpenAPI  OpenAPI  `yaml:"openapi,omitempty"`
}

// Project identifies the scaffolded project and its scene/platform axes.
type Project struct {
	Name     string `yaml:"name"`
	Scene    string `yaml:"scene"`
	Platform string `yaml:"platform"`
	Region   string `yaml:"region"`
}

// RTC holds the room-join identifiers. app_id/room_id/user_id live here; AppKey
// is an env-only credential and is intentionally absent. Token is backfilled by
// `token issue --write`.
type RTC struct {
	AppID  string `yaml:"app_id"`
	RoomID string `yaml:"room_id"`
	UserID string `yaml:"user_id"`
	Token  string `yaml:"token,omitempty"`
}

// Agent holds the conversational-agent identity and a reference to its
// server-owned StartVoiceChat configuration. OpenAPI credentials
// never live here; quick-start requests stay inside the CLI auth boundary.
type Agent struct {
	UserID       string `yaml:"user_id"`        // AI's id (AgentConfig.UserId)
	TargetUserID string `yaml:"target_user_id"` // the real user (== rtc.user_id)
	TaskID       string `yaml:"task_id,omitempty"`
	ConfigFile   string `yaml:"config_file"`             // server-owned AI configuration
	ConfigSource string `yaml:"config_source,omitempty"` // template-default or console-bot after first dev setup
}

const (
	AgentConfigSourceDefault = "template-default"
	AgentConfigSourceBot     = "console-bot"
)

// Features toggles optional capabilities.
type Features struct {
	Subtitles bool `yaml:"subtitles"`
	Interrupt bool `yaml:"interrupt"`
	MCP       bool `yaml:"mcp"`
}

// OpenAPI holds defaults for signed VolcEngine OpenAPI calls.
type OpenAPI struct {
	Endpoint string `yaml:"endpoint,omitempty"`
	Service  string `yaml:"service,omitempty"`
	Region   string `yaml:"region,omitempty"`
}

const (
	DefaultOpenAPIEndpoint = "https://rtc.volcengineapi.com"
	DefaultOpenAPIService  = "rtc"
	DefaultOpenAPIRegion   = "cn-north-1"
)

// Default returns a config pre-populated with `${ENV}` placeholders and the
// given project identity, suitable for scaffolding.
func Default(name, scene, platform string) *Config {
	c := &Config{
		Version: SchemaVersion,
		Project: Project{
			Name:     name,
			Scene:    scene,
			Platform: platform,
			Region:   "cn-north-1",
		},
		RTC: RTC{
			AppID:  "${RTC_APP_ID}",
			RoomID: "room-01",
			UserID: "user-01",
		},
		Agent:    Agent{},
		Features: Features{},
		OpenAPI: OpenAPI{
			Endpoint: DefaultOpenAPIEndpoint,
			Service:  DefaultOpenAPIService,
			Region:   DefaultOpenAPIRegion,
		},
	}
	if scene == "voice-agent" {
		c.Agent = Agent{
			UserID: "voice_agent", // matches the generated server AI config
			// TargetUserID left empty so it auto-follows rtc.user_id: agent start
			// falls back to rtc.user_id when target is unset, and doctor's
			// consistency check only warns on a non-empty mismatch. This keeps the
			// AI's conversation partner aligned with the web user even after
			// `token issue --user-id ... --write` writes through a new rtc.user_id.
			TargetUserID: "",
			ConfigFile:   "server/scenes/default.json", // server-owned AI scene configuration
		}
		c.Features.Subtitles = true
		c.Features.Interrupt = true
	}
	return c
}
