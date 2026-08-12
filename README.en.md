# vertc

[![CI](https://github.com/volcengine/VolcEngineRTC_CLI/actions/workflows/ci.yml/badge.svg)](https://github.com/volcengine/VolcEngineRTC_CLI/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/volcengine/VolcEngineRTC_CLI?label=release)](https://github.com/volcengine/VolcEngineRTC_CLI/releases)
[![npm](https://img.shields.io/npm/v/@volcengine/rtc-cli?label=npm)](https://www.npmjs.com/package/@volcengine/rtc-cli)
[![Go Version](https://img.shields.io/badge/Go-%3E%3D1.25.12-00ADD8?logo=go)](./go.mod)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](./LICENSE)

[简体中文](./README.md) | English

`vertc` is a CLI for Volcengine RTC projects. Developers and coding agents can use it to create, configure, run, and diagnose AI audio/video applications.

> **Current scope:** `voice-agent × web`. More scenes and platforms will be added when their end-to-end flows are ready.

[Install](#install) · [Quick start](#quick-start) · [Core capabilities](#core-capabilities) · [Agent and CI](#agent-and-ci) · [Security](#configuration-and-security) · [Documentation](#documentation)

## Install

### Prebuilt binaries

Available for macOS, Linux, and Windows on amd64 and arm64. Installation requires Node.js 16 or later:

```bash
npm install -g @volcengine/rtc-cli
vertc version
```

The installer downloads the binary matching the npm package version and current platform from GitHub Releases, then verifies it against `checksums.txt`.

### Build locally

Requires Go 1.25.12 or later:

```bash
git clone https://github.com/volcengine/VolcEngineRTC_CLI.git
cd VolcEngineRTC_CLI
make build
./bin/vertc version
```

## Quick start

Before you start, make sure your Volcengine account has an RTC application. A conversational-AI agent is optional; if the account has none, `dev` uses the built-in default scene.

```bash
# 1. Create a project
vertc init ./my-agent --scene voice-agent --platform web
cd my-agent

# 2. Sign in to Volcengine
vertc auth login

# 3. Configure RTC resources and start the web app and local server
vertc dev
```

Open the URL printed in the terminal and click **Start** to join the room and talk. On the first run, `vertc` finds the RTC applications and conversational-AI agents in the account. It prompts only when there is more than one choice. If setup or runtime fails, run `vertc doctor`; it inspects the problem without changing the project.

See [Voice-agent projects](./docs/voice-agent.md) for the generated layout, runtime options, and identity management.

## Core capabilities

- **Create and run projects** — `init` creates a project from a pinned official template. `dev` configures Console resources and starts the web app and local server.
- **Diagnose problems** — `doctor` reports PASS/WARN/SKIP/UNKNOWN/FAIL checks; `explain-error` looks up SDK and conversational-AI errors offline.
- **Read RTC documentation** — `docs search/fetch/list` queries the live, read-only documentation endpoint without project configuration or signin.
- **Use it from agents and scripts** — pipes and redirected output default to JSON, stdout contains data only, and failures return a stable `error.code`. Side-effecting commands support `--dry-run`.
- **Keep configuration and credentials separate** — project metadata, runtime secrets, scene data, and Signin credentials use separate storage.

### Common commands

| Command | Purpose |
| --- | --- |
| `init [dir] --scene <scene> --platform <platform>` | List or scaffold supported project templates |
| `auth login` / `auth status` / `auth logout` | Manage the Volcengine Signin session |
| `dev [--web-port N] [--server-port N] [--auto-port]` | Configure RTC resources, check ports, and run the project |
| `doctor [cli\|project]` | Check CLI and project readiness without changing the project |
| `explain-error <code>` | Look up SDK and conversational-AI errors offline |
| `docs search/fetch/list` | Search, fetch, or browse RTC documentation |
| `skills list/read/sync` | Inspect or synchronize the official Skill embedded in the current release |
| `update [--check\|--force]` | Check or update an npm-managed installation |

Run `vertc <command> --help` for prerequisites, complete options, and examples.

## Agent and CI

Install the official Skill:

```bash
npx skills add volcengine/VolcEngineRTC_CLI -g -y
```

To match the Skill to the current `vertc` version or repair an existing installation:

```bash
vertc skills sync
```

Agents and automation scripts should pass `--format json` explicitly so output does not depend on the terminal environment. stdout contains data only, while progress and warnings go to stderr. Failures return a non-zero exit code and a stable `error.code`. See [Automation and structured output](./docs/automation.md) for headless authorization, non-interactive resource selection, error handling, and notification settings.

## Configuration and security

- `vertc.config.yaml` contains only non-secret project metadata and `${ENV}` references.
- `.env.local` contains local runtime values, is Gitignored, and is written with restricted permissions; never commit it.
- Signin credentials are stored in the protected `$VERTC_HOME/auth.json` file by default. You can choose the operating-system keyring instead.
- `RTC_APP_KEY` is never written to project configuration, `VITE_*` frontend variables, logs, command arguments, or structured output; never provide AppKey in chat.

See [Automation and structured output](./docs/automation.md) for authentication modes, credential storage, and automation safety boundaries. See [SECURITY.md](./SECURITY.md) for vulnerability reporting.

## Documentation

- [Voice-agent projects](./docs/voice-agent.md) — generated layout, first-run configuration, runtime modes, and identity behavior
- [Automation and structured output](./docs/automation.md) — authentication, JSON envelopes, error routing, dry runs, notices, and Skills
- [Troubleshooting](./docs/troubleshooting.md) — installation, authentication, templates, credentials, and runtime recovery
- [CHANGELOG.md](./CHANGELOG.md) — release changes
- [SUPPORT.md](./SUPPORT.md) — where to ask questions or report bugs
- [CONTRIBUTING.md](./CONTRIBUTING.md) — development setup and contribution workflow

## Development and contributing

```bash
make build
make test
make ci
```

Read [CONTRIBUTING.md](./CONTRIBUTING.md) before opening a pull request. This project is licensed under the [MIT License](./LICENSE).
