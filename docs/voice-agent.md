<a id="zh-cn"></a>

# 语音智能体项目

[English](#english) | 简体中文

本文介绍 `vertc init` 生成的 `voice-agent × web` 项目。第一次使用时，先按 [README 快速开始](../README.md#快速开始)完成创建、登录和启动。

## 生成的项目

项目来自官方 `volcengine/rtc-aigc-demo` 模板。`vertc` 将模板固定到完整 commit 和 SHA-256 摘要：首次创建项目时从 GitHub codeload 下载并校验，之后可以复用系统缓存，离线创建同一版本的项目。

生成目录保留模板完整的多场景应用：

```text
my-agent/
├── package.json                 根目录 Yarn 启动入口
├── vertc.config.yaml            不含密钥的项目和身份配置
├── .env.local                   Git 忽略的本地运行凭据
├── web/                         CRA Web 应用
└── server/                      Koa 本地开发服务
    └── scenes/
        ├── default.json         默认 VoiceChat 场景
        └── bot-<id>.json        从控制台选择的其他智能体场景
```

场景文件保存从控制台取得的 VoiceChat `Config` 和 `AgentConfig`，其中包括 ASR、LLM 和 TTS 配置。运行凭据不在这些文件里。分享生成项目之前，先检查场景文件是否含有不适合公开的业务配置。

## 首次从控制台配置

`vertc init` 只创建项目，不要求登录，也不会调用控制台 API。创建完成后，运行 `vertc auth login`，通过 Authorization Code + PKCE 登录火山引擎。Signin 凭据默认保存在权限受限的 `$VERTC_HOME/auth.json`；只有显式传入 `--store=keyring` 时才使用操作系统凭据库。

交互终端会在打开浏览器前询问用户。Agent 或无界面环境需要先取得用户同意，再明确传入 `--browser=open`，或者使用 `--browser=manual --start` 和 `--resume` 分两步授权。

第一次在项目目录运行 `vertc dev` 时，CLI 会：

1. 查询可用的 RTC 应用，并获取所选应用的 AppID/AppKey；
2. 查询已配置的对话式 AI 智能体：只要存在智能体，就请用户选择一个或多个；没有智能体时使用内置默认场景；
3. 将 `RTC_APP_ID` 和 `RTC_APP_KEY` 写入 `.env.local`；
4. 将每个已选智能体的 VoiceChat 配置写入 `server/scenes/bot-<id>.json`，不覆盖 `default.json`。

只有一个 RTC 应用时会自动选择；存在多个应用时才要求用户选择。智能体不同：只要查询到智能体，即使只有一个，也需要用户确认。这样可以把内置默认场景明确保留为“账号下没有智能体”时的回退方案。

非交互终端无法代替用户选择。此时 `dev` 返回带公开候选项的类型化错误：

```bash
vertc dev
# 从 error.details.apps 或 error.details.bots 中选择
vertc dev --app-id <app-id> --bot-id <bot-id>
```

需要选择多个智能体时，重复传入 `--bot-id`。要重新选择 RTC 应用或智能体场景，运行 `vertc dev --reconfigure`；已有的其他场景文件不会被删除。

### 端口检查

安装依赖前，`dev` 会检查模板声明的本地端口。Web + Server 模板可以在 taskfile 中声明：

```yaml
runtime:
  ports:
    web: 3000
    server: 3001
```

使用 `--web-port` 或 `--server-port` 可以指定端口；`--auto-port` 只替换已经被占用的端口。最终端口通过 `VERTC_WEB_PORT` 和 `VERTC_SERVER_PORT` 传给模板任务。长时间运行的任务启动前，stdout 会先输出 `ports` 和 `urls`。

端口冲突时，如果操作系统允许，错误信息会给出监听进程的 PID、进程名和工作目录。`vertc` 不会替你结束该进程。

### 手动写入 RTC 凭据

推荐使用交互式 `vertc dev`，因为它能取得并写入 AppKey，且不会把 AppKey 放进进程参数。如果控制台查询不可用，请通过本地编辑器或密钥管理工具直接修改 Git 已忽略的 `.env.local`：

```dotenv
RTC_APP_ID=<app-id>
RTC_APP_KEY=<app-key>
```

`.env.local` 应限制为仅当前用户可读写。不要把 AppKey 放进命令参数，也不要粘贴到日志、Issue 或聊天消息里。手动配置凭据后，还需要检查或替换 `server/scenes/default.json` 中的 `VoiceChat` 对象。

## 开发启动流程

只要 `RTC_APP_ID` 或 `RTC_APP_KEY` 缺失，`vertc dev` 就会在 Signin 登录后查询 RTC 应用和智能体。TTY 中需要选择时会显示交互界面；非交互调用方应读取 `vertc.dev.selection_required` 返回的候选 ID，再通过 `--app-id` 或 `--bot-id` 重试。

选定的凭据写入 `.env.local`。智能体配置写入 `server/scenes/bot-<id>.json`，原有 `default.json` 和无关场景保持不变。当前场景记录在 `agent.config_file`；如果账号下没有智能体，则记录内置默认场景。已有配置时，`--reconfigure` 可以强制重新执行这套选择流程。

CLI 不会自动创建云端智能体。需要定制默认体验时，请打开[对话式 AI 智能体管理](https://console.volcengine.com/conversational-ai/agentManage)。

`vertc doctor` 始终只读。它会指出缺少的配置和下一条建议命令，但不会补写凭据或修改场景文件。

## VoiceChat 运行方式

配套 Server 有两种控制方式：

- 如果环境中同时存在 `VOLCENGINE_ACCESS_KEY_ID` 和 `VOLCENGINE_SECRET_ACCESS_KEY`，本地 Server 直接签名并调用 OpenAPI。
- 否则，Server 通过隐藏命令 `vertc agent start/stop` 启停 VoiceChat。

快速开始不会暴露浏览器可访问的 STS 接口。不要把 `RTC_APP_KEY`、长期 AccessKey 或 SecretKey 放进 `VITE_*` 变量或前端代码，也不要通过聊天消息或命令参数传递 AppKey。

## 房间与用户身份

标准全栈模板会为每个页面会话分别生成房间、用户、进房 Token、任务和目标用户身份。正常的 `init → auth login → dev` 流程不需要手动指定这些值。

### 由 CLI 管理身份

高级用法可以通过 `vertc.config.yaml` 管理身份：

- `rtc.room_id`：房间 ID；
- `rtc.user_id`：真实用户 ID；
- `agent.user_id`：AI 智能体 ID；
- `agent.target_user_id`：为空时自动跟随 `rtc.user_id`。

创建项目时指定初始值：

```bash
vertc init --scene voice-agent --platform web \
  --room-id room-2 --user-id alice
```

也可以签发 Token，并把新身份同步写回项目：

```bash
vertc token issue --room-id room-2 --user-id alice --write
```

该命令会更新 `vertc.config.yaml`，并把派生出的前端值同步到 `.env.local`，让浏览器用户和智能体留在同一房间。`RTC_APP_KEY` 永远不会写入前端变量。如果显式设置的目标用户与 `rtc.user_id` 不一致，`doctor` 会报告问题。

## 模板完整性与 dry-run

远程模板必须同时通过归档 SHA-256 和 manifest 契约校验。解压过程会拒绝绝对路径和父目录穿越。

使用 `--dry-run` 可以从内存或现有缓存校验模板，不创建目标目录，也不写入缓存：

```bash
vertc init --scene voice-agent --platform web --dry-run --format pretty
```

---

<a id="english"></a>

# Voice-agent projects

[简体中文](#zh-cn) | English

This guide describes the `voice-agent × web` project produced by `vertc init`.
For the shortest supported path, start with the repository
[README](../README.en.md#quick-start).

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

Before installing dependencies, `dev` checks the local ports declared by the
template. A web-server taskfile can declare them explicitly:

```yaml
runtime:
  ports:
    web: 3000
    server: 3001
```

Use `--web-port` and `--server-port` to override those values, or `--auto-port`
to replace only occupied ports. The resolved values are passed to template
tasks as `VERTC_WEB_PORT` and `VERTC_SERVER_PORT`, and stdout reports `ports`
and `urls` before the long-running task starts. A conflict reports the listener
PID, process name, and working directory when the operating system exposes
them. `vertc` never terminates the listener.

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
