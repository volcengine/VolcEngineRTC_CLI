// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"fmt"
	"os/exec"
	"runtime"
)

func OpenBrowser(targetURL string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", targetURL)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", targetURL)
	case "linux":
		cmd = exec.Command("xdg-open", targetURL)
	default:
		return fmt.Errorf("opening browser is unsupported on %s", runtime.GOOS)
	}
	return cmd.Start()
}
