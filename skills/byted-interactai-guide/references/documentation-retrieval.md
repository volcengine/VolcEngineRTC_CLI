# RTC 文档检索与原文核验

本 reference 用于需要核对当前 RTC 文档的咨询。命令只读、无需项目配置或登录，不会
读取或转发 AppKey、Signin 凭据和 `.env.local` 内容。

## 推荐路径：search → fetch

1. 用用户问题中的产品名、API 名或症状执行检索：

   ```bash
   vertc docs search "publish audio" --limit 10 --format json
   ```

2. 按返回顺序查看 `results[].id`、`score` 和 `snippets`。`--limit` 只在本地截取服务端排序，
   默认 10、最大 50；snippet 仅用于选文档，不能当作完整依据。
3. 对最相关的精确 ID 获取原始 Markdown：

   ```bash
   vertc docs fetch <doc-id> --format json
   ```

   引用或下结论前以 `data.content` 为准；`data.bytes` 与 `data.sha256` 可用于记录同一正文证据。
   面向人直接阅读时可改用 `--format pretty`，stdout 会原样输出 Markdown。
4. 若关键词不明确或需要浏览目录，使用：

   ```bash
   vertc docs list --query audio --offset 0 --limit 20 --format json
   ```

   `list` 会先读取完整索引，再在本地对标题/摘要做大小写不敏感过滤和分页；它不是相关性排序。

## 输出与失败路由

- JSON 成功结果在 stdout 单一 envelope 的 `data` 中；重试进度和警告只在 stderr。
- `vertc.docs.document_not_found`：回到 search/list 获取当前精确 ID，不猜测路径。
- `vertc.docs.tool_schema_changed`、`vertc.docs.unsupported_protocol`：停止自动解释，报告服务契约
  已变化，等待 CLI 或服务更新。
- `vertc.docs.timeout`、`vertc.docs.request_failed`、`vertc.docs.rate_limited`：可按错误提示稍后重试；
  不要把网络失败解释为产品不支持。
- 其他 `vertc.docs.*` 错误先按稳定 `error.code` 分流，禁止把原始响应、query、session 或正文
  复制进日志。

## 离线与降级

该命令不提供缓存或隐式摘要。服务不可达时，显式告诉用户“当前实时文档未核验”，再按需求：

- 使用本 Skill 已内嵌的 `capabilities.md`、`voicechat-api.md` 等 reference，并同时说明其中的
  `verified_at` 或可信状态；
- 使用用户已提供的文档正文继续分析；
- 若结论依赖当前版本、计费、配额、模型兼容或公测状态，则等待恢复后重试，不用旧资料给出
  确定性结论。

不要自动切换 endpoint，不要要求用户提供鉴权 header，也不要通过 query 携带凭据或个人信息。
