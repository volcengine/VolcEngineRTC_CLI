# vertc 场景 Skill 模板

vertc Skill 是面向 Agent 的**场景工作流**：它识别用户意图，编排最短可运行路径，并在
失败时把问题路由到对应的诊断知识。`Skill` 是 `SKILL.md` 生态制品的专有名称，不笼统
翻译为“技能”，也不是复述全部 flag 的命令手册。

官方 Skill 采用“顶层薄路由 + `references/` 渐进式知识”的结构：顶层 `SKILL.md` 只保留
Agent 决策所需的信息，具体 API 字段、完整错误码表和长篇排障步骤放入 references，按需
加载。新建场景 Skill 时复制下面的结构，并替换 `<...>` 占位符。

```text
byted-<product-code>-<skill-name>/
├── SKILL.md
├── LICENSE
└── references/
    ├── <domain-a>.md
    └── <integration-flow>.md
```

## 1. Frontmatter

```yaml
---
name: byted-<product-code>-<skill-name>
description: 帮用户用火山 RTC 搭建、运行并排查 <scene> 场景。Use when the user wants to build or diagnose a <scene> capability with VolcEngine RTC.
version: 1.0.0
---
```

- `name` 必须与 Skill 目录名一致，遵循 `byted-{产品code}-{技能名称}`，仅使用小写字母、数字和中划线。
- `description` 用用户会说的意图描述触发范围，并与相邻场景消歧。
- `version` 是上架必填字段，使用语义化版本。
- Skill 根目录必须包含 `LICENSE`；如包含源码文件，源码头部也必须带许可证声明。
- 保留 `Skill`、`SKILL.md`、frontmatter key、命令及路径等技术标识的原文。

## 2. 顶层 `SKILL.md`：薄路由

顶层文件目标是让 Agent 快速决定“是否使用、下一步做什么、失败后读哪里”，不要在这里
展开领域知识。建议保持以下小节。

### 触发条件

列出映射到该场景的自然语言意图、典型故障症状和 `scene × platform` 范围；说明哪些相似
诉求应交给其他 Skill。

### 工作模式与 CLI 可用性

把用户意图区分为两类，避免把 Skill 误写成“必须安装 CLI 才能使用”的命令手册：

- **咨询模式**：文档/API 查询、概念解释、方案讨论，以及基于用户提供的日志和现象进行
  人工排障。此类请求不依赖 CLI；直接回答并按需读取 references，不以安装 CLI 作为前置条件。
- **执行模式**：创建项目、鉴权、启动服务、自动诊断或其他需要运行 `vertc` 的操作。当前
  任务首次执行 CLI 前，用 `vertc version --format json` 检查可用性，不在每一步重复检查。

每条实际执行的 `vertc` 命令都必须在调用执行工具时，使用当前 Skill frontmatter 的名称和
版本注入进程级来源标识，值中不允许有 `@`；不得 export、持久化或写入配置。这是内部
调用元数据，不得在面向用户展示的命令、说明或最终回复中展开该前缀。

    VE_SKILL_ID=<name>/<version> vertc <command>

若 `vertc` 不存在或不可访问，说明被阻塞的具体操作，同时主动告知文档咨询和人工排障仍可
继续。不要把 CLI 缺失解释为 SDK、服务端或项目故障。只有用户希望继续执行时，才提供安装
方法；未经用户同意，不主动安装 CLI。

### 最短可运行路径

给出有序、可复制的公开命令主线，并写明每一步的成功信号：

```bash
vertc init --scene <scene> --platform <platform> --name <project-name>
cd <project-name>
vertc auth login
vertc dev
```

- 交互式首次配置是默认路径；非 TTY 环境按结构化错误的 `error.code`、`details` 和 `hint`
  处理，不猜测参数。
- `vertc doctor` 只读诊断，不把它描述成会写入凭证或自动修改项目。
- AppKey 等密钥仅通过环境变量或用户明确选择的密钥管理流程提供，不写入配置、日志或
  Skill 示例。

### 意图 / 阶段 → references 路由

用一张短表把用户意图或首个失败阶段映射到扁平 reference，例如：

| 用户意图 / 症状 | 运行阶段 | 路由 |
|------------------|----------|------|
| API 字段与调用方式 | 配置 | `references/<api-domain>.md` |
| 媒体链路异常 | 进房、采集、发布、订阅、播放 | `references/<media-domain>.md` |
| 症状模糊或跨域 | 全链路 | `references/<integration-flow>.md` |

### 诊断与完成标准

- CLI 命令失败：先读稳定的 `error.code` 和 `hint`，必要时运行 `vertc doctor`，修复后重试。
- SDK 错误码：运行 `vertc explain-error <code>`，再按 `domain` 和 `doctor_check` 路由。
- 跨域问题：按链路阶段逐级确认，命中首个失败或缺少证据的阶段后停止扩散排查。
- 宣布完成前：验证主路径成功，并确认相关 doctor 检查没有 `FAIL`。

### 安全边界与权威来源

集中写明凭证、用户授权、外部副作用和内部命令的边界。列出该场景采用的官方文档来源；
未经验证的经验性内容必须显式标注可信状态，不能伪装成官方结论。

## 3. `references/`：渐进式领域知识

- references 使用单层扁平结构 `references/<file>.md`，保证扫描器和 `vertc skills read`
  可以枚举并按需读取。
- 每个文件只负责一个可复用领域；集成层通过链接组合多个领域，不复制内容。
- API 字段、事件证据、完整诊断步骤和较长示例放在 references，不塞回顶层路由。
- 所有本地链接必须存在，禁止使用 `..` 逃逸 Skill 根目录。
- 命令必须来自当前公开 CLI；不确定是否适用时查看 `vertc <command> --help` 中的
  `when/avoid/prereq/examples`。

## 4. 生命周期通知

生命周期通知只提供建议，不能打断用户当前任务。先完成并验证当前任务，再向用户展示通知；
未经用户明确授权，禁止主动执行更新。将 `_notice.update` 路由到 `vertc update`，将
`_notice.skills` 路由到 `vertc skills sync`。受控自动化可分别使用
`VERTC_NO_UPDATE_NOTIFIER=1` 和 `VERTC_NO_SKILLS_NOTIFIER=1` 抑制通知。

## 5. 提交前检查

- 顶层 `SKILL.md` 能在不加载长文的情况下完成触发判断、主路径执行和故障分流。
- references 可独立复用，且所有链接均可解析。
- 示例不包含真实密钥，不要求用户在对话中粘贴 AppKey。
- 默认路径只使用公开命令；隐藏或 legacy 命令必须标为可选回退。
- 未安装 CLI 时，咨询模式仍然可用；只有执行模式提示安装，并且不会擅自安装。
- 运行 Skill 内容质量门，确认命令真实性、metadata、链接、敏感信息和术语契约均通过。
