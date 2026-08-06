#!/usr/bin/env bash
# Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
# SPDX-License-Identifier: MIT

set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
check="$root/scripts/check-change-contract.sh"

expect_pass() {
  CHANGE_CONTRACT_CHANGED_FILES="$1" "$check" >/dev/null
}

expect_fail() {
  if CHANGE_CONTRACT_CHANGED_FILES="$1" "$check" >/dev/null 2>&1; then
    echo "expected contract check to fail for: $1" >&2
    exit 1
  fi
}

expect_pass $'cmd/root.go\ncmd/root_test.go'
expect_fail 'cmd/root.go'
expect_pass $'internal/errs/catalog.go\ninternal/errs/testdata/error-codes.snapshot'
# Catalog metadata (for example, an error description) can change without
# changing the stable error.code set. The catalog snapshot test is responsible
# for detecting actual code-set drift.
expect_pass 'internal/errs/catalog.go'
expect_pass $'internal/template/files/.keep\ninternal/template/render_test.go'
expect_fail 'internal/template/files/.keep'
expect_pass 'README.md'

make_fixture() {
  local dir
  dir="$(mktemp -d)"
  git -C "$dir" init -q -b main
  git -C "$dir" config user.email ci@example.test
  git -C "$dir" config user.name CI
  printf 'base\n' >"$dir/README.md"
  git -C "$dir" add README.md
  git -C "$dir" commit -qm base
  printf '%s\n' "$dir"
}

expect_ci_before_sha_catches_all_pushed_commits() {
  local dir base
  dir="$(make_fixture)"
  trap 'rm -rf "$dir"' RETURN
  base="$(git -C "$dir" rev-parse HEAD)"
  mkdir -p "$dir/cmd"
  printf 'package cmd\n' >"$dir/cmd/new.go"
  git -C "$dir" add cmd/new.go
  git -C "$dir" commit -qm command-change
  printf 'docs\n' >>"$dir/README.md"
  git -C "$dir" add README.md
  git -C "$dir" commit -qm docs-change

  if (
    unset CHANGE_CONTRACT_BASE CI_MERGE_REQUEST_DIFF_BASE_SHA CHANGE_CONTRACT_CHANGED_FILES
    cd "$dir"
    CI_COMMIT_BEFORE_SHA="$base" "$check" >/dev/null 2>&1
  ); then
    echo 'expected CI_COMMIT_BEFORE_SHA to include the earlier command change' >&2
    exit 1
  fi
}

expect_origin_head_catches_all_local_commits() {
  local dir base
  dir="$(make_fixture)"
  trap 'rm -rf "$dir"' RETURN
  base="$(git -C "$dir" rev-parse HEAD)"
  git -C "$dir" update-ref refs/remotes/origin/main "$base"
  git -C "$dir" symbolic-ref refs/remotes/origin/HEAD refs/remotes/origin/main
  mkdir -p "$dir/cmd"
  printf 'package cmd\n' >"$dir/cmd/new.go"
  git -C "$dir" add cmd/new.go
  git -C "$dir" commit -qm command-change
  printf 'docs\n' >>"$dir/README.md"
  git -C "$dir" add README.md
  git -C "$dir" commit -qm docs-change

  if (
    unset CHANGE_CONTRACT_BASE CI_MERGE_REQUEST_DIFF_BASE_SHA CI_COMMIT_BEFORE_SHA CHANGE_CONTRACT_CHANGED_FILES
    cd "$dir"
    "$check" >/dev/null 2>&1
  ); then
    echo 'expected origin/HEAD to include the earlier command change' >&2
    exit 1
  fi
}

expect_ci_before_sha_catches_all_pushed_commits
expect_origin_head_catches_all_local_commits

echo "check-change-contract tests: passed"
