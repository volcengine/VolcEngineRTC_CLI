# vertc

[![CI](https://github.com/volcengine/VolcEngineRTC_CLI/actions/workflows/ci.yml/badge.svg)](https://github.com/volcengine/VolcEngineRTC_CLI/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/Go-%3E%3D1.23-00ADD8?logo=go)](./go.mod)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](./LICENSE)

[English](./README.md) | 简体中文

`vertc` 是面向火山引擎 RTC 开发工作流的 CLI，用于创建、配置、诊断和运行
AI 音视频项目。当前版本将官方 `rtc-aigc-demo` Web 语音智能体模板整理为一套
开发者和 Coding Agent 都可以执行的引导式工作流。

> **当前范围：** `voice-agent × web`。其他场景和平台会在端到端工作流准备好后
> 再加入公开版本。

[快速开始](#五分钟快速开始) · [Agent 与 CI](#agent-与-ci-工作流) ·
[命令](#命令) · [安全](#配置与安全) · [文档](#文档) · [参与贡献](#参与贡献)

## 为什么选择 vertc？

- **一套引导式工作流**：生成固定版本的官方模板，从控制台完成配置，检查就绪状态，
  然后启动本地开发环境。
- **感知控制台配置**：登录一次，首次运行 `dev` 时发现账号下的 RTC 应用和对话式
  AI 智能体，并配置当前项目。
- **诊断优先**：`doctor` 输出可执行的 PASS/WARN/FAIL 检查；`explain-error` 可离线
  查询 SDK 与对话式 AI 错误码。
- **Agent 原生输出**：默认输出 JSON，stdout 保持机器可读，错误提供稳定
  `error.code`，官方 Skills 提供完整工作流剧本。

## 环境要求

- macOS、Linux 或 Windows，支持 amd64 和 arm64
- 推荐通过 npm 安装，需要 Node.js 16 或更高版本
- 仅从源码安装或构建时需要 Go 1.23 或更高版本
- 火山引擎账号下已有 RTC 应用；对话式 AI 智能体为可选，`dev` 可使用内置默认 Scene
- npm 安装需要访问 GitHub Releases；首次下载模板需要访问 GitHub codeload，之后可
  复用已校验的本地缓存

## 安装

推荐通过 npm 安装预编译二进制：

```bash
npm install -g @volcengine/rtc-cli
vertc version --format pretty
```

安装器会下载与 npm 包版本和当前平台匹配的二进制，并根据 Release 中的
`checksums.txt` 完成校验后再将其暴露为 `vertc`。

也可以从源码构建：

```bash
git clone https://github.com/volcengine/VolcEngineRTC_CLI.git
cd VolcEngineRTC_CLI
make build
./bin/vertc version --format pretty
```

`vertc update` 不会覆盖手工管理的二进制，而会输出对应安装方式的升级指引。

## 五分钟快速开始

无需预先创建对话式 AI 智能体。登录账号下没有智能体时，`dev` 会使用内置默认 Scene
快速启动，并提供控制台地址供后续定制。

```bash
# 1. 从经过校验的 rtc-aigc-demo 模板创建项目。
vertc init ./my-agent --scene voice-agent --platform web --format pretty
cd my-agent

# 2. 登录并保存可刷新的 Signin Token。
vertc auth login --format pretty

# 3. 首次运行时配置 RTC 应用，并按需选择智能体；vertc 写入本地运行配置后，
#    启动 Web 应用和配套服务端。
vertc dev --format pretty
```

需要授权时，交互式 `auth login` 会先询问打开浏览器、使用手动链接/授权码或取消。
在 Agent、远程或无图形界面的终端中，先获得用户同意，再显式运行
`vertc auth login --browser=open` 或 `vertc auth login --browser=manual`；CLI 不会隐式弹出 UI。
首次交互式 `dev` 会自动使用唯一 RTC 应用；存在一个或
多个智能体时要求用户选择，确认没有智能体时使用内置默认 Scene。之后可运行
`vertc dev --reconfigure` 重新选择；已有 Bot Scene 文件会保留。

当前 store 中已有可用 Signin Token 时，`auth login` 会直接复用（临近过期时先刷新），
不会询问或打开浏览器。如需重新授权或切换账号，组合使用 `--force` 和显式 browser 模式。
Signin access/refresh token 默认保存在受保护的 `$VERTC_HOME/auth.json`（默认
`~/.vertc/auth.json`）；运行 `vertc auth login --store=keyring` 可显式改用系统凭据存储。

在 Agent 或其他非交互终端中直接运行 `vertc dev`。若返回
`vertc.dev.selection_required`，从 `error.details.apps` 或 `error.details.bots` 读取公开
候选，询问用户后用 `--app-id <id>` 或 `--bot-id <id>` 重试。
CLI 会通过登录态自行获取 AppKey；不要把 AppKey 发给 Agent 或放入命令参数。

生成的项目包含完整 `rtc-aigc-demo` Web UI 和本地服务。首次配置完成后，`vertc dev`
会启动两个进程；每个浏览器页面 session 都拥有独立的房间、用户、Token、Task 和
目标用户身份。配置或运行失败时可执行 `vertc doctor --format pretty`；它只做诊断，
不会修改项目。项目结构、运行模式、手动配置和高级身份管理详见
[语音智能体项目](./docs/voice-agent.md)。

## Agent 与 CI 工作流

可用以下任一方式注册官方工作流 Skill：

发布标识为 `byted-interactai-guide`。

```bash
# 通过已安装的 CLI（无需仓库权限）
vertc skills sync

# 或直接从公开仓库注册
npx skills add volcengine/VolcEngineRTC_CLI -g -y
```

Agent 和自动化运行应保留默认 JSON 格式：

```bash
vertc init ./my-agent --scene voice-agent --platform web
cd my-agent
vertc auth status
vertc dev
vertc doctor
```

- stdout 输出一个 JSON 信封，包含 `ok` 以及 `data` 或 `error`。
- 进度、警告和恢复提示写入 stderr。
- 失败使用非零退出码和稳定 `error.code`。
- 有副作用的命令支持全局 `--dry-run`。
- 受控环境可设置 `VERTC_NO_UPDATE_NOTIFIER=1` 和
  `VERTC_NO_SKILLS_NOTIFIER=1`，关闭附加的生命周期通知。

无界面授权、非交互资源选择、JSON 信封、错误路由和 Skill 契约详见
[自动化与结构化输出](./docs/automation.md)。

## 命令

| 命令 | 用途 |
| --- | --- |
| `init [dir] --scene <scene> --platform <platform>` | 离线列出或生成受支持的项目模板到 `[dir]`；支持 `--dry-run` |
| `auth login [--browser=ask\|open\|manual] [--store=file\|keyring]` / `auth status/logout` | 以显式 UI 与存储选择管理火山引擎登录凭证 |
| `doctor [cli\|project]` | 只读诊断 CLI 与项目就绪状态 |
| `dev [--reconfigure] [--app-id <id>] [--bot-id <id>]` | 补全 RTC 配置并执行模板；无歧义时自动选择，有歧义时返回公开候选，零智能体时使用内置默认 Scene |
| `explain-error <code>` | 离线查询 Web SDK 与对话式 AI 错误码 |
| `skills list/read/sync` | 查看、安装或刷新当前 Release 内嵌的官方 Skill 内容 |
| `update [--check\|--force]` | 检查或更新 npm 管理的安装，并同步官方 Skills |
| `version` | 输出构建版本、commit 和日期 |

运行 `vertc <command> --help` 可查看前置条件、示例和适用/不适用场景。高级手动及调试
命令不会出现在主帮助中，以保持公开接入路径精简。

## 配置与安全

`vertc` 将项目元数据、运行时密钥、场景数据和登录凭证分开管理：

| 位置 | 内容 | 安全边界 |
| --- | --- | --- |
| `vertc.config.yaml` | 非敏感项目元数据及 CLI 管理的 RTC/Agent 身份引用 | 可审阅；敏感值通过 `${ENV}` 占位引用 |
| `.env.local` | `RTC_APP_ID`、`RTC_APP_KEY` 等本地运行值 | 已被 Git 忽略并以受限权限写入；禁止提交 |
| `server/scenes/*.json` | 从控制台选择的 VoiceChat ASR/LLM/TTS 与智能体配置 | 服务端场景配置；分享前应检查 |
| `$VERTC_HOME/auth.json`（默认 `~/.vertc/auth.json`） | store 选择；`file` 模式下还包含登录 access/refresh token | 目录 `0700`、文件 `0600`；应像密码一样保护且禁止提交 |
| 操作系统凭据存储（可选 `keyring` 模式） | 登录 access/refresh token | 仅在显式运行 `auth login --store=keyring` 后使用 |

`RTC_APP_KEY` 不会写入 `vertc.config.yaml`、任何 `VITE_*` 前端变量、日志或结构化输出。
为了让新 shell 和配套服务端可复用，它可能保存在已被 Git 忽略的 `.env.local` 中。
进程中显式导出的环境变量优先于该文件中的值。禁止在聊天或命令参数中提供 AppKey。

漏洞报告方式见 [SECURITY.md](./SECURITY.md)。请勿在公开 Issue 中提交凭证、Token、
私有 endpoint 或敏感控制台数据。

## 文档

- [语音智能体项目](./docs/voice-agent.md)：生成目录、首次配置、运行模式与身份行为
- [自动化与结构化输出](./docs/automation.md)：JSON 信封、错误路由、dry-run、通知与 Skills
- [故障排查](./docs/troubleshooting.md)：安装、登录、模板、凭证和运行时恢复
- [CONTRIBUTING.md](./CONTRIBUTING.md)：开发环境与贡献流程
- [AGENTS.md](./AGENTS.md)：架构和仓库工程契约
- [SUPPORT.md](./SUPPORT.md)：问题咨询与 Bug 报告渠道
- [CHANGELOG.md](./CHANGELOG.md)：版本变更

## 开发

```bash
make build              # 构建 ./bin/vertc 并注入版本信息
make test               # 运行单元测试和端到端测试
make check-error-codes  # 校验稳定 error.code 目录
make ci                 # 运行完整仓库门禁
```

## 参与贡献

欢迎参与贡献。提交 Pull Request 前请阅读 [CONTRIBUTING.md](./CONTRIBUTING.md)，
问题咨询、Bug、功能建议和安全报告的渠道选择见 [SUPPORT.md](./SUPPORT.md)。

## 许可证

本项目采用 [MIT License](./LICENSE)。
