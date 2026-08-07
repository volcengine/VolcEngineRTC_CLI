#!/usr/bin/env bash
# Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
# SPDX-License-Identifier: MIT

# Validate a requested public version against one exact committed source before
# an irreversible tag is created. This performs the same release-contract
# preparation used by the publication workflow, but does not build or publish.
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/.." && pwd)"
stability=""
version=""
source_ref=""

usage() {
  cat <<'EOF'
Usage: scripts/preflight-public-release.sh --stability stable|prerelease --version VERSION --ref REF

Builds the prepared source for one exact commit, stamps the requested version,
and validates the resulting release manifest. It writes no repository, tag,
release, or npm state.
EOF
}

die() {
  echo "preflight-public-release: $*" >&2
  exit 1
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --stability) [[ $# -ge 2 ]] || die "--stability requires a value"; stability="$2"; shift 2 ;;
    --version) [[ $# -ge 2 ]] || die "--version requires a value"; version="$2"; shift 2 ;;
    --ref) [[ $# -ge 2 ]] || die "--ref requires a value"; source_ref="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown argument: $1" ;;
  esac
done

case "$stability" in
  stable|prerelease) ;;
  *) die "--stability must be stable or prerelease" ;;
esac
[[ -n "$version" ]] || die "--version is required"
[[ -n "$source_ref" ]] || die "--ref is required"
for command_name in git go jq; do
  command -v "$command_name" >/dev/null 2>&1 || die "missing required command: $command_name"
done

source_commit="$(git -C "$repo_root" rev-parse --verify "$source_ref^{commit}" 2>/dev/null || true)"
[[ -n "$source_commit" ]] || die "source ref is not a commit: $source_ref"

tmp_root="$(mktemp -d)"
trap 'rm -rf "$tmp_root"' EXIT
prepared_source="$tmp_root/source"
manifest="$tmp_root/release-manifest.json"

resolved_version="$(cd "$repo_root" && go run ./internal/releasecmd prepare \
  --repo "$repo_root" \
  --destination "$prepared_source" \
  --manifest "$manifest" \
  --stability "$stability" \
  --publication public \
  --version "$version" \
  --source commit \
  --ref "$source_commit")"
[[ -n "$resolved_version" && "$resolved_version" != null ]] || die "release contract returned an empty version"
jq -e --arg version "$resolved_version" \
  '(.skills | length) > 0 and all(.skills[]; .version == $version)' \
  "$manifest" >/dev/null || die "prepared Skill versions do not match $resolved_version"
prepared_skills="$(jq -r '.skills | length' "$manifest")"

echo "preflight-public-release: version=$resolved_version"
echo "preflight-public-release: source_commit=$source_commit"
echo "preflight-public-release: prepared_skills=$prepared_skills"
echo "preflight-public-release: contract=passed"
