# vertc

[![CI](https://github.com/volcengine/VolcEngineRTC_CLI/actions/workflows/ci.yml/badge.svg)](https://github.com/volcengine/VolcEngineRTC_CLI/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/Go-%3E%3D1.23-00ADD8?logo=go)](./go.mod)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](./LICENSE)

English | [简体中文](./README.zh-CN.md)

`vertc` is the Volcengine RTC developer-workflow CLI for creating,
configuring, diagnosing, and running AI audio/video projects. The current
release turns the official `rtc-aigc-demo` web voice-agent template into a
guided, runnable workflow for both developers and coding agents.

> **Current scope:** `voice-agent × web`. More scenes and platforms will be
> added only after their end-to-end workflows are ready.

[Quick start](#five-minute-quick-start) · [Agent & CI](#agent-and-ci-workflows) ·
[Commands](#commands) · [Security](#configuration-and-security) ·
[Documentation](#documentation) · [Contributing](#contributing)

## Why vertc?

- **One guided workflow** — scaffold a pinned official template, configure it
  from the Console, check readiness, and start local development.
- **Console-aware setup** — Sign in once, then let the first `dev` run discover
  your RTC applications and conversational-AI agents and configure the project.
- **Diagnostics first** — `doctor` reports evidence-based PASS/WARN/SKIP/UNKNOWN/FAIL checks;
  `explain-error` provides an offline SDK and conversational-AI error catalog.
- **Agent-native output** — JSON is the default, stdout stays machine-readable,
  errors have stable `error.code` values, and official Skills provide workflow
  playbooks.

## Requirements

- macOS, Linux, or Windows on amd64 or arm64
- Node.js 16 or later for the recommended npm installation
- Go 1.23 or later only when installing or building from source
- A Volcengine account with an RTC application; a conversational-AI agent is
  optional because `dev` can use the built-in default scene
- Access to GitHub Releases for npm installation and GitHub codeload for the
  first template download; later scaffolds can reuse the verified local cache

## Install

Install the prebuilt binary from npm:

```bash
npm install -g @volcengine/rtc-cli
vertc version --format pretty
```

The installer downloads the binary matching the package version and current
platform, then verifies it against the release `checksums.txt` before making it
available as `vertc`.

You can also build from source:

```bash
git clone https://github.com/volcengine/VolcEngineRTC_CLI.git
cd VolcEngineRTC_CLI
make build
./bin/vertc version --format pretty
```

Manually managed binaries are not overwritten by `vertc update`; the command
prints the appropriate upgrade guidance instead.

## Five-minute quick start

You can start without creating a conversational-AI agent first. When the
signed-in account has no agent, `dev` uses the built-in default scene and links
to the Console for later customization.

```bash
# 1. Create the project from the verified rtc-aigc-demo template.
vertc init ./my-agent --scene voice-agent --platform web --format pretty
cd my-agent

# 2. Sign in and store the refreshable Signin token.
vertc auth login --format pretty

# 3. On first run, configure an RTC application and optionally select agents;
#    vertc then writes the local runtime configuration and starts web + server.
vertc dev --format pretty
```

The first positional argument to `init` is the target directory; here the
project is scaffolded into `./my-agent`.

When authorization is needed, interactive `auth login` asks whether to open a
browser, use a manual link/code flow, or cancel. In an agent, remote, or
headless session, obtain user consent and run `vertc auth login --browser=open`
or use the resumable manual flow; no UI opens implicitly:

```bash
# Agent turn 1: return data.authorization_url to the user, then exit.
vertc auth login --browser=manual --start

# Agent turn 2: send the code returned by that URL to the saved transaction.
printf '%s\n' '<authorization-code>' | vertc auth login --resume
```

The second command must use `--resume`: starting `--browser=manual` again creates
a different OAuth state and invalidates the code from the first URL. A pending
manual authorization expires after `--timeout` (five minutes by default) and can
be consumed only once. The first interactive `dev` run automatically uses a
unique RTC application and prompts when a choice is required. If one or more
conversational-AI agents exist, the user chooses which to use; if none exist,
the built-in default scene is used. Run `vertc dev --reconfigure` to select
again later; existing bot scene files are preserved.

When a usable Signin token is already stored, `auth login` reuses it (refreshing
it when near expiry) without prompting or opening a browser. Use
`vertc auth login --force` with an explicit browser mode to reauthorize or
switch accounts.

Signin access and refresh tokens default to the protected
`$VERTC_HOME/auth.json` file (`~/.vertc/auth.json` when `VERTC_HOME` is unset).
Choose the operating-system credential store explicitly with
`vertc auth login --store=keyring`; later commands keep using the selected store.

In an agent or another non-interactive terminal, run `vertc dev`. A
`vertc.dev.selection_required` error contains public candidates in
`error.details.apps` or `error.details.bots`; ask the user to choose, then retry
with `vertc dev --app-id <id>` or `vertc dev --bot-id <id>`.
The signed-in CLI retrieves AppKey itself; never send AppKey to an agent or put
it in command arguments.

The generated project contains the full `rtc-aigc-demo` web UI and local server.
After first-run configuration, `vertc dev` starts both processes; each browser
page session receives isolated room, user, token, task, and target-user
identities. Run `vertc doctor --format pretty` whenever setup or runtime fails;
it diagnoses without modifying the project.
See [Voice-agent projects](./docs/voice-agent.md) for the generated layout,
runtime modes, manual configuration, and advanced identity management.

## Agent and CI workflows

Register the official workflow Skill using either form:

The published Skill identifier is `byted-interactai-guide`.

```bash
# From the installed CLI (no repository access required)
vertc skills sync

# Or directly from the public repository
npx skills add volcengine/VolcEngineRTC_CLI -g -y
```

Agent and automation runs should keep the default JSON format:

```bash
vertc init ./my-agent --scene voice-agent --platform web
cd my-agent
vertc auth status
vertc dev
vertc doctor
```

- stdout contains one JSON envelope with `ok` plus `data` or `error`.
- progress, warnings, and recovery hints go to stderr.
- failures use non-zero exit codes and stable `error.code` values.
- side-effecting commands accept the global `--dry-run` flag.
- `VERTC_NO_UPDATE_NOTIFIER=1` and `VERTC_NO_SKILLS_NOTIFIER=1` suppress
  additive lifecycle notices when required by a controlled environment.

See [Automation and structured output](./docs/automation.md) for headless
authorization, non-interactive resource selection, JSON envelopes, failure
routing, and Skill contracts.

## Commands

| Command | Purpose |
| --- | --- |
| `init [dir] --scene <scene> --platform <platform>` | List or scaffold supported project templates into `[dir]`; supports `--dry-run` |
| `auth login [--browser=ask\|open\|manual] [--start\|--resume] [--store=file\|keyring]` / `auth status/logout` | Manage Volcengine Signin credentials with explicit UI, resumable Agent login, and storage choices |
| `doctor [cli\|project]` | Diagnose CLI and project readiness without modifying the project |
| `dev [--reconfigure] [--app-id <id>] [--bot-id <id>]` | Configure RTC resources and run the template; unambiguous choices are automatic, selection errors return public candidates, and zero agents use the built-in default scene |
| `explain-error <code>` | Look up Web SDK and conversational-AI errors offline |
| `skills list/read/sync` | Inspect, install, or refresh the official Skill content embedded in this release |
| `update [--check\|--force] [--dry-run]` | Check, preview, or update npm-managed installs and synchronize official Skills |
| `version` | Print build version, commit, and date |

Run `vertc <command> --help` for prerequisites, examples, and advice about when
to use or avoid each command. Advanced manual/debugging commands remain hidden
from the main help so the supported onboarding path stays small.

## Configuration and security

`vertc` deliberately separates project metadata, runtime secrets, scene data,
and Signin credentials:

| Location | Contains | Safety boundary |
| --- | --- | --- |
| `vertc.config.yaml` | Non-secret project metadata and CLI-managed RTC/agent identity references | Safe to review; secrets are referenced through `${ENV}` placeholders |
| `.env.local` | Local runtime values such as `RTC_APP_ID` and `RTC_APP_KEY` | Gitignored and written with restricted permissions; never commit it |
| `server/scenes/*.json` | VoiceChat ASR/LLM/TTS and agent configuration selected from the Console | Server-side scene configuration; review before sharing |
| `$VERTC_HOME/auth.json` (default `~/.vertc/auth.json`) | Selected store and, in `file` mode, Signin access/refresh tokens | Directory `0700`, file `0600`; treat it like a password and never commit it |
| OS credential store (optional `keyring` mode) | Signin access and refresh tokens | Used only after explicit `auth login --store=keyring` selection |

`RTC_APP_KEY` is never written to `vertc.config.yaml`, a `VITE_*` frontend
variable, logs, or structured output. It may be stored in the gitignored
`.env.local` file so new shells and the companion server can use it. Exported
process environment variables take precedence over values in that file. Never
provide AppKey in chat or command arguments.

For vulnerability reporting, see [SECURITY.md](./SECURITY.md). Do not include
credentials, tokens, private endpoints, or sensitive Console data in a public
issue.

## Documentation

- [Voice-agent projects](./docs/voice-agent.md) — generated layout, first-run
  configuration, runtime modes, and identity behavior
- [Automation and structured output](./docs/automation.md) — JSON envelopes,
  error routing, dry runs, notices, and Skills
- [Troubleshooting](./docs/troubleshooting.md) — installation, authentication,
  template, credential, and runtime recovery
- [CONTRIBUTING.md](./CONTRIBUTING.md) — setup and contribution recipes
- [AGENTS.md](./AGENTS.md) — architecture and repository engineering contracts
- [SUPPORT.md](./SUPPORT.md) — where to ask questions or report bugs
- [CHANGELOG.md](./CHANGELOG.md) — release changes

## Development

```bash
make build              # build ./bin/vertc with version metadata
make test               # unit and end-to-end tests
make check-error-codes  # validate the stable error.code catalog
make ci                 # run the complete repository gate
```

## Contributing

Contributions are welcome. Read [CONTRIBUTING.md](./CONTRIBUTING.md) before
opening a pull request, and use [SUPPORT.md](./SUPPORT.md) to choose the right
channel for questions, bugs, feature requests, and security reports.

## License

This project is licensed under the [MIT License](./LICENSE).
