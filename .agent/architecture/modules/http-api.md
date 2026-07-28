<!--
    Panvara
    docs/modules/http-api.md    2026-07-15
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# HTTP API 使用指南

## 用途

HTTP API 是 App、Website、后台或脚本访问 Panvara 的入口。alpha.2 提供 Public/Create 与 Admin CRUD；alpha.3a/3b 增加 Registry、Draft、Validation 与 Plan；P0-01b 增加 project-local Principal/Credential/Grant 管理。所有新旧 Admin 用例统一经 `Credential → Principal → project.owner Grant` 授权。本页说明 URL、认证、状态码与错误格式；业务旅程见 [CRM Leads](crm-leads.md)。

## 当前状态

| 类别 | 路径 | 认证 |
| --- | --- | --- |
| 运维 | `GET /healthz`、`/readyz`、`/version` | 无 |
| 模块生成物 | `GET /api/core/v1alpha1/modules/{module}/openapi.json`、`ui-schema.json` | 无 |
| Revision List | `GET /api/admin/core/v1alpha1/modules/{module}/revisions` | Bearer Token |
| Revision Detail/Source | `GET /api/admin/core/v1alpha1/modules/{module}/revisions/{revision}`、`.../{revision}/source` | Bearer Token |
| Draft Metadata/Source | `POST .../modules/{module}/drafts`、`GET .../drafts/{draft}`、`GET/PUT .../drafts/{draft}/source` | Bearer Token |
| Validation/Plan | `POST/GET .../drafts/{draft}/validations[/validation]`、`POST/GET .../drafts/{draft}/plans[/plan]` | Bearer Token |
| Principal | `GET/POST /api/admin/core/v1alpha1/access/principals`、`POST .../principals/{principal}/disable` | Owner Credential |
| Credential | `GET/POST .../principals/{principal}/credentials`、`POST .../credentials/{credential}/revoke` | Owner Credential |
| Grant | `GET .../principals/{principal}/grants`、`PUT/DELETE .../grants/project.owner` | Owner Credential |
| Public Create | `POST /api/public/v1alpha1/{module}/{resource}` | 无 |
| Admin List/Create | `GET/POST /api/admin/v1alpha1/{module}/{resource}` | Bearer Token |
| Admin Get/Patch/Delete | `GET/PATCH/DELETE /api/admin/v1alpha1/{module}/{resource}/{id}` | Bearer Token |

操作是否可用仍由 AppModule allowlist 决定。路径存在不表示每个 Resource 都开放所有方法。

## 前置条件

- Lite 足以验收三个运维端点。
- 业务 API 需要 Server、PostgreSQL 和已编译 AppModule。
- Admin API 需要 active Credential 对应 active project-local Principal，并拥有当前 Project/Environment 的 active `project.owner` Grant；可以是 bootstrap 或新签发 Credential。
- Registry、Draft Planning、Record Admin 与 Access Administration 都使用同一授权链；接口不接受 Project 查询参数。
- Record POST/PATCH 和 Plan POST 使用 `Content-Type: application/json`；Draft Create/Replace 的 Body 是原始 YAML/JSON，使用 `application/yaml` 或 `application/json`。
- Patch/Delete 需要最近一次 Record 响应中的 ETag。

## 最小示例

```bash
curl -i http://127.0.0.1:8080/healthz
curl -i http://127.0.0.1:8080/readyz
curl -i http://127.0.0.1:8080/version
curl -i http://127.0.0.1:8080/api/core/v1alpha1/modules/crm.leads/openapi.json
```

前三个请求成功时返回 200；`healthz` 与 `readyz` 的 Body 分别包含 `{"status":"alive"}` 和 `{"status":"ready"}`。

查询 Admin 列表：

```bash
curl -i http://127.0.0.1:8080/api/admin/v1alpha1/crm.leads/organization \
  -H "Authorization: Bearer $PANVARA_ADMIN_TOKEN"
```

## 配置

### 常用请求头

| Header | 用途 |
| --- | --- |
| `Authorization: Bearer ...` | 所有 Admin 请求；bootstrap Token 为 32–1024 字节、仅含 Bearer 安全 ASCII、非 `pvk1.` 前缀；新签发 Token 使用 `pvk1.` |
| `Content-Type` | Record/Plan JSON；Draft Source 使用 `application/json` 或 `application/yaml` |
| `If-Match: "2"` | Record Patch/Delete 或 Draft Replace/Validate/Plan；必须是最新强数字 ETag |
| `Idempotency-Key` | alpha.3b Create Draft 必需；同项目、同模块、同 Key 且 Baseline、Source Format、Source 字节全部相同时安全重放 |
| `X-Request-ID` | 可选追踪 ID，合法值会在响应中回显 |
| `If-None-Match` | 模块生成物或 Registry Source 缓存命中时返回 304 |

Record 响应形态：

```json
{
  "id": "0198...",
  "version": 1,
  "data": {"name": "Example"},
  "created_at": "2026-07-14T00:00:00Z",
  "updated_at": "2026-07-14T00:00:00Z"
}
```

Record `ETag` 是带双引号的版本，例如 `"1"`。生成物 ETag 是其内容 SHA-256，不等于模块 Revision Hash。

错误响应统一包含：

```json
{
  "code": "validation_failed",
  "message": "record validation failed",
  "request_id": "req_...",
  "details": [{"code":"required","path":"$.email","message":"required field is missing"}]
}
```

常见状态码：400 参数/JSON 错误；401 Credential 缺失/错误/撤销；403 已认证但缺 Grant、Scope inactive 或模型未开放；404 不存在；409 生命周期冲突或最后 Owner path；412 ETag 过期；415 Content-Type 错误；422 字段校验失败；428 缺少 If-Match；503 权威认证/授权状态不可用。

List 支持 `limit`（1–100，默认 20）、`cursor` 和已声明的 `filter[field]=value`：

```bash
curl --get http://127.0.0.1:8080/api/admin/v1alpha1/crm.leads/lead \
  -H "Authorization: Bearer $PANVARA_ADMIN_TOKEN" \
  --data-urlencode 'filter[stage]=new' \
  --data-urlencode 'limit=20'
```

### Project-local 访问管理

访问管理路径不接受查询参数；Create Principal Body 只接受 `{"display_name":"..."}`，Issue Credential Body 只接受 `{"label":"..."}`，其余 mutation 要求空 Body。所有响应使用 `private, no-store`；签发 Credential 的 201 响应会把原始 `token` 返回且仅返回一次，并额外使用 `Pragma: no-cache`。

```sh
BASE=http://127.0.0.1:8080

curl -fsS "$BASE/api/admin/core/v1alpha1/access/principals" \
  -H "Authorization: Bearer $PANVARA_ADMIN_TOKEN" \
  | jq '.data'
```

固定契约如下：

- `GET/POST /api/admin/core/v1alpha1/access/principals`
- `POST /api/admin/core/v1alpha1/access/principals/{principal}/disable`
- `GET/POST /api/admin/core/v1alpha1/access/principals/{principal}/credentials`
- `POST /api/admin/core/v1alpha1/access/credentials/{credential}/revoke`
- `GET /api/admin/core/v1alpha1/access/principals/{principal}/grants`
- `PUT/DELETE /api/admin/core/v1alpha1/access/principals/{principal}/grants/project.owner`

Principal disable 与 Credential revoke 是不可恢复终态。Grant 可由另一个有效 Owner Credential 显式 PUT 重新授予，但 restart/bootstrap 不会自动恢复。完整的、不输出 Secret 的 `curl`/`jq` 旅程见[访问管理指南](access-administration.md#验收)。

### Revision Registry 读取

::: danger 登记不等于发布或激活
Registry List 没有 active 语义。当前运行 Revision 必须从 OpenAPI 顶层 `x-panvara-revision` 读取，不能使用 `.data[0]`。
:::

Registry List 与 Record List 是两个不同契约。Registry List 只接受一个 `limit`（1–100，默认 20），不支持 cursor、filter 或其他查询参数：

```sh
curl -fsS \
  'http://127.0.0.1:8080/api/admin/core/v1alpha1/modules/crm.leads/revisions?limit=100' \
  -H "Authorization: Bearer $PANVARA_ADMIN_TOKEN"
```

List 的 `data` 元素与 Detail 都使用且仅使用以下字段：

```json
{
  "module": "crm.leads",
  "revision": "sha256:...",
  "module_version": "1.0.0",
  "data_schema_identities": [
    {"format": 1, "fingerprint": "sha256:..."}
  ],
  "spec_version": "panvara.dev/v1alpha1",
  "ir_format": 1,
  "source_format": "yaml",
  "source_hash": "sha256:...",
  "origin": "bootstrap",
  "registered_by": "system:bootstrap",
  "registered_at": "2026-07-15T00:00:00Z"
}
```

`data_schema_identities` 按 format 升序，每个 format 最多一项。消费者必须按 format 查找，例如：

```sh
jq -er '.data_schema_identities[] | select(.format == 1) | .fingerprint'
```

父 Module Revision 保存 Source、IR、生成物和首次登记信息；新投影算法可以追加新的 Data Schema Identity，但不能改变父 Revision Hash 或已有 format。List 与 Detail 使用 `Cache-Control: private, no-store`。

Source 返回第一次登记的原始字节。YAML 使用 `application/yaml; charset=utf-8`，JSON 使用 `application/json; charset=utf-8`；强 ETag 精确为带双引号的 `source_hash`，并带 `Cache-Control: private, no-cache`。带相同 `If-None-Match` 时返回 304。

### Draft、Validation 与 Plan

::: danger 这些接口不执行发布
Draft、Validation 和 Plan 不登记 Candidate，不发布、不激活、不迁移 Record，也不改变当前运行 OpenAPI Revision。
:::

Create Draft 把 Baseline 放在查询参数中，Body 直接传原始 Source：

```sh
BASELINE="$(curl -fsS \
  http://127.0.0.1:8080/api/core/v1alpha1/modules/crm.leads/openapi.json \
  | jq -er '."x-panvara-revision"')"

curl -X POST \
  "http://127.0.0.1:8080/api/admin/core/v1alpha1/modules/crm.leads/drafts?baseline_revision=$BASELINE" \
  -H "Authorization: Bearer $PANVARA_ADMIN_TOKEN" \
  -H 'Content-Type: application/yaml; charset=utf-8' \
  -H 'Idempotency-Key: draft-client-request-001' \
  --data-binary @examples/drafts/crm-leads-invalid.yaml
```

首次创建返回 201；同 Key、同 Baseline、同格式和同字节重放返回 200 与原 Draft；同 Key 对应不同请求返回 409。Draft 可以保存不能编译的 Source。`baseline_revision=none` 表示调用方明确不提供 Baseline，通常用于全新模块；Server 不检查模块是否已有 Revision，也不得从 Registry List 自动选择 Baseline。无 Baseline Plan 至少返回 `review_required`，不能宣称兼容或无需迁移。

Draft 元数据返回 `draft_id`、`module`、`baseline_revision`、`draft_version`、`source_format`、`source_hash` 和创建/更新审计。GET 元数据与 GET Source 都使用 Draft Version 强 ETag，例如 `"1"`；Source 响应另用 `X-Panvara-Source-Hash` 返回精确内容 Hash。GET Source 的 ETag 可以直接用于同路径 PUT。

Replace、Validate 和 Plan 都要求 `If-Match`。Replace 使用 `PUT .../source` 与 raw Source Body；相同格式和字节是 no-op，不增加 `draft_version`。Validation 使用 `POST .../validations` 且没有 Body；作者 Source 无效时仍返回 201/200 和 `valid=false`、结构化 `violations`，而不是 422。通过时 `candidate_revision` 与 `data_schema_identities` 描述 Candidate，但不会写入 Registry。

Plan 使用 JSON Body 引用当前 Draft Version 中有效 Validation：

```json
{"validation_id":"sha256:..."}
```

首次生成返回 201，重放相同 Validation 返回 200 与原 Plan。`plan_id` 精确绑定 Project、Draft Version、Validation 和 Source；`plan_hash` 只绑定规范计划语义，二者不能混用。语义等价的新 Draft Version 会得到不同 `plan_id`、相同 `plan_hash`。响应还包含 Baseline/Candidate、`changes`、`summary`、`data_schema_changed`、`record_namespace_changed` 与 `migration_execution_supported=false`。Validation/Plan 必须显式返回 false effects，证明没有登记、发布、激活、迁移或 Runtime 切换。

所有 Draft 元数据、Validation、Plan 和 Draft Source 使用 `Cache-Control: private, no-store`。完整命令、Token 安全与冲突恢复见 [Draft → Validate → Plan 完整验收](../getting-started/draft-plan-acceptance.md)。

### Module Release Publish 与 Detail

Publish 接受一个已持久化 `plan_id`，并要求发布专用 Idempotency Key：

```sh
curl -X POST \
  http://127.0.0.1:8080/api/admin/core/v1alpha1/modules/crm.leads/releases \
  -H "Authorization: Bearer $PANVARA_ADMIN_TOKEN" \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: publish-client-request-001' \
  --data '{"plan_id":"sha256:..."}'
```

首次 Publish 返回 201 与 `Location`；同 Key/同 Plan 或同 Plan/新 Key 返回 200 和同一 Release，即使 Draft 后来已使 Plan stale。相同 Key 对应不同意图会在 Plan 查找前返回 409。请求不接受查询参数，Body 只接受原始字节中精确 ASCII `plan_id` 字段，`\u` 转义的字段名也会被拒绝。

Detail 使用：

```sh
curl -fsS \
  http://127.0.0.1:8080/api/admin/core/v1alpha1/modules/crm.leads/releases/019f... \
  -H "Authorization: Bearer $PANVARA_ADMIN_TOKEN"
```

Publish 先在短事务中二次授权并解析已绑定 Key 或已发布 Plan；未命中时才重新核对当前 Draft generation、有效 Validation、非 stale Plan，并复编译精确 Source。首次成功后 Candidate Revision 与 Module Release 在同一数据库事务写入。响应必须同时返回 `published=true`、`activated=false`、`records_migrated=false`、`runtime_changed=false` 与 `activation_supported=false`。所有 Release 响应使用 `Cache-Control: private, no-store`。完整验收见 [Draft → Publish 完整验收](../getting-started/draft-publish-acceptance.md)。

## 验收

1. Lite 的三个运维端点分别返回 200。
2. Server 依赖就绪时 `/readyz` 返回 200；数据库不可用时返回 503。
3. OpenAPI 与 UI Schema 返回 200、JSON Content-Type 和 ETag。
4. 不带 Token 的 Admin 请求返回 401，并带 `WWW-Authenticate`。
5. 正确 Token 的已开放操作成功；未开放操作返回 403。
6. 错误字段返回 422，并包含 `request_id`。
7. Patch 不带 If-Match 返回 428；使用旧 ETag 返回 412。
8. 未声明路径返回 404。
9. Registry 不带 Token 返回 401；List/Detail/Source 正常读取；不存在的 Revision 返回 404。
10. 对 Revision Detail 发送 DELETE 返回 405 且 `Allow: GET`。
11. Registry List/Detail 返回 `private, no-store`；Source 返回 `private, no-cache`。
12. Draft Create 首次 201、同请求重放 200；同 Source Replace 不增加 `draft_version`，旧 ETag 返回 412。
13. 无效 Validation 返回 2xx + `valid=false`；有效 Validation 与 Plan 重放保持同一 ID/Hash。
14. Plan 后 OpenAPI Revision、Registry Candidate 与业务 Record 均不改变，全部执行 effects 为 false。
15. Publish 首次返回 201；Draft stale 与进程重启后，同 Key 或同 Plan 新 Key 重放仍返回 200 和同一 Release；同 Key/异 Plan 优先返回 409。
16. Publish 后 Candidate 可从 Registry 读取，但 OpenAPI Revision 与 Record 在发布前后及重启后保持不变。

## 常见问题

### `/healthz` 成功但 `/readyz` 失败意味着什么？

进程存活，但依赖尚未就绪。Server 通常要检查 PostgreSQL；Lite 正常启动后两者都会成功。

### 401 和 403 有什么区别？

401 表示没有通过 Credential 认证；403 表示已认证，但当前 Scope 缺少 active Owner Grant，或 AppModule 没开放该操作。认证/授权权威状态读失败返回 503。

### 为什么能下载 UI Schema，却看不到网页？

它是给未来 Manager 前端使用的 JSON 描述。alpha.2 没有 Manager Web 应用。

### 怎样确认当前运行的是哪个 Registry Revision？

读取 `/api/core/v1alpha1/modules/{module}/openapi.json` 的 `x-panvara-revision`，再按该值请求 Detail。不要假设 List 第一条是当前版本。

### 怎样定位失败请求？

记录 `X-Request-ID` 或 Body 的 `request_id`。客户端也可发送自己的合法 `X-Request-ID`。

### 可以把 Admin API 直接暴露到公网吗？

不建议。P0-01b 虽有 Service Principal/API Credential 与固定 Owner Grant，但仍没有 Account/Session/MFA、细粒度动态授权、限流、TLS 终止或生产级边缘安全。

### Validation 返回 201 但 `valid=false` 是失败吗？

Validation 请求成功，作者 Source 没有通过。读取 `violations` 修正 Source，再用最新 Draft ETag Replace 和 Validate。存储或运行故障才返回非 2xx Error Envelope。

### Plan 的 `compatible` 表示可以激活吗？

不表示。Plan 只解释结构变化。P0-02a 可以把它发布成不可变事实，但 `activation_supported=false`；当前 Record namespace 仍按完整 Module Revision 隔离，也没有 Activate 或迁移执行。

### `published=true` 表示线上已经切换吗？

不表示。请读取同一响应的 `activated=false` 与 `runtime_changed=false`。当前运行 Revision 仍以 OpenAPI 顶层 `x-panvara-revision` 为准。

## 当前限制

- 路径仍是 `v1alpha1` 实验契约；Public 仅支持 Create。
- Admin 支持 bootstrap/Service Principal Credential，但不是 Account/Session 身份体系。
- 没有 CORS 配置、内置 TLS、限流或 Webhook；Idempotency Key 当前只覆盖 Create Draft 与 Module Release Publish 的专用事实，不是通用 HTTP 中间件。
- 没有 gRPC、GraphQL、批量 API 或流式响应。
- Record Body 最大 256 KiB；Draft Source 最大 1 MiB；请求 Header 最大 1 MiB。
- 写超时 30 秒，不适合长任务。
- Registry 只有 owner 只读 API，没有 cursor、通用写入 API 或活动指针；Candidate 只由 Publish 用例登记。
- Draft Planning 没有 List/Delete/Rebase；P0-02a 只有 Publish/Detail，没有 Activate/Rollback、数据迁移或活动指针。

## 兼容与升级

客户端应以当前模块生成的 OpenAPI 为准。完整 Canonical IR 改变后，生成物 ETag 与 `x-panvara-revision` 会变化，Record API 也会访问新 Revision 的独立数据 Scope。Registry 中同一 format 的 fingerprint 相同或存在 Published Release 都不会自动迁移或激活数据；客户端不能依赖 `data_schema_identities[0]`。Draft Version、Validation Format、Plan Format 与 Release ID 是独立轴，不能只按时间选择“最新结果”。

正式版本前不承诺 v1alpha1 长期兼容；破坏性变更必须同时更新本指南、生成契约和验收测试。
