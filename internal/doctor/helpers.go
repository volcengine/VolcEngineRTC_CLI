// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package doctor

import (
	"github.com/volcengine/VolcEngineRTC_CLI/internal/auth"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/template"
)

// authed reports whether a local auth session exists.
func authed() bool { return auth.IsAuthenticated() }

// loadSDK returns the pinned SDK version from a project's taskfile, if present.
func loadSDK(dir string) (string, error) {
	if dir == "" {
		return "", nil
	}
	tf, err := template.LoadTaskfile(dir)
	if err != nil {
		return "", err
	}
	if tf.SDK.Name != "" && tf.SDK.Version != "" {
		return tf.SDK.Name + "@" + tf.SDK.Version, nil
	}
	return "", nil
}
