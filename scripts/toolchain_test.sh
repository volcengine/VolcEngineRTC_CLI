#!/usr/bin/env bash
# Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
# SPDX-License-Identifier: MIT

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

mkdir -p "$tmp/bin"
capture="$tmp/gotoolchain"

printf '%s\n' \
  '#!/usr/bin/env bash' \
  'set -euo pipefail' \
  'if [[ "${1:-}" == "install" ]]; then' \
  '  printf "%s\n" "${GOTOOLCHAIN:-}" >"$TOOLCHAIN_CAPTURE"' \
  '  exit 0' \
  'fi' \
  'echo "unexpected fake go invocation: $*" >&2' \
  'exit 1' >"$tmp/bin/go"
chmod +x "$tmp/bin/go"

PATH="$tmp/bin:$PATH" TOOLCHAIN_CAPTURE="$capture" make -s -C "$repo_root" tools

expected="go$(awk '/^go / { print $2; exit }' "$repo_root/go.mod")"
actual="$(cat "$capture")"
[[ "$actual" == "$expected" ]] || {
  echo "tools used GOTOOLCHAIN=${actual:-<unset>}; expected $expected" >&2
  exit 1
}

echo "toolchain tests: passed"
