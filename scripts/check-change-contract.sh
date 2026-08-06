#!/usr/bin/env bash
# Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
# SPDX-License-Identifier: MIT

# Enforce focused validation when stable CLI surfaces change.
set -euo pipefail

changed_files="${CHANGE_CONTRACT_CHANGED_FILES:-}"

if [[ -z "$changed_files" ]]; then
  base="${CHANGE_CONTRACT_BASE:-}"
  if [[ -z "$base" ]]; then
    base="${CI_MERGE_REQUEST_DIFF_BASE_SHA:-}"
  fi
  if [[ -z "$base" || "$base" =~ ^0+$ ]]; then
    base="${CI_COMMIT_BEFORE_SHA:-}"
  fi
  if [[ -z "$base" || "$base" =~ ^0+$ ]]; then
    default_ref="$(git symbolic-ref --quiet refs/remotes/origin/HEAD 2>/dev/null || true)"
    if [[ -n "$default_ref" ]] && git rev-parse --verify --quiet "$default_ref" >/dev/null; then
      base="$(git merge-base "$default_ref" HEAD)"
    else
      base=""
    fi
  fi
  if [[ -z "$base" ]] && git rev-parse --verify --quiet HEAD^ >/dev/null; then
    base="HEAD^"
  fi
  if [[ -z "$base" ]] || ! git rev-parse --verify --quiet "$base^{commit}" >/dev/null; then
    echo "check-change-contract: unable to determine comparison base; set CHANGE_CONTRACT_BASE" >&2
    exit 1
  fi

  # Include local staged, unstaged, and new files so `make ci` catches the same
  # omissions before a developer creates the commit that CI will inspect.
  changed_files="$({
    git diff --name-only "$base"...HEAD
    git diff --name-only
    git diff --name-only --cached
    git ls-files --others --exclude-standard
  } | sort -u)"
fi

if [[ -z "$changed_files" ]]; then
  echo "check-change-contract: no changed files"
  exit 0
fi

has_change() {
  grep -Eq "$1" <<<"$changed_files"
}

require_change() {
  local changed_pattern="$1"
  local required_pattern="$2"
  local message="$3"

  if has_change "$changed_pattern" && ! has_change "$required_pattern"; then
    echo "check-change-contract: $message" >&2
    return 1
  fi
}

status=0
require_change '^cmd/' '(^tests/.*_test\.go$|^cmd/.*_test\.go$)' \
  'changes under cmd/ require a command unit test or tests/ E2E update' || status=1
# Error-code set drift is checked directly by make check-error-codes. Do not
# require the codes-only snapshot to change when catalog metadata changes.
require_change '^internal/output/' '(^tests/.*_test\.go$|^internal/output/.*_test\.go$)' \
  'changes under internal/output/ require an output test or tests/ E2E update' || status=1
require_change '^internal/config/' '(^tests/.*_test\.go$|^internal/config/.*_test\.go$)' \
  'changes under internal/config/ require a config test or tests/ E2E update' || status=1
require_change '^internal/token/' '(^tests/.*_test\.go$|^internal/token/.*_test\.go$)' \
  'changes under internal/token/ require a token test or tests/ E2E update' || status=1
require_change '^internal/template/files/' '^internal/template/.*_test\.go$' \
  'template source changes require an internal/template render or registry test update' || status=1

if [[ "$status" -ne 0 ]]; then
  exit "$status"
fi

echo "check-change-contract: contract coverage verified"
