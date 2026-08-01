<!--
    Panvara
    docs/modules/release-publishing.md    2026-07-19
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# Module Release 发布事实

## 用途

Module Release 把一个已经 Validate、已经生成 Change Plan 的候选模型登记为不可变发布事实。它回答“哪个 Project/Environment、哪个计划、哪个 Candidate、由谁在什么时候发布”，并允许客户端安全重试。

它不回答“线上正在运行哪个版本”。Publish 不激活、不迁移 Record，也不热切换 Runtime；当前版本必须从 Active Snapshot 或 OpenAPI 读取。

## 当前状态

当前 Publish 为 **P0-02a runnable slice**，另有 P0-02b 的窄 compatible Activate：

- 可以用一个 `plan_id` 发布当前、有效、可复验的 Draft 计划。
- Candidate Revision 会幂等进入 Revision Registry。
- Module Release、发布专用幂等绑定与成功安全审计在同一 PostgreSQL 事务提交。
- 可以按 Release ID 读取发布事实。
- `compatible`、`review_required`、`migration_required` 可以发布；`unsupported` 被拒绝。
- 数据身份完全不变的 compatible Release 可显式 Activate，并持久化 active pointer/epoch；没有 Review/Migration 激活、Rollback、Migration Run、Outbox、Worker 或多节点收敛。

::: danger Published 不等于 Activated
响应中的 `published=true` 只表示不可变发布事实已经写入。请同时检查 `activated=false`、`records_migrated=false`、`runtime_changed=false` 和 `activation_supported=false`。
:::

## 前置条件

- 已按[本地环境](../getting-started/local-environment.md)启动 PostgreSQL 与 Server。
- 已完成 [Draft → Validate → Plan](../getting-started/draft-plan-acceptance.md)，并取得有效 `PLAN_ID`。
- 调用 Credential 对当前 Project、默认 Environment 具有 active `project.owner` Grant。
- 本机有 `curl` 与 `jq`。

不要把 Token 写进命令历史、截图或 Git。示例使用已经由 `.env.local` 加载的 `PANVARA_ADMIN_TOKEN`。

## 最小示例

```sh
BASE=http://127.0.0.1:8080
MODULE=crm.leads
PLAN_ID='sha256:替换为实际计划身份'
PUBLISH_KEY="publish-$(date +%s)-001"

curl -fsS -D /tmp/panvara-release-headers.txt \
  -X POST "$BASE/api/admin/core/v1alpha1/modules/$MODULE/releases" \
  -H "Authorization: Bearer $PANVARA_ADMIN_TOKEN" \
  -H 'Content-Type: application/json' \
  -H "Idempotency-Key: $PUBLISH_KEY" \
  --data "{\"plan_id\":\"$PLAN_ID\"}" \
  | tee /tmp/panvara-release.json \
  | jq '{release_id,plan_id,candidate_revision,outcome,effects}'

RELEASE_ID="$(jq -er '.release_id' /tmp/panvara-release.json)"

curl -fsS \
  "$BASE/api/admin/core/v1alpha1/modules/$MODULE/releases/$RELEASE_ID" \
  -H "Authorization: Bearer $PANVARA_ADMIN_TOKEN" \
  | jq '{release_id,plan_id,candidate_revision,effects}'
```

首次 POST 预期为 201，并带 `Location: /api/admin/core/v1alpha1/modules/crm.leads/releases/<release-id>`。GET 预期为 200，且两次响应中的 `release_id`、`plan_id` 与 `candidate_revision` 完全相同。

## 配置

Publish 不增加环境变量。它沿用 Server 的：

- `PANVARA_PROJECT_ID`：当前进程装配的 Project。
- 持久化默认 Environment：由 P0-01a bootstrap 建立，当前不接受非默认 Environment。
- `PANVARA_ADMIN_TOKEN`：文档示例所用 Credential；其他 active Owner Credential 同样可用。
- `PANVARA_MODULE_SOURCE` 与 `PANVARA_MODULE_FORMAT`：只决定当前启动 Runtime，不由 Publish 改写。

POST Body 必须是且只能是 `{"plan_id":"sha256:..."}`。字段按原始字节中的精确 ASCII 匹配；未知字段、重复字段、大小写别名、Unicode 混淆、`\u` 转义字段名、多个 `Idempotency-Key`、查询参数或多余 Body 都返回 400。Key 是不透明客户端请求身份，不应包含 Token 或个人信息。

成功响应字段：

```json
{
  "release_id": "019f...",
  "module": "crm.leads",
  "draft_id": "019f...",
  "draft_generation": 2,
  "validation_id": "sha256:...",
  "plan_id": "sha256:...",
  "plan_hash": "sha256:...",
  "baseline_revision": "sha256:...",
  "candidate_revision": "sha256:...",
  "data_schema_identity": {"format": 1, "fingerprint": "sha256:..."},
  "source_hash": "sha256:...",
  "outcome": "compatible",
  "risk": "low",
  "published_by": "bootstrap-admin",
  "published_credential_id": "019f...",
  "request_id": "req_...",
  "published_at": "2026-07-19T00:00:00Z",
  "effects": {
    "revision_registered": true,
    "published": true,
    "activated": false,
    "records_migrated": false,
    "runtime_changed": false,
    "activation_supported": false
  }
}
```

`baseline_revision` 在 Draft 明确使用 `none` 时为 `null`。`revision_registered=true` 表示 Candidate 在 Registry 中可用；若同一 Revision 早已由 bootstrap 登记，Registry 会保留首次 `origin` 和 `registered_by`，不会伪造为本次 Publish 创建。

## 验收

完整、可复制且会验证重放、Registry 与 Runtime 不变的旅程见 [Draft → Publish 完整验收](../getting-started/draft-publish-acceptance.md)。最低成功标志是：

1. 首次 POST 返回 201，同 Key 重放返回 200，Release ID 不变。
2. Draft 变化使 Plan stale 后，原 Key/原 Plan 与相同 Plan/新 Key 仍返回 200 和同一 Release；已绑定 Key 改成另一 Plan 优先返回 409。
3. GET 返回同一不可变事实，并带 `Cache-Control: private, no-store`。
4. Candidate 可从 Revision Registry 读取。
5. 发布前后的 OpenAPI `x-panvara-revision` 与已有 Record 完全不变；重启后仍不改变。
6. 数据库不可用时不会返回成功，也不会留下可见半状态。

工程门禁对应 `make verify`、`make test-integration`、`make test-server-smoke`、`make test-e2e` 与 `make docs-check`。

## 常见问题

### 为什么发布后接口仍是旧模型？

因为 Publish 只记录事实。只有随后调用 compatible Activate 才会切换 Runtime；active pointer 存在后，`PANVARA_MODULE_SOURCE` 仍用于启动登记，但不会覆盖活动版本。

### `migration_required` 为什么还能发布？

发布是在确认一个稳定候选和计划，激活才会影响运行。保存 `migration_required` 事实可以让后续 Migration Run 引用它；当前窄 Activate 会拒绝该结果，所以不能上线。

### 为什么同一 Plan 换 Key 还是原 Release？

一个环境内同一模块和 Plan 只有一个发布事实。新 Key 会作为该事实的另一个安全重放入口，不制造重复发布。

### Draft 已经改变，为什么旧 Plan 还能重放？

stale 检查决定一个尚未发布的 Plan 能否首次发布，不会撤销已经提交的事实。服务端会先在短事务里二次授权并解析专用幂等绑定：已绑定 Key/相同意图，或已发布 Plan/新 Key，直接返回原 Release；只有未命中的新发布才读取并复验当前 Draft/Validation/Plan。

### Candidate 已经在 Registry，是否说明它正在运行？

不是。Registry 只保存不可变制品；当前运行 Revision 仍只能从公开 OpenAPI 顶层 `x-panvara-revision` 读取。

### 发布失败后可以直接重试吗？

可以使用同一个 `Idempotency-Key` 和相同 `plan_id` 重试，即使其 Draft 后来已经变化。不要在不确定时改 Plan；同一个已绑定 Key 表达不同 `plan_id` 会稳定返回 409，先 GET 或重放原请求确认结果。

## 当前限制

- 只有 POST Publish、GET Detail、compatible Activate 与 GET Active，没有 List、Rollback 或 Delete。
- 没有数据迁移执行；`migration_required` 只是一项事实。
- 有单进程 active Snapshot、epoch 与重启恢复；没有多节点通知、ACK 或自动收敛。
- 只有固定 `project.owner` 可发布，没有发布审批角色或四眼原则。
- Release 已绑定 Environment，但 Draft、Revision、Record 仍主要是 project-scoped，未实现真正多 Environment 数据隔离。
- 发布专用幂等和安全审计不是 P0-05 通用框架。
- Plan 固定 Candidate identity 与 Canonical IR，但不保存当时的 OpenAPI/Manager Schema 字节；Publish 使用当前 Core 生成它们。跨 Core 版本发布前应重新 Validate/Plan。
- 当前 Runtime 仍是一个进程、一个 Project、一个启动模块。

## 兼容与升级

Migration `0006` 以 forward-only 方式新增 append-only Release 与幂等绑定，并允许 Revision 保存受控 Publish provenance。它不会改写旧 Revision、Draft、Validation、Plan 或 Record。

Release、幂等绑定和 Registry 都不能用 SQL UPDATE/DELETE/TRUNCATE 清理。未来 Activate、Migration 与 Rollback 必须追加新事实并引用当前 Release；不能通过覆盖本行表达状态变化。回退到旧二进制前需要停止发布写入并恢复兼容备份。

完整决策见 [ADR-0006](../adr/0006-immutable-module-release-publish-facts.md)。
