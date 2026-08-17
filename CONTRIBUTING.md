<a id="zh-cn"></a>

# 为 vertc 贡献代码

[English](#english) | 简体中文

本文说明常见贡献流程。架构和强制规则写在 [AGENTS.md](./AGENTS.md) 中，开始前请先阅读。

## 准备开发环境

```bash
make tools     # 安装固定版本的 golangci-lint 和 Gitleaks，只需执行一次
make ci        # 运行完整本地检查，包括公开接口和开源发布检查
```

从 `main` 创建分支，一次改动只处理一项能力。提交 Pull Request 前确保 `make ci` 通过。格式和 import 可以用 `make fmt` 自动修复。

`make check-change-contract` 会将当前分支与 merge base 对比，检查稳定 CLI 接口是否连同验证一起更新：命令变更需要命令测试或 E2E 测试，错误码变更需要更新 snapshot，模板源变更需要模板测试。在 CI 中，它使用 MR diff base 或 push base；本地运行时使用 `origin/HEAD`。

### Go 单元测试覆盖率

在仓库根目录运行标准覆盖率流程：

```bash
make coverage        # 生成 coverage.out 和 coverage.html，并输出总覆盖率
make check-coverage  # 重新生成报告并检查覆盖率下限
```

覆盖率统计使用不带缓存的 atomic statement profile，范围是 `cmd/` 和 `internal/` 下的生产包及同目录测试。`tests/` 下的子进程 E2E 测试是单独的功能门禁，因为子 CLI 进程中的执行不会进入普通 Go 单元测试 profile。

CI 通过同一个 Make target 检查 70.0% 的总覆盖率，不会给每个包套用同一个阈值。命令入口、平台适配和纯逻辑代码的可测试性不同。新增测试应覆盖可观察的成功、失败、边界、dry-run 或回滚行为，不要只为提高数字而执行代码行。

需要调整本地报告路径或阈值时，可以使用 `COVERAGE_PROFILE`、`COVERAGE_HTML` 和 `COVERAGE_THRESHOLD`。

## 常见改动

### 新增或修改命令

1. 添加 `cmd/<name>.go`，并在 `cmd/root.go` 中注册。
2. 通过 `output.Writer.Data` 输出结果，保持 **stdout 只写数据**；进度和提示通过 `Progress`/`Warn` 写入 stderr。
3. 添加 `affordance.Affordance`，写清 when / avoid / prereq / examples。它会显示在 `--help` 中，也会被 Skills 使用。
4. 有副作用的命令必须支持 `--dry-run`，且该模式下不能写入任何内容。
5. 添加单元测试；新增公开命令面时，还要在 `tests/` 中补 E2E 用例。

### 新增或修改 `error.code`

1. 在 `internal/errs/catalog.go` 的 `errs.Catalog` 中登记，并写一行摘要。
2. 通过 `errs.New(...)` 或 `errs.Wrap(...)` 返回，格式为 `vertc.<domain>.<subtype>`。
3. 重新生成 snapshot：

   ```bash
   UPDATE_SNAPSHOT=1 go test ./internal/errs/ -run TestCatalogSnapshot
   ```

4. 删除或重命名错误码属于破坏性契约变更，需要维护者审查、`CHANGELOG.md` 记录和 MAJOR 版本升级。

### 添加 `scene × platform` 模板

1. 将源文件以 `*.tmpl` 形式放在 `internal/template/files/<scene>/<platform>/`。
2. 在 `internal/template/registry.go` 中注册，设置 `Available: true`，并固定 SDK 版本。
3. 提供 `vertc.taskfile.yaml.tmpl`，包含 `post_create`、`install` 和 `dev`。
4. 添加 render 测试，检查文件树和核心链路标记，参考 `render_test.go`。
5. 填好配置和 Token 后，生成项目必须能通过 `vertc doctor`。

添加模板不需要修改 `cmd/` 或 `internal/errs/`。

### 添加场景 Skill

1. 将 `skill-template/skill-template.md` 复制到 `skills/byted-<product-code>-<skill-name>/SKILL.md`。
2. Skill 应写成接入流程：识别场景、安排命令顺序、处理失败；不要写成 flag 手册。

## 稳定契约

`error.code`、stdout JSON 信封、退出码语义和 affordance 字段是 Agent 和 Skill 依赖的公开契约。不要随意修改。相关变更属于 MAJOR 版本升级，需要维护者确认，详见 [CHANGELOG.md](./CHANGELOG.md)。

## 安全要求

不要提交凭据、私有端点、本地文件路径或其他环境相关数据。密钥使用文档规定的环境变量。

## Pull Request 检查清单

- [ ] 一次改动只处理一项能力
- [ ] `make ci` 通过
- [ ] 已新增或更新测试
- [ ] 命令面变化时已更新 `README.md`
- [ ] 已在 `CHANGELOG.md` 的 `Unreleased` 下记录变更

---

<a id="english"></a>

# Contributing to vertc

[简体中文](#zh-cn) | English

Thanks for helping build vertc. This is the *how-to* for common contributions;
the architecture and hard rules live in [AGENTS.md](./AGENTS.md) — read it first.

## Setup

```bash
make tools     # install pinned golangci-lint and Gitleaks (once)
make ci        # full local gate, including public-surface and open-source release checks
```

Branch off `main`, keep the diff focused on one capability, and make sure
`make ci` is green before opening a pull request. Auto-fix formatting with
`make fmt`.

`make check-change-contract` compares the branch with its merge base and makes
sure stable CLI surfaces change alongside their validation: commands need a
command or E2E test, error codes need their snapshot, and template sources need
template tests. It uses the MR diff base or push base in CI, and `origin/HEAD`
when run locally.

### Go unit coverage

Run the canonical coverage workflow from the repository root:

```bash
make coverage        # writes coverage.out and coverage.html, then prints the total
make check-coverage  # regenerates both reports and enforces the configured floor
```

The metric uses uncached atomic statement coverage for production packages in
`cmd/` and `internal/`, driven by tests in those same package trees. Subprocess
E2E tests under `tests/` remain a separate functional gate because execution in
their child CLI binary is not represented by a normal Go unit-test profile.

CI pipelines enforce an aggregate floor of 70.0% through the same Make target.
They intentionally do not apply
one threshold to every package: command entry points, platform adapters, and
pure logic have different testability. New tests should assert observable
success, failure, boundary, dry-run, or rollback behavior rather than execute
lines only to increase the percentage. Override report paths or the local floor
when needed with `COVERAGE_PROFILE`, `COVERAGE_HTML`, and
`COVERAGE_THRESHOLD`.

## Contribution recipes

### Add / change a command
1. Add `cmd/<name>.go` and register it in `cmd/root.go`.
2. Emit results via `output.Writer.Data` (**stdout = data only**); send progress
   and hints to stderr via `Progress`/`Warn`.
3. Attach an `affordance.Affordance` (when / avoid / prereq / examples) — it is
   surfaced in `--help` and consumed by Skills.
4. Support `--dry-run` if the command has side effects, and **write nothing** in
   that mode.
5. Add unit tests, plus an E2E case in `tests/` for any new command surface.

### Add / change an `error.code` (CLI contract)
1. Register it in `errs.Catalog` (`internal/errs/catalog.go`) with a one-line summary.
2. Emit it via `errs.New(...)` / `errs.Wrap(...)`; shape is `vertc.<domain>.<subtype>`.
3. Regenerate the snapshot:
   `UPDATE_SNAPSHOT=1 go test ./internal/errs/ -run TestCatalogSnapshot`.
4. **Deleting or renaming** a code is a breaking contract change → needs
   maintainer review + a `CHANGELOG.md` entry + a MAJOR version bump.

### Add a `scene × platform` template (low-barrier)
1. Add sources under `internal/template/files/<scene>/<platform>/` as `*.tmpl`.
2. Register it in `internal/template/registry.go` (`Available: true`, pin the SDK).
3. Include a `vertc.taskfile.yaml.tmpl` with `post_create` / `install` / `dev`.
4. Add a render test asserting the file tree and core workflow markers (see `render_test.go`).
5. A generated project must pass `vertc doctor` once config + token are filled.

You do not need to touch `cmd/` or `internal/errs/` to add a template.

### Add a scene skill (low-barrier)
1. Copy `skill-template/skill-template.md` into `skills/byted-<product-code>-<skill-name>/SKILL.md`.
2. Keep it an integration playbook (scene recognition → command order → failure routing),
   not a flag manual.

## Stable contracts — don't break casually

The `error.code` values, the stdout JSON envelope shape, exit-code semantics, and
affordance fields are the **public contract** that Agents and Skills depend on.
Changing them is a MAJOR version bump and requires maintainer sign-off. See
[CHANGELOG.md](./CHANGELOG.md).

## Security

Do not commit credentials, private endpoints, local filesystem paths, or other
environment-specific data. Use documented environment variables for secrets.

## Pull request checklist

- [ ] One capability per change
- [ ] `make ci` green
- [ ] Tests added/updated
- [ ] `README.md` updated if the command surface changed
- [ ] `CHANGELOG.md` entry under `Unreleased`
