<a id="zh-cn"></a>

# 获取帮助

[English](#english) | 简体中文

## 使用问题

先看 [README](./README.md)、[语音智能体项目](./docs/voice-agent.md)和[故障排查](./docs/troubleshooting.md)。求助前运行一次 `vertc doctor`，它通常能直接指出下一步怎么处理。

文档没有覆盖你的问题时，可以提交 GitHub Issue，并说明你想完成什么。

## Bug 报告

在这里新建 Issue：

<https://github.com/volcengine/VolcEngineRTC_CLI/issues/new>

请提供：

- `vertc version --format pretty` 输出；
- 操作系统和架构；
- 安装方式；
- 完整命令和退出码；
- 最小复现步骤；
- 脱敏后的 JSON 错误信封，或相关 `doctor` 检查；
- 预期行为和实际行为。

提交前先搜索是否已有相同 Issue。不要上传 `.env.local`，也不要粘贴未脱敏的控制台输出。

## 功能建议

可以通过 GitHub Issue 说明：用户遇到了什么问题、希望怎样操作、涉及哪个场景和平台，以及现有命令为什么不能解决。

公开命令面会保持精简。建议优先描述完整的开发结果，不要只围绕新增一个 flag 展开。

## 安全问题

漏洞不要提交到公开 Issue。请按 [SECURITY.md](./SECURITY.md) 中的方式私下报告。

---

<a id="english"></a>

# Support

[简体中文](#zh-cn) | English

## Usage questions

Start with the [README](./README.en.md),
[voice-agent guide](./docs/voice-agent.md#english), and
[troubleshooting guide](./docs/troubleshooting.md#english). Run `vertc doctor` before
requesting help; its checks usually identify the next action.

If the documentation does not answer the question, open a GitHub issue and
describe what you are trying to accomplish.

## Bug reports

Open an issue at:

<https://github.com/volcengine/VolcEngineRTC_CLI/issues/new>

Include:

- `vertc version --format pretty` output;
- operating system and architecture;
- installation method;
- the exact command and exit code;
- a minimal reproduction;
- the redacted JSON error envelope or relevant `doctor` checks;
- expected and actual behavior.

Search existing issues before filing a duplicate. Never attach `.env.local` or
unredacted Console output.

## Feature requests

Open a GitHub issue describing the user problem, desired workflow, scene and
platform, and why existing commands do not cover it. The public command surface
is intentionally kept small, so proposals should focus on an end-to-end
developer outcome rather than an isolated flag.

## Security reports

Do not open a public issue for a vulnerability. Follow the private reporting
process in [SECURITY.md](./SECURITY.md#english).
