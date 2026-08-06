#!/usr/bin/env bash
# Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
# SPDX-License-Identifier: MIT

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/.." && pwd)"
goreleaser="${GORELEASER:-$(go env GOPATH)/bin/goreleaser}"
[[ -x "$goreleaser" ]] || { echo "release-snapshot-integration-test: missing goreleaser" >&2; exit 1; }

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
output="$tmp/dist/"
before="$(git -C "$repo_root" status --porcelain=v1 --untracked-files=all)"
GORELEASER="$goreleaser" "$repo_root/scripts/build-release-artifacts.sh" --stability snapshot --publication none --output "$output" >/dev/null
after="$(git -C "$repo_root" status --porcelain=v1 --untracked-files=all)"
[[ "$after" == "$before" ]]

version="$(find "$output" -maxdepth 1 -type f -name 'vertc_*_darwin_amd64.tar.gz' -exec basename {} \; | sed -E 's/^vertc_(.*)_darwin_amd64[.]tar[.]gz$/\1/')"
[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+-snapshot$ ]]
archive_count="$(find "$output" -maxdepth 1 -type f \( -name "vertc_${version}_*.tar.gz" -o -name "vertc_${version}_*.zip" \) | wc -l | tr -d ' ')"
[[ "$archive_count" -eq 6 ]]
[[ -s "$output/checksums.txt" ]]

echo "release-snapshot-integration-test: passed ($version)"
