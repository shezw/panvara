<!--
    Panvara
    docs/guides records.md    2026-07-28
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# 操作 Record

::: info Current Distribution
当前 Distribution 标识 **v0.1.0-alpha.2** 对应的 Record 能力提供 Create、Get、List、Patch 和软 Delete，以及字段校验、唯一值、引用完整性、稳定分页和 ETag 并发控制。
:::

以下示例使用正在运行的 `crm.leads` Server：

```sh
export BASE_URL=http://127.0.0.1:8080
set -a; . ./.env; . ./.env.local; set +a
```

## Create 与 Get

```sh
SUFFIX="$(date +%s)"

RECORD_ID="$(curl -fsS -X POST \
  "$BASE_URL/api/admin/v1alpha1/crm.leads/organization" \
  -H "Authorization: Bearer $PANVARA_ADMIN_TOKEN" \
  -H 'Content-Type: application/json' \
  -d "{\"name\":\"Record Example ${SUFFIX}\"}" \
  | jq -er '.id')"

curl -i \
  "$BASE_URL/api/admin/v1alpha1/crm.leads/organization/$RECORD_ID" \
  -H "Authorization: Bearer $PANVARA_ADMIN_TOKEN"
```

Create 返回 `201`、`Location` 与 `ETag: "1"`。Get 返回 `200` 和同一数据。

## Patch 与乐观并发

读取当前 ETag，然后更新：

```sh
ETAG="$(curl -fsS -D - -o /dev/null \
  "$BASE_URL/api/admin/v1alpha1/crm.leads/organization/$RECORD_ID" \
  -H "Authorization: Bearer $PANVARA_ADMIN_TOKEN" \
  | awk 'tolower($1) == "etag:" {gsub(/\r/, "", $2); print $2}')"

curl -i -X PATCH \
  "$BASE_URL/api/admin/v1alpha1/crm.leads/organization/$RECORD_ID" \
  -H "Authorization: Bearer $PANVARA_ADMIN_TOKEN" \
  -H 'Content-Type: application/json' \
  -H "If-Match: $ETAG" \
  -d "{\"name\":\"Record Updated ${SUFFIX}\"}"
```

成功返回 `200`、递增后的 `version` 和新 ETag。缺少 `If-Match` 返回 `428`；另一个请求已更新 Record 时，旧 ETag 返回 `412`。遇到 `412` 应重新 Get、比较变化，再决定是否重试。

## List、分页与过滤

```sh
curl -fsS --get \
  "$BASE_URL/api/admin/v1alpha1/crm.leads/lead" \
  -H "Authorization: Bearer $PANVARA_ADMIN_TOKEN" \
  --data-urlencode 'limit=20' \
  --data-urlencode 'filter[stage]=new'
```

- `limit` 范围是 1–100，默认 20；
- 只能过滤 AppModule `filterable` 声明的标量字段；
- 最多 16 个过滤条件；
- 若响应包含 `next_cursor`，下一页把它原样作为 `cursor`；
- Cursor 与过滤条件绑定，改变过滤后要从第一页重新开始。

## 软删除

删除也需要最新 ETag：

```sh
ETAG="$(curl -fsS -D - -o /dev/null \
  "$BASE_URL/api/admin/v1alpha1/crm.leads/organization/$RECORD_ID" \
  -H "Authorization: Bearer $PANVARA_ADMIN_TOKEN" \
  | awk 'tolower($1) == "etag:" {gsub(/\r/, "", $2); print $2}')"

curl -i -X DELETE \
  "$BASE_URL/api/admin/v1alpha1/crm.leads/organization/$RECORD_ID" \
  -H "Authorization: Bearer $PANVARA_ADMIN_TOKEN" \
  -H "If-Match: $ETAG"
```

成功返回 `204 No Content`，随后 Get 返回 `404`。被其他 Record 引用时返回 `409 record_referenced`。软删除会释放唯一值，但当前没有恢复或硬删除 API。

## 输入与错误

| 状态 | 含义 | 客户端处理 |
| --- | --- | --- |
| 400 | JSON、参数或请求格式错误 | 修正请求，不原样重试 |
| 401 | Credential 缺失、错误或已撤销 | 更换有效 Credential |
| 403 | 已认证但未授权，或模型未开放操作 | 检查权限与 AppModule |
| 409 | 唯一值、引用或生命周期冲突 | 根据 `code` 处理 |
| 412 | ETag 已过期 | 重新读取并合并 |
| 422 | 字段校验失败 | 按 `details` 修正字段 |
| 428 | 缺少 `If-Match` | 先读取当前 ETag |

`decimal` 使用字符串，`money` 使用最小货币单位对象，`datetime` 会规范为 UTC，`reference` 必须指向当前数据 Scope 中存在的 Record。Create/Patch JSON 最大 256 KiB。

## 当前限制

没有批量写入、恢复、硬删除、审计历史、自定义排序、模糊搜索或通用 Idempotency Key；Patch 只合并顶层字段，不支持 `null` 或 unset。Record 严格绑定模型 Revision，模型改变不会自动复制或升级旧数据。
