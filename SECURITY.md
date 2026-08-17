<a id="zh-cn"></a>

# 安全策略

[English](#english) | 简体中文

## 支持的版本

首个稳定版本发布前，安全修复只进入最新发布的 `0.x` 版本。稳定版本发布后，本节会明确列出仍受支持的版本线。

## 报告漏洞

不要通过公开 Issue、Discussion、Pull Request 或聊天消息报告疑似漏洞。请使用仓库的 GitHub 私密漏洞报告：

<https://github.com/volcengine/VolcEngineRTC_CLI/security/advisories/new>

条件允许时，请提供：

- 受影响的版本、平台和安装方式；
- 涉及的命令或工作流；
- 复现步骤或最小 PoC；
- 安全影响和已知前置条件；
- 建议的缓解方式或补丁；
- 该问题是否已经在其他地方披露。

不要提供真实生产凭据。请使用测试值，并对 AppKey、AccessKey、Signin Token、RTC Token、私有端点、账号标识和控制台数据脱敏。

维护者会尽快确认报告、开展调查，并与报告人协调修复和披露时间。在修复版本发布前，请不要公开漏洞细节。

## 安全边界

- Signin access token 和 refresh token 默认保存在权限受限的用户级文件 `$VERTC_HOME/auth.json`。只有用户显式选择 `--store=keyring` 后，CLI 才使用操作系统凭据库。
- `RTC_APP_KEY` 可以保存在 Git 已忽略的 `.env.local` 中，但不能进入 `vertc.config.yaml`、`VITE_*` 变量、前端代码、日志或命令输出。
- 远程 Release 和模板制品必须通过固定 SHA-256 摘要校验后才能使用。
- 分享或部署生成项目之前，应检查项目内容和场景配置。

---

<a id="english"></a>

# Security Policy

[简体中文](#zh-cn) | English

## Supported versions

Before the first stable release, security fixes are applied to the latest
published `0.x` release only. After a stable release, this section will list the
supported release lines explicitly.

## Reporting a vulnerability

Do not report suspected vulnerabilities in a public issue, discussion, pull
request, or chat. Use GitHub's private vulnerability reporting for this
repository:

<https://github.com/volcengine/VolcEngineRTC_CLI/security/advisories/new>

Include, when available:

- the affected version, platform, and installation method;
- the vulnerable command or workflow;
- reproduction steps or a minimal proof of concept;
- the security impact and any known prerequisites;
- suggested mitigations or patches;
- whether the issue has been disclosed anywhere else.

Do not include real production credentials. Use synthetic values and redact
AppKeys, access keys, Signin tokens, RTC tokens, private endpoints, account
identifiers, and Console data.

Maintainers will acknowledge the report as soon as practical, investigate it,
and coordinate remediation and disclosure with the reporter. Please allow time
for a fix and release before publishing details.

## Security boundaries

- Signin access and refresh tokens use the protected user-level
  `$VERTC_HOME/auth.json` file by default. The CLI uses the operating-system
  credential store only after the user explicitly selects `--store=keyring`.
- `RTC_APP_KEY` may be stored in the gitignored `.env.local` file but must not
  enter `vertc.config.yaml`, `VITE_*` variables, frontend code, logs, or command
  output.
- Remote release and template artifacts are verified against pinned SHA-256
  digests before use.
- Generated projects and scene configuration should be reviewed before they are
  shared or deployed.
