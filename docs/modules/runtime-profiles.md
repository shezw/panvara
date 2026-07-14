<!--
    Panvara
    docs/modules/runtime-profiles.md    2026-07-14
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# 运行模式使用指南

## 用途

Runtime Profile 决定 Panvara 进程启动哪些能力和需要哪些外部服务。它让同一个 Core 可以从不依赖数据库的最小进程开始，再按项目需要组合成业务 Server。

Profile 是部署组合，不是付费等级，也不是互相继承的产品版本。

## 当前状态

| Profile | alpha.2 状态 | 当前用途 | 依赖 |
| --- | --- | --- | --- |
| `lite` | 已实现 | Core 生命周期、健康和版本验收 | 无 |
| `server` | 已实现 | 一个 AppModule 的 HTTP API 与持久化 CRUD | PostgreSQL |
| `manager` | 计划 | Server + 管理控制面 | 尚不可启动 |
| `site` | 计划 | Website 与 Assets | 尚不可启动 |
| `commerce` | 计划 | Commerce 与 Payments | 尚不可启动 |
| `distributed` | 计划 | Server/Worker 分进程 | 尚不可启动 |

计划中的 Profile 会明确报错并退出，不会伪装成已经可用。

## 前置条件

Lite 需要 Go 1.26.5（最低兼容 1.25）和 Make。

Server 额外需要：

- PostgreSQL 18.4，推荐使用仓库 Compose。
- 一个有效 AppModule YAML/JSON。
- [Project Context](project-context.md) 配置。
- 至少 32 字节、无空白字符的管理员 Token。

## 最小示例

### 启动 Lite

```bash
make run
```

另一个终端验收：

```bash
curl http://127.0.0.1:8080/healthz
curl http://127.0.0.1:8080/readyz
curl http://127.0.0.1:8080/version
```

Lite 不读取 AppModule，也不提供 `/api/...` 业务接口。

### 启动 Server

```bash
make local-init
set -a
. ./.env
. ./.env.local
set +a
make infra-up
make run-server
```

成功时会输出 Profile、监听地址、模块名称和 Revision。另一个终端调用 Admin API 前也要加载同一份 `.env` 和 `.env.local`。

停止进程使用 `Ctrl+C`，关闭本地数据库容器使用：

```bash
make infra-down
```

`infra-down` 默认保留数据库卷，方便下次继续验收。

## 配置

Panvara 不会自动读取 `.env`。所有配置可使用环境变量；命令行 Flag 主要用于调试。

| 环境变量 | 默认值 | Lite | Server |
| --- | --- | --- | --- |
| `PANVARA_PROFILE` | `lite` | 可选 | 必须为 `server` |
| `PANVARA_HTTP_ADDR` | `127.0.0.1:8080` | 可选 | 可选 |
| `PANVARA_DATABASE_URL` | 无 | 忽略 | 必需 |
| `PANVARA_MODULE_SOURCE` | 无 | 忽略 | 必需 |
| `PANVARA_MODULE_FORMAT` | `auto` | 忽略 | `auto`、`yaml`、`json` |
| `PANVARA_PROJECT_ID` | 无 | 忽略 | 必需 UUIDv7 |
| `PANVARA_PROJECT_KEY` | `default` | 忽略 | 可选 |
| `PANVARA_PROJECT_LOCALE` | `en-US` | 忽略 | 可选 |
| `PANVARA_PROJECT_TIME_ZONE` | `UTC` | 忽略 | 可选 |
| `PANVARA_PROJECT_CURRENCY` | `USD` | 忽略 | 可选 |
| `PANVARA_ADMIN_TOKEN` | 无 | 忽略 | 必需，至少 32 字节 |

对应 CLI Flag 包括 `--profile`、`--http`、`--database-url`、`--module-source`、`--module-format`、`--project-*` 和 `--admin-token`。真实 Token 不应放在命令行，因为同机其他进程可能看到参数。

查看版本而不启动服务：

```bash
go run ./cmd/panvara --version
```

## 验收

### Lite 验收

1. 启动日志显示 `profile=lite`。
2. `/healthz` 返回 `{"status":"alive"}`。
3. `/readyz` 返回 `{"status":"ready"}`。
4. `/version` 的 Distribution 为 `0.1.0-alpha.2`。
5. 不启动 Docker 也能完成以上步骤。

### Server 验收

1. `make infra-up` 等待 PostgreSQL 健康。
2. 启动日志显示 `profile=server`、`module=...`、`revision=sha256:...`。
3. `/readyz` 返回 200。
4. OpenAPI 和 UI Schema 可访问。
5. 完成 [CRM Leads](crm-leads.md) 的创建与查询。
6. 重启 Server 后记录仍存在。

## 常见问题

### 为什么 `make run` 没有 CRM API？

`make run` 启动 Lite，只验证 Core 和运维端点。业务 API 必须使用 `make run-server`。

### 为什么 `.env` 没有自动生效？

Panvara 不自动加载文件。请按示例显式导入 `.env` 和 `.env.local`，或者在 IDE 中设置环境变量。

### 为什么 Server 提示缺少 administrator token？

先执行 `make local-init`，再在当前 Shell 加载 `.env.local`。仓库中的 `.env.example` 故意把 Token 留空。

### 可以运行 `--profile=manager` 看 UI 吗？

不可以。alpha.2 会提示该 Profile 尚未实现并退出。

### 端口 5432 或 8080 被占用怎么办？

设置 `PANVARA_POSTGRES_PORT` 并同步修改数据库 URL；设置 `PANVARA_HTTP_ADDR` 修改 HTTP 端口。多个工作目录还应使用不同的 `PANVARA_COMPOSE_PROJECT`。

## 当前限制

- Lite 不是“无数据库业务 Server”，它只提供 Core 运维端点。
- Server 每次启动只装配一个项目和一个 AppModule。
- 没有热重载、后台 Worker、Manager UI 或独立 Provider 进程。
- 没有多节点配置收敛、服务发现或分布式发布控制面。
- Compose 仅供本地开发，不适用于生产。
- Server readiness 只检查 Core 与 PostgreSQL。

## 兼容与升级

Profile 名称是配置契约，但 alpha 阶段的内部组合仍可能变化。自动化脚本应检查启动退出码和 `/readyz`，不要只判断进程存在。

从 Lite 切换到 Server 不会自动创建业务模型；必须显式提供数据库、AppModule、Project Context 和 Token。未来 Manager/Site/Commerce/Distributed 真正落地时，应同时增加本页的启动和验收步骤，并保持未实现组合 fail-fast。
