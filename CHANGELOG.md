# 变更日志

`vertc` 的所有重要变更都会记录在此文件中。

本文档格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，
版本号遵循[语义化版本](https://semver.org/lang/zh-CN/)。

## 未发布

## 0.0.7

_发布日期：2026-09-08_

### 新增

- `docs fetch` 新增 `--match` 参数，支持重复指定，按关键词提取 Markdown 章节或表格，并返回原文身份元数据、匹配情况和截断状态。
- InteractAI Skill 支持生成、修改和校验 VoiceChat/Aibot 配置，配置模型、验证流程和输出格式分别放入独立参考文档。

### 修复

- 将 Go 工具链升级至 1.25.13，修复标准库 HTTP、TLS、URL 和 ASN.1 处理中的安全问题。
- flag 解析或通用位置参数校验失败时，返回当前命令的用法；`docs search/fetch/list` 的参数错误补充位置参数要求和示例。
- 兼容 RTC 文档 MCP 的 `fetch_doc`、`list_docs` 分页参数及 `list_docs` 过滤参数 schema，校验完整文档标记并移除协议尾部元数据。
- Skill 命令校验识别 `--` 参数终止符，支持其后的负数错误码参数。

### 改进

- 文档检索优先通过精确专题标题执行 `docs list → fetch --match`，未唯一命中时回退 `docs search → fetch`。
- Voice Agent 配置以 StartVoiceChat 核心模型为准，可转换为 AibotCreate 配置或 AibotUpdate JSON Merge Patch。
- 字段范围、枚举和 Provider 兼容性依据当前官方正文或服务端结果验证；证据不足或冲突时返回 `valid=null`，保留候选预览，可执行配置为空。
- 集成诊断支持快速判断和完整阶段排查，并说明 VoiceChat 事件和客户端音频证据能确认哪些阶段。
- 调整 CLI 和 Skill 更新提示：用户询问 Runtime、安装或更新时展示，产品咨询、配置和诊断时忽略。
- 为每次 CLI 调用生成 invocation ID，并写入 User-Agent。
- 调整 AI 调用方的识别优先级和环境变量匹配范围，修改 User-Agent 超长时的裁剪规则，并支持版本构建元数据。

## 0.0.6

_发布日期：2026-08-13_

### 改进

- RTC 文档 MCP 调用会复用 CLI 的 OpenAPI invocation User-Agent，便于下游服务识别来源并保持调用链路一致。
- 新增 Go 测试覆盖率治理门禁，`make ci` 和公开 CI 会统一执行覆盖率检查。

### 测试

- 补充根命令行为、affordance、路径、防回归发布命令、自更新、模板渲染和 E2E 等测试覆盖，降低公开发布前的回归风险。

## 0.0.5

_发布日期：2026-08-12_

### 新增

- 新增 RTC 文档命令，支持按主题检索和读取嵌入式文档内容。

### 改进

- 完善公开发布上下文收集，发布说明以已提交的 CHANGELOG、公开差异和已合并 PR 为事实来源，并默认使用中文。
- Windows CI 统一通过 `scripts/ci-windows.ps1` 使用 Go 1.25.12 执行完整测试、构建和基础命令验证。

### 修复

- 保留 annotated tag 中发布说明的原始 Markdown，避免 Git 清理规则剥离标题。
- npm 发布与验证固定使用官方 registry 和隔离缓存，仅对 E404 传播延迟重试，并支持恢复缺失的 dist-tag。
- 发布版本检查现在要求存在对应版本的完整 CHANGELOG 条目，空内容或占位内容会阻止公开同步。

## 0.0.4

_发布日期：2026-08-11_

### 新增

- `vertc dev` 新增 `--web-port`、`--server-port` 和 `--auto-port`，支持显式指定或自动选择本地端口。
- 结构化输出会返回最终使用的 Web/Server 端口和本地访问地址，同时不修改项目文件。
- 新增确定性的发布版本准备与检查命令，在打 tag 前统一校验 npm 包和官方 Skill 的版本元数据。

### 改进

- 在安装依赖前和启动服务前分别检查模板端口，避免依赖安装完成后才发现端口冲突。
- 端口被占用时会报告端口号，并在可获取时补充监听进程、PID 和工作目录，错误码稳定为 `vertc.dev.port_in_use`。
- 加强 annotated tag、GitHub Release、发布产物中的 Skill 版本、npm 版本和 dist-tag 的发布后验证。
- 更新安装、自动化、语音智能体和公开发布流程文档。

### 修复

- Windows 原生 `cmd.exe` 执行任务命令时，正确保留带引号的可执行文件路径、参数和退出码。

## 0.0.3

_发布日期：2026-08-07_

首个稳定公开版本。

### 新增

- 中文 README 成为默认项目说明，同时提供独立英文 README。
- 终端交互默认使用易读输出，管道、重定向和自动化场景继续使用 JSON。

### 改进

- 更新 GitHub Actions 与 Go 依赖版本，并在公开 CI 中执行测试和漏洞检查。
- 加强七个发布文件、校验和、内嵌 Skill 与 npm 输入的完整性验证。
- 增加精确到 commit 的公开发布预检，拒绝保留版本或与源码不一致的发布身份。
- 预发布版本使用 npm `next`，稳定版本使用 npm `latest`，避免不同发布通道互相覆盖。
- 改进 InteractAI 工作流提示，使 CLI 与 Skill 生命周期通知在当前任务完成后再展示。

## 0.0.1-rc.3

_发布日期：2026-08-06_

### 修复

- 修正内嵌 Skill 的发布后验证路径，使 tar.gz 和 zip 中位于根目录的 Skill 清单都能被正确识别。
- 为 macOS、Linux 和 Windows 的 amd64/arm64 发布产物补充端到端回归覆盖。

## 0.0.1-rc.2

_发布日期：2026-08-06_

首个实际分发的公开候选版本，聚焦 `voice-agent × web` 工作流。

### 新增

- 提供 `init`、`auth login` 和 `dev` 引导流程，可创建固定版本的官方 Web 模板、登录火山引擎、选择 RTC 与对话式 AI 资源并启动本地开发。
- 提供 `doctor` 就绪检查和 `explain-error` 离线错误知识库，用于诊断 SDK 与对话式 AI 问题。
- 提供面向 Agent 和自动化的 JSON 输出、稳定类型化 `error.code`、stdout/stderr 分离、dry-run 和内嵌工作流 Skill。
- 项目配置与本地凭据分离，RTC AppKey 不写入项目配置、命令参数、日志或结构化输出。
- 提供 macOS、Linux 和 Windows 的 amd64/arm64 预构建产物，并在 npm 安装与更新时校验 SHA-256。
- 发布产物绑定不可变的公开源码身份，可从同一 commit 重复构建。
