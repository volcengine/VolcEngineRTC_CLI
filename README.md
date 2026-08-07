# vertc

[![CI](https://github.com/volcengine/VolcEngineRTC_CLI/actions/workflows/ci.yml/badge.svg)](https://github.com/volcengine/VolcEngineRTC_CLI/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/volcengine/VolcEngineRTC_CLI?label=release)](https://github.com/volcengine/VolcEngineRTC_CLI/releases)
[![npm](https://img.shields.io/npm/v/@volcengine/rtc-cli?label=npm)](https://www.npmjs.com/package/@volcengine/rtc-cli)
[![Go Version](https://img.shields.io/badge/Go-%3E%3D1.25.12-00ADD8?logo=go)](./go.mod)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](./LICENSE)

[English](./README.en.md) | 简体中文

`vertc` 是火山引擎 RTC 的开发工作流 CLI，供开发者和 Coding Agent 创建、配置、运行和排查 AI 音视频项目。

> **当前范围：** `voice-agent × web`。其他场景和平台会在端到端工作流准备好后加入。

[安装](#安装) · [快速开始](#快速开始) · [核心能力](#核心能力) · [Agent 与 CI](#agent-与-ci) · [安全](#配置与安全) · [文档](#文档)

## 安装

预编译版本支持 macOS、Linux 和 Windows 的 amd64/arm64。使用 Node.js 16 或更高版本安装：

```bash
npm install -g @volcengine/rtc-cli
vertc version
```

安装器会从 GitHub Releases 下载对应版本和平台的二进制，并使用 `checksums.txt` 校验。从源码构建需要 Go 1.25.12 或更高版本：

```bash
git clone https://github.com/volcengine/VolcEngineRTC_CLI.git
cd VolcEngineRTC_CLI
make build
./bin/vertc version
```

## 快速开始

开始前，账号下需要有一个 RTC 应用。对话式 AI 智能体不是必需的；如果没有，`dev` 会使用内置默认 Scene。

```bash
# 1. 创建项目
vertc init ./my-agent --scene voice-agent --platform web
cd my-agent

# 2. 登录火山引擎
vertc auth login

# 3. 配置 RTC 资源并启动 Web 应用和本地服务
vertc dev
```

打开终端给出的地址，点击 **Start** 即可进房对话。首次运行时，`vertc` 会查找账号下的 RTC 应用和智能体；有多个候选时再提示选择。配置或运行失败时，可以用 `vertc doctor` 检查问题；该命令不会修改项目。

项目结构、运行方式和身份管理见[语音智能体项目](./docs/voice-agent.md)。

## 核心能力

- **创建和运行项目**：`init` 从固定版本的官方模板创建项目，`dev` 配置控制台资源并启动 Web 应用和本地服务。
- **排查问题**：`doctor` 给出 PASS/WARN/SKIP/UNKNOWN/FAIL 检查结果；`explain-error` 可离线查询 SDK 与对话式 AI 错误码。
- **用于 Agent 和脚本**：管道和重定向默认输出 JSON，stdout 只写数据，失败时返回稳定的 `error.code`。有副作用的命令支持 `--dry-run`。
- **分开保存配置和凭据**：项目元数据、运行时密钥、场景数据和登录凭据各自存放。

### 常用命令

| 命令 | 用途 |
| --- | --- |
| `init [dir] --scene <scene> --platform <platform>` | 列出或生成受支持的项目模板 |
| `auth login` / `auth status` / `auth logout` | 管理火山引擎登录状态 |
| `dev [--reconfigure]` | 配置 RTC 资源并运行项目 |
| `doctor [cli\|project]` | 检查 CLI 和项目是否就绪，不修改项目 |
| `explain-error <code>` | 离线查询 SDK 与对话式 AI 错误码 |
| `skills list/read/sync` | 查看或同步当前 Release 内嵌的官方 Skill |
| `update [--check\|--force]` | 检查或更新 npm 管理的安装 |

运行 `vertc <command> --help` 查看前置条件、完整参数和示例。

## Agent 与 CI

官方工作流 Skill 标识为 `byted-interactai-guide`：

```bash
vertc skills sync
# 或
npx skills add volcengine/VolcEngineRTC_CLI -g -y
```

Agent 和自动化脚本应显式传入 `--format json`，避免输出格式受终端环境影响。stdout 只写数据，进度和警告写入 stderr；命令失败时返回非零退出码和稳定的 `error.code`。无界面授权、非交互资源选择、错误处理和通知设置见[自动化与结构化输出](./docs/automation.md)。

## 配置与安全

- `vertc.config.yaml` 只保存非敏感项目元数据和 `${ENV}` 引用。
- `.env.local` 保存本地运行值。该文件已被 Git 忽略，并以受限权限写入；不要提交。
- Signin 凭据默认保存在受保护的 `$VERTC_HOME/auth.json`。也可以主动选择系统 keyring。
- `RTC_APP_KEY` 不会写入项目配置、`VITE_*` 前端变量、日志、命令参数或结构化输出；不要在聊天中提供 AppKey。

认证模式、凭据存储和自动化安全边界见[自动化与结构化输出](./docs/automation.md)；漏洞报告方式见 [SECURITY.md](./SECURITY.md)。

## 文档

- [语音智能体项目](./docs/voice-agent.md)：生成目录、首次配置、运行模式与身份行为
- [自动化与结构化输出](./docs/automation.md)：认证、JSON 信封、错误路由、dry-run、通知与 Skills
- [故障排查](./docs/troubleshooting.md)：安装、登录、模板、凭据和运行时恢复
- [CHANGELOG.md](./CHANGELOG.md)：版本变更
- [SUPPORT.md](./SUPPORT.md)：问题咨询与 Bug 报告渠道
- [CONTRIBUTING.md](./CONTRIBUTING.md)：开发环境与贡献流程

## 开发与贡献

```bash
make build
make test
make ci
```

提交 Pull Request 前请阅读 [CONTRIBUTING.md](./CONTRIBUTING.md)。本项目采用 [MIT License](./LICENSE)。
