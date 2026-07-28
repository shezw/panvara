<!--
    Panvara
    docs/guides server.md    2026-07-28
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# 运行 PostgreSQL Server

::: info Current Distribution
当前 Distribution 标识 **v0.1.0-alpha.2** 对应的 Server 会加载一个 AppModule、连接 PostgreSQL，并开放业务 API、OpenAPI 与 Manager UI Schema。仓库中的 Compose 只用于本地开发。
:::

## 前置条件

- 已完成[快速开始](/getting-started/)；
- Docker Engine 与 Docker Compose v2 正在运行；
- 已安装 OpenSSL；
- 本机 5432 和 8080 端口可用；
- 当前目录是 Panvara 仓库根目录。

```sh
make doctor-server
```

成功时最后一行是 `Environment check passed.`。

## 1. 创建本地配置

```sh
make local-init
```

该命令在不存在时创建：

- `.env`：数据库、模块和项目等普通配置；
- `.env.local`：只允许当前用户读取的本地管理员凭据。

它不会覆盖已有文件。不要提交或打印 `.env.local`。

每个新终端都要显式加载配置：

```sh
set -a
. ./.env
. ./.env.local
set +a
```

Panvara 不会自动读取 `.env`。

## 2. 启动 PostgreSQL

```sh
make infra-up
docker compose -f deploy/compose/compose.yaml ps
```

`postgres` 的状态应包含 `healthy`。默认数据库只绑定本机 `127.0.0.1:5432`。

## 3. 启动 Server

在终端 A 保持配置已加载，然后执行：

```sh
make run-server
```

成功日志应包含：

```text
panvara 0.1.0-alpha.2 profile=server address=127.0.0.1:8080
module=crm.leads revision=sha256:<64个十六进制字符>
```

在终端 B 加载同一配置并检查：

```sh
curl -fsS http://127.0.0.1:8080/readyz
curl -fsS http://127.0.0.1:8080/api/core/v1alpha1/modules/crm.leads/openapi.json
curl -fsS http://127.0.0.1:8080/api/core/v1alpha1/modules/crm.leads/ui-schema.json
```

`readyz` 应返回 `{"status":"ready"}`，后两条应返回 JSON。

## 4. 验证持久化

按 [CRM Leads](./crm-leads) 创建一条数据。然后：

1. 在终端 A 按 `Ctrl+C` 停止 Server；
2. 保持 PostgreSQL 运行；
3. 使用完全相同的 `.env`、模块 Source 和数据库再次执行 `make run-server`；
4. 重新查询刚才的 Record。

Record 仍存在且 `/readyz` 再次返回 200，才表示持久化链路通过。

## 停止与保留数据

`Ctrl+C` 只停止 Server。停止本地数据库容器：

```sh
make infra-down
```

该命令会保留数据卷。再次 `make infra-up` 后数据仍在。永久删除数据的危险操作见[故障排查](/reference/troubleshooting#彻底重置本地数据库)。

## 常用配置来源

优先级是 CLI 参数 > 环境变量 > 内置默认值。完整列表见[配置参考](/reference/configuration)。

| 变量 | 用途 |
| --- | --- |
| `PANVARA_DATABASE_URL` | PostgreSQL 连接地址 |
| `PANVARA_MODULE_SOURCE` | 要加载的 YAML/JSON AppModule |
| `PANVARA_HTTP_ADDR` | HTTP 监听地址，默认 `127.0.0.1:8080` |
| `PANVARA_PROJECT_ID` | 当前本地项目 UUIDv7 |
| `PANVARA_ADMIN_TOKEN` | 本地 Admin 请求凭据；只放在安全环境变量中 |

## 当前限制

- 一个 Server 进程只加载一个项目和一个 AppModule；
- 没有热重载、可视化 Manager、内置 TLS、限流或多节点控制面；
- Readiness 只代表当前进程与必需依赖就绪；
- 当前版本不适合生产部署。
