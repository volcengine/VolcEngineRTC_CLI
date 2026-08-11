# Automation and structured output

`vertc` uses readable output when stdout is a terminal and JSON when stdout is
piped or redirected. Coding agents, scripts, and CI should pass `--format json`
explicitly so their output does not depend on the terminal environment.

## Stream contract

- stdout contains command data only.
- stderr contains progress, warnings, and recovery hints.
- in JSON mode, a command writes one envelope to stdout.
- success exits with code 0; failure exits non-zero.

A successful result has this shape:

```json
{
  "ok": true,
  "data": {}
}
```

A known failure has this shape:

```json
{
  "ok": false,
  "error": {
    "code": "vertc.example.code",
    "type": "validation",
    "message": "...",
    "hint": "..."
  }
}
```

Fields such as `subtype` and `param` are included when relevant. Automation
should branch on `ok`, process exit status, and `error.code` before interpreting
human-language messages.

## Output formats

```bash
vertc doctor                 # pretty in a terminal, JSON when redirected
vertc doctor --format json   # stable choice for automation
vertc doctor --format pretty
vertc init --list --format table
```

Progress on stderr never changes the stdout representation. Redirect streams
separately when a pipeline needs both structured results and diagnostic logs.

## Dry runs

The global `--dry-run` flag previews side effects without applying them:

```bash
vertc init --scene voice-agent --platform web --dry-run
vertc dev --dry-run
vertc update --dry-run
```

For remote templates, a dry run may validate from memory or an existing cache,
but it does not create the target or populate the cache.
An update dry run may query the npm registry and detect the install method, but
it never invokes npm, replaces a binary, writes lifecycle state, or synchronizes
Skills. Its `status` is `would_update`, `up_to_date`, or `manual_required`.

## Authentication in automation

Authorization Code with PKCE requires a user to approve access. On a headless
host, use:

```bash
vertc auth login --browser=manual --store=file
```

The command prints the authorization URL and accepts the returned code through
stdin. Signin tokens default to the protected `$VERTC_HOME/auth.json` file,
outside the project; use `--store=keyring` only after explicit user consent.
`--browser=ask` returns `vertc.auth.interaction_required` in non-interactive
terminals instead of opening UI. After login, run `dev` directly. If a user decision is required,
it returns a typed JSON error containing public candidates:

```bash
vertc dev
# on vertc.dev.selection_required, choose from error.details
vertc dev --app-id <app-id> --bot-id <bot-id>
```

Repeat `--bot-id` when needed. A unique RTC application is selected
automatically. Existing agents require user selection; when the successful
query returns none, the built-in default scene is used. These arguments are public resource IDs; the CLI
retrieves AppKey with Signin STS and never returns it. Automation must never ask
the user for AppKey or place it in chat, command arguments, logs, or `VITE_*`.

For an environment whose project credentials and scene configuration are
already present, run `vertc doctor` as the readiness gate before `vertc dev`.

## Lifecycle notices

JSON envelopes may contain additive `_notice.update` and `_notice.skills`
objects. They are composed from local cached state and do not add synchronous
network I/O to the output path. Consumers must tolerate unknown additive notice
fields.

Agents must finish the current user task before surfacing a lifecycle notice and
must not update autonomously. `_notice.update` maps to `vertc update`;
`_notice.skills` maps to `vertc skills sync`.

Suppress them when necessary:

```bash
export VERTC_NO_UPDATE_NOTIFIER=1
export VERTC_NO_SKILLS_NOTIFIER=1
```

## Official Skills

Install the latest released official scene workflow directly from the public
repository; this path does not require `vertc` to be installed first:

```bash
npx skills add volcengine/VolcEngineRTC_CLI -g -y
```

When the CLI is already installed, align or repair global Agent runtimes from
the scene workflow embedded in that exact CLI release:

```bash
vertc skills sync
```

Synchronization copies the embedded content into supported global Agent
runtimes and requires `npx skills` to be available. To preview without writing,
run `vertc skills sync --dry-run`.

The CLI also embeds the Skill content shipped with its release:

```bash
vertc skills list
vertc skills read byted-interactai-guide
vertc skills sync
```

For every public release and snapshot artifact, the value printed
by `vertc version`, the `version` frontmatter returned by
`vertc skills read <name>`, and the corresponding archived `SKILL.md` version
are identical. Release postflight reads the embedded Markdown itself; consumers
therefore do not need repository access or a separate Skill-version mapping.

`vertc update` reuses the same synchronization after updating an npm-managed
binary. The direct public-repository path follows the latest released content;
`vertc skills sync` instead keeps the installed content tied to the running
binary release.

## Failure routing

1. Inspect the non-zero exit status and JSON `error.code`.
2. Follow the structured `hint` when present.
3. Run `vertc doctor` for readiness or configuration failures.
4. Run `vertc explain-error <code>` for Web SDK or conversational-AI runtime
   codes. These runtime codes are separate from CLI `error.code` values.
