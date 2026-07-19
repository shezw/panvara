<!--
    Panvara
    docs/getting-started/troubleshooting.md    2026-07-14
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# 故障排查

先重新执行对应检查：Lite 使用 `make doctor`，Server 使用 `make doctor-server`。不要在检查失败时继续后面的步骤。

## 找不到 go、make、docker 或 curl

现象通常是 `command not found`。返回[安装开发工具](./prerequisites)，安装后关闭并重新打开终端，使新的 PATH 生效。

## Go 正在下载工具链

Panvara 推荐 Go 1.26.5。较旧但兼容的 Go 可能自动下载项目指定工具链，这是正常行为。如果下载失败，请检查网络代理，或从 [go.dev/dl](https://go.dev/dl/) 安装 1.26.5 后重试。

## Docker daemon is not reachable

Docker 命令已安装，但后台服务没有运行。macOS/Windows 请打开 Docker Desktop并等待启动完成；Linux 请启动 Docker 服务。随后执行：

```sh
docker info
make doctor-server
```

## unknown flag: --wait

本机 Docker Compose 版本过旧，不支持 Panvara 用于等待数据库健康的 `up --wait`。升级 Docker Desktop 或 Docker Compose plugin，再执行 `make doctor-server`。

## cgo: C compiler not found

Linux 的 Race Detector 需要 C 编译器。Ubuntu/WSL2 执行：

```sh
sudo apt update
sudo apt install -y build-essential
```

然后重新运行 `make verify`。

## PostgreSQL 的 5432 端口已占用

在 `.env.local` 增加以下两行，改用 55432：

```dotenv
PANVARA_POSTGRES_PORT=55432
PANVARA_DATABASE_URL=postgres://panvara:panvara-dev@127.0.0.1:55432/panvara?sslmode=disable
```

重新加载 `.env` 与 `.env.local`，再执行 `make infra-up`。

## HTTP 的 8080 端口已占用

在 `.env.local` 增加：

```dotenv
PANVARA_HTTP_ADDR=127.0.0.1:18080
```

重新加载配置并启动。把文档中所有 `127.0.0.1:8080` 临时替换成 `127.0.0.1:18080`。

## PANVARA_ADMIN_TOKEN must be exported

当前终端尚未加载私密配置。执行：

```sh
set -a
. ./.env
. ./.env.local
set +a
make run-server
```

如果 `.env.local` 不存在，执行 `make local-init`。

## 请求返回 401 Unauthorized

管理接口需要 `Authorization: Bearer ...`。首次验收时，终端 B 必须加载与 Server 初始化时相同的 `.env.local`；也可以使用仍为 active、Principal 未停用且已获 `project.owner` Grant 的 Service Credential。重新加载配置，再确认请求包含：

```sh
-H "Authorization: Bearer ${PANVARA_ADMIN_TOKEN}"
```

不要把 Token 直接粘贴进会提交的脚本或文档。

如果 Header 正确仍返回 401，请检查 Credential 是否已撤销或 Principal 是否已停用；若返回 403，则 Credential 已通过认证，但当前作用域缺少 active `project.owner` Grant。参见[访问管理指南](../modules/access-administration.md)。

## 创建组织或邮箱返回冲突

CRM 示例要求组织名称和邮箱在同一模型版本中唯一。更换示例名称或邮箱，例如 `hello-2@example.com`；或者保留现有数据继续后续步骤。

## 修改模型后看不到旧数据

这是 alpha.2 的已知版本隔离行为。恢复原来的 `examples/modules/crm-leads.yaml` 内容并重启，旧数据会重新可见。不要反复修改生产模型；当前版本尚无自动数据迁移。

## make infra-down 后数据仍然存在

这是预期行为。普通停止不会删除数据卷。

## 彻底重置本地数据库

::: danger 此操作永久删除本机 Panvara Compose 数据
先停止 Server，确认数据不再需要，再执行下面命令。删除后无法撤销。
:::

```sh
docker compose -f deploy/compose/compose.yaml down --volumes
```

随后执行 `make infra-up` 会创建空数据库。

## 仍然无法解决

收集以下结果后再提交 Issue，不要附带 `.env.local` 或 Token：

```sh
git status --short --branch
go version
docker version
docker compose version
make doctor-server
./bin/panvara --version
```
