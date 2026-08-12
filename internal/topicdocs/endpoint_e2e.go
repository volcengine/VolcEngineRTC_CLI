// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

//go:build topicdocs_e2e

package topicdocs

// e2eEndpoint exists only in explicitly tagged test binaries. Ordinary and
// release builds compile endpoint_default.go and have no endpoint variable for
// linker injection.
var e2eEndpoint string

func newDefaultClient(version string, options ...Option) (*Client, error) {
	return newClient(e2eEndpoint, version, nil, options...)
}
