// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"encoding/json"
	"fmt"
	"strings"
)

type STSCredential struct {
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	SessionToken    string `json:"session_token"`
}

type stsCredentialWire struct {
	AccessKeyID        string `json:"access_key_id"`
	AccessKeyIDAlt     string `json:"AccessKeyID"`
	AccessKeyIDAlt2    string `json:"AccessKeyId"`
	SecretAccessKey    string `json:"secret_access_key"`
	SecretAccessKeyAlt string `json:"SecretAccessKey"`
	SessionToken       string `json:"session_token"`
	SessionTokenAlt    string `json:"SessionToken"`
}

func ParseSTSCredential(serialized string) (STSCredential, error) {
	if strings.TrimSpace(serialized) == "" {
		return STSCredential{}, fmt.Errorf("sts access_token is empty")
	}
	var wire stsCredentialWire
	if err := json.Unmarshal([]byte(serialized), &wire); err != nil {
		return STSCredential{}, fmt.Errorf("parse sts access_token: %w", err)
	}
	credential := STSCredential{
		AccessKeyID:     firstNonEmpty(wire.AccessKeyID, wire.AccessKeyIDAlt, wire.AccessKeyIDAlt2),
		SecretAccessKey: firstNonEmpty(wire.SecretAccessKey, wire.SecretAccessKeyAlt),
		SessionToken:    firstNonEmpty(wire.SessionToken, wire.SessionTokenAlt),
	}
	if strings.TrimSpace(credential.AccessKeyID) == "" {
		return STSCredential{}, fmt.Errorf("sts access_token missing access_key_id")
	}
	if strings.TrimSpace(credential.SecretAccessKey) == "" {
		return STSCredential{}, fmt.Errorf("sts access_token missing secret_access_key")
	}
	if strings.TrimSpace(credential.SessionToken) == "" {
		return STSCredential{}, fmt.Errorf("sts access_token missing session_token")
	}
	return credential, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
