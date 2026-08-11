// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"net"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/template"
)

func TestResolveDevPortsRejectsInvalidTaskfilePort(t *testing.T) {
	tf := &template.Taskfile{Version: 2}
	tf.Runtime.Ports.Web = 70000
	_, err := resolveDevPorts(tf, 0, 0)
	typed, ok := errs.As(err)
	if !ok || typed.Code != "vertc.template.contract_invalid" || typed.Param != "runtime.ports.web" {
		t.Fatalf("error = %v, want invalid runtime.ports.web", err)
	}
}

func TestPrepareDevPortTaskEnvWritesNodeRoleBridge(t *testing.T) {
	t.Setenv("NODE_OPTIONS", "")
	env, cleanup, err := prepareDevPortTaskEnv(template.Ports{Web: 3002, Server: 3001})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	var option string
	for _, value := range env {
		if strings.HasPrefix(value, "NODE_OPTIONS=--require=") {
			option = strings.TrimPrefix(value, "NODE_OPTIONS=--require=")
		}
	}
	path, err := strconv.Unquote(option)
	if err != nil {
		t.Fatalf("NODE_OPTIONS require path %q: %v", option, err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	script := string(raw)
	for _, expected := range []string{
		`role === "web"`,
		`process.env.PORT = process.env.VERTC_WEB_PORT`,
		`process.env.REACT_APP_AIGC_PROXY_HOST = "http://127.0.0.1:" + process.env.VERTC_SERVER_PORT`,
		`role === "server"`,
		`process.env.PORT = process.env.VERTC_SERVER_PORT`,
	} {
		if !strings.Contains(script, expected) {
			t.Fatalf("Node port bridge missing %q:\n%s", expected, script)
		}
	}
}

func TestDevPortAvailableRejectsIPv6Listener(t *testing.T) {
	listener, err := net.Listen("tcp", "[::1]:0")
	if err != nil {
		t.Skipf("IPv6 loopback unavailable: %v", err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	if devPortAvailable(port) {
		t.Fatalf("port %d reported available while IPv6 listener is active", port)
	}
}

func TestResolveDevPortsRejectsDuplicateOverrides(t *testing.T) {
	tf := &template.Taskfile{Version: 2}
	_, err := resolveDevPorts(tf, 3002, 3002)
	typed, ok := errs.As(err)
	if !ok || typed.Code != "vertc.cli.invalid_flag" {
		t.Fatalf("error = %v, want invalid port flags", err)
	}
}
