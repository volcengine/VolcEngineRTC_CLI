// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/template"
)

func validateDevPortOverrides(webChanged bool, webPort int, serverChanged bool, serverPort int) error {
	for _, value := range []struct {
		changed bool
		name    string
		port    int
	}{{webChanged, "--web-port", webPort}, {serverChanged, "--server-port", serverPort}} {
		if value.changed && (value.port < 1 || value.port > 65535) {
			return errs.New("vertc.cli.invalid_flag", errs.TypeValidation,
				"%s must be between 1 and 65535", value.name).WithParam(value.name)
		}
	}
	return nil
}

func preflightDevPorts(ports template.Ports) error {
	for _, endpoint := range []struct {
		role string
		port int
	}{{"web", ports.Web}, {"server", ports.Server}} {
		role, port := endpoint.role, endpoint.port
		if port == 0 {
			continue
		}
		if !devPortAvailable(port) {
			details := map[string]any{"role": role, "port": port}
			message := fmt.Sprintf("%s port %d is already in use", role, port)
			if occupant := inspectDevPortOccupant(port); occupant != nil {
				details["occupant"] = occupant
				message += fmt.Sprintf(" by %s (PID %d) in %s", occupant["process"], occupant["pid"], occupant["directory"])
			}
			return errs.New("vertc.dev.port_in_use", errs.TypePrecondition, "%s", message).
				WithDetails(details).
				WithHint("stop or reconfigure the process using port %d, or pass --%s-port/--auto-port", port, role)
		}
	}
	return nil
}

func inspectDevPortOccupant(port int) map[string]any {
	if _, err := exec.LookPath("lsof"); err != nil {
		return nil
	}
	raw, err := exec.Command("lsof", "-nP", "-iTCP:"+strconv.Itoa(port), "-sTCP:LISTEN", "-Fpc").Output()
	if err != nil {
		return nil
	}
	var pid, process string
	for _, line := range strings.Split(string(raw), "\n") {
		switch {
		case strings.HasPrefix(line, "p") && pid == "":
			pid = strings.TrimPrefix(line, "p")
		case strings.HasPrefix(line, "c") && process == "":
			process = strings.TrimPrefix(line, "c")
		}
	}
	pidNumber, err := strconv.Atoi(pid)
	if err != nil {
		return nil
	}
	directory := devProcessDirectory(pid)
	if process == "" {
		process = "unknown"
	}
	if directory == "" {
		directory = "unknown"
	}
	return map[string]any{"pid": pidNumber, "process": process, "directory": directory}
}

func devProcessDirectory(pid string) string {
	if directory, err := os.Readlink(filepath.Join("/proc", pid, "cwd")); err == nil {
		return directory
	}
	raw, err := exec.Command("lsof", "-a", "-p", pid, "-d", "cwd", "-Fn").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, "n") {
			return strings.TrimPrefix(line, "n")
		}
	}
	return ""
}

func resolveDevPorts(tf *template.Taskfile, webOverride, serverOverride int) (template.Ports, error) {
	resolved := tf.Runtime.Ports
	for _, declared := range []struct {
		name string
		port int
	}{{"runtime.ports.web", resolved.Web}, {"runtime.ports.server", resolved.Server}} {
		if declared.port < 0 || declared.port > 65535 {
			return template.Ports{}, errs.New("vertc.template.contract_invalid", errs.TypeValidation,
				"%s must be between 1 and 65535", declared.name).WithParam(declared.name)
		}
	}
	if resolved.Web > 0 && resolved.Web == resolved.Server {
		return template.Ports{}, errs.New("vertc.template.contract_invalid", errs.TypeValidation,
			"runtime web and server ports must be distinct").WithParam("runtime.ports")
	}
	if tf.Version >= 2 && tf.Runtime.Topology == "web-server" && resolved.Web == 0 && resolved.Server == 0 {
		resolved = template.Ports{Web: 3000, Server: 3001}
	}
	for _, override := range []struct {
		name  string
		value int
		dest  *int
	}{{"--web-port", webOverride, &resolved.Web}, {"--server-port", serverOverride, &resolved.Server}} {
		if override.value == 0 {
			continue
		}
		if override.value < 1 || override.value > 65535 {
			return template.Ports{}, errs.New("vertc.cli.invalid_flag", errs.TypeValidation,
				"%s must be between 1 and 65535", override.name).WithParam(override.name)
		}
		*override.dest = override.value
	}
	if resolved.Web > 0 && resolved.Web == resolved.Server {
		return template.Ports{}, errs.New("vertc.cli.invalid_flag", errs.TypeValidation,
			"web and server ports must be distinct").WithParam("--web-port/--server-port")
	}
	return resolved, nil
}

func devPortEnv(ports template.Ports) []string {
	var values []string
	if ports.Web > 0 {
		values = append(values, "VERTC_WEB_PORT="+strconv.Itoa(ports.Web))
	}
	if ports.Server > 0 {
		values = append(values, "VERTC_SERVER_PORT="+strconv.Itoa(ports.Server))
	}
	return values
}

// prepareDevPortTaskEnv bridges the taskfile's role-specific ports to the
// conventional PORT variables consumed by the pinned Node template. NODE_OPTIONS
// makes the mapping apply to the web/server child processes without modifying
// user files or relying on a particular package-manager command line.
func prepareDevPortTaskEnv(ports template.Ports) ([]string, func(), error) {
	values := devPortEnv(ports)
	if ports.Web == 0 && ports.Server == 0 {
		return values, func() {}, nil
	}
	file, err := os.CreateTemp("", "vertc-dev-ports-*.cjs")
	if err != nil {
		return nil, nil, errs.New("vertc.dev.task_failed", errs.TypeIO,
			"prepare dev port environment: %s", err).WithCause(err)
	}
	cleanup := func() { _ = os.Remove(file.Name()) }
	script := `const path = require("node:path");
const role = path.basename(process.cwd()).toLowerCase();
if (role === "web" && process.env.VERTC_WEB_PORT) {
  process.env.PORT = process.env.VERTC_WEB_PORT;
  if (process.env.VERTC_SERVER_PORT) process.env.REACT_APP_AIGC_PROXY_HOST = "http://127.0.0.1:" + process.env.VERTC_SERVER_PORT;
}
if (role === "server" && process.env.VERTC_SERVER_PORT) process.env.PORT = process.env.VERTC_SERVER_PORT;
`
	if _, err := file.WriteString(script); err != nil {
		_ = file.Close()
		cleanup()
		return nil, nil, errs.New("vertc.dev.task_failed", errs.TypeIO,
			"prepare dev port environment: %s", err).WithCause(err)
	}
	if err := file.Close(); err != nil {
		cleanup()
		return nil, nil, errs.New("vertc.dev.task_failed", errs.TypeIO,
			"prepare dev port environment: %s", err).WithCause(err)
	}
	requireOption := "--require=" + strconv.Quote(file.Name())
	if existing := strings.TrimSpace(os.Getenv("NODE_OPTIONS")); existing != "" {
		requireOption = existing + " " + requireOption
	}
	return append(values, "NODE_OPTIONS="+requireOption), cleanup, nil
}

func devRuntimeData(dir, status string, ports template.Ports) map[string]any {
	data := map[string]any{"dir": dir, "status": status}
	portValues := map[string]int{}
	urls := map[string]string{}
	if ports.Web > 0 {
		portValues["web"] = ports.Web
		urls["web"] = "http://localhost:" + strconv.Itoa(ports.Web)
	}
	if ports.Server > 0 {
		portValues["server"] = ports.Server
		urls["server"] = "http://localhost:" + strconv.Itoa(ports.Server)
	}
	if len(portValues) > 0 {
		data["ports"] = portValues
		data["urls"] = urls
	}
	return data
}

func autoSelectDevPorts(ports template.Ports) (template.Ports, error) {
	excluded := map[int]bool{}
	if ports.Web > 0 {
		excluded[ports.Web] = true
	}
	if ports.Server > 0 {
		excluded[ports.Server] = true
	}
	for _, port := range []*int{&ports.Web, &ports.Server} {
		if *port == 0 || devPortAvailable(*port) {
			continue
		}
		selected, err := findAvailableDevPort(excluded)
		if err != nil {
			return template.Ports{}, err
		}
		*port = selected
		excluded[selected] = true
	}
	return ports, nil
}

func devPortAvailable(port int) bool {
	for _, host := range []string{"127.0.0.1", "::1"} {
		listener, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
		if err != nil {
			// IPv6 can be disabled on the host. Only treat an IPv6 bind failure as
			// an occupied port when the loopback family itself is available.
			if host == "::1" {
				probe, probeErr := net.Listen("tcp", "[::1]:0")
				if probeErr != nil {
					continue
				}
				_ = probe.Close()
			}
			return false
		}
		_ = listener.Close()
	}
	return true
}

func findAvailableDevPort(excluded map[int]bool) (int, error) {
	for range 10 {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return 0, errs.New("vertc.dev.port_in_use", errs.TypePrecondition,
				"cannot select an available local port: %s", err).WithCause(err)
		}
		port := listener.Addr().(*net.TCPAddr).Port
		_ = listener.Close()
		if !excluded[port] {
			return port, nil
		}
	}
	return 0, errs.New("vertc.dev.port_in_use", errs.TypePrecondition,
		"cannot select a distinct available local port")
}
