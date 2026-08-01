<!--
    Panvara
    docs/guides crm-leads.md    2026-07-28
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# 运行 CRM Leads 示例

::: info Current Distribution
CRM Leads 是当前 Distribution 标识 **v0.1.0-alpha.2** 对应的参考模块，用来验证模型声明、Public Create、Admin CRUD、关系约束、生成契约和 PostgreSQL 持久化。它不是完整 CRM 产品。
:::

## 模型内容

| Resource | 主要字段 | Public | Admin |
| --- | --- | --- | --- |
| `organization` | 唯一且必填的 `name` | 无 | List/Get/Create/Patch/Delete |
| `lead` | `organization`、唯一 `email`、`stage`、可选 `score` | Create | List/Get/Create/Patch/Delete |

`lead.organization` 必须引用现有 Organization；`stage` 只能是 `new`、`qualified` 或 `won`。

## 1. 启动 Server

先按[运行 Server](./server)完成本地配置。在终端 A：

```sh
set -a; . ./.env; . ./.env.local; set +a
make infra-up
make run-server
```

保持 Server 运行。

## 2. 创建 Organization

终端 B 需要 `curl` 和 `jq`。在仓库根目录加载同一配置：

```sh
set -eu
set -a; . ./.env; . ./.env.local; set +a

BASE_URL=http://127.0.0.1:8080
DEMO_SUFFIX="$(date +%s)"

curl -fsS "$BASE_URL/readyz" | jq -e '.status == "ready"'

ORGANIZATION_ID="$(curl -fsS -X POST \
  "$BASE_URL/api/admin/v1alpha1/crm.leads/organization" \
  -H "Authorization: Bearer $PANVARA_ADMIN_TOKEN" \
  -H 'Content-Type: application/json' \
  -d "{\"name\":\"Panvara Demo ${DEMO_SUFFIX}\"}" \
  | jq -er '.id')"

printf 'organization=%s\n' "$ORGANIZATION_ID"
```

Create 成功状态是 `201 Created`，响应带 `ETag: "1"`；上面的 `-f` 会在 HTTP 错误时停止。

## 3. 通过 Public API 创建 Lead

```sh
LEAD_EMAIL="first-${DEMO_SUFFIX}@example.com"

LEAD_ID="$(curl -fsS -X POST \
  "$BASE_URL/api/public/v1alpha1/crm.leads/lead" \
  -H 'Content-Type: application/json' \
  -d "{\"organization\":\"${ORGANIZATION_ID}\",\"email\":\"${LEAD_EMAIL}\",\"stage\":\"new\"}" \
  | jq -er '.id')"

printf 'lead=%s\n' "$LEAD_ID"
```

Public Create 不需要 Admin Token。模型不会允许 Public 写入 `score`。

## 4. 查询并检查生成物

```sh
curl -fsS --get \
  "$BASE_URL/api/admin/v1alpha1/crm.leads/lead" \
  -H "Authorization: Bearer $PANVARA_ADMIN_TOKEN" \
  --data-urlencode 'filter[stage]=new' \
  | jq -e --arg email "$LEAD_EMAIL" '.data | any(.data.email == $email)'

curl -fsS \
  "$BASE_URL/api/core/v1alpha1/modules/crm.leads/openapi.json" \
  | jq -e '.openapi == "3.1.0"'

curl -fsS \
  "$BASE_URL/api/core/v1alpha1/modules/crm.leads/ui-schema.json" \
  | jq -e '.'
```

三条命令均应输出真值或有效 JSON。

## 5. 验证重启持久化

在终端 A 按 `Ctrl+C`，保持 PostgreSQL 运行，再执行：

```sh
make run-server
```

终端 B 重新查询：

```sh
curl -fsS \
  "$BASE_URL/api/admin/v1alpha1/crm.leads/lead/$LEAD_ID" \
  -H "Authorization: Bearer $PANVARA_ADMIN_TOKEN" \
  | jq -e --arg email "$LEAD_EMAIL" '.data.email == $email'
```

返回 `true` 才表示数据跨 Server 重启保留。

## 常见结果

- 相同 Organization 名称或 Lead email 再次创建：`409 unique_conflict`；
- 不存在的 Organization ID：字段或引用校验失败；
- 有 Lead 引用时删除 Organization：`409 record_referenced`；
- 错误 Admin Credential：`401`；
- 没有被模型开放的操作：`403`。

## 当前限制

示例没有登录、团队、备注、附件、搜索、通知、批量导入或网页 Manager；Public API 只能创建 Lead。Current Distribution 直接修改模块 Source 会产生新的数据 Scope，不会自动升级本页创建的数据；Source Preview 只有数据结构完全未变化的 compatible Activate 会保留原 Record 存储标识。
