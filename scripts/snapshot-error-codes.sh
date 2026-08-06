#!/usr/bin/env bash
# Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
# SPDX-License-Identifier: MIT

# CI check for the CLI error.code catalog using a committed snapshot. Fails when:
#   - a catalog entry is malformed or duplicated,
#   - a vertc.* error code is emitted in source but not registered,
#   - the catalog drifts from testdata/error-codes.snapshot.
#
# Regenerate the snapshot intentionally with:
#   UPDATE_SNAPSHOT=1 go test ./internal/errs/ -run TestCatalogSnapshot
set -euo pipefail
cd "$(dirname "$0")/.."

echo "==> validating error.code catalog"
go test ./internal/errs/ -run 'TestCatalog|TestAllEmittedCodesRegistered' -count=1
echo "OK: error.code catalog is consistent"
