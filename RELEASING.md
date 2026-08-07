# Releasing vertc

Public versions are published from annotated tags on GitHub `main`. The
tag-triggered workflow builds and verifies the platform archives, creates the
GitHub Release, and publishes `@volcengine/rtc-cli` to npm.

## Version identity and channels

Every checked-in official `skills/<name>/SKILL.md` uses the reserved valid
SemVer placeholder `0.0.0-dev`. Release preparation treats the requested tag as
the version authority and replaces that placeholder only in an isolated source
copy. One canonical version is then reused for CLI metadata, embedded and
archived Skills, archive names, checksums, and npm metadata.

The public-source audit is version-independent. After the reviewed tree reaches
GitHub `main`, choose the release version and run the committed-source contract
preflight against that exact commit before requesting publication approval. A
later version change leaves the unchanged source review valid but invalidates
prepared release artifacts, release notes, and publication approval.

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
- every checked-in official Skill uses `0.0.0-dev`, while every prepared and
  archived Skill uses the requested release version;
- the release commit is present on GitHub `main` and all required checks pass;
- `CHANGELOG.md`, `LICENSE`, `NOTICE`, the READMEs, and every official Skill are
  ready;
- GitHub Actions has `contents: write` through `GITHUB_TOKEN`;
- the `NPM_TOKEN` repository secret can publish `@volcengine/rtc-cli` publicly;
- `make ci` and `make check-release-files` pass; and
- the requested tag and npm version do not already exist.

For local artifact confidence without publication, install the pinned release
tool once and run:

```bash
make release-tools
make release-snapshot
make release-snapshot-test
```

## Publish

1. After public-source review and GitHub synchronization, freeze the version and
   run the committed-source contract preflight against exact GitHub `main`.
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
