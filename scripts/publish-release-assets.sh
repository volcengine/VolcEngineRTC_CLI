#!/usr/bin/env bash
# Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
# SPDX-License-Identifier: MIT

# Stage and publish one already-built public release. This script is intended
# for the tag-triggered GitHub workflow and is safely retryable for identical
# GitHub/npm state.
set -euo pipefail

tag=""
target=""
artifacts=""
npm_root=""
notes_file=""
package_name="@volcengine/rtc-cli"
npm_tag="latest"
prerelease="false"
latest="true"

usage() {
  cat <<'EOF'
Usage: scripts/publish-release-assets.sh --tag TAG --target SHA --artifacts DIR --npm-root DIR --notes-file FILE [options]

Options:
  --package NAME       npm package name (default: @volcengine/rtc-cli)
  --npm-tag TAG        npm dist-tag (default: latest)
  --prerelease BOOL    true or false (default: false)
  --latest BOOL        true or false (default: true)
EOF
}

die() { echo "publish-release-assets: $*" >&2; exit 1; }

normalized_notes() {
  jq -Rs 'gsub("\r\n"; "\n") | sub("\n+$"; "")' "$1"
}

semver_compare() {
  local left="$1" right="$2" left_core right_core left_pre right_pre
  [[ "$left" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$ ]] || die "invalid npm channel version: $left"
  [[ "$right" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$ ]] || die "invalid npm channel version: $right"
  left_core="${left%%-*}"; right_core="${right%%-*}"
  left_pre=""; right_pre=""
  [[ "$left" == *-* ]] && left_pre="${left#*-}"
  [[ "$right" == *-* ]] && right_pre="${right#*-}"
  local -a left_parts right_parts left_ids right_ids
  IFS=. read -r -a left_parts <<< "$left_core"
  IFS=. read -r -a right_parts <<< "$right_core"
  local index left_number right_number
  for index in 0 1 2; do
    left_number=$((10#${left_parts[$index]}))
    right_number=$((10#${right_parts[$index]}))
    (( left_number < right_number )) && { echo -1; return; }
    (( left_number > right_number )) && { echo 1; return; }
  done
  [[ -z "$left_pre" && -z "$right_pre" ]] && { echo 0; return; }
  [[ -z "$left_pre" ]] && { echo 1; return; }
  [[ -z "$right_pre" ]] && { echo -1; return; }
  IFS=. read -r -a left_ids <<< "$left_pre"
  IFS=. read -r -a right_ids <<< "$right_pre"
  for ((index=0; index<${#left_ids[@]} || index<${#right_ids[@]}; index++)); do
    (( index >= ${#left_ids[@]} )) && { echo -1; return; }
    (( index >= ${#right_ids[@]} )) && { echo 1; return; }
    [[ "${left_ids[$index]}" == "${right_ids[$index]}" ]] && continue
    if [[ "${left_ids[$index]}" =~ ^[0-9]+$ && "${right_ids[$index]}" =~ ^[0-9]+$ ]]; then
      left_number=$((10#${left_ids[$index]})); right_number=$((10#${right_ids[$index]}))
      (( left_number < right_number )) && { echo -1; return; }
      echo 1; return
    fi
    [[ "${left_ids[$index]}" =~ ^[0-9]+$ ]] && { echo -1; return; }
    [[ "${right_ids[$index]}" =~ ^[0-9]+$ ]] && { echo 1; return; }
    [[ "${left_ids[$index]}" < "${right_ids[$index]}" ]] && echo -1 || echo 1
    return
  done
  echo 0
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --tag) tag="$2"; shift 2 ;;
    --target) target="$2"; shift 2 ;;
    --artifacts) artifacts="$2"; shift 2 ;;
    --npm-root) npm_root="$2"; shift 2 ;;
    --notes-file) notes_file="$2"; shift 2 ;;
    --package) package_name="$2"; shift 2 ;;
    --npm-tag) npm_tag="$2"; shift 2 ;;
    --prerelease) prerelease="$2"; shift 2 ;;
    --latest) latest="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown argument: $1" ;;
  esac
done

[[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$ ]] || die "invalid public tag: $tag"
[[ "$target" =~ ^[0-9a-f]{40}$ ]] || die "target must be a full commit SHA"
[[ -d "$artifacts" && -d "$npm_root" && -f "$notes_file" ]] || die "artifacts, npm root, or notes file is missing"
[[ "$prerelease" == "true" || "$prerelease" == "false" ]] || die "--prerelease must be true or false"
[[ "$latest" == "true" || "$latest" == "false" ]] || die "--latest must be true or false"
[[ "$prerelease" != "true" || "$latest" == "false" ]] || die "GitHub prereleases cannot be Latest"
for command_name in git gh npm jq shasum cmp find; do
  command -v "$command_name" >/dev/null 2>&1 || die "missing required command: $command_name"
done

version="${tag#v}"
expected_title="vertc $version"
tag_target="$(git rev-parse --verify "$tag^{commit}" 2>/dev/null || true)"
[[ "$tag_target" == "$target" ]] || die "tag $tag targets ${tag_target:-<missing>}, expected $target"
[[ "$(git rev-parse --verify HEAD^{commit})" == "$target" ]] || die "checked-out source is not the tag target"

assets=()
while IFS= read -r asset; do
  assets+=("$asset")
done < <(find "$artifacts" -maxdepth 1 -type f \
  \( -name "vertc_${version}_*.tar.gz" -o -name "vertc_${version}_*.zip" -o -name checksums.txt \) \
  -print | LC_ALL=C sort)
[[ "${#assets[@]}" -eq 7 ]] || die "expected seven release assets, found ${#assets[@]}"
(cd "$artifacts" && shasum -a 256 -c checksums.txt)

tmp_root="$(mktemp -d)"
trap 'rm -rf "$tmp_root"' EXIT
pack_json="$tmp_root/npm-pack.json"
npm pack "$npm_root" --json --pack-destination "$tmp_root" > "$pack_json"
local_shasum="$(jq -er '.[0].shasum' "$pack_json")"
npm_tarballs=()
while IFS= read -r tarball; do npm_tarballs+=("$tarball"); done < <(find "$tmp_root" -maxdepth 1 -type f -name '*.tgz' -print | LC_ALL=C sort)
[[ "${#npm_tarballs[@]}" -eq 1 ]] || die "npm pack must create exactly one tarball"
npm_tarball="${npm_tarballs[0]}"

release_json="$tmp_root/release.json"
release_exists=0
release_complete=0
if gh release view "$tag" --json tagName,name,body,isDraft,isPrerelease,targetCommitish,assets > "$release_json" 2>/dev/null; then
  release_exists=1
  [[ "$(jq -r '.tagName' "$release_json")" == "$tag" ]] || die "existing GitHub Release tag conflicts"
  [[ "$(jq -r '.targetCommitish' "$release_json")" == "$target" ]] || die "existing GitHub Release target conflicts"
  [[ "$(jq -r '.name' "$release_json")" == "$expected_title" ]] || die "existing GitHub Release title conflicts"
  existing_notes="$tmp_root/existing-release-notes.md"
  jq -j '.body' "$release_json" > "$existing_notes"
  [[ "$(normalized_notes "$notes_file")" == "$(normalized_notes "$existing_notes")" ]] \
    || die "existing GitHub Release notes conflict"
  observed_draft="$(jq -r '.isDraft' "$release_json")"
  if [[ "$observed_draft" == "false" ]]; then
    [[ "$(jq -r '.isPrerelease' "$release_json")" == "$prerelease" ]] || die "published GitHub Release flags conflict"
    release_complete=1
  fi
  observed_names=()
  while IFS= read -r name; do observed_names+=("$name"); done < <(jq -r '.assets[].name' "$release_json" | LC_ALL=C sort)
  expected_names=()
  while IFS= read -r name; do expected_names+=("$name"); done < <(printf '%s\n' "${assets[@]##*/}" | LC_ALL=C sort)
  for observed_name in "${observed_names[@]}"; do
    [[ " ${expected_names[*]} " == *" $observed_name "* ]] || die "existing GitHub Release has unexpected asset: $observed_name"
  done
  download_dir="$tmp_root/github-assets"
  mkdir -p "$download_dir"
  [[ "${#observed_names[@]}" -eq 0 ]] || gh release download "$tag" --dir "$download_dir"
  missing_assets=()
  for asset in "${assets[@]}"; do
    asset_name="$(basename "$asset")"
    if [[ -f "$download_dir/$asset_name" ]]; then
      cmp "$asset" "$download_dir/$asset_name" >/dev/null || die "existing GitHub asset differs: $asset_name"
    else
      missing_assets+=("$asset")
    fi
  done
  if [[ "${#missing_assets[@]}" -gt 0 ]]; then
    [[ "$observed_draft" == "true" ]] || die "published GitHub Release is missing assets"
    gh release upload "$tag" "${missing_assets[@]}"
    gh release view "$tag" --json tagName,name,body,isDraft,isPrerelease,targetCommitish,assets > "$release_json"
    actual_names="$(jq -r '.assets[].name' "$release_json" | LC_ALL=C sort)"
    expected_names_text="$(printf '%s\n' "${expected_names[@]}")"
    [[ "$actual_names" == "$expected_names_text" ]] || die "GitHub draft asset recovery incomplete"
  fi
fi

if [[ "$release_exists" -eq 0 ]]; then
  create_args=(release create "$tag" "${assets[@]}" --draft --verify-tag --target "$target" --title "$expected_title" --notes-file "$notes_file")
  [[ "$prerelease" == "false" ]] || create_args+=(--prerelease)
  gh "${create_args[@]}"
  echo "publish-release-assets: staged draft GitHub Release $tag"
else
  echo "publish-release-assets: verified existing GitHub Release $tag"
fi

observed_tag="$(npm view "$package_name" "dist-tags.$npm_tag" --json 2>/dev/null | jq -r 'select(type == "string" and length > 0)' || true)"
if [[ -n "$observed_tag" && "$observed_tag" != "$version" ]]; then
  channel_order="$(semver_compare "$version" "$observed_tag")"
  [[ "$channel_order" != "-1" ]] || die "refusing to move npm $npm_tag backward from $observed_tag to $version"
fi

remote_version="$(npm view "$package_name@$version" version --json 2>/dev/null | jq -r 'select(type == "string")' || true)"
if [[ -n "$remote_version" ]]; then
  [[ "$remote_version" == "$version" ]] || die "npm returned conflicting version $remote_version"
  remote_shasum="$(npm view "$package_name@$version" dist.shasum --json | jq -er '.')"
  [[ "$remote_shasum" == "$local_shasum" ]] || die "existing npm package content conflicts for $package_name@$version"
  echo "publish-release-assets: verified existing npm package $package_name@$version"
else
  npm publish "$npm_tarball" --access public --tag "$npm_tag"
  remote_shasum="$(npm view "$package_name@$version" dist.shasum --json | jq -er '.')"
  [[ "$remote_shasum" == "$local_shasum" ]] || die "published npm package shasum verification failed"
fi

observed_tag="$(npm view "$package_name" "dist-tags.$npm_tag" --json | jq -er '.')"
if [[ "$observed_tag" != "$version" ]]; then
  npm dist-tag add "$package_name@$version" "$npm_tag"
  observed_tag="$(npm view "$package_name" "dist-tags.$npm_tag" --json | jq -er '.')"
fi
[[ "$observed_tag" == "$version" ]] || die "npm $npm_tag dist-tag does not resolve to $version"

if [[ "$release_complete" -eq 0 ]]; then
  edit_args=(release edit "$tag" --draft=false "--prerelease=$prerelease" "--latest=$latest")
  gh "${edit_args[@]}"
fi
final_json="$tmp_root/final-release.json"
gh release view "$tag" --json tagName,name,body,isDraft,isPrerelease,targetCommitish,assets > "$final_json"
[[ "$(jq -r '.isDraft' "$final_json")" == "false" ]] || die "GitHub Release remains a draft"
[[ "$(jq -r '.isPrerelease' "$final_json")" == "$prerelease" ]] || die "GitHub Release prerelease flag mismatch"
[[ "$(jq -r '.targetCommitish' "$final_json")" == "$target" ]] || die "GitHub Release target changed"
[[ "$(jq -r '.name' "$final_json")" == "$expected_title" ]] || die "GitHub Release title changed"
final_notes="$tmp_root/final-release-notes.md"
jq -j '.body' "$final_json" > "$final_notes"
[[ "$(normalized_notes "$notes_file")" == "$(normalized_notes "$final_notes")" ]] \
  || die "GitHub Release notes changed"
echo "publish-release-assets: complete tag=$tag npm=$package_name@$version npm_tag=$npm_tag prerelease=$prerelease latest=$latest"
