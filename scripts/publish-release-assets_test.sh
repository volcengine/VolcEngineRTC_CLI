#!/usr/bin/env bash
# Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
# SPDX-License-Identifier: MIT

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
fixture="$tmp/repo"
state="$tmp/state"
fakebin="$tmp/bin"
mkdir -p "$fixture" "$state/assets" "$fakebin"

git -C "$fixture" init -q
git -C "$fixture" config user.name Test
git -C "$fixture" config user.email test@example.com
printf 'fixture\n' > "$fixture/README.md"
git -C "$fixture" add README.md
git -C "$fixture" commit -qm fixture
git -C "$fixture" tag -a v0.0.1-rc.1 -m 'Release candidate notes'
target="$(git -C "$fixture" rev-parse HEAD)"

artifacts="$tmp/artifacts"
mkdir -p "$artifacts"
for target_name in darwin_amd64 darwin_arm64 linux_amd64 linux_arm64; do
  printf '%s\n' "$target_name" > "$artifacts/vertc_0.0.1-rc.1_${target_name}.tar.gz"
done
for target_name in windows_amd64 windows_arm64; do
  printf '%s\n' "$target_name" > "$artifacts/vertc_0.0.1-rc.1_${target_name}.zip"
done
(cd "$artifacts" && shasum -a 256 vertc_* > checksums.txt)
notes="$tmp/notes.md"
printf 'Release candidate notes\n' > "$notes"
npm_root="$tmp/npm"
mkdir -p "$npm_root"
printf '{"name":"@volcengine/rtc-cli","version":"0.0.1-rc.1","files":["README.md"]}\n' > "$npm_root/package.json"
printf 'fixture npm package\n' > "$npm_root/README.md"

cat > "$fakebin/gh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
command_name="${1:-} ${2:-}"
case "$command_name" in
  'release view')
    [[ -f "$TEST_STATE/release.json" ]] || exit 1
    cat "$TEST_STATE/release.json"
    ;;
  'release create')
    printf 'create\n' >> "$TEST_STATE/gh.log"
    shift 2
    tag="$1"; shift
	    target=""
	    prerelease=false
	    title=""
	    notes_file=""
	    names='[]'
    while [[ $# -gt 0 ]]; do
      case "$1" in
        --target) target="$2"; shift 2 ;;
        --prerelease) prerelease=true; shift ;;
	        --title) title="$2"; shift 2 ;;
	        --notes-file) notes_file="$2"; shift 2 ;;
        --draft|--verify-tag) shift ;;
        --*) shift ;;
        *) cp "$1" "$TEST_STATE/assets/"; names="$(jq --arg name "$(basename "$1")" '. + [{name:$name}]' <<< "$names")"; shift ;;
      esac
    done
	    jq -n --arg tag "$tag" --arg target "$target" --arg title "$title" --rawfile body "$notes_file" \
	      --argjson pre "$prerelease" --argjson assets "$names" \
	      '{tagName:$tag,name:$title,body:$body,isDraft:true,isPrerelease:$pre,targetCommitish:$target,assets:$assets}' > "$TEST_STATE/release.json"
    ;;
  'release download')
    destination=""
    while [[ $# -gt 0 ]]; do
      [[ "$1" == --dir ]] && { destination="$2"; shift 2; continue; }
      shift
    done
    cp "$TEST_STATE/assets/"* "$destination/"
    ;;
  'release upload')
    printf 'upload\n' >> "$TEST_STATE/gh.log"
    shift 3
    while [[ $# -gt 0 ]]; do
      asset="$1"; shift
      cp "$asset" "$TEST_STATE/assets/"
      tmp_json="$TEST_STATE/release.tmp"
      jq --arg name "$(basename "$asset")" '.assets += [{name:$name}]' "$TEST_STATE/release.json" > "$tmp_json"
      mv "$tmp_json" "$TEST_STATE/release.json"
    done
    ;;
  'release edit')
    printf 'edit %s\n' "$*" >> "$TEST_STATE/gh.log"
    prerelease=false
    for argument in "$@"; do
      [[ "$argument" == --prerelease=true ]] && prerelease=true
    done
    tmp_json="$TEST_STATE/release.tmp"
    jq --argjson pre "$prerelease" '.isDraft=false | .isPrerelease=$pre' "$TEST_STATE/release.json" > "$tmp_json"
    mv "$tmp_json" "$TEST_STATE/release.json"
    ;;
  *) echo "unexpected gh command: $*" >&2; exit 2 ;;
esac
EOF

cat > "$fakebin/npm" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
case "$1" in
  pack) exec "$REAL_NPM" "$@" ;;
  view)
    spec="$2"; field="$3"
    case "$field" in
      version)
        [[ -f "$TEST_STATE/npm.shasum" ]] || { echo 'npm error code E404' >&2; exit 1; }
        printf '"%s"\n' "$TEST_VERSION"
        ;;
      dist.shasum)
        [[ -f "$TEST_STATE/npm.shasum" ]] || exit 1
        jq -Rn --arg value "$(cat "$TEST_STATE/npm.shasum")" '$value'
        ;;
      dist-tags.latest)
        [[ "${FAIL_NPM_LATEST_LOOKUP:-0}" == 0 ]] || exit 43
        jq -Rn --arg value "$(cat "$TEST_STATE/npm.latest" 2>/dev/null || true)" '$value'
        ;;
      *) echo "unexpected npm view: $*" >&2; exit 2 ;;
    esac
    ;;
  publish)
    printf 'attempt\n' >> "$TEST_STATE/npm.attempts"
    [[ "${FAIL_NPM:-0}" == 0 ]] || exit 42
    printf 'publish\n' >> "$TEST_STATE/npm.log"
    shasum -a 1 "$2" | awk '{print $1}' > "$TEST_STATE/npm.shasum"
    printf '%s\n' "$TEST_VERSION" > "$TEST_STATE/npm.latest"
    ;;
  dist-tag)
    printf '%s\n' "$TEST_VERSION" > "$TEST_STATE/npm.latest"
    ;;
  *) echo "unexpected npm command: $*" >&2; exit 2 ;;
esac
EOF
chmod +x "$fakebin/gh" "$fakebin/npm"

export PATH="$fakebin:$PATH"
export TEST_STATE="$state"
export TEST_VERSION="0.0.1-rc.1"
export npm_config_cache="$tmp/npm-cache"
if [[ -z "${REAL_NPM:-}" ]]; then
  # PATH now resolves the fake npm; locate the real executable explicitly.
  export REAL_NPM="$(PATH="${PATH#*:}" command -v npm)"
fi
publish=("$repo_root/scripts/publish-release-assets.sh" --tag v0.0.1-rc.1 --target "$target" --artifacts "$artifacts" --npm-root "$npm_root" --notes-file "$notes" --prerelease true --latest false --npm-tag latest)

if (cd "$fixture" && "${publish[@]:0:${#publish[@]}-4}" --prerelease true --latest true --npm-tag latest) >/dev/null 2>&1; then
  echo "publish-release-assets-test: impossible prerelease/latest combination was accepted" >&2
  exit 1
fi

if (cd "$fixture" && FAIL_NPM=1 "${publish[@]}") >/dev/null 2>&1; then
  echo "publish-release-assets-test: expected npm failure" >&2
  exit 1
fi
[[ "$(jq -r '.isDraft' "$state/release.json")" == true ]]
[[ "$(grep -c '^create$' "$state/gh.log")" -eq 1 ]]
if [[ "$(grep -c '^attempt$' "$state/npm.attempts")" -ne 1 ]]; then
  echo "publish-release-assets-test: initial npm failure did not produce exactly one publish attempt" >&2
  exit 1
fi

# Registry lookup failures must stop before any npm write is attempted.
if (cd "$fixture" && FAIL_NPM=1 FAIL_NPM_LATEST_LOOKUP=1 "${publish[@]}") >/dev/null 2>&1; then
  echo "publish-release-assets-test: npm latest lookup failure was ignored" >&2
  exit 1
fi
if [[ "$(grep -c '^attempt$' "$state/npm.attempts")" -ne 1 ]]; then
  echo "publish-release-assets-test: npm write was attempted after latest lookup failure" >&2
  exit 1
fi

# A retry must reject changed release notes before attempting npm publication.
jq '.body="unapproved notes"' "$state/release.json" > "$state/release.tmp"
mv "$state/release.tmp" "$state/release.json"
if (cd "$fixture" && "${publish[@]}") >/dev/null 2>&1; then
  echo "publish-release-assets-test: conflicting release notes were accepted" >&2
  exit 1
fi
[[ ! -f "$state/npm.log" ]]
jq --rawfile body "$notes" '.body=$body' "$state/release.json" > "$state/release.tmp"
mv "$state/release.tmp" "$state/release.json"

jq '.name="unapproved title"' "$state/release.json" > "$state/release.tmp"
mv "$state/release.tmp" "$state/release.json"
if (cd "$fixture" && "${publish[@]}") >/dev/null 2>&1; then
  echo "publish-release-assets-test: conflicting release title was accepted" >&2
  exit 1
fi
[[ ! -f "$state/npm.log" ]]
jq '.name="vertc 0.0.1-rc.1"' "$state/release.json" > "$state/release.tmp"
mv "$state/release.tmp" "$state/release.json"

# A draft left with only some uploaded assets is completed in place.
missing_asset="vertc_0.0.1-rc.1_windows_arm64.zip"
rm "$state/assets/$missing_asset"
jq --arg name "$missing_asset" '.assets |= map(select(.name != $name))' "$state/release.json" > "$state/release.tmp"
mv "$state/release.tmp" "$state/release.json"
(cd "$fixture" && "${publish[@]}") >/dev/null
[[ "$(jq -r '.isDraft' "$state/release.json")" == false ]]
[[ "$(jq -r '.isPrerelease' "$state/release.json")" == true ]]
grep -Fq -- '--latest=false' "$state/gh.log"
[[ "$(cat "$state/npm.latest")" == 0.0.1-rc.1 ]]
[[ "$(grep -c '^create$' "$state/gh.log")" -eq 1 ]]
[[ "$(grep -c '^upload$' "$state/gh.log")" -eq 1 ]]

# An identical completed retry verifies and exits without recreating state.
(cd "$fixture" && "${publish[@]}") >/dev/null
[[ "$(grep -c '^create$' "$state/gh.log")" -eq 1 ]]
[[ "$(grep -c '^publish$' "$state/npm.log")" -eq 1 ]]

# Rerunning an older workflow must never roll npm latest backward.
printf '0.0.2\n' > "$state/npm.latest"
if (cd "$fixture" && "${publish[@]}") >/dev/null 2>&1; then
  echo "publish-release-assets-test: older retry was allowed to move npm latest backward" >&2
  exit 1
fi
[[ "$(cat "$state/npm.latest")" == 0.0.2 ]]
printf '%s\n' "$TEST_VERSION" > "$state/npm.latest"

# Conflicting npm content leaves the GitHub Release safely in draft state.
jq '.isDraft=true' "$state/release.json" > "$state/release.tmp"
mv "$state/release.tmp" "$state/release.json"
printf 'conflict\n' > "$state/npm.shasum"
if (cd "$fixture" && "${publish[@]}") >/dev/null 2>&1; then
  echo "publish-release-assets-test: conflicting npm package was accepted" >&2
  exit 1
fi
[[ "$(jq -r '.isDraft' "$state/release.json")" == true ]]

# Conflicting draft assets are rejected rather than overwritten.
printf 'conflict\n' > "$state/assets/vertc_0.0.1-rc.1_linux_amd64.tar.gz"
if (cd "$fixture" && "${publish[@]}") >/dev/null 2>&1; then
  echo "publish-release-assets-test: conflicting GitHub asset was accepted" >&2
  exit 1
fi

echo "publish-release-assets-test: passed"
