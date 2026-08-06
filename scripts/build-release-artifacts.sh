#!/usr/bin/env bash
# Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
# SPDX-License-Identifier: MIT

# Build and verify public or snapshot artifacts without publishing them.
if [[ -z "${BASH_VERSION:-}" ]]; then
  exec bash "$0" "$@"
fi
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/.." && pwd)"
stability=""
publication=""
version=""
source_ref="HEAD"
output_dir=""

usage() {
  cat <<'EOF'
Usage: ./scripts/build-release-artifacts.sh --stability stable|prerelease|snapshot --publication public|none --output DIR [options]

Builds all release targets from an isolated, version-stamped source tree,
verifies every binary/archive/Skill, then delivers the complete dist directory.

Options:
  --stability TYPE  stable, prerelease, or snapshot (required)
  --publication TO  public or none (required)
  --version VERSION public vX.Y.Z or vX.Y.Z-ID tag (required for public; forbidden for snapshot)
  --ref REF         committed public source ref (default: HEAD)
  --output DIR      absent or empty final output directory (required)
  -h, --help        show this help
EOF
}

die() {
  echo "build-release-artifacts: $*" >&2
  exit 1
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --stability) [[ $# -ge 2 ]] || die "--stability requires a value"; stability="$2"; shift 2 ;;
    --publication) [[ $# -ge 2 ]] || die "--publication requires a value"; publication="$2"; shift 2 ;;
    --version) [[ $# -ge 2 ]] || die "--version requires a value"; version="$2"; shift 2 ;;
    --ref) [[ $# -ge 2 ]] || die "--ref requires a value"; source_ref="$2"; shift 2 ;;
    --output) [[ $# -ge 2 ]] || die "--output requires a value"; output_dir="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown argument: $1" ;;
  esac
done

case "$stability/$publication" in
  stable/public|prerelease/public|snapshot/none) ;;
  *) die "unsupported stability/publication combination: $stability/$publication" ;;
esac
[[ -n "$output_dir" ]] || die "--output is required"
if [[ "$publication" == "public" ]]; then
  [[ -n "$version" ]] || die "--version is required for public releases"
else
  [[ -z "$version" ]] || die "--version is derived and forbidden for snapshots"
fi
for command_name in git go cp find; do
  command -v "$command_name" >/dev/null 2>&1 || die "missing required command: $command_name"
done
goreleaser="${GORELEASER:-$(go env GOPATH)/bin/goreleaser}"
[[ -x "$goreleaser" ]] || die "goreleaser not found — run 'make release-tools'"

case "$output_dir" in
  /*) ;;
  *) output_dir="$PWD/$output_dir" ;;
esac
while [[ "$output_dir" != "/" && "$output_dir" == */ ]]; do
  output_dir="${output_dir%/}"
done
if [[ -d "$output_dir" && -n "$(find "$output_dir" -mindepth 1 -maxdepth 1 -print -quit)" ]]; then
  die "output directory must be empty: $output_dir"
fi
[[ ! -e "$output_dir" || -d "$output_dir" ]] || die "output path is not a directory: $output_dir"

tmp_root="$(mktemp -d)"
output_parent="$(dirname "$output_dir")"
output_name="$(basename "$output_dir")"
delivery="$output_parent/.${output_name}.release-delivery.$$"
cleanup() {
  status=$?
  trap - EXIT INT TERM
  rm -rf "$delivery" "$tmp_root" || true
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
source_root="$tmp_root/source"
manifest="$tmp_root/release-manifest.json"

prepare_args=(
  run ./internal/releasecmd prepare
  --repo "$repo_root"
  --destination "$source_root"
  --manifest "$manifest"
  --stability "$stability"
  --publication "$publication"
  --ref "$source_ref"
)
if [[ "$publication" == "public" ]]; then
  prepare_args+=(--source commit --version "$version")
else
  prepare_args+=(--source worktree --allow-dirty)
fi
echo "build-release-artifacts: preparing isolated $stability/$publication source"
resolved_version="$(cd "$repo_root" && go "${prepare_args[@]}")"
[[ -n "$resolved_version" ]] || die "release helper returned an empty version"

# GoReleaser requires Git metadata. Create it only inside the disposable source
# tree; this is not a linked worktree and never writes invoking-repo Git state.
git -C "$source_root" init -q
git -C "$source_root" config user.name "vertc release"
git -C "$source_root" config user.email "release@localhost"
git -C "$source_root" add .
git -C "$source_root" commit -qm "Release $resolved_version"
git -C "$source_root" tag "v$resolved_version"

echo "build-release-artifacts: building $resolved_version without publication"
if [[ "$stability" == "snapshot" ]]; then
  (cd "$source_root" && RELEASE_VERSION="$resolved_version" "$goreleaser" release --snapshot --clean --config .goreleaser.yaml)
else
  (cd "$source_root" && RELEASE_VERSION="$resolved_version" "$goreleaser" release --clean --skip=publish --config .goreleaser.yaml)
fi

echo "build-release-artifacts: verifying $resolved_version"
(cd "$repo_root" && go run ./internal/releasecmd verify \
  --manifest "$manifest" \
  --artifacts "$source_root/dist" \
  --checksums "$source_root/dist/checksums.txt" \
  --package-json "$source_root/package.json")

[[ ! -e "$delivery" ]] || die "temporary delivery path already exists: $delivery"
mkdir -p "$delivery"
cp -R "$source_root/dist/." "$delivery/"
if [[ -d "$output_dir" ]]; then
  rmdir "$output_dir"
fi
mv "$delivery" "$output_dir"
echo "build-release-artifacts: verified output=$output_dir"
echo "build-release-artifacts: version=$resolved_version"
