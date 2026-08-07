#!/usr/bin/env bash
# Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
# SPDX-License-Identifier: MIT

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
preflight="$repo_root/scripts/preflight-public-release.sh"

[[ -x "$preflight" ]] || {
  echo "preflight-public-release-test: missing executable $preflight" >&2
  exit 1
}

source_version="$(git -C "$repo_root" show HEAD:skills/byted-interactai-guide/SKILL.md | \
  awk '$1 == "version:" { print $2; exit }')"
source_version="${source_version#\"}"
source_version="${source_version%\"}"
source_version="${source_version#\'}"
source_version="${source_version%\'}"
[[ "$source_version" == "0.0.0-dev" ]] || {
  echo "preflight-public-release-test: unexpected source Skill placeholder: $source_version" >&2
  exit 1
}

stable_version="9.8.7"
stable_output="$($preflight --stability stable --version "$stable_version" --ref HEAD)"
grep -Fqx "preflight-public-release: version=$stable_version" <<< "$stable_output"
grep -Fqx "preflight-public-release: source_commit=$(git -C "$repo_root" rev-parse HEAD)" <<< "$stable_output"
grep -Eq '^preflight-public-release: prepared_skills=[1-9][0-9]*$' <<< "$stable_output"
grep -Fqx "preflight-public-release: contract=passed" <<< "$stable_output"

prerelease="4.5.6-rc.1"
prerelease_output="$($preflight --stability prerelease --version "$prerelease" --ref HEAD)"
grep -Fqx "preflight-public-release: version=$prerelease" <<< "$prerelease_output"

if "$preflight" --stability stable --version 1.2 --ref HEAD >/dev/null 2>&1; then
  echo "preflight-public-release-test: invalid stable version passed" >&2
  exit 1
fi

for reserved_version in 0.0.0-dev 0.0.0-snapshot; do
  if "$preflight" --stability prerelease --version "$reserved_version" --ref HEAD >/dev/null 2>&1; then
    echo "preflight-public-release-test: reserved version passed: $reserved_version" >&2
    exit 1
  fi
done

echo "preflight-public-release-test: passed"
