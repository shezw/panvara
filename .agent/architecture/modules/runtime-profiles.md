<!--
    Panvara
    docs/modules/runtime-profiles.md    2026-07-18
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

| Profile | 当前状态 | 当前用途 | 依赖 |
| --- | --- | --- | --- |
| `lite` | 可运行基础切片 | Core 生命周期、健康和版本验收 | 无 |
| `server` | 可运行最小纵向切片；Server Core 未完成 | 一个 AppModule 的 HTTP API、持久化 CRUD 与 P0-01a/P0-01b 访问闭环 | PostgreSQL |
| `manager` | 规划中 | Server + 管理控制面 | 尚不可启动 |
| `site` | 规划中 | Website 与 Assets | 尚不可启动 |
| `commerce` | 规划中 | Commerce 与 Payments | 尚不可启动 |
| `distributed` | 规划中 | Server/Worker 分进程 | 尚不可启动 |

`Runnable` 只表示仓库有可执行装配路径，不表示功能完整或生产就绪。没有装配路径的 Profile 会明确报错并退出；Server Core 的完整待办和完成门禁见 [Server Core 能力清单](../roadmap/server-core.md)。

## 前置条件

Lite 需要 Go 1.26.5（最低兼容 1.25）和 Make。

Server 额外需要：

- PostgreSQL 18.4，推荐使用仓库 Compose。
- 一个有效 AppModule YAML/JSON。
- [Project Context](project-context.md) 配置。
- [执行作用域与访问内核](project-access.md) 使用的默认 Environment Key；未设置时为 `default`。
- marker 尚未初始化时需要 32–1024 字节、仅含 Bearer 安全 ASCII且不以 `pvk1.` 开头的 bootstrap Token；完成后重启可省略。

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

首次成功启动会先持久化 Project、生成默认 Environment，创建 `bootstrap-admin` Principal/Owner Grant，并把 bootstrap Token 的 digest、hint 与永久 marker 原子写入 PostgreSQL。后续相同配置重启会复用这些事实；Token 可省略，相同值可核对，不同值和 Project/Environment 配置漂移都会在监听 HTTP 前拒绝启动。另一个终端调用 Admin API 前仍要加载可用 Credential。

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
| `PANVARA_ENVIRONMENT_KEY` | `default` | 忽略 | 可选；当前唯一默认 Environment Key |
| `PANVARA_ADMIN_TOKEN` | 无 | 忽略 | marker 不存在时必需；32–1024 字节、仅 Bearer 安全 ASCII、非 `pvk1.` 前缀；完成后可省略 |

对应 CLI Flag 包括 `--profile`、`--http`、`--database-url`、`--module-source`、`--module-format`、`--project-*`、`--environment-key` 和 `--admin-token`。真实 Token 不应放在命令行，因为同机其他进程可能看到参数。

Bearer Credential 只把请求认证为一个 project-local Principal。Admin API 是否放行由 Application 层读取 active Credential/Principal 与持久化 Owner Grant 决定；Credential 无效/撤销返回 401，Grant 缺失/撤销返回 403，权威状态读取失败返回 503。重启不会复活 revoked Credential 或自动补回 Grant；Grant 只能由另一个 Owner 显式 PUT 重新授予。

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
5. 按[执行作用域与访问内核](project-access.md#最小示例)确认 Project、生成的默认 Environment、Principal 与 Owner Grant 已持久化。
6. 完成 [CRM Leads](crm-leads.md) 的创建与查询。
7. 重启 Server 后 Environment ID 与记录都保持不变。
8. 在可丢弃环境按[访问管理验收](access-administration.md#验收)创建第二 Owner path，验证轮换、Credential revoke 的 401、Grant revoke 的 403、显式重新授予及 last-owner 409。

## 常见问题

### 为什么 `make run` 没有 CRM API？

`make run` 启动 Lite，只验证 Core 和运维端点。业务 API 必须使用 `make run-server`。

### 为什么 `.env` 没有自动生效？

Panvara 不自动加载文件。请按示例显式导入 `.env` 和 `.env.local`，或者在 IDE 中设置环境变量。

### 为什么 Server 提示缺少 administrator token？

marker 尚不存在时，先执行 `make local-init`，再在当前 Shell 加载 `.env.local`。marker 已存在时可以省略 Token；如果仍提供，必须与永久 marker 的 digest 相同。

### 为什么 Token 正确，Admin API 仍返回 403？

Token 只完成认证。请确认持久化 Project、默认 Environment 与 `bootstrap-admin` Principal 都是 active，并且精确作用域内的 `project.owner` Grant 未撤销。不要通过重启恢复权限；应在受控流程中检查和修复数据库授权事实。

### 为什么修改 Project 或 Environment 配置后无法启动？

某个 Project ID 第一次启动后，数据库是该 Project/Environment 身份与设置的权威来源。Server 会拒绝同一 Project ID 下的 Project Key、Locale、Time Zone、Currency 或 Environment Key 漂移；恢复原配置后再启动。新的 Project ID 与新的唯一 Key 会创建另一 Project，不是修改旧 Project。当前没有通过 `.env` 修改持久化 Project 的流程。

### 可以运行 `--profile=manager` 看 UI 吗？

不可以。当前版本会提示该 Profile 没有可运行装配并退出。

### 端口 5432 或 8080 被占用怎么办？

设置 `PANVARA_POSTGRES_PORT` 并同步修改数据库 URL；设置 `PANVARA_HTTP_ADDR` 修改 HTTP 端口。多个工作目录还应使用不同的 `PANVARA_COMPOSE_PROJECT`。

## 当前限制

- Lite 不是“无数据库业务 Server”，它只提供 Core 运维端点。
- Server 每次启动只装配一个项目和一个 AppModule。
- P0-01b 只有 project-local Service Principal/API Credential 与固定 Owner Grant，不是完整 IAM；没有 Account、ExternalIdentity、Session、ProjectMembership、动态 Role/Policy 或 RecordOwner。
- 只有默认 Environment 可以执行现有用例；业务事实表尚无 `environment_id`，没有多 Environment 数据隔离。
- 没有热重载、后台 Worker、Manager UI 或独立 Provider 进程。
- 没有多节点配置收敛、服务发现或分布式发布控制面。
- Compose 仅供本地开发，不适用于生产。
- Server readiness 只检查 Core 与 PostgreSQL。

## 兼容与升级

Profile 名称是配置契约，但 alpha 阶段的内部组合仍可能变化。自动化脚本应检查启动退出码和 `/readyz`，不要只判断进程存在。

从 Lite 切换到 Server 不会自动创建业务模型；必须显式提供数据库、AppModule、Project Context，且首次 marker 初始化还必须提供 Token。Migration 0004/0005 后，同一 Project ID 的后续启动要求 Project/Environment 配置一致，并尊重 Credential/Grant 权威状态。Principal disable 与 Credential revoke 不可恢复；Grant 只接受已授权显式 PUT，restart/bootstrap 不自动恢复。未来 Manager/Site/Commerce/Distributed 真正落地时，应同时增加本页的启动和验收步骤，并保持未实现组合 fail-fast。
