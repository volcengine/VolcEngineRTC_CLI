// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

//go:build !topicdocs_e2e

package topicdocs

func newDefaultClient(version string, options ...Option) (*Client, error) {
	if err := validateProductionEndpoint(productionEndpoint); err != nil {
		return nil, err
	}
	return newClient(productionEndpoint, version, nil, options...)
}
