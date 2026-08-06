# Contributing to vertc

Thanks for helping build vertc. This is the *how-to* for common contributions;
the architecture and hard rules live in [AGENTS.md](./AGENTS.md) — read it first.

## Setup

```bash
make tools     # install pinned golangci-lint and Gitleaks (once)
make ci        # full local gate, including public-surface and open-source release checks
```

Branch off `main`, keep the diff focused on one capability, and make sure
`make ci` is green before opening a pull request. Auto-fix formatting with
`make fmt`.

`make check-change-contract` compares the branch with its merge base and makes
sure stable CLI surfaces change alongside their validation: commands need a
command or E2E test, error codes need their snapshot, and template sources need
template tests. It uses the MR diff base or push base in CI, and `origin/HEAD`
when run locally.

## Contribution recipes

### Add / change a command
1. Add `cmd/<name>.go` and register it in `cmd/root.go`.
2. Emit results via `output.Writer.Data` (**stdout = data only**); send progress
   and hints to stderr via `Progress`/`Warn`.
3. Attach an `affordance.Affordance` (when / avoid / prereq / examples) — it is
   surfaced in `--help` and consumed by Skills.
4. Support `--dry-run` if the command has side effects, and **write nothing** in
   that mode.
5. Add unit tests, plus an E2E case in `tests/` for any new command surface.

### Add / change an `error.code` (CLI contract)
1. Register it in `errs.Catalog` (`internal/errs/catalog.go`) with a one-line summary.
2. Emit it via `errs.New(...)` / `errs.Wrap(...)`; shape is `vertc.<domain>.<subtype>`.
3. Regenerate the snapshot:
   `UPDATE_SNAPSHOT=1 go test ./internal/errs/ -run TestCatalogSnapshot`.
4. **Deleting or renaming** a code is a breaking contract change → needs
   maintainer review + a `CHANGELOG.md` entry + a MAJOR version bump.

### Add a `scene × platform` template (low-barrier)
1. Add sources under `internal/template/files/<scene>/<platform>/` as `*.tmpl`.
2. Register it in `internal/template/registry.go` (`Available: true`, pin the SDK).
3. Include a `vertc.taskfile.yaml.tmpl` with `post_create` / `install` / `dev`.
4. Add a render test asserting the file tree + core链路 markers (see `render_test.go`).
5. A generated project must pass `vertc doctor` once config + token are filled.

You do not need to touch `cmd/` or `internal/errs/` to add a template.

### Add a scene skill (low-barrier)
1. Copy `skill-template/skill-template.md` into `skills/byted-<product-code>-<skill-name>/SKILL.md`.
2. Keep it an 接入剧本 (scene recognition → command order → failure routing),
   not a flag manual.

## Stable contracts — don't break casually

The `error.code` values, the stdout JSON envelope shape, exit-code semantics, and
affordance fields are the **public contract** that Agents and Skills depend on.
Changing them is a MAJOR version bump and requires maintainer sign-off. See
[CHANGELOG.md](./CHANGELOG.md).

## Security

Do not commit credentials, private endpoints, local filesystem paths, or other
environment-specific data. Use documented environment variables for secrets.

## Pull request checklist

- [ ] One capability per change
- [ ] `make ci` green
- [ ] Tests added/updated
- [ ] `README.md` updated if the command surface changed
- [ ] `CHANGELOG.md` entry under `Unreleased`
