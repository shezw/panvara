<!--
    Panvara
    docs/getting-started/crm-leads-acceptance.md    2026-07-14
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# CRM Leads 完整验收

本页证明 alpha.2 的完整链路真实可用。你会启动数据库和 Server，创建一条销售线索，再通过重启确认数据已经持久化。

## 验收前检查

确保已经完成[创建本地环境](./local-environment)，并且 Lite 已停止。执行：

```sh
make doctor-server
set -a
. ./.env
. ./.env.local
set +a
make infra-up
```

必须先加载配置再启动 Compose，这样自定义的 PostgreSQL 端口才会同时生效。

## 1. 启动 Server

在终端 A 的 Panvara 根目录加载配置并启动：

```sh
set -a
. ./.env
. ./.env.local
set +a
make run-server
```

预期输出包含：

```text
panvara 0.1.0-alpha.2 profile=server address=127.0.0.1:8080
module=crm.leads revision=sha256:...
```

`revision` 是当前模型的版本指纹，每份内容相同的模型都会得到相同指纹。

## 2. 检查服务和生成产物

打开终端 B，进入同一个 Panvara 目录并加载配置：

```sh
set -a
. ./.env
. ./.env.local
set +a
```

检查服务：

```sh
curl -fsS http://127.0.0.1:8080/readyz
curl -fsS http://127.0.0.1:8080/api/core/v1alpha1/modules/crm.leads/openapi.json
curl -fsS http://127.0.0.1:8080/api/core/v1alpha1/modules/crm.leads/ui-schema.json
```

第一条返回 `{"status":"ready"}`。后两条会返回较长 JSON：OpenAPI 描述可调用接口，UI Schema 描述未来 Manager 可以怎样展示字段；UI Schema 本身不是可视化页面。

## 3. 创建组织

先生成本次验收专用后缀，让同一数据库可以重复执行本页：

```sh
export PANVARA_ACCEPTANCE_SUFFIX="$(date +%s)"
export LEAD_EMAIL="hello-${PANVARA_ACCEPTANCE_SUFFIX}@example.com"
```

```sh
curl -i -sS \
  -X POST http://127.0.0.1:8080/api/admin/v1alpha1/crm.leads/organization \
  -H "Authorization: Bearer ${PANVARA_ADMIN_TOKEN}" \
  -H 'Content-Type: application/json' \
  -d "{\"name\":\"Panvara Demo ${PANVARA_ACCEPTANCE_SUFFIX}\"}"
```

成功标志：第一行包含 `201 Created`，响应 JSON 类似：

```json
{
  "id": "019...",
  "version": 1,
  "data": { "name": "Panvara Demo 17..." },
  "created_at": "...",
  "updated_at": "..."
}
```

复制返回的 `id`，在终端 B 设置为环境变量；把尖括号内容替换成真实值：

```sh
export ORGANIZATION_ID='<粘贴组织 id>'
```

## 4. 公开创建销售线索

```sh
curl -i -sS \
  -X POST http://127.0.0.1:8080/api/public/v1alpha1/crm.leads/lead \
  -H 'Content-Type: application/json' \
  -d "{\"organization\":\"${ORGANIZATION_ID}\",\"email\":\"${LEAD_EMAIL}\",\"stage\":\"new\"}"
```

成功标志同样是 `201 Created`，且响应中包含 `${LEAD_EMAIL}` 的实际值和 `"stage":"new"`。这个 Public 接口不需要管理员 Token，但只能执行模型明确开放的 Create。

## 5. 通过管理接口查询

```sh
curl -fsS \
  'http://127.0.0.1:8080/api/admin/v1alpha1/crm.leads/lead?filter%5Bstage%5D=new' \
  -H "Authorization: Bearer ${PANVARA_ADMIN_TOKEN}"
```

返回的 `data` 数组应该包含刚才创建的 `${LEAD_EMAIL}` 实际值。

## 6. 验证重启后仍有数据

1. 回到终端 A，按 `Ctrl+C` 停止 Server。
2. 不修改 `.env`、`.env.local` 或 `examples/modules/crm-leads.yaml`。
3. 在终端 A 再次执行 `make run-server`。
4. 在终端 B 重复上一步查询命令。

仍然看到本次 `${LEAD_EMAIL}` 的实际值，就证明 PostgreSQL 持久化闭环验收通过。

## 7. 安全停止

在终端 A 按 `Ctrl+C`，然后在终端 B 执行：

```sh
make infra-down
```

这会停止数据库容器但保留数据。`.env.local` 只用于本地开发；如果不再使用该项目，可以手工删除它。

::: warning 不要直接修改模型后继续本验收
alpha.2 修改模型内容会产生新的数据空间，看起来像“数据不见了”。旧数据仍与旧版本指纹关联。理解这一限制后再阅读 [AppModule 指南](/modules/appmodule)。
:::
