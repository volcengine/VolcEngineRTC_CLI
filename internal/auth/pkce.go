// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

const CodeChallengeMethodS256 = "S256"

func NewPKCEPair() (verifier string, challenge string, err error) {
	verifier, err = RandomURLSafe(64)
	if err != nil {
		return "", "", err
	}
	return verifier, CodeChallengeS256(verifier), nil
}

func CodeChallengeS256(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func RandomURLSafe(byteLen int) (string, error) {
	if byteLen <= 0 {
		return "", fmt.Errorf("byteLen must be positive")
	}
	data := make([]byte, byteLen)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}
