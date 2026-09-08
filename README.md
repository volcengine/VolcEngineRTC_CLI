# vertc

[![CI](https://github.com/volcengine/VolcEngineRTC_CLI/actions/workflows/ci.yml/badge.svg)](https://github.com/volcengine/VolcEngineRTC_CLI/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/volcengine/VolcEngineRTC_CLI?label=release)](https://github.com/volcengine/VolcEngineRTC_CLI/releases)
[![npm](https://img.shields.io/npm/v/@volcengine/rtc-cli?label=npm)](https://www.npmjs.com/package/@volcengine/rtc-cli)
[![Go Version](https://img.shields.io/badge/Go-%3E%3D1.25.12-00ADD8?logo=go)](./go.mod)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](./LICENSE)

[English](./README.en.md) | 简体中文

`vertc` 是面向火山引擎 RTC 项目的 CLI。开发者和 Coding Agent 可以用它创建、配置、运行和排查 AI 音视频项目。

> **当前范围：** `voice-agent × web`。其他场景和平台会在端到端流程可用后加入。

[安装](#安装) · [快速开始](#快速开始) · [核心能力](#核心能力) · [Agent 与 CI](#agent-与-ci) · [安全](#配置与安全) · [文档](#文档)

## 安装

### 预编译版本

支持 macOS、Linux 和 Windows，覆盖 amd64 和 arm64。安装需要 Node.js 16 或更高版本：

```bash
npm install -g @volcengine/rtc-cli
vertc version
```

安装器会从 GitHub Releases 下载与 npm 包版本和当前平台对应的二进制，并使用 `checksums.txt` 校验。

### 本地编译

需要 Go 1.25.12 或更高版本：

```bash
git clone https://github.com/volcengine/VolcEngineRTC_CLI.git
cd VolcEngineRTC_CLI
make build
./bin/vertc version
```

## 快速开始

开始前，请确保账号已开通 RTC 服务（[开通指引](https://docs.volcengine.com/docs/6348/69865?lang=zh)）。对话式 AI 智能体可选；如果没有，`dev` 会使用内置默认场景。

```bash
# 1. 创建项目
vertc init ./my-agent --scene voice-agent --platform web
cd my-agent

# 2. 登录火山引擎
vertc auth login

# 3. 配置 RTC 资源并启动 Web 应用和本地服务
vertc dev
```

打开终端中的地址，点击 **Start** 进房对话。首次运行时，`vertc` 会查询账号下的 RTC 应用和智能体：只有一个 RTC 应用时自动选择，存在多个应用时请用户选择；只要账号下存在智能体，即使只有一个，也需要用户确认。没有智能体时使用内置默认场景。配置或运行失败时，运行 `vertc doctor` 检查；该命令不会修改项目。

项目结构、运行方式和身份管理见[语音智能体项目](./docs/voice-agent.md)。

## 核心能力

- **创建和运行项目**：`init` 使用固定版本的官方模板创建项目；`dev` 配置控制台资源并启动 Web 应用和本地服务。
- **排查问题**：`doctor` 给出 PASS/WARN/SKIP/UNKNOWN/FAIL 检查结果；`explain-error` 可离线查询 SDK 与对话式 AI 错误码。
- **查询 RTC 文档**：`docs search/fetch/list` 通过内置只读文档端点查询实时内容，无需项目配置或登录。
- **供 Agent 和脚本调用**：管道和重定向默认输出 JSON；stdout 只写数据；失败时返回稳定的 `error.code`。有副作用的命令支持 `--dry-run`。
- **分开保存配置和凭据**：项目元数据、运行时密钥、场景数据和登录凭据各自存放。

### 常用命令

| 命令                                                 | 用途                          |
| -------------------------------------------------- | --------------------------- |
| `init [dir] --scene <scene> --platform <platform>` | 列出或生成受支持的项目模板               |
| `auth login` / `auth status` / `auth logout`       | 管理火山引擎登录状态                  |
| `dev [--web-port N] [--server-port N] [--auto-port]` | 配置 RTC 资源、检查端口并运行项目         |
| `doctor [cli\|project]`                            | 检查 CLI 和项目是否就绪，不修改项目        |
| `explain-error <code>`                             | 离线查询 SDK 与对话式 AI 错误码        |
| `docs search/fetch/list`                           | 搜索、读取或浏览 RTC 文档；`fetch --match` 可定向提取章节/表格 |
| `skills list/read/sync`                            | 查看或同步当前 Release 内嵌的官方 Skill |
| `update [--check\|--force]`                        | 检查或更新 npm 管理的安装             |

运行 `vertc <command> --help` 查看前置条件、完整参数和示例。

## Agent 与 CI

安装官方 Skill：

```bash
npx skills add volcengine/VolcEngineRTC_CLI -g -y
```

如需让 Skill 与当前 `vertc` 版本一致，或修复已有安装：

```bash
vertc skills sync
```

Agent 和自动化脚本应显式传入 `--format json`，避免输出格式随终端环境变化。stdout 只写数据，进度和警告写入 stderr；失败时返回非零退出码和稳定的 `error.code`。无界面授权、非交互资源选择、错误处理和通知设置见[自动化与结构化输出](./docs/automation.md)。

## 配置与安全

- `vertc.config.yaml` 只保存非敏感项目元数据和 `${ENV}` 引用。
- `.env.local` 保存本地运行值，已被 Git 忽略，并以受限权限写入；不要提交。
- Signin 凭据默认保存在受保护的 `$VERTC_HOME/auth.json` 中，也可以选择系统 keyring。
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
