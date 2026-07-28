<!--
    Panvara
    docs/reference http-api.md    2026-07-28
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# HTTP API

::: info Current Distribution
当前 Distribution 标识 **v0.1.0-alpha.2** 对应的 HTTP 能力提供运维端点、模块生成物、Public Create 与 Admin CRUD。每个 AppModule 生成的 OpenAPI 3.1 是当前业务 HTTP 契约的精确来源。
:::

默认地址是 `http://127.0.0.1:8080`。路径中的 `{module}` 与 `{resource}` 来自当前 AppModule。

## Current Distribution 端点

| 类别 | 方法与路径 | 认证 | 结果 |
| --- | --- | --- | --- |
| 存活 | `GET /healthz` | 无 | 进程存活时 200 |
| 就绪 | `GET /readyz` | 无 | 依赖就绪时 200，否则 503 |
| 版本 | `GET /version` | 无 | Distribution 与构建信息 |
| OpenAPI | `GET /api/core/v1alpha1/modules/{module}/openapi.json` | 无 | OpenAPI 3.1 |
| UI Schema | `GET /api/core/v1alpha1/modules/{module}/ui-schema.json` | 无 | Manager 界面描述 JSON |
| Public Create | `POST /api/public/v1alpha1/{module}/{resource}` | 无 | 模型允许时创建 Record |
| Admin List/Create | `GET/POST /api/admin/v1alpha1/{module}/{resource}` | Bearer | 列表或创建 |
| Admin Get/Patch/Delete | `GET/PATCH/DELETE /api/admin/v1alpha1/{module}/{resource}/{id}` | Bearer | 单条读取、修改或软删除 |

路径存在不代表每个 Resource 都开放所有方法；最终以 AppModule allowlist 与生成 OpenAPI 为准。

## 最小请求

```sh
BASE_URL=http://127.0.0.1:8080

curl -i "$BASE_URL/healthz"
curl -i "$BASE_URL/readyz"
curl -i "$BASE_URL/version"
curl -i "$BASE_URL/api/core/v1alpha1/modules/crm.leads/openapi.json"
```

Admin List：

```sh
curl -i \
  "$BASE_URL/api/admin/v1alpha1/crm.leads/organization" \
  -H "Authorization: Bearer $PANVARA_ADMIN_TOKEN"
```

Public Create：

```sh
curl -i -X POST \
  "$BASE_URL/api/public/v1alpha1/crm.leads/lead" \
  -H 'Content-Type: application/json' \
  -d '{
    "organization":"<organization-uuidv7>",
    "email":"first@example.com",
    "stage":"new"
  }'
```

## 请求头

| Header | 何时使用 |
| --- | --- |
| `Authorization: Bearer ...` | 所有 Admin 请求 |
| `Content-Type: application/json` | Record Create/Patch |
| `If-Match: "2"` | Patch/Delete；必须是当前强 ETag |
| `X-Request-ID` | 可选追踪 ID；合法值会在响应中回显 |
| `If-None-Match` | 缓存模块生成物；命中时返回 304 |

真实 Credential 不应放入 URL、提交到脚本，或作为可被同机进程看到的 CLI Flag。

## Record 响应

```json
{
  "id": "0198...",
  "version": 1,
  "data": {"name": "Example"},
  "created_at": "2026-07-14T00:00:00Z",
  "updated_at": "2026-07-14T00:00:00Z"
}
```

Create 返回 `201`、`Location` 与 `ETag: "1"`。Get/Patch 返回当前 ETag。Delete 成功返回 `204`，没有 Body。

## List、分页与过滤

```sh
curl -fsS --get \
  "$BASE_URL/api/admin/v1alpha1/crm.leads/lead" \
  -H "Authorization: Bearer $PANVARA_ADMIN_TOKEN" \
  --data-urlencode 'limit=20' \
  --data-urlencode 'filter[stage]=new'
```

- `limit` 为 1–100，默认 20；
- 只能使用模型声明的 `filterable` 字段；
- 响应有 `next_cursor` 时，将它原样传给下一次请求的 `cursor`；
- Cursor 与过滤条件绑定，不要跨查询复用。

## 错误格式

```json
{
  "code": "validation_failed",
  "message": "record validation failed",
  "request_id": "req_...",
  "details": [
    {"code":"required","path":"$.email","message":"required field is missing"}
  ]
}
```

| 状态 | 含义 |
| --- | --- |
| 400 | JSON、参数或请求格式错误 |
| 401 | Credential 缺失、错误或已撤销 |
| 403 | 已认证但未授权，或模型未开放操作 |
| 404 | 路径或 Record 不存在 |
| 409 | 唯一值、引用或生命周期冲突 |
| 412 | `If-Match` 已过期 |
| 415 | Content-Type 不支持 |
| 422 | 字段校验失败 |
| 428 | 缺少 `If-Match` |
| 503 | Server 依赖或权威访问状态不可用 |

记录 `X-Request-ID` 或 Body 的 `request_id`，但不要记录 Authorization Header。

## Source Preview 端点

::: warning Source Preview
固定源码快照还包含 Revision、Draft/Validation/Plan、Publish Facts 与访问管理 Admin API。它们不属于 **v0.1.0-alpha.2 Distribution 能力边界**，不适合生产，并且没有 Activate、Rollback、数据升级、登录或完整权限系统。只按[访问管理预览](/guides/access-preview)与[模型变更预览](/guides/model-change-preview)操作。
:::

| 预览类别 | 路径前缀 |
| --- | --- |
| Revision | `/api/admin/core/v1alpha1/modules/{module}/revisions` |
| Draft / Validation / Plan | `/api/admin/core/v1alpha1/modules/{module}/drafts` |
| Publish Facts | `/api/admin/core/v1alpha1/modules/{module}/releases` |
| Principal / Credential / Grant | `/api/admin/core/v1alpha1/access` |

不要因为路径存在就把预览契约生成到生产客户端。

## 当前限制

HTTP 路径仍是 `v1alpha1`；没有 CORS 配置、内置 TLS、限流、Webhook、GraphQL、gRPC、批量 API 或流式响应。Admin Credential 不等于用户登录体系。客户端升级前必须重新获取当前模块 OpenAPI。
