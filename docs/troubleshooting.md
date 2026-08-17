<a id="zh-cn"></a>

# 故障排查

[English](#english) | 简体中文

先运行和问题范围对应的诊断命令：

```bash
vertc doctor --format pretty          # CLI 和当前项目
vertc doctor cli --format pretty      # 只检查安装环境
vertc doctor project --format pretty  # 只检查当前项目
```

`doctor` 只读，不会修改项目。每个失败检查都会给出恢复提示；JSON 输出中还会包含稳定的检查 ID。

## 找不到 `vertc`

确认已经安装 Node.js 16 或更高版本，然后重新安装 npm 包，并检查 npm 全局二进制目录是否在 `PATH` 中：

```bash
npm install -g @volcengine/rtc-cli
vertc version --format pretty
```

如果 npm 包安装成功，但二进制下载失败，请检查是否能访问 GitHub Releases。也可以使用 Go 1.25.12 或更高版本从源码构建：

```bash
git clone https://github.com/volcengine/VolcEngineRTC_CLI.git
cd VolcEngineRTC_CLI
make build
./bin/vertc version --format pretty
```

## 模板下载失败

第一次创建 `voice-agent` 项目时，CLI 会从 GitHub codeload 下载固定版本的归档。请检查网络或代理，然后重试原命令。已经完整下载并校验通过的归档会进入缓存，之后可以离线复用。

空缓存下运行 `--dry-run` 不会预下载模板。它只能校验一次成功网络请求得到的内存内容，或复用已有缓存。

## 登录时没有打开浏览器

可以改用手动登录，并自行打开命令输出的 URL：

```bash
vertc auth login --browser=manual --format pretty
```

把授权后返回的授权码粘贴到命令的 stdin。登录只建立 Signin，RTC 应用和智能体配置要等下一次 `vertc dev` 才会处理。

非交互终端使用默认 `--browser=ask` 时，CLI 返回 `vertc.auth.interaction_required`，不会打开界面。只有取得用户同意后，才使用 `--browser=open`。凭据默认保存在权限受限的文件中；显式选择 `--store=keyring` 后才使用操作系统凭据库。

非交互终端运行 `vertc dev` 后，如果收到 `vertc.dev.selection_required`，请从 `error.details.apps` 或 `error.details.bots` 选择公开 ID。选择参数需要累积保留：先用 `--app-id <app-id>` 选应用；随后收到智能体候选时，用 `--app-id <app-id> --bot-id <bot-id>` 重试。配置失败不会清除 Signin 登录态，可以运行 `vertc dev --reconfigure` 重试，或按[首次从控制台配置](./voice-agent.md#首次从控制台配置)手动处理。

## 没有可用的 RTC 应用或对话式 AI 智能体

打开火山引擎控制台，确认当前账号具备：

- 当前登录身份可用的 RTC 应用；
- 对应应用的 AppKey；
- 可选：已经配置 ASR、LLM 和 TTS 的对话式 AI 智能体。

只要账号下存在智能体，`vertc dev` 就会让用户选择；没有智能体时使用内置默认场景，并给出[智能体管理](https://console.volcengine.com/conversational-ai/agentManage)入口。需要重新选择时运行 `vertc dev --reconfigure`。

`doctor` 会检查 Signin、AppKey、场景数据和目标身份是否就绪。

## `RTC_APP_KEY is not set`

先运行 `vertc auth login`，再运行 `vertc dev`。非交互终端收到 `vertc.dev.selection_required` 时，只从错误 `details` 中选择公开的 AppID 或 BotID。

CLI 会把 AppID/AppKey 写入 `.env.local`，不会把 AppKey 暴露在进程参数中。如果必须手动配置，请使用本地编辑器或密钥管理工具，具体步骤见[首次从控制台配置](./voice-agent.md#首次从控制台配置)。

不要把 AppKey 放进命令参数、`vertc.config.yaml`、`VITE_*` 变量、前端代码、聊天、日志或 Issue。确认 `.env.local` 仍然被 Git 忽略。

## 智能体和浏览器没有互相响应

运行 `vertc doctor project --format pretty`，检查房间和用户身份是否一致。在 CLI-managed 流程中，`rtc.room_id`、`rtc.user_id` 和智能体目标用户必须属于同一次对话。详见[房间与用户身份](./voice-agent.md#房间与用户身份)。

## 出现运行时错误码

查询内嵌的离线知识库：

```bash
vertc explain-error <code> --format pretty
```

如果知识库收录了该错误，结果会给出含义、建议处理方式和相关 doctor 检查。

## 仍然无法解决

提交 Issue 前先阅读 [SUPPORT.md](../SUPPORT.md)。请提供 CLI 版本、操作系统、命令、退出码、脱敏后的 JSON 错误信封，以及相关 `doctor` 检查。

不要提交凭据、Token、`.env.local` 或控制台私有数据。

---

<a id="english"></a>

# Troubleshooting

[简体中文](#zh-cn) | English

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
to GitHub Releases. A source build with Go 1.25.12 or later is an alternative:

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
`vertc.dev.selection_required`, choose a public ID from `error.details.apps` or
`error.details.bots`. Keep earlier selections on every retry: first select the
application with `--app-id <app-id>`, then retry an agent selection with
`--app-id <app-id> --bot-id <bot-id>`. If configuration fails, Signin remains
valid; retry `vertc dev --reconfigure` or use the manual configuration path in
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

Read [SUPPORT.md](../SUPPORT.md#english) before opening an issue. Include the CLI
version, operating system, command, exit code, redacted JSON error envelope, and
relevant `doctor` checks. Never include credentials, tokens, `.env.local`, or
private Console data.
