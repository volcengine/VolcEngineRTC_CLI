#!/usr/bin/env bash
# Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
# SPDX-License-Identifier: MIT

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/.." && pwd)"
export GOCACHE="${GOCACHE:-/tmp/rtc-cli-go-cache}"

"$script_dir/prepare-release-version.sh" --version v0.0.1 --dry-run >/dev/null

if "$script_dir/prepare-release-version.sh" --version 9.9.9 --check >/dev/null 2>&1; then
  echo "prepare-release-version-test: mismatched source unexpectedly passed check mode" >&2
  exit 1
fi

if "$script_dir/prepare-release-version.sh" --version 0.0.0-dev --dry-run >/dev/null 2>&1; then
  echo "prepare-release-version-test: synthetic dev version was accepted" >&2
  exit 1
fi
if "$script_dir/prepare-release-version.sh" --version 0.0.1 --check --dry-run >/dev/null 2>&1; then
  echo "prepare-release-version-test: conflicting modes were accepted" >&2
  exit 1
fi

echo "prepare-release-version-test: passed"
