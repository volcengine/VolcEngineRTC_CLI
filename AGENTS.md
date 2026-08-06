# AGENTS.md — contributing to vertc

Conventions for humans and Agents working in this repo. Keep changes minimal,
scoped, and consistent with the surrounding code.

## Architecture at a glance

```
main.go                 → cmd.Execute()
cmd/                    → cobra command tree (one file per command group)
internal/
  meta/                 → BinName/Version — the single identity source
  output/               → JSON envelope, stdout(data)/stderr(progress) split, --format
  errs/                 → typed error envelope + stable error.code catalog (CI-gated)
  config/               → vertc.config.yaml schema, load/save/validate, ${ENV}, sync stub
  token/                → vendored VRTC AccessToken + issue/check/inspect
  template/             → registry + pinned remote downloader/cache + renderer
    files/              → reserved location for future embedded template sources
  errorcodes/           → embedded Web SDK error-code KB (web.yaml) + loader
  doctor/               → two-level readiness checks
  affordance/           → per-command when/avoid/prereq/examples (surfaced in --help)
  env/                  → dotenv read/merge/write (secrets masked)
  auth/                 → local auth session
  skillsfs/             → reader over the embedded skills FS (list/read, path guards)
  skillscan/            → Skill quality gate (command authenticity + metadata/links/secrets)
skills/                 → Agent scene workflows (SKILL.md); skills.Content embeds them
skill-template/         → standard structure for new scene skills
tests/                  → E2E: builds the binary, drives it as a subprocess
scripts/                → CI helpers (snapshot-error-codes.sh)
```

## Rules

1. **Never hard-code the binary name.** Use `meta.BinName` in help text, hints,
   skills and affordances. Renaming the CLI must stay a one-line change.
2. **Every CLI failure is a typed `*errs.Error` with a registered `error.code`.**
   Add new codes to `errs.Catalog`, then regenerate the snapshot:
   `UPDATE_SNAPSHOT=1 go test ./internal/errs/ -run TestCatalogSnapshot`.
   `make check-error-codes` gates this in CI. CLI `error.code`s are a **separate
   system** from SDK runtime codes (those live in `internal/errorcodes`, queried
   by `explain-error`).
3. **stdout is data only.** Emit results via `output.Writer.Data`; send progress,
   warnings and hints to stderr via `Progress`/`Warn`. Never `fmt.Println` to
   stdout from a command. If a command pre-emits data then fails, mark the error
   `.Reported()` so no second envelope is written.
4. **`vertc.config.yaml` is the single source of truth.** New state belongs in
   the schema (or an `${ENV}` reference), not scattered per-command files.
   Secrets (AppKey) are env-only — never persisted to config.
5. **Side-effecting commands support `--dry-run`** and must write nothing in that
   mode (`init`, `config set`, `env write`, `dev`).
6. **Templates are immutable and version-pinned.** Embedded sources live under
   `internal/template/files/<scene>/<platform>/` as `*.tmpl`. Remote sources must
   pin a full commit and SHA-256, validate `vertc.template.yaml`, taskfile and
   scene contracts, safely extract, and remain covered by offline-cache tests.
7. **Add tests.** Unit tests colocated with the package; E2E in `tests/` for any
   new command surface. Run `make test` before sending changes.
8. **Skills are Agent 场景工作流, not command manuals** — use a thin top-level
   router for scene decisions, command order, and failure routing; put detailed
   domain knowledge in flat `references/`. Follow `skill-template/skill-template.md`.

## Commit / PR

- Keep the diff focused on one capability.
- Run `make ci` locally before pushing — it covers gofmt-check →
  vet → golangci-lint → tests → error/contract checks → build → public-surface /
  safety checks → Node installer tests. Run `make tools` once to install the
  configured pinned development tools. Auto-fix formatting/imports with `make fmt`.
- Update `README.md` when the command surface changes.
