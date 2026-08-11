#!/usr/bin/env bash
# Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
# SPDX-License-Identifier: MIT

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
assets="$tmp/assets"
archive_root="$tmp/archive-root"
fakebin="$tmp/bin"
mkdir -p "$assets" "$archive_root/skills/release-test" "$fakebin"

version="0.0.1-rc.2"
tag="v$version"
target="0123456789abcdef0123456789abcdef01234567"
cat > "$archive_root/skills/release-test/SKILL.md" <<EOF
---
name: release-test
version: "$version"
---
EOF

for target_name in darwin_amd64 darwin_arm64 linux_amd64 linux_arm64; do
  tar -C "$archive_root" -czf "$assets/vertc_${version}_${target_name}.tar.gz" skills
done
for target_name in windows_amd64 windows_arm64; do
  (cd "$archive_root" && zip -qr "$assets/vertc_${version}_${target_name}.zip" skills)
done
(cd "$assets" && shasum -a 256 vertc_* > checksums.txt)

notes="$tmp/notes.md"
printf 'Release candidate notes\n' > "$notes"
assets_json="$(find "$assets" -maxdepth 1 -type f -exec basename {} \; | LC_ALL=C sort | jq -R . | jq -s 'map({name:.})')"
jq -n --arg tag "$tag" --arg target "$target" --rawfile body "$notes" --argjson assets "$assets_json" \
  '{tagName:$tag,name:"vertc 0.0.1-rc.2",body:$body,isDraft:false,isPrerelease:true,targetCommitish:$target,assets:$assets}' \
  > "$tmp/release.json"

cat > "$fakebin/gh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
case "${1:-} ${2:-}" in
  'api repos/test/repo/git/ref/tags/v0.0.1-rc.2')
    case "${4:-}" in
      .object.type) printf '%s\n' tag ;;
      .object.sha) printf '%s\n' test-tag-object ;;
      *) echo "unexpected tag ref query: $*" >&2; exit 2 ;;
    esac
    ;;
  'api repos/test/repo/git/tags/test-tag-object')
    printf '%s\n' "$TEST_TARGET"
    ;;
  'api repos/test/repo/contents/package.json?ref=v0.0.1-rc.2')
    printf '%s\n' '{"name":"@volcengine/rtc-cli","version":"0.0.1-rc.2"}'
    ;;
  "api repos/test/repo/git/trees/$TEST_TARGET?recursive=1")
    printf '%s\n' 'skills/release-test/SKILL.md'
    ;;
  'api repos/test/repo/contents/skills/release-test/SKILL.md?ref=v0.0.1-rc.2')
    printf '%s\n' '---' 'name: release-test' "version: \"${TEST_TAG_SKILL_VERSION:-0.0.1-rc.2}\"" '---' '' '# Fixture' "${TEST_TAG_SKILL_BODY:-}"
    ;;
  'api repos/test/repo/releases/latest')
    printf '%s\n' v0.0.1
    ;;
  'release view')
    cat "$TEST_RELEASE_JSON"
    ;;
  'release download')
    destination=""
    while [[ $# -gt 0 ]]; do
      if [[ "$1" == --dir ]]; then destination="$2"; shift 2; else shift; fi
    done
    cp "$TEST_ASSETS/"* "$destination/"
    ;;
  *)
    echo "unexpected gh command: $*" >&2
    exit 2
    ;;
esac
EOF

cat > "$fakebin/npm" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
case "${2:-} ${3:-}" in
  '@volcengine/rtc-cli@0.0.1-rc.2 version') printf '%s\n' '"0.0.1-rc.2"' ;;
  '@volcengine/rtc-cli dist-tags.latest') printf '%s\n' '"0.0.1-rc.2"' ;;
  *) echo "unexpected npm command: $*" >&2; exit 2 ;;
esac
EOF
chmod +x "$fakebin/gh" "$fakebin/npm"

PATH="$fakebin:$PATH" \
TEST_TARGET="$target" \
TEST_RELEASE_JSON="$tmp/release.json" \
TEST_ASSETS="$assets" \
  "$repo_root/scripts/verify-release-publication.sh" \
    --repository test/repo \
    --tag "$tag" \
    --target "$target" \
    --notes-file "$notes" \
    --prerelease true \
    --latest false \
    --npm-tag latest >/dev/null

if PATH="$fakebin:$PATH" \
  TEST_TARGET="$target" \
  TEST_RELEASE_JSON="$tmp/release.json" \
  TEST_ASSETS="$assets" \
  TEST_TAG_SKILL_VERSION="0.0.0-dev" \
  TEST_TAG_SKILL_BODY='version: "0.0.1-rc.2"' \
  "$repo_root/scripts/verify-release-publication.sh" \
    --repository test/repo \
    --tag "$tag" \
    --target "$target" \
    --notes-file "$notes" \
    --prerelease true \
    --latest false \
    --npm-tag latest >/dev/null 2>&1; then
  echo "verify-release-publication-test: mismatched tag Skill version was accepted" >&2
  exit 1
fi

echo "verify-release-publication-test: passed"
