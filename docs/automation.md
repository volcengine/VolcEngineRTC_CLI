<a id="zh-cn"></a>

# 自动化与结构化输出

[English](#english) | 简体中文

stdout 指向终端时，`vertc` 默认使用便于阅读的输出；stdout 被管道或重定向时，默认输出 JSON。Coding Agent、脚本和 CI 应显式传入 `--format json`，避免结果随终端环境变化。

## 输出流约定

- stdout 只写命令数据；
- stderr 写进度、警告和恢复提示；
- JSON 模式下，每条命令只向 stdout 写一个信封；
- 成功退出码为 0，失败为非零。

成功结果格式：

```json
{
  "ok": true,
  "data": {}
}
```

已知错误格式：

```json
{
  "ok": false,
  "error": {
    "code": "vertc.example.code",
    "type": "validation",
    "message": "...",
    "hint": "..."
  }
}
```

需要时，错误中还会包含 `subtype` 和 `param`。自动化逻辑应先判断 `ok`、进程退出码和 `error.code`，不要依赖给人阅读的 `message` 文案。

## 输出格式

```bash
vertc doctor                 # 终端中默认 pretty，重定向时默认 JSON
vertc doctor --format json   # 自动化建议明确指定
vertc doctor --format pretty
vertc init --list --format table
```

stderr 中的进度不会改变 stdout 的数据格式。流水线同时需要结果和诊断日志时，应分别重定向两个流。

## dry-run

全局 `--dry-run` 用来预览副作用，不真正写入：

```bash
vertc init --scene voice-agent --platform web --dry-run
vertc dev --dry-run
vertc update --dry-run
```

远程模板的 dry-run 可以使用内存中的下载结果或现有缓存做校验，但不会创建目标目录，也不会填充空缓存。

`update --dry-run` 可能查询 npm registry 并识别安装方式，但不会调用 npm、替换二进制、写入生命周期状态或同步 Skills。返回的 `status` 为 `would_update`、`up_to_date` 或 `manual_required`。

## 自动化环境中的登录

Authorization Code + PKCE 必须由用户批准。在无界面主机上，可以先运行：

```bash
vertc auth login --browser=manual --store=file
```

命令会输出授权 URL，并从 stdin 读取返回的授权码。Signin Token 默认保存在项目外、权限受限的 `$VERTC_HOME/auth.json`。只有用户明确同意时才使用 `--store=keyring` 切换到操作系统凭据库。

非交互终端使用默认 `--browser=ask` 时，CLI 不会自动打开界面，而是返回 `vertc.auth.interaction_required`。登录完成后直接运行 `vertc dev`。如果需要选择公开资源，命令会返回包含候选项的类型化 JSON 错误：

```bash
vertc dev
# 收到 vertc.dev.selection_required 后，从 error.details 中选择
vertc dev --app-id <app-id> --bot-id <bot-id>
```

需要多个智能体时，重复传入 `--bot-id`。唯一的 RTC 应用会自动选择；存在多个 RTC 应用时才要求选择。智能体只要存在就需要用户选择；查询结果为空时才使用内置默认场景。

`--app-id` 和 `--bot-id` 都是公开资源 ID。CLI 通过 Signin STS 取得 AppKey，并且不会返回 AppKey。自动化流程不得向用户索取 AppKey，也不得把它写入聊天、命令参数、日志或 `VITE_*` 变量。

如果项目凭据和场景配置都已经准备好，先用 `vertc doctor` 做只读检查，再运行 `vertc dev`。

## 生命周期通知

JSON 信封可能包含可选的 `_notice.update` 和 `_notice.skills`。这些通知只读取本地缓存，不会在输出路径上增加同步网络请求。调用方必须允许出现未知的新增通知字段。

Agent 应先完成当前用户任务，再展示生命周期通知，不得自行更新。`_notice.update` 对应 `vertc update`，`_notice.skills` 对应 `vertc skills sync`。

必要时可以关闭通知：

```bash
export VERTC_NO_UPDATE_NOTIFIER=1
export VERTC_NO_SKILLS_NOTIFIER=1
```

## 官方 Skills

直接从公开仓库安装最新发布的官方场景工作流，不要求预先安装 `vertc`：

```bash
npx skills add volcengine/VolcEngineRTC_CLI -g -y
```

如果 CLI 已安装，可以用当前版本内嵌的内容对齐或修复全局 Agent 运行时：

```bash
vertc skills sync
```

同步过程需要可用的 `npx skills`。它会把当前 CLI 版本内嵌的 Skill 复制到支持的全局 Agent 运行时；使用 `vertc skills sync --dry-run` 可以只预览，不写入。

也可以直接查看 CLI 内嵌的 Skill：

```bash
vertc skills list
vertc skills read byted-interactai-guide
vertc skills sync
```

每个公开 Release 和 snapshot artifact 都保持三处版本一致：`vertc version` 输出、`vertc skills read <name>` 返回的 `version` frontmatter，以及归档 `SKILL.md` 的版本。Release 后置检查直接读取内嵌 Markdown，因此调用方不需要访问仓库，也不需要单独维护 Skill 版本映射。

`vertc update` 更新 npm 管理的二进制后，会复用同一套同步逻辑。公开仓库安装方式跟随最新 Release；`vertc skills sync` 则让已安装的 Skill 与当前 CLI 版本对齐。

## 失败处理

1. 先检查非零退出码和 JSON `error.code`。
2. 如果有结构化 `hint`，按提示处理。
3. 就绪状态或配置失败时，运行 `vertc doctor`。
4. Web SDK 或对话式 AI 运行时错误码使用 `vertc explain-error <code>` 查询；它们与 CLI 自身的 `error.code` 是两套体系。

---

<a id="english"></a>

# Automation and structured output

[简体中文](#zh-cn) | English

`vertc` uses readable output when stdout is a terminal and JSON when stdout is
piped or redirected. Coding agents, scripts, and CI should pass `--format json`
explicitly so their output does not depend on the terminal environment.

## Stream contract

- stdout contains command data only.
- stderr contains progress, warnings, and recovery hints.
- in JSON mode, a command writes one envelope to stdout.
- success exits with code 0; failure exits non-zero.

A successful result has this shape:

```json
{
  "ok": true,
  "data": {}
}
```

A known failure has this shape:

```json
{
  "ok": false,
  "error": {
    "code": "vertc.example.code",
    "type": "validation",
    "message": "...",
    "hint": "..."
  }
}
```

Fields such as `subtype` and `param` are included when relevant. Automation
should branch on `ok`, process exit status, and `error.code` before interpreting
human-language messages.

## Output formats

```bash
vertc doctor                 # pretty in a terminal, JSON when redirected
vertc doctor --format json   # stable choice for automation
vertc doctor --format pretty
vertc init --list --format table
```

Progress on stderr never changes the stdout representation. Redirect streams
separately when a pipeline needs both structured results and diagnostic logs.

## Dry runs

The global `--dry-run` flag previews side effects without applying them:

```bash
vertc init --scene voice-agent --platform web --dry-run
vertc dev --dry-run
vertc update --dry-run
```

For remote templates, a dry run may validate from memory or an existing cache,
but it does not create the target or populate the cache.
An update dry run may query the npm registry and detect the install method, but
it never invokes npm, replaces a binary, writes lifecycle state, or synchronizes
Skills. Its `status` is `would_update`, `up_to_date`, or `manual_required`.

## Authentication in automation

Authorization Code with PKCE requires a user to approve access. On a headless
host, use:

```bash
vertc auth login --browser=manual --store=file
```

The command prints the authorization URL and accepts the returned code through
stdin. Signin tokens default to the protected `$VERTC_HOME/auth.json` file,
outside the project; use `--store=keyring` only after explicit user consent.
`--browser=ask` returns `vertc.auth.interaction_required` in non-interactive
terminals instead of opening UI. After login, run `dev` directly. If a user decision is required,
it returns a typed JSON error containing public candidates:

```bash
vertc dev
# on vertc.dev.selection_required, choose from error.details
vertc dev --app-id <app-id> --bot-id <bot-id>
```

Repeat `--bot-id` when needed. A unique RTC application is selected
automatically. Existing agents require user selection; when the successful
query returns none, the built-in default scene is used. These arguments are public resource IDs; the CLI
retrieves AppKey with Signin STS and never returns it. Automation must never ask
the user for AppKey or place it in chat, command arguments, logs, or `VITE_*`.

For an environment whose project credentials and scene configuration are
already present, run `vertc doctor` as the readiness gate before `vertc dev`.

## Lifecycle notices

JSON envelopes may contain additive `_notice.update` and `_notice.skills`
objects. They are composed from local cached state and do not add synchronous
network I/O to the output path. Consumers must tolerate unknown additive notice
fields.

Agents must finish the current user task before surfacing a lifecycle notice and
must not update autonomously. `_notice.update` maps to `vertc update`;
`_notice.skills` maps to `vertc skills sync`.

Suppress them when necessary:

```bash
export VERTC_NO_UPDATE_NOTIFIER=1
export VERTC_NO_SKILLS_NOTIFIER=1
```

## Official Skills

Install the latest released official scene workflow directly from the public
repository; this path does not require `vertc` to be installed first:

```bash
npx skills add volcengine/VolcEngineRTC_CLI -g -y
```

When the CLI is already installed, align or repair global Agent runtimes from
the scene workflow embedded in that exact CLI release:

```bash
vertc skills sync
```

Synchronization copies the embedded content into supported global Agent
runtimes and requires `npx skills` to be available. To preview without writing,
run `vertc skills sync --dry-run`.

The CLI also embeds the Skill content shipped with its release:

```bash
vertc skills list
vertc skills read byted-interactai-guide
vertc skills sync
```

For every public release and snapshot artifact, the value printed
by `vertc version`, the `version` frontmatter returned by
`vertc skills read <name>`, and the corresponding archived `SKILL.md` version
are identical. Release postflight reads the embedded Markdown itself; consumers
therefore do not need repository access or a separate Skill-version mapping.

`vertc update` reuses the same synchronization after updating an npm-managed
binary. The direct public-repository path follows the latest released content;
`vertc skills sync` instead keeps the installed content tied to the running
binary release.

## Failure routing

1. Inspect the non-zero exit status and JSON `error.code`.
2. Follow the structured `hint` when present.
3. Run `vertc doctor` for readiness or configuration failures.
4. Run `vertc explain-error <code>` for Web SDK or conversational-AI runtime
   codes. These runtime codes are separate from CLI `error.code` values.
