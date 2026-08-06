#!/usr/bin/env bash
# Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
# SPDX-License-Identifier: MIT

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

# A build failure after isolated preparation must not mutate the invoking tree,
# expose a partial output directory, or leave the owned temporary root behind.
fake_goreleaser="$tmp/goreleaser"
cat > "$fake_goreleaser" <<'EOF'
#!/usr/bin/env bash
mkdir -p dist
printf 'partial\n' > dist/partial.txt
exit 17
EOF
chmod +x "$fake_goreleaser"

release_tmp="$tmp/release-tmp"
mkdir -p "$release_tmp"
output="$tmp/final-dist"
before_status="$(git -C "$repo_root" status --porcelain=v1 --untracked-files=all)"
if TMPDIR="$release_tmp" GORELEASER="$fake_goreleaser" \
  "$repo_root/scripts/build-release-artifacts.sh" --stability snapshot --publication none --output "$output" >/dev/null 2>&1; then
  echo "build-release-artifacts-test: expected injected snapshot build failure" >&2
  exit 1
fi
after_status="$(git -C "$repo_root" status --porcelain=v1 --untracked-files=all)"
[[ "$after_status" == "$before_status" ]]
[[ ! -e "$output" ]]
if find "$release_tmp" -mindepth 1 -maxdepth 1 -print -quit | grep -q .; then
  echo "build-release-artifacts-test: temporary release state leaked" >&2
  find "$release_tmp" -mindepth 1 -maxdepth 2 -print >&2
  exit 1
fi

# Public input validation precedes tool/build side effects.
if GORELEASER="$fake_goreleaser" \
  "$repo_root/scripts/build-release-artifacts.sh" --stability prerelease --publication public --output "$output" >/dev/null 2>&1; then
  echo "build-release-artifacts-test: public prerelease accepted a missing version" >&2
  exit 1
fi
invalid_prerelease_error="$(
  GOCACHE="$tmp/go-cache" GORELEASER="$fake_goreleaser" \
    "$repo_root/scripts/build-release-artifacts.sh" \
      --stability prerelease --publication public --version 0.0.1 --ref HEAD --output "$output" 2>&1 || true
)"
grep -Fq 'must be valid X.Y.Z-prerelease SemVer' <<< "$invalid_prerelease_error"

workflow="$repo_root/.github/workflows/publish-release.yml"
build_line="$(grep -n 'name: Build and verify all seven release assets' "$workflow" | cut -d: -f1)"
npm_input_line="$(grep -n 'name: Prepare verified npm package input' "$workflow" | cut -d: -f1)"
publish_line="$(grep -n 'name: Stage draft, publish npm latest, then finalize GitHub release' "$workflow" | cut -d: -f1)"
verify_line="$(grep -n 'name: Verify GitHub and npm publication' "$workflow" | cut -d: -f1)"
[[ "$build_line" -lt "$npm_input_line" && "$npm_input_line" -lt "$publish_line" && "$publish_line" -lt "$verify_line" ]]
grep -Fq -- '--skip=publish' "$repo_root/scripts/build-release-artifacts.sh"
grep -Fq 'delivery="$output_parent/.${output_name}.release-delivery.$$"' "$repo_root/scripts/build-release-artifacts.sh"
grep -Fq './scripts/publish-release-assets.sh' "$workflow"
grep -Fq './scripts/verify-release-publication.sh' "$workflow"
grep -Fq 'git cat-file -t "refs/tags/$GITHUB_REF_NAME"' "$workflow"
grep -Fq 'test "${#assets[@]}" -eq 7' "$workflow"
grep -Fq -- '--publication public' "$workflow"
grep -Fq -- '--npm-tag latest' "$workflow"
grep -Fq -- '--prerelease "$VERTC_PRERELEASE"' "$workflow"
grep -Fq 'stability=prerelease' "$workflow"
grep -Fq 'prerelease=true' "$workflow"
grep -Fq 'latest=true' "$workflow"
grep -Fq 'latest=false' "$workflow"
grep -Fq -- '--latest "$VERTC_LATEST"' "$workflow"
if [[ "$(grep -Fc -- '--notes-file "$VERTC_NOTES_FILE"' "$workflow")" -ne 2 ]]; then
  echo "build-release-artifacts-test: publication and verification must share release notes" >&2
  exit 1
fi
if grep -Fq 'goreleaser/goreleaser-action' "$workflow"; then
  echo "build-release-artifacts-test: workflow still has an implicit GoReleaser publisher" >&2
  exit 1
fi

echo "build-release-artifacts-test: passed"
