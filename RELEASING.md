# Releasing vertc

Public versions are published from annotated tags on GitHub `main`. The
tag-triggered workflow builds and verifies the platform archives, creates the
GitHub Release, and publishes `@volcengine/rtc-cli` to npm.

## Version identity and channels

Every official `skills/<name>/SKILL.md` and the root `package.json` carry the
real stable or prerelease version that will be committed before publication.
Stable and prerelease tags must exactly match those committed values. One
canonical version is reused for Git source, CLI metadata, embedded and archived
Skills, archive names, checksums, and npm metadata. Never reset checked-in
Skills to `0.0.0-dev` after a release.

Prepare the version as a reviewed change before publication. After the reviewed
commit reaches GitHub `main`, run the committed-source contract preflight
against that exact commit before creating the tag. A later version change
requires a new reviewed commit and invalidates prepared artifacts, release
notes, and previous verification results.

```bash
./scripts/preflight-public-release.sh \
  --stability stable --version X.Y.Z --ref <github-main-commit>
```

| Tag | GitHub channel | npm channel |
| --- | --- | --- |
| `vX.Y.Z` | Latest release | `latest` |
| `vX.Y.Z-ID` | Pre-release, not Latest | `next` |

For example, `v0.0.1-rc.1` produces a GitHub Pre-release that is not Latest,
while npm `next` resolves to `@volcengine/rtc-cli@0.0.1-rc.1` and npm
`latest` remains unchanged.

## Prerequisites

Before publishing, confirm:

- the requested version passed `preflight-public-release.sh` against the exact
  GitHub `main` commit;
- root `package.json` and every checked-in official Skill use the exact requested
  stable or prerelease version;
- the release commit is present on GitHub `main` and all required checks pass;
- `CHANGELOG.md`, `LICENSE`, `NOTICE`, the READMEs, and every official Skill are
  ready;
- GitHub Actions has `contents: write` through `GITHUB_TOKEN`;
- the `NPM_TOKEN` repository secret can publish `@volcengine/rtc-cli` publicly;
- `make ci` and `make check-release-files` pass; and
- the requested tag and npm version do not already exist.

Prepare the version as a reviewed change:

```bash
make prepare-release-version RELEASE_VERSION=0.0.4
git diff -- package.json 'skills/*/SKILL.md'
make ci
```

Commit the resulting metadata on a branch and send it through the maintainers'
normal review process. The command changes files only; it never commits, pushes,
tags, or publishes. After the reviewed commit reaches GitHub `main`, verify the
exact source before continuing:

```bash
make check-release-version RELEASE_VERSION=0.0.4
```

For local artifact confidence without publication, install the pinned release
tool once and run:

```bash
make release-tools
make release-snapshot
make release-snapshot-test
```

## Publish

1. Verify the committed version with `make check-release-version`, then run the
   committed-source contract preflight against exact GitHub `main`.
2. Prepare complete release notes.
3. Create an annotated SemVer tag on the reviewed GitHub `main` commit. Stable
   tags use `vX.Y.Z`; prerelease tags use a SemVer suffix such as
   `vX.Y.Z-rc.1`.
4. Push the tag without force. This triggers
   `.github/workflows/publish-release.yml`.
5. Wait for the workflow to build and verify the six platform archives plus
   `checksums.txt`, publish npm, finalize the GitHub Release, and verify the
   resulting channels.
6. Run `scripts/verify-release-publication.sh` with the tag, target commit, same
   release-notes file, expected channel flags, and workflow run ID for an
   independent final check.

The workflow publishes npm before making the GitHub Release visible. A failed
npm publication therefore leaves the GitHub Release as a Draft.

## Retry and correction policy

Never move a published tag, replace Release assets, or overwrite an npm
version. A retry may reuse a Draft only when its tag, target, title, notes, flags,
and assets exactly match the requested release. It may reuse an npm version only
when the package content matches. Any conflict stops publication with evidence.

Infrastructure failures may retry the same version. Content changes require a
new version.
