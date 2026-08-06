# 集成层 — 跨域协作与排障顺序

**Domain**: integration

本文描述 Web SDK 与 Voice Agent 两域的**协作关系与跨域排障顺序**，只用链接引用两域
的具体知识，**不复制**其细节：

- RTC 媒体链路细节 → `references/web-sdk-diagnosis.md`
- 对话运行态细节 → `references/voice-agent-runtime.md`
- VoiceChat OpenAPI 字段 → `references/voicechat-api.md`

## 统一诊断结果协议（全域通用）

任一诊断产出以下七字段；缺证据的阶段记为「证据缺失」而非「确认失败」：

| 字段 | 含义 |
|------|------|
| `symptom` | 用户看到的现象 |
| `domain` | `web-sdk` / `voice-agent` / `integration` / `auth` |
| `last_success` | 最后一个确认成功的阶段 |
| `first_failure` | 首个失败或缺少证据的阶段 |
| `evidence` | 日志 / 事件 / 错误码 / 配置证据 |
| `action` | 建议执行的最小修复动作 |
| `verification` | 修复后的验证方式 |

## 端到端典型链路

```text
鉴权 → Demo 配置 → Web SDK 初始化 → 用户进房 → 麦克风采集与发布
→ StartVoiceChat → Agent 进房 → Agent 订阅用户音频
→ ASR/VAD → LLM → TTS → 用户订阅并播放 Agent 音频
```

## 故障阶段 ↔ 领域归属表

| # | 阶段 | domain | 关键证据 | 成功判定 | 细节 |
|---|------|--------|----------|----------|------|
| 0 | 鉴权 | auth | Signin STS 有效（`vertc auth status`） | STS 可用未过期 | — |
| 1 | Demo 配置 | integration | 配置/场景 JSON 校验（`vertc doctor`） | doctor 项目项 PASS | — |
| 2 | Web SDK 初始化 | web-sdk | 引擎创建、安全上下文 | 引擎就绪 | web-sdk-diagnosis |
| 3 | 用户进房 | web-sdk | join 回调、进房错误码 | 进房成功 | web-sdk-diagnosis |
| 4 | 麦克风采集与发布 | web-sdk | 设备权限、发布事件 | 本地音频已发布 | web-sdk-diagnosis |
| 5 | StartVoiceChat 下发 | voice-agent | 直接 OpenAPI 返回；或 CLI/配套 Server 成功 envelope/typed error | 直接调用 `Result=ok`；适配层调用成功且无错误。仅表示下发成功 | voice-agent-runtime |
| 6 | 任务初始化 | voice-agent | 可选 VoiceChat 回调、AI 状态或更强下游证据 | `taskStart`、Agent 进房或任一下游运行态证据；缺少未配置的回调不算失败 | voice-agent-runtime |
| 7 | Agent 进房 | voice-agent | Agent 参与者出现；AI 状态可作辅助 | Agent 已进房 | voice-agent-runtime |
| 8 | Agent 订阅用户音频 | integration | Agent 侧订阅 / 服务端日志 | 已订阅目标 UID | 两域衔接 |
| 9 | ASR/VAD | voice-agent | 识别事件 / 字幕增量 | 有识别结果 | voice-agent-runtime |
| 10 | LLM | voice-agent | 对话事件 / 服务端返回 | LLM 返回文本 | voice-agent-runtime |
| 11 | TTS | voice-agent | 合成事件 / 错误码 | 生成音频 | voice-agent-runtime |
| 12 | 用户订阅并播放 Agent 音频 | web-sdk / integration | 远端发现 / 订阅 / 自动播放 | 用户可听到 | web-sdk-diagnosis |

阶段 5–7 的失败来源归 `voice-agent`；阶段 8 的媒体绑定归 `integration`；阶段 12
根据证据归 `web-sdk`（客户端订阅/播放）或 `integration`（跨域流衔接）。仅链接两域，不重复保存细节。

## 「AI 没回答」分阶段收敛路由

不要一次抛出泛化清单。按下列顺序逐级确认「上一步成功」再前进，**命中首个失败/缺证据
阶段即停止**并按七字段协议输出。

| 步 | 问题 | domain | 可观察证据 | 检查方法 | 成功判定 | 失败下一步 |
|----|------|--------|-----------|----------|----------|-----------|
| 1 | 用户是否成功进房 | web-sdk | join 回调/错误码 | 页面日志 + `vertc doctor` | 进房成功 | web-sdk-diagnosis §进房失败 |
| 2 | 麦克风是否采集并发布 | web-sdk | 设备权限、发布事件 | 页面日志 | 本地音频已发布 | web-sdk-diagnosis §设备权限/未发布 |
| 3 | StartVoiceChat 是否成功下发 | voice-agent | 直接 OpenAPI 返回；或 CLI/配套 Server 返回 | 服务端/CLI 日志 + `vertc explain-error <code>` | 直接调用 `Result=ok`；适配层成功且无 typed error。不代表已进房 | voice-agent-runtime §StartVoiceChat 下发失败 |
| 4 | 是否存在任务初始化错误（可选观测） | voice-agent | 已配置的 VoiceChat 回调、AI 状态 | 回调/状态日志；未配置则记录「未观测」并继续 | 无显式错误；`taskStart` 或更强下游证据可确认启动 | voice-agent-runtime §任务初始化与 Agent 进房 |
| 5 | Agent 是否进房 | voice-agent | Agent 参与者出现、AI 状态 | 房间成员/远端用户日志 | Agent 已进房；其出现也可反向确认任务已启动 | voice-agent-runtime §任务初始化与 Agent 进房 |
| 6 | Agent 是否订阅到目标用户音频 | integration | Agent 侧订阅/服务端日志 | 服务端日志 + 目标绑定核对 | 已订阅目标 UID | 核对 Target UID 与 User ID |
| 7 | ASR/VAD 是否产生结果 | voice-agent | 识别事件/字幕 | 字幕/服务端日志 | 有识别结果 | voice-agent-runtime §ASR 无结果 |
| 8 | LLM 是否返回 | voice-agent | 对话事件 | 服务端/模型端点日志 | LLM 返回 | voice-agent-runtime §LLM 无返回 |
| 9 | TTS 是否生成音频 | voice-agent | 合成事件/错误 | 服务端日志 + `vertc explain-error <code>` | 生成音频 | voice-agent-runtime §TTS 失败 |
| 10 | Web SDK 是否订阅并播放 Agent 音频 | web-sdk / integration | 远端发现/订阅/自动播放 | 页面日志 | 用户可听到 | web-sdk-diagnosis §订阅/自动播放 |

## 验收场景 → 领域 / 首个失败阶段

| 场景 | domain | first_failure |
|------|--------|---------------|
| Token 无效导致进房失败 | web-sdk | 用户进房 |
| 麦克风权限拒绝 | web-sdk | 麦克风采集与发布 |
| 用户进房但未发布音频 | web-sdk | 麦克风采集与发布（last_success=用户进房）|
| StartVoiceChat 失败 | voice-agent | StartVoiceChat |
| 下发成功后收到异步初始化错误 | voice-agent | 任务初始化（last_success=StartVoiceChat 下发）|
| 任务已启动但 Agent 未进房 | voice-agent | Agent 进房（last_success=任务初始化）|
| 未配置任务回调但 Agent 已进房 | voice-agent | 回调未观测，不构成失败；继续后续阶段 |
| Agent 已进房但 ASR 无结果 | voice-agent | ASR/VAD（last_success=Agent 订阅用户音频）|
| LLM 有结果但 TTS 或播放失败 | voice-agent / web-sdk | TTS 或 用户订阅并播放（由证据区分）|
| 纯 Web SDK 远端音频无法播放 | web-sdk | 用户订阅并播放 |

## 辅助命令

- `vertc doctor`（只读定位，不写凭证/不改配置）
- `vertc explain-error <code>`（离线反查错误码含义与修复建议）
- 鉴权修复：`vertc auth login`；缺少 RTC App/Bot 配置时运行 `vertc dev`，需要重新选择时
  运行 `vertc dev --reconfigure`
