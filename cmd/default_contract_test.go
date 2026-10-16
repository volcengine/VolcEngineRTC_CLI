// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/auth"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/config"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/doctor"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/meta"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/openapi"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/product"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/template"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/topicdocs"
)

type defaultContractFixture struct {
	SchemaVersion  int      `json:"schema_version"`
	ProtectedPaths []string `json:"protected_paths"`
	Identity       struct {
		DisplayName      string `json:"display_name"`
		Binary           string `json:"binary"`
		Module           string `json:"module"`
		ConfigFile       string `json:"config_file"`
		BusinessID       string `json:"business_id"`
		UserAgentProduct string `json:"user_agent_product"`
	} `json:"identity"`
	Commands struct {
		Visible []string `json:"visible"`
		Hidden  []string `json:"hidden"`
	} `json:"commands"`
	Config struct {
		ProjectRegion   string `json:"project_region"`
		OpenAPIEndpoint string `json:"openapi_endpoint"`
		OpenAPIService  string `json:"openapi_service"`
		OpenAPIRegion   string `json:"openapi_region"`
		RTCAppID        string `json:"rtc_app_id"`
		RTCRoomID       string `json:"rtc_room_id"`
		RTCUserID       string `json:"rtc_user_id"`
		AgentUserID     string `json:"agent_user_id"`
		AgentConfigFile string `json:"agent_config_file"`
	} `json:"config"`
	Authentication struct {
		AuthorizeEndpoint string `json:"authorize_endpoint"`
		TokenEndpoint     string `json:"token_endpoint"`
		LocalClientID     string `json:"local_client_id"`
		RemoteClientID    string `json:"remote_client_id"`
		Scope             string `json:"scope"`
		RemoteRedirectURI string `json:"remote_redirect_uri"`
		StorageNamespace  string `json:"storage_namespace"`
	} `json:"authentication"`
	OpenAPI struct {
		Endpoint               string `json:"endpoint"`
		Service                string `json:"service"`
		Region                 string `json:"region"`
		VoiceChatVersion       string `json:"voicechat_version"`
		AgentUserIDField       string `json:"agent_user_id_field"`
		AgentTargetUserIDField string `json:"agent_target_user_id_field"`
	} `json:"openapi"`
	VoiceChat struct {
		StartAction      string `json:"start_action"`
		UpdateAction     string `json:"update_action"`
		StopAction       string `json:"stop_action"`
		StartRequestJSON string `json:"start_request_json"`
	} `json:"voicechat"`
	Template struct {
		Scene      string `json:"scene"`
		Platform   string `json:"platform"`
		SDKName    string `json:"sdk_name"`
		SDKVersion string `json:"sdk_version"`
		Repository string `json:"repository"`
		Commit     string `json:"commit"`
		SHA256     string `json:"sha256"`
	} `json:"template"`
	DoctorCheckIDs  []string `json:"doctor_check_ids"`
	DocsMCPEndpoint string   `json:"docs_mcp_endpoint"`
}

func loadDefaultContractFixture(t *testing.T) defaultContractFixture {
	t.Helper()
	path := filepath.Join("..", "internal", "product", "testdata", "default-contract.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fixture defaultContractFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.SchemaVersion != 1 {
		t.Fatalf("fixture schema_version = %d", fixture.SchemaVersion)
	}
	return fixture
}

func TestDefaultContractFixtureMatchesDevBehavior(t *testing.T) {
	fixture := loadDefaultContractFixture(t)
	root := NewRootCmd()
	root.InitDefaultHelpCmd()
	root.InitDefaultCompletionCmd()
	var visible, hidden []string
	for _, command := range root.Commands() {
		if command.Hidden {
			hidden = append(hidden, command.Name())
		} else {
			visible = append(visible, command.Name())
		}
	}
	sort.Strings(visible)
	sort.Strings(hidden)
	if !reflect.DeepEqual(visible, fixture.Commands.Visible) || !reflect.DeepEqual(hidden, fixture.Commands.Hidden) {
		t.Fatalf("command contract drifted: visible=%v hidden=%v", visible, hidden)
	}
	if fixture.Identity.Binary != meta.BinName || fixture.Identity.ConfigFile != meta.ConfigFileName ||
		fixture.Identity.BusinessID != meta.BusinessID || fixture.Identity.UserAgentProduct != meta.UserAgentProduct {
		t.Fatalf("identity contract drifted: %+v", fixture.Identity)
	}
	profile := product.Current()
	if profile.DisplayName != fixture.Identity.DisplayName || profile.BinaryName != fixture.Identity.Binary ||
		profile.ConfigFileName != fixture.Identity.ConfigFile || profile.BusinessID != fixture.Identity.BusinessID ||
		profile.UserAgentProduct != fixture.Identity.UserAgentProduct || profile.AuthStorageNamespace != fixture.Authentication.StorageNamespace ||
		profile.AgentUserIDField != fixture.OpenAPI.AgentUserIDField || profile.AgentTargetUserIDField != fixture.OpenAPI.AgentTargetUserIDField {
		t.Fatalf("profile contract drifted: %+v", profile)
	}
	goMod, err := os.ReadFile(filepath.Join("..", "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	goMod = bytes.ReplaceAll(goMod, []byte("\r\n"), []byte("\n"))
	if !strings.HasPrefix(string(goMod), "module "+fixture.Identity.Module+"\n") {
		t.Fatalf("module identity drifted: %s", goMod)
	}
	cfg := config.Default("fixture", "voice-agent", "web")
	if cfg.Project.Region != fixture.Config.ProjectRegion || cfg.OpenAPI.Endpoint != fixture.Config.OpenAPIEndpoint ||
		cfg.OpenAPI.Service != fixture.Config.OpenAPIService || cfg.OpenAPI.Region != fixture.Config.OpenAPIRegion ||
		cfg.RTC.AppID != fixture.Config.RTCAppID || cfg.RTC.RoomID != fixture.Config.RTCRoomID ||
		cfg.RTC.UserID != fixture.Config.RTCUserID || cfg.Agent.UserID != fixture.Config.AgentUserID ||
		cfg.Agent.ConfigFile != fixture.Config.AgentConfigFile {
		t.Fatalf("config contract drifted: %+v", cfg)
	}
	if auth.DefaultAuthorizeEndpoint != fixture.Authentication.AuthorizeEndpoint ||
		auth.DefaultTokenEndpoint != fixture.Authentication.TokenEndpoint || auth.DefaultLocalClientID != fixture.Authentication.LocalClientID ||
		auth.DefaultRemoteClientID != fixture.Authentication.RemoteClientID || auth.DefaultScope != fixture.Authentication.Scope ||
		auth.DefaultRemoteRedirectURI != fixture.Authentication.RemoteRedirectURI {
		t.Fatalf("authentication contract drifted: %+v", fixture.Authentication)
	}
	if openapi.DefaultEndpoint != fixture.OpenAPI.Endpoint || openapi.DefaultService != fixture.OpenAPI.Service ||
		openapi.DefaultRegion != fixture.OpenAPI.Region || openapi.DefaultVersion != fixture.OpenAPI.VoiceChatVersion {
		t.Fatalf("OpenAPI contract drifted: %+v", fixture.OpenAPI)
	}
	tmpl, ok := template.Find(fixture.Template.Scene, fixture.Template.Platform)
	if !ok || tmpl.Remote == nil || tmpl.SDK.Name != fixture.Template.SDKName || tmpl.SDK.Version != fixture.Template.SDKVersion ||
		tmpl.Remote.Repository != fixture.Template.Repository || tmpl.Remote.Commit != fixture.Template.Commit || tmpl.Remote.SHA256 != fixture.Template.SHA256 {
		t.Fatalf("template contract drifted: %+v", tmpl)
	}
	if topicdocs.ProductionEndpoint != fixture.DocsMCPEndpoint {
		t.Fatalf("docs endpoint = %q", topicdocs.ProductionEndpoint)
	}
	checks := doctor.RunCLI()
	checkIDs := make([]string, 0, len(checks))
	for _, check := range checks {
		checkIDs = append(checkIDs, check.ID)
	}
	if !reflect.DeepEqual(checkIDs, fixture.DoctorCheckIDs) {
		t.Fatalf("doctor check IDs drifted: %v", checkIDs)
	}
	for _, protected := range fixture.ProtectedPaths {
		if _, err := os.Stat(filepath.Join("..", protected)); err != nil {
			t.Errorf("protected path %q: %v", protected, err)
		}
	}
}

func TestDefaultVoiceChatWireFixture(t *testing.T) {
	fixture := loadDefaultContractFixture(t)
	req := openapi.StartVoiceChatRequest{
		AppID: "app", BusinessID: meta.BusinessID, RoomID: "room", TaskID: "task",
		Config:      json.RawMessage(`{"ASRConfig":{}}`),
		AgentConfig: map[string]any{"TargetUserId": []string{"user"}, "UserId": "agent"},
	}
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != fixture.VoiceChat.StartRequestJSON {
		t.Fatalf("VoiceChat request drifted:\n got: %s\nwant: %s", raw, fixture.VoiceChat.StartRequestJSON)
	}
}
