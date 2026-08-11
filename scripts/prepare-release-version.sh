#!/usr/bin/env bash
# Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
# SPDX-License-Identifier: MIT

# Prepare or verify the real version committed before a public release.
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/.." && pwd)"
version=""
mode="write"

usage() {
  cat <<'EOF'
Usage: ./scripts/prepare-release-version.sh --version X.Y.Z[-ID] [--dry-run|--check]

Updates package.json and every official skills/*/SKILL.md to the exact public
release version. The default write mode requires a clean Git worktree and does
not commit, push, tag, or publish anything.

Options:
  --version VERSION  stable or prerelease version, with optional leading v
  --dry-run          list files that would change without writing
  --check            require every target to already match without writing
  -h, --help         show this help
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version) version="$2"; shift 2 ;;
    --dry-run) [[ "$mode" == "write" ]] || { echo "prepare-release-version: select only one mode" >&2; exit 2; }; mode="dry-run"; shift ;;
    --check) [[ "$mode" == "write" ]] || { echo "prepare-release-version: select only one mode" >&2; exit 2; }; mode="check"; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "prepare-release-version: unknown argument: $1" >&2; usage >&2; exit 2 ;;
  esac
done

[[ -n "$version" ]] || { echo "prepare-release-version: --version is required" >&2; exit 2; }
command -v go >/dev/null 2>&1 || { echo "prepare-release-version: go is required" >&2; exit 1; }
command -v git >/dev/null 2>&1 || { echo "prepare-release-version: git is required" >&2; exit 1; }

if [[ "$mode" == "write" ]]; then
  status="$(git -C "$repo_root" status --porcelain=v1 --untracked-files=all)"
  [[ -z "$status" ]] || {
    echo "prepare-release-version: refusing to modify a dirty worktree" >&2
    exit 1
  }
fi

args=(source-version --repo "$repo_root" --version "$version")
case "$mode" in
  dry-run) args+=(--dry-run) ;;
  check) args+=(--check) ;;
esac
go run "$repo_root/internal/releasecmd" "${args[@]}"
