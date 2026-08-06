#!/usr/bin/env bash
# Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
# SPDX-License-Identifier: MIT

# Verify the user-visible GitHub and npm postconditions for one public release.
set -euo pipefail

repository="${GITHUB_REPOSITORY:-}"
tag=""
target=""
package_name="@volcengine/rtc-cli"
npm_tag="latest"
prerelease="false"
latest="true"
run_id=""
notes_file=""

die() { echo "verify-release-publication: $*" >&2; exit 1; }

normalized_notes() {
  jq -Rs 'gsub("\r\n"; "\n") | sub("\n+$"; "")' "$1"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repository) repository="$2"; shift 2 ;;
    --tag) tag="$2"; shift 2 ;;
    --target) target="$2"; shift 2 ;;
    --package) package_name="$2"; shift 2 ;;
    --npm-tag) npm_tag="$2"; shift 2 ;;
    --prerelease) prerelease="$2"; shift 2 ;;
    --latest) latest="$2"; shift 2 ;;
    --run-id) run_id="$2"; shift 2 ;;
    --notes-file) notes_file="$2"; shift 2 ;;
    *) die "unknown argument: $1" ;;
  esac
done

[[ -n "$repository" && -n "$tag" && "$target" =~ ^[0-9a-f]{40}$ && -f "$notes_file" ]] \
  || die "--repository, --tag, full --target, and --notes-file are required"
[[ "$prerelease" == "true" || "$prerelease" == "false" ]] || die "--prerelease must be true or false"
[[ "$latest" == "true" || "$latest" == "false" ]] || die "--latest must be true or false"
[[ "$prerelease" != "true" || "$latest" == "false" ]] || die "GitHub prereleases cannot be Latest"
for command_name in gh npm jq shasum tar unzip grep find; do
  command -v "$command_name" >/dev/null 2>&1 || die "missing required command: $command_name"
done

version="${tag#v}"
tag_object_type="$(gh api "repos/$repository/git/ref/tags/$tag" --jq '.object.type')"
tag_object_sha="$(gh api "repos/$repository/git/ref/tags/$tag" --jq '.object.sha')"
[[ "$tag_object_type" == "tag" ]] || die "public version tag must be annotated"
remote_target="$(gh api "repos/$repository/git/tags/$tag_object_sha" --jq '.object.sha')"
[[ "$remote_target" == "$target" ]] || die "tag target mismatch: expected $target, observed $remote_target"

if [[ -n "$run_id" ]]; then
  run_json="$(gh run view "$run_id" --repo "$repository" --json status,conclusion,headSha,event)"
  [[ "$(jq -r '.status' <<< "$run_json")" == "completed" ]] || die "workflow run $run_id is not complete"
  [[ "$(jq -r '.conclusion' <<< "$run_json")" == "success" ]] || die "workflow run $run_id did not succeed"
  [[ "$(jq -r '.headSha' <<< "$run_json")" == "$target" ]] || die "workflow run target differs from $target"
  [[ "$(jq -r '.event' <<< "$run_json")" == "push" ]] || die "workflow run was not tag-triggered"
fi

tmp_root="$(mktemp -d)"
trap 'rm -rf "$tmp_root"' EXIT
release_json="$tmp_root/release.json"
gh release view "$tag" --repo "$repository" --json tagName,name,body,isDraft,isPrerelease,targetCommitish,assets > "$release_json"
[[ "$(jq -r '.isDraft' "$release_json")" == "false" ]] || die "release is still a draft"
[[ "$(jq -r '.isPrerelease' "$release_json")" == "$prerelease" ]] || die "release prerelease flag mismatch"
[[ "$(jq -r '.name' "$release_json")" == "vertc $version" ]] || die "release title mismatch"
observed_notes="$tmp_root/release-notes.md"
jq -j '.body' "$release_json" > "$observed_notes"
[[ "$(normalized_notes "$notes_file")" == "$(normalized_notes "$observed_notes")" ]] \
  || die "release notes mismatch"
observed_latest="$(gh api "repos/$repository/releases/latest" --jq '.tag_name' 2>/dev/null || true)"
if [[ "$latest" == "true" ]]; then
  [[ "$observed_latest" == "$tag" ]] || die "GitHub Latest does not resolve to $tag"
else
  [[ "$observed_latest" != "$tag" ]] || die "GitHub prerelease unexpectedly resolves as Latest"
fi
[[ "$(jq '.assets | length' "$release_json")" -eq 7 ]] || die "release does not contain seven assets"

assets_dir="$tmp_root/assets"
mkdir -p "$assets_dir"
gh release download "$tag" --repo "$repository" --dir "$assets_dir"
(cd "$assets_dir" && shasum -a 256 -c checksums.txt)
archives=()
while IFS= read -r archive; do archives+=("$archive"); done < <(find "$assets_dir" -maxdepth 1 -type f \( -name "vertc_${version}_*.tar.gz" -o -name "vertc_${version}_*.zip" \) | LC_ALL=C sort)
[[ "${#archives[@]}" -eq 6 ]] || die "expected six versioned archives"
for archive in "${archives[@]}"; do
  case "$archive" in
    *.tar.gz)
      skill_paths="$(tar -tzf "$archive" | grep -E '^skills/[^/]+/SKILL\.md$' || true)"
      [[ -n "$skill_paths" ]] || die "archive lacks embedded Skills: $(basename "$archive")"
      while IFS= read -r skill_path; do
        tar -xOzf "$archive" "$skill_path" | grep -Fq "version: \"$version\"" || die "Skill version mismatch in $(basename "$archive")"
      done <<< "$skill_paths"
      ;;
    *.zip)
      skill_paths="$(unzip -Z1 "$archive" | grep -E '^skills/[^/]+/SKILL\.md$' || true)"
      [[ -n "$skill_paths" ]] || die "archive lacks embedded Skills: $(basename "$archive")"
      while IFS= read -r skill_path; do
        unzip -p "$archive" "$skill_path" | grep -Fq "version: \"$version\"" || die "Skill version mismatch in $(basename "$archive")"
      done <<< "$skill_paths"
      ;;
  esac
done

[[ "$(npm view "$package_name@$version" version --json | jq -er '.')" == "$version" ]] || die "npm version is missing"
[[ "$(npm view "$package_name" "dist-tags.$npm_tag" --json | jq -er '.')" == "$version" ]] || die "npm $npm_tag does not resolve to $version"
echo "verify-release-publication: complete tag=$tag target=$target assets=7 npm=$package_name@$version npm_tag=$npm_tag prerelease=$prerelease latest=$latest"
