// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package template

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadTaskfileV1KeepsLegacyRuntime(t *testing.T) {
	dir := writeTaskfile(t, `
version: 1
scene: voice-agent
platform: web
sdk:
  name: "@volcengine/rtc"
  version: "4.68.1"
tasks:
  dev: ["npm run dev"]
`)
	tf, err := LoadTaskfile(dir)
	if err != nil {
		t.Fatal(err)
	}
	if tf.ServerManagedAgent() {
		t.Fatal("v1 taskfile must retain CLI-managed agent behavior")
	}
	if len(tf.Tasks.Dev) != 1 || tf.Tasks.Dev[0] != "npm run dev" {
		t.Fatalf("v1 dev tasks not parsed: %+v", tf.Tasks.Dev)
	}
}

func TestLoadTaskfileV2ParsesServerManagedRuntime(t *testing.T) {
	dir := writeTaskfile(t, `
version: 2
scene: voice-agent
platform: web
runtime:
  topology: web-server
  agent_control: server
  ports:
    web: 3000
    server: 3001
tasks:
  dev: ["pnpm dev"]
`)
	tf, err := LoadTaskfile(dir)
	if err != nil {
		t.Fatal(err)
	}
	if tf.Runtime.Topology != "web-server" || !tf.ServerManagedAgent() {
		t.Fatalf("v2 runtime not parsed: %+v", tf.Runtime)
	}
	if tf.Runtime.Ports.Web != 3000 || tf.Runtime.Ports.Server != 3001 {
		t.Fatalf("v2 runtime ports not parsed: %+v", tf.Runtime.Ports)
	}
}

func TestTaskfileV1DoesNotOptInViaUnknownRuntimeFields(t *testing.T) {
	dir := writeTaskfile(t, `
version: 1
scene: voice-agent
platform: web
runtime:
  topology: web-server
  agent_control: server
`)
	tf, err := LoadTaskfile(dir)
	if err != nil {
		t.Fatal(err)
	}
	if tf.ServerManagedAgent() {
		t.Fatal("only taskfile v2 may opt into server-managed agent control")
	}
}

func writeTaskfile(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, TaskfileName), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}
