// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package product

import "time"

const (
	DefaultID               = "volcengine"
	DefaultBinaryName       = "vertc"
	DefaultConfigFileName   = "vertc.config.yaml"
	DefaultBusinessID       = "vertc_cli"
	DefaultUserAgentProduct = "vertc"
	DefaultProjectRegion    = "cn-north-1"
	DefaultOpenAPIEndpoint  = "https://rtc.volcengineapi.com"
	DefaultOpenAPIService   = "rtc"
	DefaultOpenAPIRegion    = "cn-north-1"
	DefaultVoiceChatVersion = "2025-06-01"
	DefaultAuthorizeURL     = "https://signin.volcengine.com/authorize/oauth/authorize"
	DefaultTokenURL         = "https://signin.volcengine.com/authorize/oauth/token"
	DefaultOAuthScope       = "Console:All:All"
	DefaultLocalClientID    = "trn:signin:::devtools/same-device"
	DefaultRemoteClientID   = "trn:signin:::devtools/cross-device"
	DefaultAuthNamespace    = "default"
	DefaultSDKVersion       = "4.69.3"
	DefaultDocsMCPEndpoint  = "https://topic.bytedance.com/mcp/rtc"
)

var defaultProfile = Profile{
	ID:                     DefaultID,
	DisplayName:            "VolcEngine",
	BinaryName:             DefaultBinaryName,
	ConfigFileName:         DefaultConfigFileName,
	BusinessID:             DefaultBusinessID,
	UserAgentProduct:       DefaultUserAgentProduct,
	ProjectRegion:          DefaultProjectRegion,
	OpenAPIEndpoint:        DefaultOpenAPIEndpoint,
	OpenAPIService:         DefaultOpenAPIService,
	OpenAPIRegion:          DefaultOpenAPIRegion,
	VoiceChatVersion:       DefaultVoiceChatVersion,
	AgentUserIDField:       "UserId",
	AgentTargetUserIDField: "TargetUserId",
	SDKName:                "@volcengine/rtc",
	SDKVersion:             DefaultSDKVersion,
	DocsMCPEndpoint:        DefaultDocsMCPEndpoint,
	AccessKeyEnv:           "VOLCENGINE_ACCESS_KEY_ID",
	SecretKeyEnv:           "VOLCENGINE_SECRET_ACCESS_KEY",
	SessionTokenEnv:        "VOLCENGINE_SESSION_TOKEN",
	AuthStorageNamespace:   DefaultAuthNamespace,
	OAuthAuthorizeEndpoint: DefaultAuthorizeURL,
	OAuthTokenEndpoint:     DefaultTokenURL,
	OAuthLocalClientID:     DefaultLocalClientID,
	OAuthRemoteClientID:    DefaultRemoteClientID,
	OAuthScope:             DefaultOAuthScope,
	OAuthRemoteRedirectURI: DefaultAuthorizeURL,
	OAuthRefreshSkew:       15 * time.Minute,
	Capabilities: Capabilities{
		RemoteOAuth:       true,
		RTCAppBootstrap:   true,
		BotSceneDiscovery: true,
		AgentUpdate:       true,
		RemoteTemplate:    true,
	},
}

func init() { register(defaultProfile) }
