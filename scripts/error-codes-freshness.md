# 错误码知识库过期巡检约定

`internal/errorcodes/*.yaml` 是 `vertc explain-error` 的离线知识库，转录自火山官方
公开文档。官方页会更新，本地转录会过期。本约定用来**人工/半自动**巡检过期，
**不做自动爬虫**（explain-error 保持离线、无网络依赖）。

## 巡检对象

按 `source` + `source_version` 对照官方页的「页面更新日期」。当前基线：

| domain | 文件 | 来源页 | source_version（本地基线） | 说明 |
|--------|------|--------|----------------------------|------|
| web-sdk | `web.yaml` | https://www.volcengine.com/docs/6348/104480 | 2025-10-17 | Web SDK 错误码 |
| voice-agent | `voice-agent.yaml`（VoiceChat 运行态） | https://www.volcengine.com/docs/6348/1928198 | 2026-05-09 | 事件和错误码（回调 EventType=1） |
| voice-agent | `voice-agent.yaml`（OpenAPI 层） | https://www.volcengine.com/docs/6348/70426 | —（页面无显式版本） | RTC 公共错误码 |
| voice-agent | `voice-agent.yaml`（登录凭证与请求签名） | 无逐项公开来源 | —（`verified: curated-seed`） | 通用排障建议，不声明为官方错误说明 |

> VoiceChat / OpenAPI 快变内容（如 StartVoiceChat 请求参数）以**官方链接优先**，
> 本地只保留错误码摘要，不镜像完整接口文档。

## 巡检流程（发版前 / 定期）

1. 逐行取上表的来源页与本地 `source_version`。
2. 打开官方页，核对其标注的「页面更新日期」。
3. 若官方页更新日期 > 本地 `source_version`：
   - 对照差异，更新对应 yaml 条目的 `meaning` / `fix` / 新增码；
   - 刷新该来源组条目的 `source_version` 与 `verified_at`；
   - 保持 `verified: verified`（已核对）。
4. 若某条无法在官方页核对到：降级为 `verified: curated-seed`，并说明。
5. 改动后跑 `make test`；若触及 CLI `error.code` 或输出快照，按 `AGENTS.md` 刷新快照。

## 可信状态（verified）语义

- `verified`：转录自官方公开文档且已核对。
- `curated-seed`：通用排障建议，未逐条从官方公开文档核验。
- `unverified`：来源不明 / 待核。

`explain-error` 对 `verified != verified` 的条目会显式提示「以官方文档为准」。
