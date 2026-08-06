# Troubleshooting

Start with the diagnostic command that matches the failure:

```bash
vertc doctor --format pretty          # CLI and current project
vertc doctor cli --format pretty      # installation only
vertc doctor project --format pretty  # current project only
```

`doctor` is read-only. Every failed check includes a recovery hint and a JSON
check identifier when run in the default format.

## `vertc` is not found

Confirm Node.js 16 or later is installed, reinstall the package, and check the
npm global binary directory on `PATH`:

```bash
npm install -g @volcengine/rtc-cli
vertc version --format pretty
```

If the package install completed but its binary download failed, verify access
to GitHub Releases. A source build with Go 1.23 or later is an alternative:

```bash
git clone https://github.com/volcengine/VolcEngineRTC_CLI.git
cd VolcEngineRTC_CLI
make build
./bin/vertc version --format pretty
```

## Template download fails

The first `voice-agent` scaffold downloads a pinned archive from GitHub
codeload. Check network or proxy access and retry the same command. A fully
verified earlier download is cached and can be reused offline.

`--dry-run` does not populate an empty cache. It can only validate from memory
during a successful network request or reuse an existing cached archive.

## Browser login does not open

Use the headless flow and open the printed URL manually:

```bash
vertc auth login --browser=manual --format pretty
```

Paste the returned authorization code into the command's stdin. Login only
establishes Signin; project App/bot configuration happens on the next `vertc dev`.
In a non-interactive terminal, the default `--browser=ask` returns
`vertc.auth.interaction_required` instead of opening UI; choose `open` only
after user consent. Credentials use protected file storage by default, or the
OS credential store after explicit `--store=keyring` selection.
For a non-interactive terminal, run `vertc dev`; when it returns
`vertc.dev.selection_required`, choose from `error.details.apps` or
`error.details.bots`, then retry with `--app-id` or `--bot-id`. If configuration fails, Signin
remains valid; retry `vertc dev --reconfigure` or use the manual configuration path in
[Voice-agent projects](./voice-agent.md#first-run-configuration-from-the-console).

## No RTC application or conversational-AI agent is available

Open the Volcengine Console and confirm that the current account has:

- an RTC application available to the signed-in identity;
- an AppKey available for that application;
- optionally, a conversational-AI agent with customized ASR, LLM, and TTS.

When agents exist, `vertc dev` asks which one to use. When none exist, it uses
the built-in default scene and links to
[agent management](https://console.volcengine.com/conversational-ai/agentManage).
Retry with `vertc dev --reconfigure` to select again.
`doctor` reports whether Signin, AppKey, scene data, and target identity are
ready.

## `RTC_APP_KEY is not set`

Run `vertc auth login`, then run `vertc dev`; in a non-interactive terminal,
choose only public App/Bot IDs from a `vertc.dev.selection_required` error's
`details`. The CLI writes AppId/AppKey to `.env.local` without exposing AppKey in
process arguments. If a manual fallback is required, use the
local editor or secret-management workflow described in
[First-run configuration from the Console](./voice-agent.md#first-run-configuration-from-the-console).
Never pass AppKey as a command-line argument or put it in `vertc.config.yaml`, a
`VITE_*` variable, client-side code, chat, logs, or an issue report. Confirm
`.env.local` remains ignored by Git.

## Agent and browser are not interacting

Run `vertc doctor project --format pretty` and check room/user consistency. In
CLI-managed workflows, `rtc.room_id`, `rtc.user_id`, and the agent target must
refer to the same conversation. See
[Room and user identity](./voice-agent.md#room-and-user-identity).

## A runtime error code appears

Query the embedded offline knowledge base:

```bash
vertc explain-error <code> --format pretty
```

The result includes the known meaning, recommended fix, and related doctor
check when available.

## Still blocked

Read [SUPPORT.md](../SUPPORT.md) before opening an issue. Include the CLI
version, operating system, command, exit code, redacted JSON error envelope, and
relevant `doctor` checks. Never include credentials, tokens, `.env.local`, or
private Console data.
