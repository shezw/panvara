<!--
    Panvara
    docs/modules/crm-leads.md    2026-07-14
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# CRM Leads 验收指南

## 用途

`crm-leads` 是 Panvara alpha.2 的参考业务模块。它用“组织与销售线索”场景演示模型声明、关系、唯一字段、Public Create、Admin CRUD、OpenAPI、Manager UI Schema 和 PostgreSQL 持久化。

这是非专业用户验收 alpha.2 的推荐入口。

## 当前状态

模块 Source 位于 [`examples/modules/crm-leads.yaml`](../../examples/modules/crm-leads.yaml)，已经实现两个 Resource：

| Resource | 主要字段 | Public | Admin |
| --- | --- | --- | --- |
| `organization` | 唯一且必填的 `name` | 无 | List/Get/Create/Patch/Delete |
| `lead` | 必填 `organization`、唯一 `email`、`stage`，可选 `score` | Create | List/Get/Create/Patch/Delete |

`lead.organization` 必须引用现有 Organization。`stage` 只能是 `new`、`qualified` 或 `won`。`score` 是 precision 12、scale 4 的十进制字符串。

alpha.2 会生成 Manager UI Schema，但没有可打开的 Manager 网页。邮件通知、Event、Provider 和发布回滚属于后续阶段。

## 前置条件

- 已安装 Go 1.26.5、Make、Docker Compose 和 `curl`。
- 当前目录是 Panvara 仓库根目录。
- 端口 5432 与 8080 可用，或已经按 [运行模式](runtime-profiles.md) 修改端口。
- 本机验收数据可以保存在 Compose 数据卷中。

建议准备两个终端：终端 A 运行 Server，终端 B 调用 API。

## 最小示例

### 1. 终端 A：启动数据库与 Server

```bash
make doctor-server
make local-init
set -a
. ./.env
. ./.env.local
set +a
make infra-up
make run-server
```

启动成功时会显示：

```text
panvara 0.1.0-alpha.2 profile=server address=127.0.0.1:8080
module=crm.leads revision=sha256:<64个十六进制字符>
```

### 2. 终端 B：准备调用环境

```bash
export BASE_URL=http://127.0.0.1:8080
set -a
. ./.env
. ./.env.local
set +a
export DEMO_SUFFIX="$(date +%s)"
export LEAD_EMAIL="first-${DEMO_SUFFIX}@example.com"
curl "$BASE_URL/readyz"
```

应看到 `{"status":"ready"}`。

### 3. 创建 Organization

```bash
curl -i -X POST "$BASE_URL/api/admin/v1alpha1/crm.leads/organization" \
  -H "Authorization: Bearer $PANVARA_ADMIN_TOKEN" \
  -H 'Content-Type: application/json' \
  -d "{\"name\":\"Panvara Demo Organization ${DEMO_SUFFIX}\"}"
```

确认返回 `201 Created` 和 `ETag: "1"`。复制 JSON 中的 `id`，下一步用它替换 `<organization-id>`。

### 4. 使用 Public API 创建 Lead

```bash
curl -i -X POST "$BASE_URL/api/public/v1alpha1/crm.leads/lead" \
  -H 'Content-Type: application/json' \
  -d "{\"organization\":\"<organization-id>\",\"email\":\"${LEAD_EMAIL}\",\"stage\":\"new\"}"
```

Public Create 不需要管理员 Token。成功时返回 201。

### 5. 查询 Lead

```bash
curl --get "$BASE_URL/api/admin/v1alpha1/crm.leads/lead" \
  -H "Authorization: Bearer $PANVARA_ADMIN_TOKEN" \
  --data-urlencode 'filter[stage]=new'
```

响应的 `data` 数组应包含刚创建的 `${LEAD_EMAIL}` 实际值。

### 6. 查看生成的产品契约

```bash
curl "$BASE_URL/api/core/v1alpha1/modules/crm.leads/openapi.json"
curl "$BASE_URL/api/core/v1alpha1/modules/crm.leads/ui-schema.json"
```

第一个响应描述 HTTP API；第二个描述未来 Manager 可渲染的列表和表单。

## 配置

### Organization

| 字段 | 类型 | 必填 | 规则 |
| --- | --- | --- | --- |
| `name` | string | 是 | 最长 128 字符，在当前 Scope 唯一 |

### Lead

| 字段 | 类型 | 必填 | Public 可写 | Admin 可写 | 规则 |
| --- | --- | --- | --- | --- | --- |
| `organization` | reference | 是 | 是 | 是 | 现有 Organization UUIDv7 |
| `email` | email | 是 | 是 | 是 | 最长 320 字符，当前 Scope 唯一 |
| `stage` | enum | 是 | 是 | 是 | `new`、`qualified`、`won` |
| `score` | decimal | 否 | 否 | 是 | 字符串，precision 12、scale 4 |

Admin List 可按 Organization `name`，或 Lead `email`、`stage` 做等值过滤。模型没有启用动态排序。

本地默认配置为 Module `crm.leads`、Project Key `crm`、Locale `en-US`、Time Zone `UTC`、Currency `USD`，PostgreSQL 只绑定 `127.0.0.1:5432`。

## 验收

- [ ] PostgreSQL 健康，Server `/readyz` 返回 200。
- [ ] 启动日志显示 `crm.leads` 与 Revision Hash。
- [ ] Admin Token 错误时创建 Organization 返回 401。
- [ ] 正确 Token 创建 Organization 返回 201、UUIDv7 和 ETag。
- [ ] Public API 无 Token 创建 Lead 返回 201。
- [ ] Admin List 按 `stage=new` 能找到该 Lead。
- [ ] 再次创建相同 email 返回 409 `unique_conflict`。
- [ ] 使用不存在的 Organization ID 创建 Lead 失败。
- [ ] 有 Lead 引用时删除 Organization 返回 409 `record_referenced`。
- [ ] OpenAPI 和 UI Schema 均能下载。
- [ ] 保持同一 Source/Project/数据库重启后，数据仍存在。

自动化验证相同链路：

```bash
make test-e2e
```

该命令要求 Docker 或显式 PostgreSQL 18.4 测试 URL，不满足条件会失败而不是跳过。

## 常见问题

### 创建 Organization 返回 409 怎么办？

示例名称是唯一字段，可能已在上一次验收中创建。换一个名称，或继续使用 List 中已有记录的 ID。

### 创建 Lead 返回 `validation_failed` 怎么办？

检查三个必填字段：`organization` 必须是 UUIDv7，`email` 必须是邮箱，`stage` 必须是允许值。

### Public API 为什么不能写 `score`？

模型只允许 Public 写 organization、email、stage。评分只允许 Admin 设置。

### 为什么删除 Organization 失败？

仍有未删除 Lead 引用它。Record Runtime 会保护引用完整性。

### 怎样看到管理页面？

alpha.2 没有管理页面，只能下载 `ui-schema.json` 检查描述。

### 怎样清空本机验收数据？

`make infra-down` 会保留数据卷。Compose 的 `down --volumes` 会不可恢复地删除该 Compose Project 的 PostgreSQL 数据，只能在确认本地数据可丢弃后手工执行。

## 当前限制

- 这是技术参考模块，不是完整 CRM 产品。
- 没有 Account、登录、团队、权限配置、备注、任务、附件或搜索。
- 没有邮件通知、Event、Outbox、Worker 或网页 Manager。
- Public API 只允许创建 Lead；没有公开查询。
- 不支持排序、模糊搜索、批量导入或导出。
- 修改 Source 会产生新 Revision，不会迁移当前验收数据。

## 兼容与升级

当前业务版本是 `1.0.0`，协议版本是 `panvara.dev/v1alpha1`，两者含义不同。alpha.2 使用 Source 编译后的 Canonical IR 计算 Revision；标签或 Manager 显示顺序等 IR 变化也可能得到新 Revision，但纯空白或无语义的 YAML Key 顺序变化不会改变 Revision。

修改 CRM Leads 时：复制 Source 并保存旧 Source/Hash；提升 `metadata.version`；在独立 Project ID 或可丢弃数据库验证；明确接受新 Revision 初始为空。发布、迁移和回滚闭环属于 alpha.3。
