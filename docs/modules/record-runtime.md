<!--
    Panvara
    docs/modules/record-runtime.md    2026-07-14
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# Record Runtime 使用指南

## 用途

Record Runtime 把 AppModule 中声明的 Resource 变成可持久化的数据记录。它负责字段校验、UUIDv7 ID、唯一值、引用完整性、版本控制、分页和 PostgreSQL 事务。

用户不需要为每个 Resource 创建一张新表；alpha.2 将模型数据保存在固定的 flex JSONB 存储结构中。

## 当前状态

alpha.2 已实现：

- Create、Get、List、Patch 和软 Delete。
- Public/Admin 两套模型白名单。
- 11 种字段类型的校验和规范化。
- 唯一字段冲突检查。
- 同一项目、模块和 Revision 内的 Resource 引用。
- 使用 ETag/If-Match 的乐观并发控制。
- 基于 `created_at + record_id` 的稳定游标分页。
- 最多 16 个模型允许的标量等值过滤条件。

所有读写都限定在 `Project ID + Module + Resource + Revision Hash` 的完整 Scope 中。

## 前置条件

- Server Profile 已启动并且 `/readyz` 返回 200。
- PostgreSQL 18.4 可用。
- 已加载 [`crm-leads`](crm-leads.md) 或你自己的 AppModule。
- Admin 操作需要与 Server 相同的 `PANVARA_ADMIN_TOKEN`。
- 使用 `curl` 或任意能发送 JSON/HTTP Header 的工具。

## 最小示例

下面使用 `crm-leads` 创建一条 Organization。先在调用接口的终端设置：

```bash
export BASE_URL=http://127.0.0.1:8080
export PANVARA_ADMIN_TOKEN='<复制启动 Server 时使用的 Token>'
export RECORD_SUFFIX="$(date +%s)"
```

创建：

```bash
curl -i -X POST "$BASE_URL/api/admin/v1alpha1/crm.leads/organization" \
  -H "Authorization: Bearer $PANVARA_ADMIN_TOKEN" \
  -H 'Content-Type: application/json' \
  -d "{\"name\":\"My First Organization ${RECORD_SUFFIX}\"}"
```

成功时返回 `201 Created`、`ETag: "1"`，响应类似：

```json
{
  "id": "<organization-id>",
  "version": 1,
  "data": {"name": "My First Organization 17..."},
  "created_at": "<UTC time>",
  "updated_at": "<UTC time>"
}
```

复制响应中的 ID，然后修改名称：

```bash
curl -i -X PATCH "$BASE_URL/api/admin/v1alpha1/crm.leads/organization/<organization-id>" \
  -H "Authorization: Bearer $PANVARA_ADMIN_TOKEN" \
  -H 'Content-Type: application/json' \
  -H 'If-Match: "1"' \
  -d '{"name":"My Updated Organization"}'
```

成功时返回 `200 OK`、`ETag: "2"` 和 `version: 2`。

## 配置

Record Runtime 本身没有单独配置文件，它从 AppModule 和 Project Context 得到行为。

### 输入规则

| 类型 | 输入规则 | 保存前的处理 |
| --- | --- | --- |
| string/text | 不含 NUL（U+0000）的 JSON 字符串 | 保留原值，按 `maxLength` 校验 Unicode 字符数 |
| int | JSON 整数 | 必须在有符号 64 位范围内 |
| bool | `true`/`false` | 原样保存 |
| decimal | JSON 字符串 | 移除无意义的末尾零，例如 `"12.3400"` 变为 `"12.34"` |
| enum | JSON 字符串 | 必须在 `options` 中 |
| date | `YYYY-MM-DD` | 严格检查日期 |
| datetime | RFC 3339 字符串 | 转换为 UTC |
| email | 不含 NUL（U+0000）的 JSON 字符串 | 校验邮箱，并将域名部分转为小写 |
| money | `{"minor":整数,"currency":"USD"}` | 币种转为大写，不使用浮点金额 |
| reference | UUIDv7 字符串 | 必须指向当前 Scope 中存在的目标记录 |

### 系统字段

响应会在业务 `data` 外返回：

- `id`：Panvara 生成的 UUIDv7。
- `version`：从 1 开始，每次 Patch 增加 1。
- `created_at`、`updated_at`：UTC 时间。
- `deleted_at`：只在刚完成软删除的内部结果中存在；Delete HTTP 响应没有 Body。

这些系统字段不能出现在 Create/Patch 的 JSON Body 中。

### 分页与过滤

```bash
curl --get "$BASE_URL/api/admin/v1alpha1/crm.leads/lead" \
  -H "Authorization: Bearer $PANVARA_ADMIN_TOKEN" \
  --data-urlencode 'limit=20' \
  --data-urlencode 'filter[stage]=new'
```

如果响应包含 `next_cursor`，将它原样放入下一次请求的 `cursor` 参数。Cursor 与过滤条件绑定，改变过滤条件后必须从第一页重新查询。

## 验收

按顺序确认：

1. Create 返回 201、`Location`、`ETag: "1"` 和 UUIDv7 ID。
2. Get 返回同一条数据。
3. Patch 带当前 If-Match 时返回 200 和递增版本。
4. 重复使用旧 If-Match 时返回 412，不覆盖新数据。
5. List 默认最多返回 20 条；合法过滤只返回匹配数据。
6. 同一个唯一值再次创建时返回 409。
7. Delete 带当前 If-Match 时返回 204；随后 Get 返回 404。
8. Server 重启后，使用相同 Source、Project ID 和数据库仍能读到未删除记录。

## 常见问题

### 为什么 Patch 返回 428？

请求缺少 `If-Match`。先 Get 记录或读取上一次响应的 `ETag`，再把这个值原样发送。

### 为什么 Patch 返回 412？

记录已被其他请求修改，你使用的是旧 ETag。重新读取记录后再决定是否更新。

### 怎样把可选字段清空？

alpha.2 不支持 `null`，也没有字段 unset 操作。Patch 只能替换已提供的顶层字段。

### 为什么删除 Organization 返回 409？

仍有 Lead 引用它。先删除或修改这些 Lead，才能删除 Organization。

### 删除后唯一值能再次使用吗？

可以。软删除会释放该记录的唯一索引和向外引用，但记录事实仍保留在数据库中。

## 当前限制

- 没有批量写入、恢复软删除、硬删除或审计历史 API。
- Patch 只做顶层合并；嵌套对象会整体替换。
- 不支持 `null` 或移除字段。
- List 只支持升序稳定顺序，不能自定义排序。
- 过滤仅支持模型声明的标量等值匹配；text 和 money 不能过滤。
- 每页最多 100 条、最多 16 个过滤条件、单个过滤值最多 512 字节。
- JSON Body 最大 256 KiB。
- Reference 不能跨 Project、Module 或 Revision。
- 没有 Idempotency Key；客户端重试 Create 可能产生第二次请求或唯一冲突。

## 兼容与升级

Record 数据严格绑定 Revision Hash。相同 Source 会得到相同 Revision，并继续访问原数据；任何进入 Canonical IR 的变化都会切换到新的数据命名空间。

alpha.2 不会：

- 把旧 Revision 的 Record 复制到新 Revision。
- 重建新 Revision 的 unique/reference 索引。
- 自动激活、回滚或清理旧数据。

因此升级前必须保存旧 Source/Hash 并备份 PostgreSQL。显式迁移、发布与回滚属于 alpha.3。
