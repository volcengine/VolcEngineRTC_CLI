# Voice-agent projects

This guide describes the `voice-agent × web` project produced by `vertc init`.
For the shortest supported path, start with the repository
[README](../README.md#five-minute-quick-start).

## Generated project

The source is the official `volcengine/rtc-aigc-demo` template pinned to a full
commit and SHA-256 digest. The first scaffold downloads the archive from GitHub
codeload and saves the verified content in the system cache; later scaffolds can
reuse that cache without a network connection.

The project keeps the template's full multi-scene application:

```text
my-agent/
├── package.json                 Root Yarn launcher
├── vertc.config.yaml            Non-secret project and identity metadata
├── .env.local                   Gitignored local runtime credentials
├── web/                         CRA web application
└── server/                      Koa local-development server
    └── scenes/
        ├── default.json         Default VoiceChat scene
        └── bot-<id>.json        Additional Console-selected bot scenes
```

Scene files contain the VoiceChat `Config` and `AgentConfig` selected from the
Console, including ASR, LLM, and TTS choices. Review them before sharing a
generated project. Runtime credentials remain outside these files.

## First-run configuration from the Console

The default `init` operation scaffolds the project without requiring a login.
It does not accept provisioning flags or contact Console APIs. Run
`vertc auth login` to complete Volcengine Signin using Authorization Code with
PKCE. Credentials default to protected `$VERTC_HOME/auth.json` file storage;
`--store=keyring` explicitly selects the OS credential store. Interactive
terminals ask before opening a browser, while agents must pass an explicit
`--browser=open|manual` mode after user consent.

The first `vertc dev` inside the generated directory then:

1. discovers available RTC applications and retrieves the selected AppId/AppKey;
2. discovers configured conversational-AI agents and asks the user to select
   when any exist; if none exist, it uses the built-in default scene;
3. writes `RTC_APP_ID` and `RTC_APP_KEY` to `.env.local`;
4. writes every selected VoiceChat configuration to a
   `server/scenes/bot-<id>.json` file without replacing `default.json`.

One RTC application is selected automatically; multiple applications require a
selection. Any existing agent requires a user selection so the built-in default
remains a zero-agent fallback. In a non-interactive terminal, `dev` returns
public candidates in a typed selection error:

```bash
vertc dev
# choose from error.details.apps or error.details.bots
vertc dev --app-id <app-id> --bot-id <bot-id>
```

Repeat `--bot-id` to select multiple bots. Run `vertc dev --reconfigure` to
select the RTC application and bot scenes again. Existing bot scene files are
preserved.

Interactive `vertc dev` is recommended because it retrieves and writes AppKey
without exposing it as a process argument. If Console discovery is unavailable,
use a local editor or secret-management workflow to write the following directly
to the Git-ignored `.env.local` file:

```dotenv
RTC_APP_ID=<app-id>
RTC_APP_KEY=<app-key>
```

Restrict `.env.local` to the current user. Never pass AppKey as a command-line
argument or paste it into logs, issue reports, or chat messages.

Then review or replace the `VoiceChat` object in
`server/scenes/default.json`.

## Development bootstrap

If either `RTC_APP_ID` or `RTC_APP_KEY` is absent, `vertc dev` discovers RTC
applications and agents after Signin. A TTY prompts for ambiguous selections; a
non-interactive caller supplies IDs returned by `vertc.dev.selection_required`. It writes the chosen
credentials to `.env.local`. Selected agents are saved as
`server/scenes/bot-<id>.json`; the existing `default.json` and unrelated scenes
are preserved. The selected scene is recorded in `agent.config_file`; a
successful zero-agent query records the built-in default fallback. `--reconfigure` forces this selection flow even when credentials
already exist.

The CLI does not create a cloud agent automatically. To customize the default
experience, open [Conversational AI agent management](https://console.volcengine.com/conversational-ai/agentManage).

`vertc doctor` is always read-only. It reports missing configuration and the
next command to run but never fills credentials or changes scene files.

## VoiceChat runtime modes

The companion server supports two control paths:

- When `VOLCENGINE_ACCESS_KEY_ID` and `VOLCENGINE_SECRET_ACCESS_KEY` are
  available, the local server signs and calls the OpenAPI directly.
- Otherwise, it delegates VoiceChat start and stop operations to the hidden
  `vertc agent start/stop` commands.

The quick-start path does not expose a browser-accessible STS endpoint. Never
place `RTC_APP_KEY`, long-term access keys, or secret keys in `VITE_*` variables
or client-side code. Never send AppKey in chat or a command argument.

## Room and user identity

The standard full-stack template generates a separate room, user, join token,
task, and target-user identity for every page session. The normal
`init → auth login → dev` flow does not require choosing these values.

### CLI-managed identity

Advanced workflows can manage identity through `vertc.config.yaml`:

- `rtc.room_id` identifies the room.
- `rtc.user_id` identifies the real user.
- `agent.user_id` identifies the AI agent.
- an empty `agent.target_user_id` follows `rtc.user_id` automatically.

Set initial values while scaffolding:

```bash
vertc init --scene voice-agent --platform web \
  --room-id room-2 --user-id alice
```

Or issue a token and write the new identity through to the project:

```bash
vertc token issue --room-id room-2 --user-id alice --write
```

The write operation updates `vertc.config.yaml` and synchronizes the derived
frontend values in `.env.local`, keeping the browser user and agent in the same
room. `RTC_APP_KEY` is never written to a frontend variable. `doctor` flags an
explicit target user that differs from `rtc.user_id`.

## Template integrity and dry runs

Remote templates are accepted only when their archive checksum and manifest
contract match the release registry. Extraction rejects absolute paths and
parent-directory traversal.

Use `--dry-run` to validate a scaffold from memory or an existing cache without
writing the target directory or cache:

```bash
vertc init --scene voice-agent --platform web --dry-run --format pretty
```
