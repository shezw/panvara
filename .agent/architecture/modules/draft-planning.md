<!--
    Panvara
    docs/modules/draft-planning.md    2026-07-16
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# Draft、Validation 与 Change Plan 使用指南

::: danger 准备变更不等于发布变更
Draft、Validation 和 Change Plan 不会登记 Candidate Revision，不会发布或激活模块，不会迁移 Record，也不会切换当前 Server 的运行模型。当前运行 Revision 始终从 OpenAPI 的 `x-panvara-revision` 读取，不能从 Registry List 推断。
:::

## 用途

Draft Planning 为人或模型提供一个短反馈环：先把仍可能有错误的 AppModule Source 保存为 Draft，再对一个精确代次执行 Validation；通过后，生成相对固定 Baseline 的机器可读 Change Plan。

开始前只需要区分五个词：

- **当前运行 Revision**：当前 Server 进程实际提供 API 的模型，从 OpenAPI 或 P0-02b Active Snapshot 读取；它不等于 Registry 中最新登记项。
- **Baseline Revision**：创建 Draft 时由调用者明确固定的比较起点，创建后不会随 Server 或 Registry 改变。
- **Draft Version**：内部 generation 的 HTTP 表达；Source 每次实质覆盖后递增，也是 Draft 元数据强 ETag 的数字值。它不是 Module Version 或 Candidate Revision。
- **Validation**：绑定一个 Draft Version 和 Source Hash 的不可变检查事实；作者模型无效是正常结果，不是服务故障。
- **Change Plan**：Baseline 与 Candidate 的确定性差异说明。它是供人审核的事实，不是已经执行的迁移任务。

## 当前状态

该能力是 **alpha.3b 开发切片**；Panvara Distribution 仍为 `v0.1.0-alpha.2`。

alpha.3b 实现项目 owner 作用域内的 Draft 创建、读取与原始 Source 覆盖，持久化、可重放的 Validation 和 Plan，以及 ETag 并发保护和创建 Idempotency Key。Draft 可以保存空白、语法错误、未知字段或领域规则不完整的 UTF-8 Source，Validation 才负责给出结构化问题；原始 Source 不能包含 NUL（U+0000）。

当前没有 Draft List/Delete/Rebase。Draft/Validation/Plan 接口本身不会 Publish；P0-02a 另提供显式 [Module Release 发布事实](release-publishing.md)，P0-02b 只为数据身份完全不变的 compatible Release 提供 Activate。仍没有 Review/Migration 激活、Rollback、数据迁移、Outbox、Worker 或多节点收敛。

## 前置条件

- 使用 Server Profile，并已完成[本地环境](../getting-started/local-environment.md)。
- PostgreSQL 18.4、`PANVARA_PROJECT_ID` 和一个有效 Owner Credential 已配置；下面的首启示例使用 `PANVARA_ADMIN_TOKEN`。
- 默认 `crm.leads` 已启动，并能从 OpenAPI 读取 `x-panvara-revision`。
- 使用 `curl` 调用 API，使用 `jq` 检查 JSON。
- 先理解 [Revision Registry](revision-registry.md) 保存的是不可变事实，不是活动版本列表。

完整的非专业验收见 [Draft → Validate → Plan 完整验收](../getting-started/draft-plan-acceptance.md)。

## 最小示例

下面的请求以当前运行 Revision 为 Baseline，把故意包含重复 enum option 的 YAML 保存为 Draft：

```sh
BASE=http://127.0.0.1:8080
MODULE=crm.leads
BASELINE="$(curl -fsS "$BASE/api/core/v1alpha1/modules/$MODULE/openapi.json" \
  | jq -er '."x-panvara-revision"')"
IDEMPOTENCY_KEY="draft-$(openssl rand -hex 16)"

curl -fsS -X POST \
  "$BASE/api/admin/core/v1alpha1/modules/$MODULE/drafts?baseline_revision=$BASELINE" \
  -H "Authorization: Bearer $PANVARA_ADMIN_TOKEN" \
  -H 'Content-Type: application/yaml; charset=utf-8' \
  -H "Idempotency-Key: $IDEMPOTENCY_KEY" \
  --data-binary @examples/drafts/crm-leads-invalid.yaml | jq
```

这是 raw-body API：Body 是 YAML 本身，不要把 Source 塞进 JSON 字符串。完整验收页还会避免让 Token 出现在 curl 进程参数中，并演示如何提取 `draft_id`、`draft_version` 和 ETag。

## 配置

### API 与操作语义

| 操作 | 方法与路径 | 请求 Body | 并发或幂等条件 |
| --- | --- | --- | --- |
| Create Draft | `POST .../drafts?baseline_revision=<hash\|none>` | raw YAML/JSON | `Idempotency-Key` |
| Get Draft | `GET .../drafts/{draft}` | 无 | 无 |
| Get Source | `GET .../drafts/{draft}/source` | 无 | 返回可直接用于 PUT 的 Draft Version ETag |
| Replace Source | `PUT .../drafts/{draft}/source` | raw YAML/JSON | Draft Version `If-Match` |
| Validate | `POST .../drafts/{draft}/validations` | 无 | Draft Version `If-Match` |
| Get Validation | `GET .../validations/{validation}` | 无 | 无 |
| Plan | `POST .../drafts/{draft}/plans` | `{"validation_id":"..."}` | Draft Version `If-Match` |
| Get Plan | `GET .../plans/{plan}` | 无 | 无 |

Create 与 Replace 只接受 `application/yaml` 或 `application/json`，可附加 UTF-8 charset；不接受 Content-Encoding。Source 必须是有效、NUL-free 的 UTF-8，最多 1 MiB。原始 NUL 字节返回 400；JSON 文本里的 `\u0000` 不是原始 NUL，但若解码到 label 等禁止位置，Validation 会以 2xx + `valid=false` 报告作者错误。Source 其他部分是否能解码、编译或通过领域规则不属于保存前置条件。

`baseline_revision=none` 表示调用方明确让这次 Draft 不使用 Baseline，通常用于全新模块。Server 不会据此检查该 Module 是否已有 Revision；已有模块通常应传入要比较的精确 Revision Hash。不要省略参数，也不要从 Registry List 第一项猜测。

没有 Baseline 的 Plan 会返回 `module.baseline_absent`、`review_required`，因为系统无法判断兼容性、既有数据或迁移需求。此时 `requires_migration=false` 只表示“尚未判定必须迁移”，不能解读为“无需迁移”。

### 统一的 Draft Version ETag

Draft 元数据的强 ETag 是带双引号的 `draft_version`，例如 `"2"`。Replace、Validate 和 Plan 必须携带这个 ETag：

```json
{
  "draft_id": "0198...",
  "module": "crm.leads",
  "baseline_revision": "sha256:...",
  "draft_version": 2,
  "source_format": "yaml",
  "source_hash": "sha256:...",
  "created_by": "bootstrap-admin",
  "created_at": "2026-07-16T00:00:00Z",
  "updated_by": "bootstrap-admin",
  "updated_at": "2026-07-16T00:01:00Z"
}
```

```sh
curl -X PUT "$DRAFT_SOURCE_URL" \
  -H "Authorization: Bearer $PANVARA_ADMIN_TOKEN" \
  -H 'Content-Type: application/yaml' \
  -H 'If-Match: "2"' \
  --data-binary @examples/drafts/crm-leads-valid.yaml
```

GET Draft 元数据和 GET Draft Source 都返回同一个数字 Draft Version ETag，因此下载 Source 后可以直接把该 ETag 用于同一路径的 PUT。精确内容 Hash 通过 Draft JSON 的 `source_hash` 和 Source 响应的 `X-Panvara-Source-Hash` 返回；它不是并发条件。缺少 Draft ETag 返回 428；畸形参数返回 400；旧 ETag 对不同 Source 返回 412。冲突后必须重新 GET Source、人工检查并合并，不能自动覆盖。

相同格式和完全相同字节的 Replace 是 no-op，不递增 `draft_version`，也不改写更新时间。语义等价但字节不同的 Source 仍是一次真实编辑，会形成新的 Source Hash 与 Draft Version。

### Validation 响应

作者 Source 无效时，创建 Validation 本身仍成功：首次返回 201，重放同一 Draft Version 返回 200。响应中的 `valid` 为 false、`candidate_revision` 为 null，`violations` 使用稳定的 `code`、`stage`、`path`、`message` 和 `severity`：

```json
{
  "validation_id": "sha256:...",
  "validation_format": 1,
  "validation_hash": "sha256:...",
  "draft_id": "0198...",
  "draft_version": 1,
  "baseline_revision": "sha256:...",
  "source_format": "yaml",
  "source_hash": "sha256:...",
  "valid": false,
  "candidate_revision": null,
  "data_schema_identities": [],
  "violations": [
    {
      "code": "appmodule_invalid",
      "stage": "compile",
      "path": "/source",
      "message": "enum field ... has duplicate option ...",
      "severity": "error"
    }
  ],
  "warnings": [],
  "stale": false,
  "created_at": "2026-07-16T00:02:00Z",
  "effects": {
    "revision_registered": false,
    "published": false,
    "activated": false,
    "records_migrated": false,
    "runtime_changed": false
  }
}
```

Validation 通过时 `valid=true`、`violations=[]`，`candidate_revision` 是完整 Module Revision Hash，`data_schema_identities` 至少包含当前 `{format,fingerprint}`。HTTP 信封、Content-Type、权限、存储或运行故障仍使用非 2xx Error Envelope，不能伪装成作者模型 invalid。

### Change Plan 响应

Plan Body 只引用当前 Draft Version 中有效 Validation 的精确 ID：

```json
{"validation_id":"sha256:..."}
```

Plan 返回 Baseline、Candidate、稳定排序的 `changes`，以及包含 `classification`、`risk`、`change_count` 和 `risk_counts` 的 `summary`，并明确：

```json
{
  "plan_id": "sha256:...",
  "plan_format": 1,
  "plan_hash": "sha256:...",
  "validation_id": "sha256:...",
  "draft_id": "0198...",
  "draft_version": 2,
  "source_format": "yaml",
  "source_hash": "sha256:...",
  "baseline_revision": "sha256:...",
  "candidate_revision": "sha256:...",
  "baseline_data_schema_identities": [
    {"format": 1, "fingerprint": "sha256:..."}
  ],
  "candidate_data_schema_identities": [
    {"format": 1, "fingerprint": "sha256:..."}
  ],
  "summary": {
    "classification": "compatible",
    "risk": "low",
    "change_count": 2,
    "risk_counts": {"low": 2, "review": 0, "destructive": 0}
  },
  "changes": [
    {
      "code": "module.version_changed",
      "path": "/version",
      "kind": "change",
      "risk": "low",
      "impact": "runtime revision metadata changes",
      "requires_migration": false,
      "before": "\"1.0.0\"",
      "after": "\"1.1.0\""
    },
    {
      "code": "field.options_changed",
      "path": "/resources/lead/fields/stage/options",
      "kind": "change",
      "risk": "low",
      "impact": "new enum options expand accepted values",
      "requires_migration": false
    }
  ],
  "warnings": [],
  "blockers": [],
  "data_schema_changed": true,
  "record_namespace_changed": true,
  "migration_execution_supported": false,
  "stale": false,
  "created_at": "2026-07-16T00:03:00Z",
  "effects": {
    "plan_recorded": true,
    "revision_registered": false,
    "published": false,
    "activated": false,
    "records_migrated": false,
    "runtime_changed": false
  }
}
```

`plan_id` 是绑定 Project、Draft Version、Validation 与 Source 的精确快照 ID，`plan_hash` 是排除这些工作流元数据后的规范计划 Hash。不要假定二者相等：相同 Validation 的重放会同时复用原 ID/Hash；字节不同但语义等价的新 Draft Version 会产生新 `plan_id`，但保留相同 `plan_hash`。

- `migration_execution_supported=false`；
- `revision_registered=false`；
- `published=false`；
- `activated=false`；
- `records_migrated=false`；
- `runtime_changed=false`。

`data_schema_changed=false` 也不能推出“可以直接激活”。当前 Record namespace 仍按完整 Module Revision 隔离；Plan 只解释差异，不检查真实 Record，也不复制数据。

### Token 安全

bootstrap Token 具有当前项目 owner 权限。不要把它放在 URL、查询参数、JSON、Idempotency Key、Git 或共享日志中；不要对管理请求使用 `curl -v`、trace 或 `set -x`。完整验收使用标准输入把 Authorization Header 传给 curl，并以 `umask 077` 保护包含 Source 的临时文件。

## 验收

按 [Draft → Validate → Plan 完整验收](../getting-started/draft-plan-acceptance.md)操作，必须同时满足：

1. Baseline 来自当前运行 OpenAPI，并能在 Registry Detail 中精确读取。
2. invalid Fixture 可以保存；Validation 返回 2xx、`valid=false` 和结构化 violations。
3. 重放 Create 不新增 Draft；相同 Source Replace 不增加 `draft_version`。
4. 旧 ETag 不能覆盖不同 Source；修正后 Validation 返回 Candidate。
5. 同一 Draft Version 的 Validation 与 Plan 重放返回相同 ID 和规范 Hash。
6. Plan 能识别 Module Version 与 enum option 变化，并显式返回全部 false effects。
7. 操作前后 OpenAPI Revision、Registry Candidate 数量和业务 Record 快照不变。

工程门禁对应 `make test`、`make test-integration`、`make test-server-smoke` 和 `make docs-check`。

## 常见问题

### 为什么无效 YAML 也能创建 Draft？

Draft 是工作副本，不是可运行 Revision。保存只检查 Content-Type、NUL-free UTF-8 和 1 MiB 上限；作者错误由 Validation 转换成可定位 violations。

### `valid=true` 是否表示可以直接发布？

不是。它只说明这一代 Source 可以确定性编译。还必须为同一代 Validation 生成当前、非 stale 的 Plan，再显式调用 P0-02a Publish。即使 Publish 成功，也只会登记 Candidate 与 Release，不会激活或迁移数据。

### 怎样判断当前运行 Revision？

读取 OpenAPI 顶层 `x-panvara-revision`。不要使用 Registry List、Draft、Validation 或 Plan 的时间顺序推断。

### 为什么我得到 412？

另一个请求已经改变 Draft Version。重新 GET Source 取得最新数字 ETag 与 `X-Panvara-Source-Hash`，比较内容后人工合并；不要无条件重试覆盖。

### 为什么旧 Validation 或 Plan 显示 stale？

它们仍是不可变审计事实，但 Draft 已进入更高版本。只有当前 Draft Version 的有效 Validation 可以生成新 Plan。

### Plan 说 compatible 是否等于数据安全？

不是。`summary.classification` 是格式化差异结论，不是迁移或激活许可。没有稳定字段 ID 时 rename 会表示为 remove + add；Data Schema fingerprint 相同也不能证明无需迁移。

## 当前限制

- 只有 bootstrap `project.owner` 可以操作，没有账号体系和细粒度角色。
- 没有 Manager UI；当前交互方式是 raw-body HTTP、`curl` 和 `jq`。
- Draft Source 最多 1 MiB；没有 List、Delete、Rebase、配额或保留清理策略。
- Plan Format 1 不启发式识别 rename，也不执行或排队迁移。
- Candidate 不进入 Registry，不能被 Record Runtime 使用。
- Draft Workflow 没有自动 Publish；P0-02a/P0-02b 只有独立显式 Publish 与窄 compatible Activate，没有 Review/Migration 激活、Rollback、Outbox、Worker 或多节点收敛。

## 兼容与升级

Draft、Validation 和 Plan 分别携带 Draft Version、Source Hash、Validation/Plan Format 与 Candidate 身份；客户端不能只凭 Distribution 版本或时间戳判断它们可重放。格式算法变化必须新增 format version，不能重解释旧 Hash。

alpha.3b 不改变当前 Record namespace，也不修改 alpha.3a Registry 的不可变约束。P0-02a Publish 只能引用这里产生的不可变事实，并把 Candidate 与 Release 追加到权威存储；它不能把现有 Plan 重新解释为已激活或已迁移。P0-02b 只允许数据身份完全不变的显式 Activate；Migration 与 Rollback 仍需独立完成执行、验证和恢复协议。

架构决策见 [ADR-0003](../adr/0003-draft-validation-change-plan.md)。
