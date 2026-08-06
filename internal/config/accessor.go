// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"sort"
	"strings"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
)

// scalarFields maps dotted paths to string get/set accessors for the
// settable scalar fields (config get/set operate on these).
type scalarField struct {
	get func(*Config) string
	set func(*Config, string)
}

var scalarFields = map[string]scalarField{
	"project.name":         {func(c *Config) string { return c.Project.Name }, func(c *Config, v string) { c.Project.Name = v }},
	"project.scene":        {func(c *Config) string { return c.Project.Scene }, func(c *Config, v string) { c.Project.Scene = v }},
	"project.platform":     {func(c *Config) string { return c.Project.Platform }, func(c *Config, v string) { c.Project.Platform = v }},
	"project.region":       {func(c *Config) string { return c.Project.Region }, func(c *Config, v string) { c.Project.Region = v }},
	"rtc.app_id":           {func(c *Config) string { return c.RTC.AppID }, func(c *Config, v string) { c.RTC.AppID = v }},
	"rtc.room_id":          {func(c *Config) string { return c.RTC.RoomID }, func(c *Config, v string) { c.RTC.RoomID = v }},
	"rtc.user_id":          {func(c *Config) string { return c.RTC.UserID }, func(c *Config, v string) { c.RTC.UserID = v }},
	"rtc.token":            {func(c *Config) string { return c.RTC.Token }, func(c *Config, v string) { c.RTC.Token = v }},
	"agent.task_id":        {func(c *Config) string { return c.Agent.TaskID }, func(c *Config, v string) { c.Agent.TaskID = v }},
	"agent.user_id":        {func(c *Config) string { return c.Agent.UserID }, func(c *Config, v string) { c.Agent.UserID = v }},
	"agent.target_user_id": {func(c *Config) string { return c.Agent.TargetUserID }, func(c *Config, v string) { c.Agent.TargetUserID = v }},
	"agent.config_file":    {func(c *Config) string { return c.Agent.ConfigFile }, func(c *Config, v string) { c.Agent.ConfigFile = v }},
	"agent.config_source":  {func(c *Config) string { return c.Agent.ConfigSource }, func(c *Config, v string) { c.Agent.ConfigSource = v }},
	"openapi.endpoint":     {func(c *Config) string { return c.OpenAPI.Endpoint }, func(c *Config, v string) { c.OpenAPI.Endpoint = v }},
	"openapi.service":      {func(c *Config) string { return c.OpenAPI.Service }, func(c *Config, v string) { c.OpenAPI.Service = v }},
	"openapi.region":       {func(c *Config) string { return c.OpenAPI.Region }, func(c *Config, v string) { c.OpenAPI.Region = v }},
}

// SettableFields returns the sorted list of settable dotted paths.
func SettableFields() []string {
	keys := make([]string, 0, len(scalarFields))
	for k := range scalarFields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Get reads a scalar field by dotted path.
func (c *Config) Get(path string) (string, error) {
	f, ok := scalarFields[path]
	if !ok {
		return "", unknownFieldErr(path)
	}
	return f.get(c), nil
}

// Set writes a scalar field by dotted path.
func (c *Config) Set(path, value string) error {
	f, ok := scalarFields[path]
	if !ok {
		return unknownFieldErr(path)
	}
	f.set(c, value)
	return nil
}

func unknownFieldErr(path string) error {
	return errs.New("vertc.config.invalid_field", errs.TypeValidation,
		"unknown config field %q", path).WithParam(path).
		WithHint("settable fields: %s", strings.Join(SettableFields(), ", "))
}
