---
name: byted-interactai-guide
description: 解释火山 AI 音视频互动的产品能力、适用边界与最新官方文档；并帮助用户搭建、运行和分阶段排查最小 InteractAI VoiceChat Web Demo。用户询问产品支持情况、能力清单、接入方案或运行故障时使用。
version: 0.0.1
---

# InteractAI Guide — 能力、接入与排障薄路由

这是唯一对外 Skill 入口，只做**路由**：识别意图与运行阶段，导向 `references/` 中的
专题知识。具体 API 字段、错误码与长排障步骤都在 references 里，按需加载。

## 触发条件

- 「搭一个能对话的语音智能体 / 语音 Demo」「跑通火山 RTC 语音对话」等接入诉求。
- 询问产品支持情况、能力清单、某项能力的工作方式或适用边界。
- 询问当前支持模型、接口版本、计费、公测状态或近期新增能力，需要核对最新官方文档。
- 运行中出现「AI 没回答」「Agent 没进房」「能进房但没声音」「没有字幕」等故障。
- 需要解释某个 RTC / VoiceChat 错误码。

## 工作模式与 CLI 可用性

先区分用户需要的是咨询还是实际执行，因为本 Skill 的知识内容可以独立使用，只有自动化操作
依赖 CLI：

- **咨询模式**：文档/API 查询、概念解释、接入方案，以及基于用户提供的日志和现象进行
  人工排障。直接回答并按需读取 `references/`；不要仅因未安装 CLI 而要求用户安装。
- **执行模式**：创建项目、登录鉴权、获取 App/Bot 配置、启动 Demo、运行自动诊断或调用
  CLI 离线错误码查询。当前任务首次执行 CLI 前，先运行 `vertc version --format json`
  检查可用性；无需在后续每一步重复检查。

本 Skill 实际执行的每条 `vertc` 命令都必须在调用执行工具时注入进程级 Skill 标识；
值必须由当前 `SKILL.md` frontmatter 的 `name` 和 `version` 组成，不允许有 `@`。不得
export、持久化或写入配置。这是内部调用元数据：不要在面向用户展示的命令、说明或最终
回复中展开该前缀。

    VE_SKILL_ID=<name>/<version> vertc <command>

若命令不存在或环境无法找到 `vertc`：

1. 明确说明当前环境未安装或无法访问 CLI，并指出因此暂时不能执行哪些操作。
2. 同时告诉用户仍可继续文档咨询、方案讨论和基于现有证据的人工排障，不要把 Skill 整体
   判定为不可用，也不要把 CLI 缺失解释成 RTC、VoiceChat 或项目故障。
3. 若用户希望继续实际执行，提供 `npm install -g @volcengine/rtc-cli`，安装后用
   `vertc version --format json` 验证。未经用户同意，不要主动安装 CLI。

## 最短可运行路径（公开命令）

```bash
vertc init --scene voice-agent --platform web --name my-agent
cd my-agent
vertc auth login
vertc dev
```

需要用户从有限候选中选择时，必须优先调用当前环境已提供的结构化提问工具（如
`request_user_input` 或 `AskUserQuestion`），等待用户选择后再继续。没有可用的结构化
提问工具时，再降级为简短的编号文本选项；不要调用当前环境未提供的工具。

- `vertc auth login`：需要授权时先询问浏览器/手动登录方式，并默认将可刷新的 Signin
  凭证保存到受保护的用户级文件，不读取或修改当前项目。Agent/非 TTY 必须先获得用户
  同意，再显式使用 `--browser=open`；若浏览器不在 CLI 所在设备，第一轮运行
  `vertc auth login --browser=manual --start` 并把返回的 `authorization_url` 交给用户，
  用户回传授权码后，第二轮将该码经 stdin 传给 `vertc auth login --resume`。收到授权码时
  **不得**重新运行 `--browser=manual` 或 `--start`，否则会生成不同 state，旧授权码必然
  失败。仅在用户选择 `--store=keyring` 后访问系统凭据存储。
- `vertc dev`：缺少 RTC 配置时，唯一 App 自动选择；存在 Bot 时要求用户选择并写入场景
  JSON，确认零 Bot 时使用内置默认 Scene，随后一次启动 Web 与 Server。非 TTY 若收到
  `vertc.dev.selection_required`，只从 `error.details` 读取公开候选，再用 `--app-id` 或
  可重复的 `--bot-id` 重试。Bot 候选超过结构化提问工具的选项上限时，不要截断或分页
  展示；告知候选总数并提供 `https://console.volcengine.com/conversational-ai/agentManage`
  供用户查看，再将用户返回的 Bot 名称或 ID 从 `error.details.bots` 解析为唯一 Bot ID；
  名称不唯一时只展示同名候选。需要重新选择时使用 `--reconfigure`。
  启动后在页面点击 **Start** 进房对话。
- 失败先跑 `vertc doctor`（只读定位），再按下方路由表处理。

## 意图 / 阶段 → references 路由

| 用户意图 / 症状 | 运行阶段 | 路由 |
|------------------|----------|------|
| 产品能力概览 / 是否支持 / 最新能力 | 咨询 | `references/capabilities.md`（快变事实查当前官方文档） |
| VoiceChat API 字段 / 调用方式 | 配置 | `references/voicechat-api.md`（官方链接优先）|
| 进房失败 / 无媒体 / 有声但播不出 | 进房·采集·发布·播放 | `references/web-sdk-diagnosis.md` |
| Agent 未进房 / 无字幕 / ASR·LLM·TTS 异常 | StartVoiceChat 之后 | `references/voice-agent-runtime.md` |
| 「AI 没回答」（症状模糊） | 全链路 | `references/integration-flow.md` → 分阶段收敛 |
| CLI 命令自身报错 | — | 读 `error.code` + `vertc doctor` |

用 `vertc skills read byted-interactai-guide/references/integration-flow.md` 可直接读取任一
reference。

## 能力回答边界

必须区分：**产品支持**、**受模型/版本/配置限制**、**当前 Demo/CLI 未覆盖**、**尚未核验**。
Demo 未实现、本地 reference 未覆盖或无法联网，都不能推导为产品不支持；确定性“不支持”必须
有当前官方文档依据。当前能力、精确 API、模型兼容、计费、配额和公测状态等快变事实，按
`references/capabilities.md` 先查官方正文；无法查询时说明本地 `verified_at` 和未核验边界。

## 运行阶段识别

端到端链路：鉴权 → Demo 配置 → Web SDK 初始化 → 用户进房 → 麦克风采集与发布 →
StartVoiceChat → Agent 进房 → Agent 订阅用户音频 → ASR/VAD → LLM → TTS →
用户订阅并播放 Agent 音频。领域归属与逐阶段证据见 `references/integration-flow.md`。

## 「AI 没回答」不要给泛化清单

必须按链路阶段**逐级收敛**（进房 → 采集发布 → StartVoiceChat → Agent 进房 → 订阅 →
ASR/VAD → LLM → TTS → 播放），确认上一步成功再前进，命中首个失败/缺证据阶段即停止，
并按统一诊断结果协议（symptom/domain/last_success/first_failure/evidence/action/
verification）给出结论。完整分阶段路由见 `references/integration-flow.md`。

## 诊断工具

- `vertc doctor`：**只读**两级体检（CLI 自检 + 项目就绪），逐项 PASS/WARN/SKIP/UNKNOWN/FAIL。
  它**不写入凭证、不修改配置、不联网补全项目**；登录态由 `vertc auth login` 修复，
  缺少 RTC App/Bot 配置时再运行 `vertc dev` 或 `vertc dev --reconfigure`。
- `vertc explain-error <code>`：离线反查 RTC/VoiceChat 错误码的含义与修复建议，输出标注
  `domain`（`web-sdk`/`voice-agent`）、`source` 与 `verified` 可信状态。VoiceChat 运行态与
  OpenAPI 公共码已对官方「事件和错误码」「公共错误码」核验（`verified`）；无逐项公开来源的
  登录凭证与签名排障条目标为 `curated-seed`，会显式提示以官方为准，勿当确定事实。

## 安全边界与提醒

- AppKey 等密钥只走环境变量、绝不写入配置或日志；不要在文档/命令中粘贴真实密钥。
- 遇到 `vertc.dev.selection_required` 或缺少凭证时，**禁止向用户索取、复述或记录
  AppKey**；只让用户从 `error.details` 选择公开 App/Bot ID。确认没有 Bot 时使用内置
  默认 Scene，并可引导用户访问 `https://console.volcengine.com/conversational-ai/agentManage` 定制。人工回退仅
  指向本地编辑器或密钥管理工作流。
- Voice Agent 错误知识已按火山官方「事件和错误码」「公共错误码」核验为 `verified`；无逐项
  公开来源的登录凭证与签名排障条目标为 `curated-seed`，输出会显式提示以官方为准。
- 生命周期 notice 不得打断当前用户任务：先完成并验证当前任务，再向用户说明。不要擅自执行
  自动更新；仅在用户明确要求时运行命令。`_notice.update` 指向 `vertc update`，
  `_notice.skills` 指向 `vertc skills sync`。受控自动化可分别设置
  `VERTC_NO_UPDATE_NOTIFIER=1`、`VERTC_NO_SKILLS_NOTIFIER=1`。
- `agent` / `env` / `token` 等为内部/legacy 命令（默认隐藏），非默认快速路径；优先用
  公开命令 `init` / `auth login` / `dev` / `doctor` / `explain-error` / `skills`。

## 权威来源

- 产品边界：[AI 音视频互动方案产品简介](https://www.volcengine.com/docs/6348/1310537)
- 最新变化：[AI 音视频互动方案发版说明](https://www.volcengine.com/docs/6348/1544162)
- 完整文档树：[AI 音视频互动方案](https://www.volcengine.com/docs/6348/1310445)
