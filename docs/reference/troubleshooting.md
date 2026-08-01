<!--
    Panvara
    docs/reference troubleshooting.md    2026-07-28
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# 故障排查

先运行对应检查：Lite 使用 `make doctor`，Server 使用 `make doctor-server`。检查失败时先修复环境，不要跳到后续步骤。

## 找不到 go、make、docker 或 curl

- **现象：** `command not found` 或 Doctor 显示 `[missing]`。
- **原因：** 工具未安装，或新安装路径尚未进入当前终端。
- **恢复：** 安装对应工具，关闭并重新打开终端，再运行 Doctor。
- **数据影响：** 无。

Go 最低版本是 1.25。Docker Server 路径还需要 Compose v2 与正在运行的 daemon。

## Docker daemon 不可用或不支持 `--wait`

- **现象：** `Docker daemon is not reachable` 或 `unknown flag: --wait`。
- **原因：** Docker Desktop/Engine 未启动，或 Compose 版本过旧。
- **恢复：**

```sh
docker info
docker compose version
make doctor-server
```

启动 Docker 服务；若缺少 `up --wait`，升级 Docker Desktop 或 Compose plugin。

- **数据影响：** 检查不会修改数据；升级 Docker 前按你的环境策略备份容器数据。

## 端口 5432 已占用

- **现象：** PostgreSQL 容器无法绑定端口。
- **原因：** 本机已有 PostgreSQL 或另一个 Compose Project。
- **恢复：** 在 `.env.local` 添加：

```dotenv
PANVARA_POSTGRES_PORT=55432
PANVARA_DATABASE_URL=postgres://panvara:panvara-dev@127.0.0.1:55432/panvara?sslmode=disable
```

重新加载配置，再执行 `make infra-up`。

- **数据影响：** 改端口不会移动旧数据库；URL 指向另一个数据库时，看到的数据也会不同。

## 端口 8080 已占用

- **现象：** Server 提示监听失败。
- **恢复：** 在 `.env.local` 添加：

```dotenv
PANVARA_HTTP_ADDR=127.0.0.1:18080
```

重新加载配置，并把请求地址同步改为 `http://127.0.0.1:18080`。

- **数据影响：** 无。

## Server 提示缺少配置

- **现象：** 提示缺少 database URL、module source、project ID 或 Admin Token。
- **原因：** 当前终端没有加载 `.env` / `.env.local`，或文件不完整。
- **恢复：**

```sh
make local-init
set -a; . ./.env; . ./.env.local; set +a
make run-server
```

- **数据影响：** `make local-init` 不覆盖已有文件。不要随意替换已经使用的 Project ID、数据库 URL 或 Secret。

## `/healthz` 成功但 `/readyz` 返回 503

- **现象：** 进程存活，但尚不能处理完整请求。
- **原因：** Server 依赖未就绪，常见为 PostgreSQL 不可达。
- **恢复：** 检查 `docker compose ... ps`、数据库 URL 与 Server 日志；恢复依赖后重新检查 readiness。
- **数据影响：** 不要在数据库状态不明时重置数据卷。

## Admin 请求返回 401、403 或 503

| 状态 | 常见原因 | 恢复 |
| --- | --- | --- |
| 401 | Credential 缺失、错误或已撤销 | 加载正确 Secret；不要把 Token 打印出来 |
| 403 | 已认证，但模型未开放操作或当前授权不足 | 检查 AppModule 操作白名单；Source Preview 用户再检查 Grant |
| 503 | 权威依赖不可用 | 恢复 PostgreSQL/授权存储后，用同一意图重试 |

不要通过重启、修改数据库表或更换随机 Token 绕过权限问题。

## 创建时返回 409

- **现象：** `unique_conflict`、`record_referenced` 或其他冲突。
- **原因：** 唯一值已存在，或目标 Record 仍被引用。
- **恢复：** 使用新唯一值；删除前先处理引用关系；根据错误 `code` 决定，不要盲目重试。
- **数据影响：** 冲突请求不会覆盖已有 Record。

## Patch 返回 428 或 412

- **428：** 缺少 `If-Match`。先 Get 并读取 ETag。
- **412：** ETag 已过期。重新读取 Record，比较并合并其他请求的变化。
- **数据影响：** 失败请求不会覆盖较新的版本。

## 修改模型后看不到旧数据

- **现象：** Server 能启动，但 List 为空。
- **原因：** 新模型产生新的 Revision 与独立数据空间。
- **恢复：** 恢复精确的旧 AppModule Source 与原数据库，再启动并核对 OpenAPI 的 Revision。
- **数据影响：** 旧数据通常仍保留，但没有自动复制到新 Revision。不要覆盖唯一的旧 Source。

::: warning Source Preview
Publish 返回成功但 API 仍使用旧模型，是模型变更预览的正确结果。必须同时看到 `activated=false`、`records_migrated=false` 与 `runtime_changed=false`。只有随后显式 Activate，且 Release 的数据结构完全未变化时，Runtime 才会切换。

- `409 activation_conflict`：当前版本已变化、发生并发切换，或目标是历史 Release；重新读取 `/active` 并从当前版本开始。
- `422 not_activatable`：变更需要复核、迁移或改变了数据结构；当前不能绕过。
- `503 release_unavailable`：发布权威或本地 Runtime 状态不确定；停止该实例，重启后先读取 `/active`，不要盲目重复激活。

Activate 成功后，恢复旧 Source 或重启不会回滚。当前只能依赖激活前验证过的数据库备份恢复。
:::

## `make infra-down` 后数据仍然存在

这是安全的默认行为：命令停止并删除容器，但保留 PostgreSQL 数据卷。再次 `make infra-up` 会复用数据。

## 彻底重置本地数据库

::: danger Planned / Unavailable
Panvara 当前没有自动备份、恢复或安全数据重置向导。下面命令会**永久删除当前 Compose Project 的本地 PostgreSQL 数据卷，无法撤销**。
:::

**再次确认：只在确定数据可丢弃、Server 已停止、Compose Project 名正确，并且不需要恢复任何 Record 时继续。**

```sh
docker compose -f deploy/compose/compose.yaml down --volumes
```

执行后 `make infra-up` 会创建空数据库。不要把这条命令用于共享或生产环境。

## 提交 Issue 前

收集以下非敏感信息：

```sh
git status --short --branch
go version
docker version
docker compose version
make doctor-server
./bin/panvara --version
```

同时提供失败命令、HTTP 状态、错误 `code` 和 `request_id`。不要附带 `.env`、`.env.local`、数据库 URL 密码、Authorization Header 或签发响应。
