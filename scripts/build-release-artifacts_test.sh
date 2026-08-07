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
remote_capture="$tmp/release-remote"
identity_capture="$tmp/release-identity"
cat > "$fake_goreleaser" <<'EOF'
#!/usr/bin/env bash
git remote get-url origin > "$REMOTE_CAPTURE"
printf '%s\n%s\n%s\n%s\n' "${RELEASE_COMMIT:-}" "${RELEASE_BUILD_DATE:-}" "${RELEASE_TIMESTAMP:-}" "$(git show -s --format=%cI HEAD)" > "$IDENTITY_CAPTURE"
mkdir -p dist
printf 'partial\n' > dist/partial.txt
exit 17
EOF
chmod +x "$fake_goreleaser"

release_tmp="$tmp/release-tmp"
mkdir -p "$release_tmp"
output="$tmp/final-dist"
before_status="$(git -C "$repo_root" status --porcelain=v1 --untracked-files=all)"
if TMPDIR="$release_tmp" GOCACHE="$tmp/go-cache" GORELEASER="$fake_goreleaser" REMOTE_CAPTURE="$remote_capture" IDENTITY_CAPTURE="$identity_capture" \
  "$repo_root/scripts/build-release-artifacts.sh" --stability snapshot --publication none --output "$output" >/dev/null 2>&1; then
  echo "build-release-artifacts-test: expected injected snapshot build failure" >&2
  exit 1
fi
expected_identity="$(git -C "$repo_root" rev-parse HEAD)
$(git -C "$repo_root" show -s --format=%cI HEAD)
$(git -C "$repo_root" show -s --format=%ct HEAD)
$(git -C "$repo_root" show -s --format=%cI HEAD)"
if [[ "$(cat "$identity_capture" 2>/dev/null || true)" != "$expected_identity" ]]; then
  echo "build-release-artifacts-test: release build did not receive deterministic source identity" >&2
  exit 1
fi
if [[ "$(cat "$remote_capture" 2>/dev/null || true)" != "https://github.com/volcengine/VolcEngineRTC_CLI.git" ]]; then
  echo "build-release-artifacts-test: disposable release repository lacks the canonical public remote" >&2
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
if ! grep -Fq 'group: publish-release' "$workflow" || grep -Fq 'group: publish-release-${{ github.ref }}' "$workflow"; then
  echo "build-release-artifacts-test: release publications are not serialized across tags" >&2
  exit 1
fi
if ! grep -Fq 'queue: max' "$workflow"; then
  echo "build-release-artifacts-test: pending release publications are not retained" >&2
  exit 1
fi
build_line="$(grep -n 'name: Build and verify all seven release assets' "$workflow" | cut -d: -f1)"
npm_input_line="$(grep -n 'name: Prepare verified npm package input' "$workflow" | cut -d: -f1)"
publish_line="$(grep -n 'name: Stage draft, publish npm package, then finalize GitHub release' "$workflow" | cut -d: -f1)"
verify_line="$(grep -n 'name: Verify GitHub and npm publication' "$workflow" | cut -d: -f1)"
[[ "$build_line" -lt "$npm_input_line" && "$npm_input_line" -lt "$publish_line" && "$publish_line" -lt "$verify_line" ]]
grep -Fq -- '--skip=publish' "$repo_root/scripts/build-release-artifacts.sh"
if ! grep -Fq -- '-buildvcs=false' "$repo_root/.goreleaser.yaml"; then
  echo "build-release-artifacts-test: release binaries still embed disposable repository VCS metadata" >&2
  exit 1
fi
if [[ "$(grep -Fc 'mtime: "{{ .Env.RELEASE_BUILD_DATE }}"' "$repo_root/.goreleaser.yaml")" -lt 5 ]]; then
  echo "build-release-artifacts-test: release archive timestamps are not deterministic" >&2
  exit 1
fi
grep -Fq 'mod_timestamp: "{{ .Env.RELEASE_TIMESTAMP }}"' "$repo_root/.goreleaser.yaml"
grep -Fq 'delivery="$output_parent/.${output_name}.release-delivery.$$"' "$repo_root/scripts/build-release-artifacts.sh"
grep -Fq './scripts/publish-release-assets.sh' "$workflow"
grep -Fq './scripts/verify-release-publication.sh' "$workflow"
grep -Fq 'git cat-file -t "refs/tags/$GITHUB_REF_NAME"' "$workflow"
grep -Fq 'test "${#assets[@]}" -eq 7' "$workflow"
grep -Fq -- '--publication public' "$workflow"
grep -Fq 'npm_tag=latest' "$workflow"
grep -Fq 'npm_tag=next' "$workflow"
grep -Fq 'echo "VERTC_NPM_TAG=$npm_tag"' "$workflow"
if [[ "$(grep -Fc -- '--npm-tag "$VERTC_NPM_TAG"' "$workflow")" -ne 2 ]]; then
  echo "build-release-artifacts-test: publication and verification must share the resolved npm dist-tag" >&2
  exit 1
fi
if grep -Fq -- '--npm-tag latest' "$workflow"; then
  echo "build-release-artifacts-test: workflow still hard-codes npm latest" >&2
  exit 1
fi
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
