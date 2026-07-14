<!--
    Panvara
    docs/modules/http-api.md    2026-07-14
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

HTTP API 是 App、Website、后台或脚本访问 Panvara 的入口。alpha.2 提供匿名 Public Create、Bearer Token 保护的 Admin CRUD，以及运维和模块描述接口。本页说明 URL、认证、状态码与错误格式；业务旅程见 [CRM Leads](crm-leads.md)。

## 当前状态

| 类别 | 路径 | 认证 |
| --- | --- | --- |
| 运维 | `GET /healthz`、`/readyz`、`/version` | 无 |
| 模块生成物 | `GET /api/core/v1alpha1/modules/{module}/openapi.json`、`ui-schema.json` | 无 |
| Public Create | `POST /api/public/v1alpha1/{module}/{resource}` | 无 |
| Admin List/Create | `GET/POST /api/admin/v1alpha1/{module}/{resource}` | Bearer Token |
| Admin Get/Patch/Delete | `GET/PATCH/DELETE /api/admin/v1alpha1/{module}/{resource}/{id}` | Bearer Token |

操作是否可用仍由 AppModule allowlist 决定。路径存在不表示每个 Resource 都开放所有方法。

## 前置条件

- Lite 足以验收三个运维端点。
- 业务 API 需要 Server、PostgreSQL 和已编译 AppModule。
- Admin API 需要启动时使用的 `PANVARA_ADMIN_TOKEN`。
- POST/PATCH 必须使用 `Content-Type: application/json`。
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
| `Authorization: Bearer ...` | 所有 Admin 请求；Token 至少 32 字节 |
| `Content-Type: application/json` | POST、PATCH；其他类型返回 415 |
| `If-Match: "2"` | PATCH、DELETE；必须是最新强数字 ETag |
| `X-Request-ID` | 可选追踪 ID，合法值会在响应中回显 |
| `If-None-Match` | 模块生成物缓存命中时返回 304 |

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

常见状态码：400 参数/JSON 错误；401 Token 错误；403 模型未开放；404 不存在；409 数据冲突；412 ETag 过期；415 Content-Type 错误；422 字段校验失败；428 缺少 If-Match。

List 支持 `limit`（1–100，默认 20）、`cursor` 和已声明的 `filter[field]=value`：

```bash
curl --get http://127.0.0.1:8080/api/admin/v1alpha1/crm.leads/lead \
  -H "Authorization: Bearer $PANVARA_ADMIN_TOKEN" \
  --data-urlencode 'filter[stage]=new' \
  --data-urlencode 'limit=20'
```

## 验收

1. Lite 的三个运维端点分别返回 200。
2. Server 依赖就绪时 `/readyz` 返回 200；数据库不可用时返回 503。
3. OpenAPI 与 UI Schema 返回 200、JSON Content-Type 和 ETag。
4. 不带 Token 的 Admin 请求返回 401，并带 `WWW-Authenticate`。
5. 正确 Token 的已开放操作成功；未开放操作返回 403。
6. 错误字段返回 422，并包含 `request_id`。
7. Patch 不带 If-Match 返回 428；使用旧 ETag 返回 412。
8. 未声明路径返回 404。

## 常见问题

### `/healthz` 成功但 `/readyz` 失败意味着什么？

进程存活，但依赖尚未就绪。Server 通常要检查 PostgreSQL；Lite 正常启动后两者都会成功。

### 401 和 403 有什么区别？

401 表示没有通过认证；403 表示 AppModule 没开放该操作。

### 为什么能下载 UI Schema，却看不到网页？

它是给未来 Manager 前端使用的 JSON 描述。alpha.2 没有 Manager Web 应用。

### 怎样定位失败请求？

记录 `X-Request-ID` 或 Body 的 `request_id`。客户端也可发送自己的合法 `X-Request-ID`。

### 可以把 Admin API 直接暴露到公网吗？

不建议。alpha.2 只有临时 bootstrap Token，没有完整身份、细粒度授权、限流或生产级边缘安全。

## 当前限制

- 路径仍是 `v1alpha1` 实验契约；Public 仅支持 Create。
- Admin 是单个 bootstrap owner Token，不是账号体系。
- 没有 CORS 配置、内置 TLS、限流、Idempotency Key 或 Webhook。
- 没有 gRPC、GraphQL、批量 API 或流式响应。
- Body 最大 256 KiB；请求 Header 最大 1 MiB。
- 写超时 30 秒，不适合长任务。

## 兼容与升级

客户端应以当前模块生成的 OpenAPI 为准。Source 改变后，生成物 ETag 与 `x-panvara-revision` 会变化，Record API 也会访问新 Revision 的独立数据 Scope。

正式版本前不承诺 v1alpha1 长期兼容；破坏性变更必须同时更新本指南、生成契约和验收测试。
